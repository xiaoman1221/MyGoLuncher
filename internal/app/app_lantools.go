package app

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"MyMinecraftGoLuncher/internal/lan"
	"MyMinecraftGoLuncher/internal/lantools"
	licensepkg "MyMinecraftGoLuncher/internal/license"
)

// LanToolSendResult is returned by chat, voice and file sending APIs.
type LanToolSendResult struct {
	Message lantools.Message `json:"message"`
	Count   int              `json:"count"`
	Warning string           `json:"warning,omitempty"`
}

// LanFileInfo describes a file selected in the native open dialog.
type LanFileInfo struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Size int64  `json:"size"`
}

type lanToolTarget struct {
	target lantools.Target
	name   string
}

func (a *App) lanToolServer() *lantools.Server {
	a.lanToolMu.Lock()
	defer a.lanToolMu.Unlock()
	return a.lanTool
}

// ensureLanTools starts the always-on LAN toolbox HTTP server. It is separate
// from version sharing because chat and files must work even when no instance
// is being shared.
func (a *App) ensureLanTools() error {
	if a.lanSvc == nil || a.store == nil {
		return errors.New("局域网工具尚未初始化")
	}
	name := "Steve"
	if settings := a.store.Settings(); strings.TrimSpace(settings.PlayerName) != "" {
		name = settings.PlayerName
	}
	a.lanToolMu.Lock()
	if a.lanTool != nil {
		a.lanTool.SetIdentity(a.lanSvc.SelfID(), name)
		a.lanToolMu.Unlock()
		return nil
	}
	inbox := filepath.Join(a.paths.Data, "lan-inbox")
	ctx := a.context()
	if ctx == nil {
		ctx = context.Background()
	}
	server, err := lantools.Start(ctx, a.lanSvc.SelfID(), name, inbox, a.handleLanToolItem)
	if err != nil {
		a.lanToolMu.Unlock()
		return err
	}
	server.SetUpdateHandler(a.handleLanToolUpdate)
	a.lanTool = server
	a.lanToolMu.Unlock()
	a.refreshLanIdentity()
	return nil
}

func (a *App) stopLanTools() {
	a.lanToolMu.Lock()
	server := a.lanTool
	a.lanTool = nil
	a.lanToolMu.Unlock()
	if server != nil {
		server.Stop()
	}
}

func (a *App) handleLanToolItem(message lantools.Message) {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "lan:tool", message)
	}
}

func (a *App) handleLanToolUpdate(message lantools.Message) {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "lan:tool:update", message)
	}
}

func (a *App) applyLanToolIdentity(identity lan.Identity) lan.Identity {
	if server := a.lanToolServer(); server != nil {
		identity.ToolPort = server.Port()
		identity.ToolToken = server.Token()
	}
	return identity
}

func (a *App) lanIdentityWithTools(instanceID string) lan.Identity {
	return a.applyLanToolIdentity(a.lanIdentity(instanceID))
}

func (a *App) refreshLanIdentity() {
	if a.lanSvc == nil || !a.lanSvc.Snapshot().Running {
		return
	}
	identity := a.applyLanShareIdentity(a.lanIdentityWithTools(a.defaultLanInstanceID()))
	a.lanSvc.SetIdentity(identity)
}

func (a *App) lanToolsReady() (*lantools.Server, error) {
	if err := a.requireFeature(licensepkg.FeatureLAN); err != nil {
		return nil, err
	}
	if err := a.ready(); err != nil {
		return nil, err
	}
	if err := a.ensureLanTools(); err != nil {
		return nil, err
	}
	server := a.lanToolServer()
	if server == nil {
		return nil, errors.New("局域网工具未启动")
	}
	return server, nil
}

// GetLanTools returns the current chat history and LAN toolbox state.
func (a *App) GetLanTools() (lantools.State, error) {
	server, err := a.lanToolsReady()
	if err != nil {
		return lantools.State{}, err
	}
	return server.State(), nil
}

// PickLanFile opens a native file picker for a file to send.
func (a *App) PickLanFile() (LanFileInfo, error) {
	if err := a.requireFeature(licensepkg.FeatureLAN); err != nil {
		return LanFileInfo{}, err
	}
	path, err := runtime.OpenFileDialog(a.context(), runtime.OpenDialogOptions{
		Title: "选择要发送到局域网的文件",
	})
	if err != nil {
		return LanFileInfo{}, err
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return LanFileInfo{}, nil
	}
	stat, err := os.Stat(path)
	if err != nil || !stat.Mode().IsRegular() {
		return LanFileInfo{}, errors.New("选择的文件不存在")
	}
	if stat.Size() > lantools.MaxFileBytes {
		return LanFileInfo{}, errors.New("文件超过 512 MB 限制")
	}
	return LanFileInfo{Path: path, Name: filepath.Base(path), Size: stat.Size()}, nil
}

// OpenLanInbox opens the folder that receives LAN chat files and voice clips.
func (a *App) OpenLanInbox() error {
	server, err := a.lanToolsReady()
	if err != nil {
		return err
	}
	return openPath(server.Inbox())
}

