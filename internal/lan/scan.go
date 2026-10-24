package lan

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

const (
	scanWorkers = 64
	maxScanNets = 4
)

// ScanSubnets probes Minecraft's default port on the local IPv4 /24 networks.
// Results are merged into the live snapshot.
func (s *Service) ScanSubnets(ctx context.Context, progress func(done, total int)) error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return errors.New("请先开始局域网发现")
	}
	if s.scanStop != nil {
		s.mu.Unlock()
		return errors.New("局域网扫描已在进行中")
	}
	s.scanID++
	scanID := s.scanID
	scanCtx, cancel := context.WithCancel(ctx)
	s.scanStop = cancel
	// Add while holding s.mu so a concurrent Stop cannot begin Wait before the
	// scan is counted.
	s.wg.Add(1)
	s.mu.Unlock()
	defer s.wg.Done()
	defer func() {
		cancel()
		s.mu.Lock()
		if s.scanID == scanID {
			s.scanStop = nil
		}
		s.mu.Unlock()
	}()

	hosts := scanHosts()
	if len(hosts) == 0 {
		return errors.New("没有可用于扫描的局域网 IPv4 地址")
	}
	var next int64
	var done int64
	var progressMu sync.Mutex
	report := func() {
		if progress == nil {
			return
		}
		progressMu.Lock()
		current := int(atomic.LoadInt64(&done))
		progressMu.Unlock()
		progress(current, len(hosts))
	}
	report()

	var workers sync.WaitGroup
	for worker := 0; worker < scanWorkers; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for {
				if scanCtx.Err() != nil {
					return
				}
				index := atomic.AddInt64(&next, 1) - 1
				if index >= int64(len(hosts)) {
					return
				}
				host := hosts[index]
				// A successful TCP connection alone is not enough: proxies,
				// unrelated services and closed-but-listening ports all accept
				// connections. Only add a world after a real Server List Ping
				// response has been parsed successfully.
				s.scanHost(scanCtx, host, 25565)
				value := atomic.AddInt64(&done, 1)
				if value%8 == 0 || value == int64(len(hosts)) {
					report()
				}
			}
		}()
	}
	workers.Wait()
	report()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

// scanHost probes one address and records it only when the target completed a
// Minecraft Server List Ping exchange. This is intentionally stricter than a
// TCP dial: the requirement is an exact 25565 status response, not merely an
// open port.
func (s *Service) scanHost(ctx context.Context, host string, port int) {
	result, err := pingMinecraft(ctx, host, port, 2*time.Second)
	if err != nil {
		return
	}
	s.observeScannedResult(host, port, result)
}

func (s *Service) observeScannedResult(host string, port int, result pingResult) {
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
		}
		s.worlds[key] = entry
	}
	entry.Source = "scan"
	entry.lastSeen = now
	entry.MOTD = result.MOTD
	entry.Version = result.Version
	entry.Online = result.Online
	entry.Max = result.Max
	entry.Players = append([]PlayerInfo{}, result.Players...)
	s.mu.Unlock()
	s.notify()
}

// scanHosts returns injective host addresses for each local private /24.
func scanHosts() []string {
	bases, own := scanNetworks()
	seen := map[uint32]bool{}
	hosts := make([]string, 0, len(bases)*254)
	for _, base := range bases {
		for offset := uint32(1); offset < 255; offset++ {
			candidate := base + offset
			if own[candidate] {
				continue
			}
			if seen[candidate] {
				continue
			}
			seen[candidate] = true
			var raw [4]byte
			binary.BigEndian.PutUint32(raw[:], candidate)
			hosts = append(hosts, net.IP(raw[:]).String())
		}
	}
	sort.Strings(hosts)
	return hosts
}

func scanNetworks() ([]uint32, map[uint32]bool) {
	own := map[uint32]bool{}
	bases := map[uint32]bool{}
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, own
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch value := addr.(type) {
			case *net.IPNet:
				ip = value.IP
			case *net.IPAddr:
				ip = value.IP
			}
			ip4 := ip.To4()
			if ip4 == nil || !ip4.IsPrivate() {
				continue
			}
			value := binary.BigEndian.Uint32(ip4)
			own[value] = true
			bases[value&0xffffff00] = true
		}
	}
	out := make([]uint32, 0, len(bases))
	for base := range bases {
		out = append(out, base)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	if len(out) > maxScanNets {
		out = out[:maxScanNets]
	}
	return out, own
}
