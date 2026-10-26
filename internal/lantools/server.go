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
	onItem   func(Message)

	mu       sync.Mutex
	messages []Message
	voices   map[string]voiceData
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
	s := &Server{
		selfID:   strings.TrimSpace(selfID),
		selfName: strings.TrimSpace(selfName),
		inbox:    inbox,
		token:    token,
		port:     port,
		listener: listener,
		onItem:   onItem,
		voices:   map[string]voiceData{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/chat", s.handleChat)
	mux.HandleFunc("/voice", s.handleVoice)
	mux.HandleFunc("/file", s.handleFile)
	s.http = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		<-ctx.Done()
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

// State returns a copy of the in-memory conversation and inbox location.
func (s *Server) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return State{
		SelfID:   s.selfID,
		SelfName: s.selfName,
		Port:     s.port,
		Inbox:    s.inbox,
		Messages: append([]Message(nil), s.messages...),
	}
}

// AddOutgoingChat records a locally sent chat message.
func (s *Server) AddOutgoingChat(text string) Message {
	s.mu.Lock()
	msg := Message{
		ID:        newID(),
		Kind:      KindChat,
		FromID:    s.selfID,
		FromName:  s.selfName,
		Text:      text,
		CreatedAt: nowString(),
	}
	s.appendLocked(msg)
	s.mu.Unlock()
	return msg
}

// AddOutgoingFile records a locally sent file.
func (s *Server) AddOutgoingFile(info FileInfo) Message {
	s.mu.Lock()
	msg := Message{
		ID:        newID(),
		Kind:      KindFile,
		FromID:    s.selfID,
		FromName:  s.selfName,
		FileName:  info.Name,
		Size:      info.Size,
		Path:      info.Path,
		CreatedAt: nowString(),
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
	s.voices[msg.ID] = voiceData{data: append([]byte(nil), data...), mime: mime}
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

func (s *Server) appendLocked(msg Message) {
	s.messages = append(s.messages, msg)
	if len(s.messages) <= MaxMessages {
		return
	}
	drop := len(s.messages) - MaxMessages
	for _, old := range s.messages[:drop] {
		delete(s.voices, old.ID)
	}
	s.messages = append([]Message(nil), s.messages[drop:]...)
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
		From     string `json:"from"`
		FromName string `json:"fromName"`
		Text     string `json:"text"`
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
		ID:        newID(),
		Kind:      KindChat,
		FromID:    from,
		FromName:  fromName,
		Text:      text,
		CreatedAt: nowString(),
		Incoming:  true,
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
	s.voices[msg.ID] = voiceData{data: append([]byte(nil), data...), mime: mime}
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
		if s.http != nil {
			_ = s.http.Close()
		}
		if s.listener != nil {
			_ = s.listener.Close()
		}
	})
}
