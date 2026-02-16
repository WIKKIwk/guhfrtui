package discovery

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	reader18 "new_era_go/internal/protocol/reader18"
)

// Candidate is a reachable host:port that looks likely to be an RFID reader.
type Candidate struct {
	Host          string
	Port          int
	Score         int
	Banner        string
	Reason        string
	Verified      bool
	ReaderAddress byte
	Protocol      string
}

// ScanOptions controls LAN discovery behavior.
type ScanOptions struct {
	Ports                 []int
	Timeout               time.Duration
	Concurrency           int
	HostLimitPerInterface int
}

func DefaultOptions() ScanOptions {
	return ScanOptions{
		Ports: []int{
			2022, 6000, 4001, 10001, 5000,
		},
		Timeout:               180 * time.Millisecond,
		Concurrency:           96,
		HostLimitPerInterface: 254,
	}
}

// Scan probes local LAN segments and returns possible reader endpoints.
func Scan(ctx context.Context, opts ScanOptions) ([]Candidate, error) {
	if len(opts.Ports) == 0 {
		opts = DefaultOptions()
	}
	if opts.Timeout <= 0 {
		opts.Timeout = DefaultOptions().Timeout
	}
	if opts.Concurrency <= 0 {
		opts.Concurrency = DefaultOptions().Concurrency
	}
	if opts.HostLimitPerInterface <= 0 {
		opts.HostLimitPerInterface = DefaultOptions().HostLimitPerInterface
	}

	prefixes, localIPs, err := localPrefixes()
	if err != nil {
		return nil, err
	}
	if len(prefixes) == 0 {
		return nil, fmt.Errorf("no active IPv4 network interfaces found")
	}

	hosts := make([]netip.Addr, 0, 512)
	seenHosts := make(map[string]struct{}, 512)
	skipLocal := make(map[string]struct{}, len(localIPs))
	for _, ip := range localIPs {
		skipLocal[ip.String()] = struct{}{}
	}

	for _, prefix := range prefixes {
		for _, host := range hostsFromPrefix(prefix, opts.HostLimitPerInterface) {
			if _, skip := skipLocal[host.String()]; skip {
				continue
			}
			key := host.String()
			if _, exists := seenHosts[key]; exists {
				continue
			}
			seenHosts[key] = struct{}{}
			hosts = append(hosts, host)
		}
	}

	for _, host := range arpNeighbors() {
		if _, skip := skipLocal[host.String()]; skip {
			continue
		}
		key := host.String()
		if _, exists := seenHosts[key]; exists {
			continue
		}
		seenHosts[key] = struct{}{}
		hosts = append(hosts, host)
	}

	for _, host := range defaultCandidateIPs(localIPs) {
		if _, skip := skipLocal[host.String()]; skip {
			continue
		}
		key := host.String()
		if _, exists := seenHosts[key]; exists {
			continue
		}
		seenHosts[key] = struct{}{}
		hosts = append(hosts, host)
	}

	type target struct {
		host netip.Addr
		port int
	}

	jobs := make(chan target)
	results := make(chan Candidate, opts.Concurrency)
	var wg sync.WaitGroup

	worker := func() {
		defer wg.Done()
		for t := range jobs {
			candidate, ok := probeTarget(ctx, t.host, t.port, opts.Timeout)
			if !ok {
				continue
			}
			select {
			case results <- candidate:
			case <-ctx.Done():
				return
			}
		}
	}

	for i := 0; i < opts.Concurrency; i++ {
		wg.Add(1)
		go worker()
	}

	go func() {
		defer close(jobs)
		for _, host := range hosts {
			for _, port := range opts.Ports {
				select {
				case jobs <- target{host: host, port: port}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	candidates := make([]Candidate, 0, 16)
	seenCandidates := make(map[string]struct{}, 16)
	for candidate := range results {
		key := net.JoinHostPort(candidate.Host, strconv.Itoa(candidate.Port))
		if _, exists := seenCandidates[key]; exists {
			continue
		}
		seenCandidates[key] = struct{}{}
		candidates = append(candidates, candidate)
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Score != candidates[j].Score {
			return candidates[i].Score > candidates[j].Score
		}
		if candidates[i].Host != candidates[j].Host {
			return candidates[i].Host < candidates[j].Host
		}
		return candidates[i].Port < candidates[j].Port
	})

	if ctx.Err() != nil {
		return candidates, ctx.Err()
	}

	return candidates, nil
}

func probeTarget(ctx context.Context, host netip.Addr, port int, timeout time.Duration) (Candidate, bool) {
	dialer := net.Dialer{Timeout: timeout}
	address := net.JoinHostPort(host.String(), strconv.Itoa(port))
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return Candidate{}, false
	}
	defer conn.Close()

	score := portScore(port)
	verified, readerAddr, protocol := probeReaderProtocol(conn, timeout)
	banner := ""
	reason := "open tcp port"
	if verified {
		score += 160
		reason = fmt.Sprintf("reader protocol: %s addr=0x%02X", protocol, readerAddr)
	} else {
		banner = readBanner(conn)
		if banner != "" {
			score += keywordScore(banner)
			reason = "banner matched"
		}
	}

	return Candidate{
		Host:          host.String(),
		Port:          port,
		Score:         score,
		Banner:        banner,
		Reason:        reason,
		Verified:      verified,
		ReaderAddress: readerAddr,
		Protocol:      protocol,
	}, true
}

func probeReaderProtocol(conn net.Conn, timeout time.Duration) (bool, byte, string) {
	try := []struct {
		name string
		cmd  func(addr byte) []byte
	}{
		{name: "get-info", cmd: reader18.GetReaderInfoCommand},
		{name: "inventory", cmd: reader18.InventorySingleCommand},
	}
	addrs := []byte{reader18.DefaultReaderAddress, reader18.BroadcastReaderAddress}

	for _, probe := range try {
		for _, addr := range addrs {
			payload := probe.cmd(addr)
			frames, _ := sendProbeAndReadFrames(conn, payload, timeout)
			for _, frame := range frames {
				switch probe.name {
				case "get-info":
					if frame.Command == reader18.CmdGetReaderInfo {
						return true, frame.Address, "reader18/" + probe.name
					}
				case "inventory":
					if frame.Command == reader18.CmdInventory {
						return true, frame.Address, "reader18/" + probe.name
					}
				}
			}
		}
	}

	return false, 0x00, ""
}

func sendProbeAndReadFrames(conn net.Conn, payload []byte, timeout time.Duration) ([]reader18.Frame, []byte) {
	if len(payload) == 0 {
		return nil, nil
	}

	_ = conn.SetWriteDeadline(time.Now().Add(timeout))
	if _, err := conn.Write(payload); err != nil {
		return nil, nil
	}

	deadline := time.Now().Add(timeout * 2)
	if timeout < 200*time.Millisecond {
		deadline = time.Now().Add(420 * time.Millisecond)
	}

	buf := make([]byte, 512)
	stream := make([]byte, 0, 1024)

	for time.Now().Before(deadline) {
		_ = conn.SetReadDeadline(time.Now().Add(90 * time.Millisecond))
		n, err := conn.Read(buf)
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}
			return nil, stream
		}
		if n <= 0 {
			continue
		}

		stream = append(stream, buf[:n]...)
		if len(stream) > 4096 {
			stream = append([]byte{}, stream[len(stream)-2048:]...)
		}

		frames, remaining := reader18.ParseFrames(stream)
		stream = remaining
		if len(frames) > 0 {
			return frames, remaining
		}
	}

	return nil, stream
}

