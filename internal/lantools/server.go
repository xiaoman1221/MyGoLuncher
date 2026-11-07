package lantools

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const tokenHeader = "X-MGL-Tool-Token"

type voiceData struct {
	data []byte
	mime string
}

// Server receives chat, voice and file items from LAN peers. It only accepts
// requests carrying the token advertised in the launcher presence packet.
type Server struct {
	selfID   string
	selfName string
	inbox    string
	token    string
	port     int
	listener net.Listener
	http     *http.Server
	stopOnce sync.Once
	cancel   context.CancelFunc
	onItem   func(Message)
	onUpdate func(Message)

	mu         sync.Mutex
	messages   []Message
	voices     map[string]voiceData
	voiceBytes int64
	signals    []Signal
	signalSeq  int64
	shares     map[string]FileShare
}

// Start creates a LAN toolbox server listening on a random TCP port.
func Start(ctx context.Context, selfID, selfName, inbox string, onItem func(Message)) (*Server, error) {
	inbox = strings.TrimSpace(inbox)
	if inbox == "" {
		return nil, errors.New("局域网工具接收目录为空")
	}
	if err := os.MkdirAll(inbox, 0o755); err != nil {
		return nil, fmt.Errorf("创建局域网工具接收目录失败: %w", err)
	}
	token, err := randomToken()
	if err != nil {
		return nil, fmt.Errorf("生成局域网工具凭证失败: %w", err)
	}
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		return nil, fmt.Errorf("启动局域网工具端口失败: %w", err)
	}
	address, _ := listener.Addr().(*net.TCPAddr)
	port := 0
	if address != nil {
		port = address.Port
	}
	serverCtx, cancel := context.WithCancel(ctx)
	s := &Server{
		selfID:   strings.TrimSpace(selfID),
		selfName: strings.TrimSpace(selfName),
		inbox:    inbox,
		token:    token,
		port:     port,
		listener: listener,
		cancel:   cancel,
		onItem:   onItem,
		voices:   map[string]voiceData{},
		shares:   map[string]FileShare{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/chat", s.handleChat)
	mux.HandleFunc("/voice", s.handleVoice)
	mux.HandleFunc("/file", s.handleFile)
	mux.HandleFunc("/signal", s.handleSignal)
	mux.HandleFunc("/share", s.handleShare)
	mux.HandleFunc("/file-offer", s.handleFileOffer)
	s.http = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		<-serverCtx.Done()
		s.Stop()
	}()
	go func() { _ = s.http.Serve(listener) }()
	return s, nil
}

func (s *Server) Port() int     { return s.port }
func (s *Server) Token() string { return s.token }
func (s *Server) Inbox() string { return s.inbox }

// SetIdentity updates the display name used for outgoing items.
func (s *Server) SetIdentity(selfID, selfName string) {
	s.mu.Lock()
	s.selfID = strings.TrimSpace(selfID)
	s.selfName = strings.TrimSpace(selfName)
	s.mu.Unlock()
}

// SetUpdateHandler registers a callback for progress and downloaded-file updates.
func (s *Server) SetUpdateHandler(handler func(Message)) {
	s.mu.Lock()
	s.onUpdate = handler
	s.mu.Unlock()
}

// Messages returns a snapshot of the current history.
func (s *Server) Messages() []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Message(nil), s.messages...)
}

// Signals returns signaling messages newer than after.
func (s *Server) Signals(after int64) SignalBatch {
	s.mu.Lock()
	defer s.mu.Unlock()
	batch := SignalBatch{Signals: []Signal{}, LastSeq: s.signalSeq}
	for _, signal := range s.signals {
		if signal.Seq > after {
			batch.Signals = append(batch.Signals, signal)
		}
	}
	return batch
}

// RegisterFile exposes a local file for range downloads.
func (s *Server) RegisterFile(path string) (FileShare, error) {
	stat, err := os.Stat(path)
	if err != nil || !stat.Mode().IsRegular() {
		return FileShare{}, errors.New("文件不存在")
	}
	if stat.Size() > MaxFileBytes {
		return FileShare{}, errors.New("文件超过 512 MB 限制")
	}
	share := FileShare{ID: newID(), Name: filepath.Base(path), Size: stat.Size(), Path: path}
	s.mu.Lock()
	s.shares[share.ID] = share
	s.mu.Unlock()
	return share, nil
}