func (a *App) lanToolTargetsByIDs(peerIDs []string) ([]lanToolTarget, error) {
	if a.lanSvc == nil {
		return nil, errors.New("局域网发现尚未启动")
	}
	if len(peerIDs) == 0 {
		snapshot := a.lanSvc.Snapshot()
		if !snapshot.Running {
			return nil, errors.New("局域网发现尚未启动")
		}
		targets := make([]lanToolTarget, 0, len(snapshot.Peers))
		for _, peer := range snapshot.Peers {
			if !peer.CanChat {
				continue
			}
			target, ok := a.lanSvc.ToolTarget(peer.ID)
			if !ok {
				continue
			}
			targets = append(targets, lanToolTarget{
				target: lantools.Target{Address: target.Address, Port: target.Port, Token: target.Token},
				name:   peer.Name,
			})
		}
		if len(targets) == 0 {
			return nil, errors.New("没有可接收的在线玩家")
		}
		return targets, nil
	}
	targets := make([]lanToolTarget, 0, len(peerIDs))
	seen := map[string]bool{}
	for _, peerID := range peerIDs {
		peerID = strings.TrimSpace(peerID)
		if peerID == "" || seen[peerID] {
			continue
		}
		seen[peerID] = true
		target, ok := a.lanSvc.ToolTarget(peerID)
		if !ok {
			continue
		}
		targets = append(targets, lanToolTarget{
			target: lantools.Target{Address: target.Address, Port: target.Port, Token: target.Token},
			name:   target.Name,
		})
	}
	if len(targets) == 0 {
		return nil, errors.New("目标玩家未开启局域网工具或已离线")
	}
	return targets, nil
}

func (a *App) lanToolTarget(peerID string) (lanToolTarget, error) {
	targets, err := a.lanToolTargetsByIDs([]string{peerID})
	if err != nil {
		return lanToolTarget{}, err
	}
	return targets[0], nil
}

func (a *App) sendLanTargets(
	peerIDs []string,
	timeout time.Duration,
	send func(context.Context, lantools.Target) error,
) (int, []string, error) {
	targets, err := a.lanToolTargetsByIDs(peerIDs)
	if err != nil {
		return 0, nil, err
	}
	sent := 0
	failures := make([]string, 0)
	for _, item := range targets {
		ctx, cancel := context.WithTimeout(a.context(), timeout)
		err := send(ctx, item.target)
		cancel()
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", item.name, err))
			continue
		}
		sent++
	}
	if sent == 0 {
		return 0, failures, fmt.Errorf("发送失败: %s", strings.Join(failures, "；"))
	}
	return sent, failures, nil
}

func failureText(count int, failures []string) string {
	if len(failures) == 0 {
		return ""
	}
	return fmt.Sprintf("已发送给 %d 个玩家，%d 个失败: %s", count, len(failures), strings.Join(failures, "；"))
}

// SendLanChat sends a message to the selected group/private peers. An empty
// peer list broadcasts to every online LAN toolbox peer.
func (a *App) SendLanChat(peerIDs []string, chatID, chatName string, members []string, text string) (LanToolSendResult, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return LanToolSendResult{}, errors.New("聊天内容为空")
	}
	if len([]byte(text)) > lantools.MaxChatTextBytes {
		return LanToolSendResult{}, errors.New("聊天内容过长")
	}
	chatID = strings.TrimSpace(chatID)
	chatName = strings.TrimSpace(chatName)
	server, err := a.lanToolsReady()
	if err != nil {
		return LanToolSendResult{}, err
	}
	state := server.State()
	sent, failures, err := a.sendLanTargets(peerIDs, 2*time.Minute, func(ctx context.Context, target lantools.Target) error {
		return lantools.SendChatMessage(ctx, target, state.SelfID, state.SelfName, chatID, chatName, members, text)
	})
	if sent == 0 {
		return LanToolSendResult{}, err
	}
	return LanToolSendResult{
		Message: server.AddOutgoingChatMessage(chatID, chatName, members, text),
		Count:   sent, Warning: failureText(sent, failures),
	}, nil
}

// SendLanFile exposes a file through the local toolbox and sends a download
// offer. The receiver downloads it with multiple HTTP Range workers.
func (a *App) SendLanFile(peerIDs []string, chatID, chatName string, members []string, path string) (LanToolSendResult, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return LanToolSendResult{}, errors.New("没有选择文件")
	}
	stat, err := os.Stat(path)
	if err != nil || !stat.Mode().IsRegular() {
		return LanToolSendResult{}, errors.New("要发送的文件不存在")
	}
	if stat.Size() > lantools.MaxFileBytes {
		return LanToolSendResult{}, errors.New("文件超过 512 MB 限制")
	}
	server, err := a.lanToolsReady()
	if err != nil {
		return LanToolSendResult{}, err
	}
	share, err := server.RegisterFile(path)
	if err != nil {
		return LanToolSendResult{}, err
	}
	state := server.State()
	sent, failures, err := a.sendLanTargets(peerIDs, 2*time.Minute, func(ctx context.Context, target lantools.Target) error {
		return lantools.SendFileOffer(ctx, target, state.SelfID, state.SelfName, share.ID, share.Name, chatID, chatName, members, share.Size)
	})
	if sent == 0 {
		return LanToolSendResult{}, err
	}
	return LanToolSendResult{
		Message: server.AddOutgoingFileShare(chatID, chatName, members, lantools.FileInfo{Path: path, Name: share.Name, Size: share.Size}, share.ID),
		Count:   sent, Warning: failureText(sent, failures),
	}, nil
}