func readBanner(conn net.Conn) string {
	_ = conn.SetReadDeadline(time.Now().Add(140 * time.Millisecond))
	buf := make([]byte, 256)
	n, err := conn.Read(buf)
	if err != nil || n <= 0 {
		return ""
	}
	text := sanitizeASCII(string(buf[:n]))
	if len(text) > 140 {
		text = text[:140]
	}
	return text
}

func sanitizeASCII(in string) string {
	var b strings.Builder
	b.Grow(len(in))
	for _, r := range in {
		if r >= 32 && r <= 126 {
			b.WriteRune(r)
			continue
		}
		if r == '\n' || r == '\r' || r == '\t' {
			b.WriteRune(' ')
		}
	}
	return strings.TrimSpace(b.String())
}

func portScore(port int) int {
	switch port {
	case 2022, 4001, 5000, 6000, 7000, 10001:
		return 55
	case 23:
		return 30
	case 80, 443, 8080:
		return 20
	default:
		return 10
	}
}

func keywordScore(banner string) int {
	text := strings.ToLower(banner)
	score := 0
	keywords := []string{"rfid", "uhf", "reader", "impinj", "epc", "e710"}
	for _, keyword := range keywords {
		if strings.Contains(text, keyword) {
			score += 18
		}
	}
	return score
}