// File returns an exposed file share.
func (s *Server) File(id string) (FileShare, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	share, ok := s.shares[id]
	return share, ok
}

// MessageByID returns one history item.
func (s *Server) MessageByID(id string) (Message, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, message := range s.messages {
		if message.ID == id {
			return message, true
		}
	}
	return Message{}, false
}

// UpdateFileProgress updates a file offer download state and emits an update.
func (s *Server) UpdateFileProgress(id string, progress int, downloaded bool, path string) {
	s.mu.Lock()
	var updated Message
	found := false
	for index := range s.messages {
		if s.messages[index].ID != id {
			continue
		}
		s.messages[index].Progress = progress
		if downloaded {
			s.messages[index].Downloaded = true
			s.messages[index].Path = path
		}
		updated = s.messages[index]
		found = true
		break
	}
	handler := s.onUpdate
	s.mu.Unlock()
	if found && handler != nil {
		handler(updated)
	}
}

// State returns a copy of the in-memory conversation and inbox location.
func (s *Server) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return State{
		SelfID:   s.selfID,
		SelfName: s.selfName,
		Port:     s.port,
		Inbox:    s.inbox,
		Messages: append([]Message{}, s.messages...),
	}
}

// AddOutgoingChat records a locally sent chat message.
func (s *Server) AddOutgoingChat(text string) Message {
	return s.AddOutgoingChatMessage("", "", nil, text)
}

// AddOutgoingChatMessage records a locally sent group/private chat message.
func (s *Server) AddOutgoingChatMessage(chatID, chatName string, members []string, text string) Message {
	s.mu.Lock()
	msg := Message{
		ID: newID(), Kind: KindChat, FromID: s.selfID, FromName: s.selfName,
		Text: text, ChatID: strings.TrimSpace(chatID), ChatName: strings.TrimSpace(chatName),
		Members: cleanMembers(members), CreatedAt: nowString(),
	}
	s.appendLocked(msg)
	s.mu.Unlock()
	return msg
}

// AddOutgoingFile records a locally sent file.
func (s *Server) AddOutgoingFile(info FileInfo) Message {
	return s.AddOutgoingFileShare("", "", nil, info, "")
}

// AddOutgoingFileShare records a file offer that the peer can download.
func (s *Server) AddOutgoingFileShare(chatID, chatName string, members []string, info FileInfo, fileID string) Message {
	s.mu.Lock()
	msg := Message{
		ID: newID(), Kind: KindFile, FromID: s.selfID, FromName: s.selfName,
		FileName: info.Name, Size: info.Size, Path: info.Path, FileID: fileID,
		ChatID: strings.TrimSpace(chatID), ChatName: strings.TrimSpace(chatName),
		Members: cleanMembers(members), CreatedAt: nowString(),
	}
	s.appendLocked(msg)
	s.mu.Unlock()
	return msg
}

// AddOutgoingVoice records a locally recorded voice message and keeps its
// bytes for playback in the current session.
func (s *Server) AddOutgoingVoice(data []byte, mime string, durationMS int) Message {
	s.mu.Lock()
	msg := Message{
		ID:         newID(),
		Kind:       KindVoice,
		FromID:     s.selfID,
		FromName:   s.selfName,
		Size:       int64(len(data)),
		MimeType:   mime,
		DurationMS: durationMS,
		CreatedAt:  nowString(),
	}
	s.addVoiceLocked(msg.ID, data, mime)
	s.appendLocked(msg)
	s.mu.Unlock()
	return msg
}

// Voice returns the bytes of a voice message in the current session.
func (s *Server) Voice(id string) ([]byte, string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	voice, ok := s.voices[id]
	if !ok {
		return nil, "", false
	}
	return append([]byte(nil), voice.data...), voice.mime, true
}

