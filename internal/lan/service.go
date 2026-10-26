package lan

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"sort"
	"strconv"
	"sync"
	"time"
)

const (
	lanWorldTTL  = 6 * time.Second
	scanWorldTTL = 2 * time.Minute
	peerTTL      = 12 * time.Second
	pingCooldown = 4 * time.Second
)

// Service discovers LAN worlds and launcher peers.
type Service struct {
	lifeMu    sync.Mutex
	mu        sync.Mutex
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	running   bool
	broadcast bool
	identity  Identity
	onChange  func(Snapshot)
	peerID    string
	worlds    map[string]*worldEntry
	peers     map[string]*peerEntry
	lastPing  map[string]time.Time
	pinging   map[string]bool
	notifyCh  chan struct{}
	closers   []io.Closer
	warnings  []string
	scanStop  context.CancelFunc
	scanID    int
}

// New creates an idle discovery service.
func New() *Service {
	return &Service{
		peerID:   randomPeerID(),
		worlds:   map[string]*worldEntry{},
		peers:    map[string]*peerEntry{},
		lastPing: map[string]time.Time{},
		pinging:  map[string]bool{},
		notifyCh: make(chan struct{}, 1),
	}
}

// Start begins listening. Calling Start while running updates the broadcast
// flag and identity.
func (s *Service) Start(ctx context.Context, cfg Config) error {
	s.lifeMu.Lock()
	defer s.lifeMu.Unlock()
	s.mu.Lock()
	if s.running {
		s.broadcast = cfg.Broadcast
		s.identity = cfg.Identity
		if cfg.OnChange != nil {
			s.onChange = cfg.OnChange
		}
		s.mu.Unlock()
		s.notify()
		return nil
	}
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.running = true
	s.broadcast = cfg.Broadcast
	s.identity = cfg.Identity
	s.onChange = cfg.OnChange
	s.worlds = map[string]*worldEntry{}
	s.peers = map[string]*peerEntry{}
	s.lastPing = map[string]time.Time{}
	s.pinging = map[string]bool{}
	s.warnings = nil
	s.mu.Unlock()

	if err := s.startWorldListener(); err != nil {
		if s.cancel != nil {
			s.cancel()
		}
		s.mu.Lock()
		s.running = false
		s.warnings = append(s.warnings, err.Error())
		s.mu.Unlock()
		s.notify()
		return err
	}
	s.startPeerListener()
	s.startPeerSender()
	s.startLoops()
	s.notify()
	return nil
}

// Stop releases all sockets and clears discovered state.
func (s *Service) Stop() {
	s.lifeMu.Lock()
	defer s.lifeMu.Unlock()
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	cancel := s.cancel
	scanStop := s.scanStop
	closers := s.closers
	s.closers = nil
	s.running = false
	s.broadcast = false
	s.scanStop = nil
	s.worlds = map[string]*worldEntry{}
	s.peers = map[string]*peerEntry{}
	s.lastPing = map[string]time.Time{}
	s.pinging = map[string]bool{}
	s.mu.Unlock()

	if scanStop != nil {
		scanStop()
	}
	if cancel != nil {
		cancel()
	}
	for _, closer := range closers {
		_ = closer.Close()
	}
	s.wg.Wait()
}

// SetBroadcast toggles presence advertisements without stopping discovery.
func (s *Service) SetBroadcast(enabled bool) {
	s.mu.Lock()
	s.broadcast = enabled
	s.mu.Unlock()
	s.notify()
}

// SetIdentity updates the advertised player/instance information.
func (s *Service) SetIdentity(identity Identity) {
	s.mu.Lock()
	s.identity = identity
	s.mu.Unlock()
	s.notify()
}

// SyncTarget returns the connection details for a peer that shares content.
func (s *Service) SyncTarget(peerID string) (SyncTarget, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.peers[peerID]
	if !ok || !entry.CanSync || entry.SyncPort <= 0 || entry.syncToken == "" {
		return SyncTarget{}, false
	}
	return SyncTarget{
		Address:       entry.Address,
		Port:          entry.SyncPort,
		Token:         entry.syncToken,
		InstanceID:    entry.InstanceID,
		Instance:      entry.Instance,
		MCVersion:     entry.MCVersion,
		Loader:        entry.Loader,
		LoaderVersion: entry.LoaderVersion,
		Dirs:          append([]string(nil), entry.SyncDirs...),
	}, true
}