// DownloadLanFile downloads an incoming file offer with 4-8 Range workers and
// reports progress through the lan:tool:update event.
func (a *App) DownloadLanFile(messageID string) (lantools.Message, error) {
	server, err := a.lanToolsReady()
	if err != nil {
		return lantools.Message{}, err
	}
	message, ok := server.MessageByID(strings.TrimSpace(messageID))
	if !ok || message.Kind != lantools.KindFile || message.FileID == "" || message.Size <= 0 {
		return lantools.Message{}, errors.New("文件消息不存在或已失效")
	}
	if message.Downloaded && message.Path != "" {
		return message, nil
	}
	targetItem, err := a.lanToolTarget(message.FromID)
	if err != nil {
		return lantools.Message{}, err
	}
	name := filepath.Base(strings.TrimSpace(message.FileName))
	if name == "" || name == "." {
		name = "received-file"
	}
	dest := uniqueLanInboxPath(server.Inbox(), name)
	workers := 6
	if message.Size < 8*1024*1024 {
		workers = 4
	}
	ctx, cancel := context.WithTimeout(a.context(), 30*time.Minute)
	defer cancel()
	if err := lantools.DownloadSharedFile(ctx, targetItem.target, message.FileID, dest, message.Size, workers, func(percent int) {
		server.UpdateFileProgress(message.ID, percent, false, "")
	}); err != nil {
		server.UpdateFileProgress(message.ID, 0, false, "")
		return lantools.Message{}, fmt.Errorf("多线程下载失败: %w", err)
	}
	server.UpdateFileProgress(message.ID, 100, true, dest)
	updated, _ := server.MessageByID(message.ID)
	return updated, nil
}

func uniqueLanInboxPath(dir, name string) string {
	name = filepath.Base(name)
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	candidate := filepath.Join(dir, name)
	for index := 1; ; index++ {
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
		candidate = filepath.Join(dir, fmt.Sprintf("%s (%d)%s", base, index, ext))
	}
}

// SendLanSignal forwards one WebRTC signaling payload to a peer.
func (a *App) SendLanSignal(peerID, session, kind, payload string) error {
	targetItem, err := a.lanToolTarget(peerID)
	if err != nil {
		return err
	}
	server, err := a.lanToolsReady()
	if err != nil {
		return err
	}
	state := server.State()
	ctx, cancel := context.WithTimeout(a.context(), 20*time.Second)
	defer cancel()
	return lantools.SendSignal(ctx, targetItem.target, state.SelfID, state.SelfName, session, kind, payload)
}

// PollLanSignals returns signaling messages received by this client.
func (a *App) PollLanSignals(after int64) (lantools.SignalBatch, error) {
	server, err := a.lanToolsReady()
	if err != nil {
		return lantools.SignalBatch{}, err
	}
	return server.Signals(after), nil
}

// SendLanVoice sends a base64-encoded voice clip to one peer or to everyone.
func (a *App) SendLanVoice(peerID, dataBase64, mime string, durationMS int) (LanToolSendResult, error) {
	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(dataBase64))
	if err != nil || len(data) == 0 {
		return LanToolSendResult{}, errors.New("语音数据无效")
	}
	if len(data) > lantools.MaxVoiceBytes {
		return LanToolSendResult{}, errors.New("语音文件超过 20 MB 限制")
	}
	mime = strings.TrimSpace(mime)
	if !strings.HasPrefix(strings.ToLower(mime), "audio/") {
		mime = "audio/webm"
	}
	if durationMS < 0 || durationMS > 24*60*60*1000 {
		durationMS = 0
	}
	server, err := a.lanToolsReady()
	if err != nil {
		return LanToolSendResult{}, err
	}
	state := server.State()
	sent, failures, err := a.sendLanTargets([]string{peerID}, 2*time.Minute, func(ctx context.Context, target lantools.Target) error {
		return lantools.SendVoice(ctx, target, state.SelfID, state.SelfName, mime, data, durationMS)
	})
	if sent == 0 {
		return LanToolSendResult{}, err
	}
	return LanToolSendResult{
		Message: server.AddOutgoingVoice(data, mime, durationMS),
		Count:   sent,
		Warning: failureText(sent, failures),
	}, nil
}

// GetLanVoice returns a cached voice clip for playback.
func (a *App) GetLanVoice(id string) (string, error) {
	if err := a.requireFeature(licensepkg.FeatureLAN); err != nil {
		return "", err
	}
	server := a.lanToolServer()
	if server == nil {
		return "", errors.New("局域网工具未启动")
	}
	data, _, ok := server.Voice(strings.TrimSpace(id))
	if !ok {
		return "", errors.New("语音消息已不可用")
	}
	return base64.StdEncoding.EncodeToString(data), nil
}