func (s *Server) addVoiceLocked(id string, data []byte, mime string) {
	cloned := append([]byte(nil), data...)
	s.voices[id] = voiceData{data: cloned, mime: mime}
	s.voiceBytes += int64(len(cloned))
}

// trimVoicesLocked keeps cached audio below MaxVoiceCacheBytes. Message
// metadata remains visible; only the payload of the oldest clips is dropped.
func (s *Server) trimVoicesLocked() {
	for s.voiceBytes > MaxVoiceCacheBytes {
		dropped := false
		for _, msg := range s.messages {
			if voice, ok := s.voices[msg.ID]; ok {
				s.voiceBytes -= int64(len(voice.data))
				delete(s.voices, msg.ID)
				dropped = true
				break
			}
		}
		if !dropped {
			s.voiceBytes = 0
			return
		}
	}
}

func (s *Server) appendLocked(msg Message) {
	s.messages = append(s.messages, msg)
	if len(s.messages) > MaxMessages {
		drop := len(s.messages) - MaxMessages
		for _, old := range s.messages[:drop] {
			if voice, ok := s.voices[old.ID]; ok {
				s.voiceBytes -= int64(len(voice.data))
				delete(s.voices, old.ID)
			}
		}
		s.messages = append([]Message(nil), s.messages[drop:]...)
	}
	s.trimVoicesLocked()
}

func (s *Server) emit(msg Message) {
	if s.onItem != nil {
		s.onItem(msg)
	}
}

func (s *Server) addIncoming(msg Message) Message {
	s.mu.Lock()
	s.appendLocked(msg)
	s.mu.Unlock()
	s.emit(msg)
	return msg
}