// Snapshot returns the current LAN state.
func (s *Service) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	sharing := s.identity.SyncPort > 0
	snapshot := Snapshot{
		Running:   s.running,
		Broadcast: s.broadcast,
		Sharing:   sharing,
		Worlds:    make([]World, 0, len(s.worlds)),
		Peers:     make([]Peer, 0, len(s.peers)),
		Warnings:  append([]string(nil), s.warnings...),
	}
	if sharing {
		snapshot.SharedInstanceID = s.identity.InstanceID
		snapshot.ShareDirs = append([]string(nil), s.identity.SyncDirs...)
	}
	peerByIP := map[string]string{}
	for _, peer := range s.peers {
		peerByIP[peer.Address] = peer.Name
	}
	for _, entry := range s.worlds {
		world := entry.World
		world.LastSeen = entry.lastSeen.Format(time.RFC3339)
		// Always emit an array (never null) so the UI can safely iterate it.
		world.Players = append([]PlayerInfo{}, entry.Players...)
		if peerByIP[world.Host] != "" {
			world.HostName = peerByIP[world.Host]
		}
		snapshot.Worlds = append(snapshot.Worlds, world)
	}
	for _, entry := range s.peers {
		peer := entry.Peer
		peer.LastSeen = entry.lastSeen.Format(time.RFC3339)
		snapshot.Peers = append(snapshot.Peers, peer)
	}
	sort.Slice(snapshot.Worlds, func(i, j int) bool {
		if snapshot.Worlds[i].Address == snapshot.Worlds[j].Address {
			return snapshot.Worlds[i].Source < snapshot.Worlds[j].Source
		}
		return snapshot.Worlds[i].Address < snapshot.Worlds[j].Address
	})
	sort.Slice(snapshot.Peers, func(i, j int) bool {
		return snapshot.Peers[i].Name < snapshot.Peers[j].Name
	})
	return snapshot
}

func (s *Service) startWorldListener() error {
	group := &net.UDPAddr{IP: net.ParseIP(lanGroup), Port: lanPort}
	conn, err := net.ListenMulticastUDP("udp4", nil, group)
	if err != nil {
		return fmt.Errorf("无法监听 Minecraft 局域网广播: %w", err)
	}
	_ = conn.SetReadBuffer(1 << 20)
	s.addCloser(conn)
	s.wg.Add(1)
	go s.readWorldLoop(conn)
	return nil
}

func (s *Service) readWorldLoop(conn *net.UDPConn) {
	defer s.wg.Done()
	buffer := make([]byte, 8192)
	for {
		n, source, err := conn.ReadFromUDP(buffer)
		if err != nil {
			if s.ctx != nil && s.ctx.Err() != nil {
				return
			}
			continue
		}
		ann, err := parseAnnouncement(buffer[:n])
		if err != nil || source == nil {
			continue
		}
		s.observeLANWorld(source.IP.String(), ann)
	}
}

func (s *Service) observeLANWorld(host string, ann announcement) {
	key := net.JoinHostPort(host, strconv.Itoa(ann.Port))
	now := time.Now()
	s.mu.Lock()
	entry, exists := s.worlds[key]
	if !exists {
		entry = &worldEntry{
			World: World{
				ID: key, Host: host, Port: ann.Port, Address: key,
				MOTD: ann.MOTD, Source: "lan", Players: []PlayerInfo{},
			},
			lastSeen: now,
		}
		s.worlds[key] = entry
		s.mu.Unlock()
		s.schedulePing(key, host, ann.Port)
		s.notify()
		return
	}
	entry.lastSeen = now
	if ann.MOTD != "" {
		entry.MOTD = ann.MOTD
	}
	entry.Source = "lan"
	s.mu.Unlock()
	s.schedulePing(key, host, ann.Port)
	s.notify()
}

func (s *Service) schedulePing(key, host string, port int) {
	s.mu.Lock()
	if !s.running || s.pinging[key] || time.Since(s.lastPing[key]) < pingCooldown {
		s.mu.Unlock()
		return
	}
	s.lastPing[key] = time.Now()
	s.pinging[key] = true
	ctx := s.ctx
	s.mu.Unlock()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		result, err := pingMinecraft(ctx, host, port, 2*time.Second)
		s.mu.Lock()
		delete(s.pinging, key)
		if err != nil {
			s.mu.Unlock()
			return
		}
		entry, ok := s.worlds[key]
		if !ok {
			s.mu.Unlock()
			return
		}
		entry.Version = result.Version
		entry.Online = result.Online
		entry.Max = result.Max
		entry.Players = append([]PlayerInfo{}, result.Players...)
		if entry.MOTD == "" {
			entry.MOTD = result.MOTD
		}
		s.mu.Unlock()
		s.notify()
	}()
}

func (s *Service) startPeerListener() {
	group := &net.UDPAddr{IP: net.ParseIP(presenceGroup), Port: presencePort}
	conn, err := net.ListenMulticastUDP("udp4", nil, group)
	if err != nil {
		s.addWarning("无法监听局域网玩家广播: " + err.Error())
		return
	}
	s.addCloser(conn)
	s.wg.Add(1)
	go s.readPeerLoop(conn)
}

