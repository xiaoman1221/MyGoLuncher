package lansync

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const tokenHeader = "X-MGL-Sync-Token"

// Server exposes one instance's selected directories to LAN peers.
type Server struct {
	instanceDir string
	token       string
	manifest    Manifest
	files       map[string]FileEntry
	listener    net.Listener
	httpServer  *http.Server
	stopOnce    sync.Once
}

// StartServer indexes the instance and starts the sharing endpoint. progress
// receives file-count progress while hashing the manifest.
func StartServer(ctx context.Context, instanceDir string, info InstanceInfo, dirs []string, progress ProgressFunc) (*Server, error) {
	stat, err := os.Stat(instanceDir)
	if err != nil || !stat.IsDir() {
		return nil, errors.New("共享的游戏目录不存在")
	}
	dirs, err = NormalizeDirs(dirs)
	if err != nil {
		return nil, err
	}
	type candidate struct {
		rel  string
		full string
		size int64
	}
	var candidates []candidate
	for _, dir := range dirs {
		base := filepath.Join(instanceDir, dir)
		if stat, err := os.Stat(base); err != nil || !stat.IsDir() {
			continue
		}
		err := filepath.WalkDir(base, func(filePath string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if entry.IsDir() {
				return nil
			}
			if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() || skipSyncName(entry.Name()) {
				return nil
			}
			rel, err := filepath.Rel(instanceDir, filePath)
			if err != nil {
				return nil
			}
			rel = filepath.ToSlash(rel)
			if _, err := safeRelativePath(rel, dirs); err != nil {
				return nil
			}
			stat, err := entry.Info()
			if err != nil {
				return err
			}
			candidates = append(candidates, candidate{rel: rel, full: filePath, size: stat.Size()})
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("扫描共享目录 %s 失败: %w", dir, err)
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].rel < candidates[j].rel })

	token, err := randomToken()
	if err != nil {
		return nil, fmt.Errorf("生成局域网共享凭证失败: %w", err)
	}
	manifest := Manifest{
		Instance:  info,
		Dirs:      append([]string(nil), dirs...),
		Files:     make([]FileEntry, 0, len(candidates)),
		CreatedAt: nowString(),
	}
	files := make(map[string]FileEntry, len(candidates))
	for index, candidate := range candidates {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if progress != nil {
			progress("scan", fmt.Sprintf("正在索引 %s", candidate.rel), int64(index), int64(len(candidates)))
		}
		hash, err := hashFile(candidate.full)
		if err != nil {
			return nil, fmt.Errorf("读取 %s 失败: %w", candidate.rel, err)
		}
		entry := FileEntry{Path: candidate.rel, Size: candidate.size, SHA256: hash}
		manifest.Files = append(manifest.Files, entry)
		files[entry.Path] = entry
	}
	if progress != nil {
		progress("scan", fmt.Sprintf("已索引 %d 个文件", len(manifest.Files)), int64(len(candidates)), int64(len(candidates)))
	}

	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		return nil, fmt.Errorf("启动局域网共享端口失败: %w", err)
	}
	server := &Server{
		instanceDir: instanceDir,
		token:       token,
		manifest:    manifest,
		files:       files,
		listener:    listener,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/manifest", server.handleManifest)
	mux.HandleFunc("/file", server.handleFile)
	server.httpServer = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	go func() { _ = server.httpServer.Serve(listener) }()
	return server, nil
}

func (s *Server) Port() int {
	if s.listener == nil {
		return 0
	}
	if address, ok := s.listener.Addr().(*net.TCPAddr); ok {
		return address.Port
	}
	return 0
}

func (s *Server) Token() string { return s.token }

func (s *Server) Manifest() Manifest {
	manifest := s.manifest
	manifest.Dirs = append([]string(nil), s.manifest.Dirs...)
	manifest.Files = append([]FileEntry(nil), s.manifest.Files...)
	return manifest
}

func (s *Server) Stop() {
	s.stopOnce.Do(func() {
		if s.httpServer != nil {
			_ = s.httpServer.Close()
		}
		if s.listener != nil {
			_ = s.listener.Close()
		}
	})
}

func (s *Server) authorized(r *http.Request) bool {
	token := strings.TrimSpace(r.Header.Get(tokenHeader))
	if token == "" {
		token = strings.TrimSpace(r.URL.Query().Get("token"))
	}
	return token != "" && subtle.ConstantTimeCompare([]byte(token), []byte(s.token)) == 1
}

func (s *Server) handleManifest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(s.manifest)
}

func (s *Server) handleFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	rel := r.URL.Query().Get("path")
	entry, ok := s.files[rel]
	if !ok {
		http.NotFound(w, r)
		return
	}
	full, err := localPath(s.instanceDir, entry.Path)
	if err != nil {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeFile(w, r, full)
}

func randomToken() (string, error) {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(data[:]), nil
}