func localPrefixes() ([]netip.Prefix, []netip.Addr, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, nil, err
	}

	prefixes := make([]netip.Prefix, 0, 8)
	locals := make([]netip.Addr, 0, 8)
	seenPrefixes := make(map[string]struct{}, 8)

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip4 := ipNet.IP.To4()
			if ip4 == nil {
				continue
			}

			localAddr, ok := netip.AddrFromSlice(ip4)
			if !ok {
				continue
			}
			locals = append(locals, localAddr)

			ones, bits := ipNet.Mask.Size()
			if bits != 32 || ones <= 0 {
				continue
			}

			scanBits := ones
			if scanBits < 24 {
				scanBits = 24
			}
			prefix := netip.PrefixFrom(localAddr, scanBits).Masked()
			key := prefix.String()
			if _, exists := seenPrefixes[key]; exists {
				continue
			}
			seenPrefixes[key] = struct{}{}
			prefixes = append(prefixes, prefix)
		}
	}

	sort.Slice(prefixes, func(i, j int) bool {
		return prefixes[i].String() < prefixes[j].String()
	})

	return prefixes, locals, nil
}

func hostsFromPrefix(prefix netip.Prefix, limit int) []netip.Addr {
	if !prefix.Addr().Is4() {
		return nil
	}
	bits := prefix.Bits()
	if bits >= 31 {
		return nil
	}

	netIP := ipv4ToUint32(prefix.Addr())
	hostBits := 32 - bits
	hostCount := uint64(1) << uint(hostBits)
	if hostCount <= 2 {
		return nil
	}

	first := netIP + 1
	last := netIP + uint32(hostCount-2)
	hosts := make([]netip.Addr, 0, min(limit, int(hostCount-2)))

	for ip := first; ip <= last; ip++ {
		hosts = append(hosts, uint32ToIPv4(ip))
		if len(hosts) >= limit {
			break
		}
	}

	return hosts
}

func ipv4ToUint32(addr netip.Addr) uint32 {
	v := addr.As4()
	return binary.BigEndian.Uint32(v[:])
}

func uint32ToIPv4(v uint32) netip.Addr {
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], v)
	return netip.AddrFrom4(buf)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func arpNeighbors() []netip.Addr {
	file, err := os.Open("/proc/net/arp")
	if err != nil {
		return nil
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	neighbors := make([]netip.Addr, 0, 32)
	seen := make(map[string]struct{}, 32)
	first := true
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if first {
			first = false
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		ipText := fields[0]
		addr, err := netip.ParseAddr(ipText)
		if err != nil || !addr.Is4() {
			continue
		}
		if addr.IsLoopback() || addr.IsUnspecified() {
			continue
		}
		key := addr.String()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		neighbors = append(neighbors, addr)
	}
	return neighbors
}

func defaultCandidateIPs(localIPs []netip.Addr) []netip.Addr {
	out := make([]netip.Addr, 0, 64)
	seen := make(map[string]struct{}, 64)

	add := func(ipText string) {
		addr, err := netip.ParseAddr(ipText)
		if err != nil || !addr.Is4() {
			return
		}
		key := addr.String()
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, addr)
	}

	staticSeeds := []string{
		"192.168.0.10", "192.168.0.100", "192.168.0.200",
		"192.168.1.10", "192.168.1.100", "192.168.1.200",
		"10.0.0.10", "10.0.0.100", "10.0.0.200",
		"10.10.10.10", "10.10.10.100", "10.10.10.200",
	}
	for _, seed := range staticSeeds {
		add(seed)
	}

	for _, local := range localIPs {
		if !local.Is4() {
			continue
		}
		v := local.As4()
		a, b, c := v[0], v[1], v[2]

		hostOctets := []byte{2, 10, 20, 50, 100, 150, 200, 250}
		for _, d := range hostOctets {
			add(fmt.Sprintf("%d.%d.%d.%d", a, b, c, d))
		}
		add(fmt.Sprintf("%d.%d.0.10", a, b))
		add(fmt.Sprintf("%d.%d.1.10", a, b))
		add(fmt.Sprintf("%d.%d.2.10", a, b))
	}

	return out
}