func (s *Server) authorized(r *http.Request) bool {
	token := strings.TrimSpace(r.Header.Get(tokenHeader))
	return token != "" && subtle.ConstantTimeCompare([]byte(token), []byte(s.token)) == 1
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	var request struct {
		From     string   `json:"from"`
		FromName string   `json:"fromName"`
		ChatID   string   `json:"chatId"`
		ChatName string   `json:"chatName"`
		Members  []string `json:"members"`
		Text     string   `json:"text"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxChatTextBytes)
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "invalid chat payload", http.StatusBadRequest)
		return
	}
	text := strings.TrimSpace(request.Text)
	if text == "" || len([]byte(text)) > MaxChatTextBytes/2 || !utf8.ValidString(text) {
		http.Error(w, "invalid chat text", http.StatusBadRequest)
		return
	}
	from := strings.TrimSpace(request.From)
	if from == "" {
		from = "peer"
	}
	fromName := strings.TrimSpace(request.FromName)
	if fromName == "" {
		fromName = "LAN"
	}
	if len([]rune(fromName)) > 32 {
		fromName = string([]rune(fromName)[:32])
	}
	msg := Message{
		ID: newID(), Kind: KindChat, FromID: from, FromName: fromName,
		Text: text, ChatID: strings.TrimSpace(request.ChatID), ChatName: strings.TrimSpace(request.ChatName),
		Members: cleanMembers(request.Members), CreatedAt: nowString(), Incoming: true,
	}
	msg = s.addIncoming(msg)
	writeJSON(w, msg)
}

func (s *Server) handleVoice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	mime := strings.TrimSpace(r.URL.Query().Get("mime"))
	if !strings.HasPrefix(strings.ToLower(mime), "audio/") {
		mime = "audio/webm"
	}
	if r.ContentLength > MaxVoiceBytes {
		http.Error(w, "voice too large", http.StatusRequestEntityTooLarge)
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxVoiceBytes))
	if err != nil {
		http.Error(w, "voice too large", http.StatusRequestEntityTooLarge)
		return
	}
	if len(data) == 0 {
		http.Error(w, "empty voice", http.StatusBadRequest)
		return
	}
	name := inboxName(r.URL.Query().Get("duration"))
	path, err := writeInbox(s.inbox, "voice-"+name+".webm", data)
	if err != nil {
		http.Error(w, "save voice failed", http.StatusInternalServerError)
		return
	}
	duration := parseInt(r.URL.Query().Get("duration"))
	from, fromName := requestSender(r)
	msg := Message{
		ID:         newID(),
		Kind:       KindVoice,
		FromID:     from,
		FromName:   fromName,
		FileName:   filepath.Base(path),
		Size:       int64(len(data)),
		MimeType:   mime,
		DurationMS: duration,
		Path:       path,
		CreatedAt:  nowString(),
		Incoming:   true,
	}
	s.mu.Lock()
	s.addVoiceLocked(msg.ID, data, mime)
	s.mu.Unlock()
	msg = s.addIncoming(msg)
	writeJSON(w, msg)
}

func (s *Server) handleFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if r.ContentLength > MaxFileBytes {
		http.Error(w, "file too large", http.StatusRequestEntityTooLarge)
		return
	}
	name := safeName(r.URL.Query().Get("name"))
	if name == "" {
		name = "received-file"
	}
	path, size, err := receiveFile(s.inbox, name, http.MaxBytesReader(w, r.Body, MaxFileBytes))
	if err != nil {
		http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
		return
	}
	from, fromName := requestSender(r)
	msg := Message{
		ID:        newID(),
		Kind:      KindFile,
		FromID:    from,
		FromName:  fromName,
		FileName:  filepath.Base(path),
		Size:      size,
		Path:      path,
		CreatedAt: nowString(),
		Incoming:  true,
	}
	msg = s.addIncoming(msg)
	writeJSON(w, msg)
}

func (s *Server) handleSignal(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		after := int64(0)
		if raw := strings.TrimSpace(r.URL.Query().Get("after")); raw != "" {
			parsed, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || parsed < 0 {
				http.Error(w, "invalid cursor", http.StatusBadRequest)
				return
			}
			after = parsed
		}
		writeJSON(w, s.Signals(after))
	case http.MethodPost:
		var request struct {
			From     string `json:"from"`
			FromName string `json:"fromName"`
			Session  string `json:"session"`
			Kind     string `json:"kind"`
			Payload  string `json:"payload"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, MaxSignalBytes+4096)
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid signal", http.StatusBadRequest)
			return
		}
		kind := strings.ToLower(strings.TrimSpace(request.Kind))
		if kind != "offer" && kind != "answer" && kind != "ice" {
			http.Error(w, "invalid signal kind", http.StatusBadRequest)
			return
		}
		session := strings.TrimSpace(request.Session)
		if session == "" || len(session) > 128 || len(request.Payload) > MaxSignalBytes {
			http.Error(w, "invalid signal payload", http.StatusBadRequest)
			return
		}
		signal := Signal{
			FromID: strings.TrimSpace(request.From), FromName: strings.TrimSpace(request.FromName),
			Session: session, Kind: kind, Payload: request.Payload, CreatedAt: nowString(),
		}
		if signal.FromID == "" {
			signal.FromID = "peer"
		}
		if signal.FromName == "" {
			signal.FromName = "LAN"
		}
		s.mu.Lock()
		s.signalSeq++
		signal.Seq = s.signalSeq
		s.signals = append(s.signals, signal)
		if len(s.signals) > MaxSignals {
			s.signals = append([]Signal(nil), s.signals[len(s.signals)-MaxSignals:]...)
		}
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(signal)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleShare(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	share, ok := s.File(strings.TrimSpace(r.URL.Query().Get("id")))
	if !ok || share.Path == "" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Accept-Ranges", "bytes")
	http.ServeFile(w, r, share.Path)
}