func (s *Service) readPeerLoop(conn *net.UDPConn) {
	defer s.wg.Done()
	buffer := make([]byte, maxPresenceSize)
	for {
		n, source, err := conn.ReadFromUDP(buffer)
		if err != nil {
			if s.ctx != nil && s.ctx.Err() != nil {
				return
			}
			continue
		}
		if source == nil {
			continue
		}
		packet, err := decodePresence(buffer[:n], s.peerID)
		if err != nil {
			continue
		}
		now := time.Now()
		s.mu.Lock()
		entry, exists := s.peers[packet.ID]
		if !exists {
			entry = &peerEntry{Peer: Peer{ID: packet.ID}}
			s.peers[packet.ID] = entry
		}
		entry.Name = packet.Name
		entry.Instance = packet.Instance
		entry.MCVersion = packet.MCVersion
		entry.InstanceID = packet.InstanceID
		entry.Loader = packet.Loader
		entry.LoaderVersion = packet.LoaderVersion
		entry.SyncPort = packet.SyncPort
		entry.SyncDirs = append([]string(nil), packet.SyncDirs...)
		entry.syncToken = packet.SyncToken
		entry.CanSync = packet.SyncPort > 0 && packet.SyncToken != ""
		entry.Address = source.IP.String()
		entry.lastSeen = now
		s.mu.Unlock()
		s.notify()
	}
}

func (s *Service) startPeerSender() {
	target := &net.UDPAddr{IP: net.ParseIP(presenceGroup), Port: presencePort}
	conn, err := net.DialUDP("udp4", nil, target)
	if err != nil {
		s.addWarning("无法向局域网广播本机玩家信息: " + err.Error())
		return
	}
	s.addCloser(conn)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		s.sendPresence(conn)
		for {
			select {
			case <-s.ctx.Done():
				return
			case <-ticker.C:
				s.sendPresence(conn)
			}
		}
	}()
}

func (s *Service) sendPresence(conn *net.UDPConn) {
	s.mu.Lock()
	if !s.running || !s.broadcast {
		s.mu.Unlock()
		return
	}
	selfID := s.peerID
	identity := s.identity
	s.mu.Unlock()
	payload, err := encodePresence(selfID, identity)
	if err != nil {
		return
	}
	_, _ = conn.Write(payload)
}

func (s *Service) startLoops() {
	s.wg.Add(2)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(150 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-s.ctx.Done():
				return
			case <-s.notifyCh:
				s.mu.Lock()
				callback := s.onChange
				s.mu.Unlock()
				if callback != nil {
					callback(s.Snapshot())
				}
				// Coalesce bursty updates (broadcasts arrive every 1.5s).
				select {
				case <-s.ctx.Done():
					return
				case <-ticker.C:
				}
			}
		}
	}()
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		refresh := time.NewTicker(15 * time.Second)
		defer refresh.Stop()
		for {
			select {
			case <-s.ctx.Done():
				return
			case <-ticker.C:
				s.expire()
			case <-refresh.C:
				s.refreshPings()
			}
		}
	}()
}

func (s *Service) expire() {
	now := time.Now()
	changed := false
	s.mu.Lock()
	for key, entry := range s.worlds {
		ttl := lanWorldTTL
		if entry.Source == "scan" {
			ttl = scanWorldTTL
		}
		if now.Sub(entry.lastSeen) > ttl {
			delete(s.worlds, key)
			delete(s.lastPing, key)
			changed = true
		}
	}
	for key, entry := range s.peers {
		if now.Sub(entry.lastSeen) > peerTTL {
			delete(s.peers, key)
			changed = true
		}
	}
	s.mu.Unlock()
	if changed {
		s.notify()
	}
}

func (s *Service) refreshPings() {
	type target struct {
		key  string
		host string
		port int
	}
	s.mu.Lock()
	targets := make([]target, 0, len(s.worlds))
	for key, entry := range s.worlds {
		targets = append(targets, target{key: key, host: entry.Host, port: entry.Port})
	}
	s.mu.Unlock()
	for _, item := range targets {
		s.schedulePing(item.key, item.host, item.port)
	}
}

func (s *Service) observeScanned(host string, port int) {
	key := net.JoinHostPort(host, strconv.Itoa(port))
	now := time.Now()
	s.mu.Lock()
	entry, exists := s.worlds[key]
	if !exists {
		entry = &worldEntry{
			World: World{
				ID: key, Host: host, Port: port, Address: key,
				Source: "scan", Players: []PlayerInfo{},
			},
			lastSeen: now,
		}
		s.worlds[key] = entry
	} else {
		entry.Source = "scan"
		entry.lastSeen = now
	}
	s.mu.Unlock()
	s.schedulePing(key, host, port)
	s.notify()
}

func (s *Service) addCloser(closer io.Closer) {
	s.mu.Lock()
	s.closers = append(s.closers, closer)
	s.mu.Unlock()
}

func (s *Service) addWarning(message string) {
	s.mu.Lock()
	s.warnings = append(s.warnings, message)
	s.mu.Unlock()
	s.notify()
}

func (s *Service) notify() {
	select {
	case s.notifyCh <- struct{}{}:
	default:
	}
}

func randomPeerID() string {
	var data [8]byte
	if _, err := rand.Read(data[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(data[:])
}