func (s *Server) handleFileOffer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	var request struct {
		From, FromName, FileID, Name, ChatID, ChatName string
		Members                                        []string
		Size                                           int64
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "invalid file offer", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(request.FileID) == "" || request.Size <= 0 || request.Size > MaxFileBytes {
		http.Error(w, "invalid file offer", http.StatusBadRequest)
		return
	}
	from := strings.TrimSpace(request.From)
	if from == "" {
		from = "peer"
	}
	fromName := strings.TrimSpace(request.FromName)
	if fromName == "" {
		fromName = "LAN"
	}
	if len([]rune(fromName)) > 32 {
		fromName = string([]rune(fromName)[:32])
	}
	name := safeName(request.Name)
	if name == "" {
		name = "received-file"
	}
	msg := Message{
		ID: newID(), Kind: KindFile, FromID: from, FromName: fromName,
		FileName: name, FileID: strings.TrimSpace(request.FileID), Size: request.Size,
		ChatID: strings.TrimSpace(request.ChatID), ChatName: strings.TrimSpace(request.ChatName),
		Members: cleanMembers(request.Members), CreatedAt: nowString(), Incoming: true,
	}
	msg = s.addIncoming(msg)
	writeJSON(w, msg)
}

func requestSender(r *http.Request) (string, string) {
	from := strings.TrimSpace(r.URL.Query().Get("from"))
	if from == "" {
		from = "peer"
	}
	name := strings.TrimSpace(r.URL.Query().Get("fromName"))
	if name == "" {
		name = "LAN"
	}
	if len([]rune(name)) > 32 {
		name = string([]rune(name)[:32])
	}
	return from, name
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(value)
}

func receiveFile(dir, name string, reader io.Reader) (string, int64, error) {
	temp, err := os.CreateTemp(dir, ".mgl-lan-*.part")
	if err != nil {
		return "", 0, fmt.Errorf("创建接收文件失败: %w", err)
	}
	tempPath := temp.Name()
	written, copyErr := io.Copy(temp, reader)
	closeErr := temp.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(tempPath)
		return "", 0, errors.New("接收文件失败")
	}
	path := uniquePath(dir, name)
	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Remove(tempPath)
		return "", 0, errors.New("保存接收文件失败")
	}
	_ = os.Chmod(path, 0o644)
	return path, written, nil
}

func writeInbox(dir, name string, data []byte) (string, error) {
	path := uniquePath(dir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func uniquePath(dir, name string) string {
	name = safeName(name)
	if name == "" {
		name = "received-file"
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	candidate := filepath.Join(dir, name)
	for index := 1; ; index++ {
		if _, err := os.Stat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate
		}
		candidate = filepath.Join(dir, fmt.Sprintf("%s (%d)%s", base, index, ext))
	}
}

func cleanMembers(members []string) []string {
	out := make([]string, 0, len(members))
	seen := map[string]bool{}
	for _, member := range members {
		member = strings.TrimSpace(member)
		if member == "" || len(member) > 128 || seen[member] {
			continue
		}
		seen[member] = true
		out = append(out, member)
		if len(out) >= 32 {
			break
		}
	}
	return out
}

func safeName(value string) string {
	value = strings.ReplaceAll(value, "\\", "/")
	value = filepath.Base(value)
	value = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || r == '/' || r == '\\' {
			return -1
		}
		return r
	}, value)
	value = strings.TrimSpace(strings.Trim(value, "."))
	if value == "" || value == "." || value == ".." {
		return ""
	}
	runes := []rune(value)
	if len(runes) > 180 {
		value = string(runes[:180])
	}
	return value
}

func inboxName(duration string) string {
	return "message-" + time.Now().Format("20060102-150405") + "-" + parseIntString(duration)
}

func parseInt(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	n := 0
	for _, r := range value {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
		if n > 24*60*60*1000 {
			return 0
		}
	}
	return n
}

func parseIntString(value string) string {
	n := parseInt(value)
	if n <= 0 {
		return "0"
	}
	return fmt.Sprintf("%d", n)
}

func newID() string {
	var data [8]byte
	if _, err := rand.Read(data[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(data[:])
}

func randomToken() (string, error) {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(data[:]), nil
}

// Stop shuts down the HTTP server and listener.
func (s *Server) Stop() {
	s.stopOnce.Do(func() {
		// Cancel our child context so the watcher goroutine exits even when
		// the parent (Wails) context is still alive.
		if s.cancel != nil {
			s.cancel()
		}
		if s.http != nil {
			_ = s.http.Close()
		}
		if s.listener != nil {
			_ = s.listener.Close()
		}
	})
}
