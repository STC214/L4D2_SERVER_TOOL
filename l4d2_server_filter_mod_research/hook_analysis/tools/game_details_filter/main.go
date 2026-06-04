package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"
	"unsafe"

	windivert "github.com/xjasonlyu/windivert-go"
)

const (
	cpGBK = 936

	protoUDP = 17
)

var (
	kernel32                = syscall.NewLazyDLL("kernel32.dll")
	procMultiByteToWideChar = kernel32.NewProc("MultiByteToWideChar")
)

type packetInfo struct {
	srcIP     string
	dstIP     string
	srcPort   int
	dstPort   int
	proto     uint8
	payload   []byte
	remoteKey string
}

type serverDetails struct {
	Name       string
	Online     string
	Local      string
	Mode       string
	Campaign   string
	Mission    string
	Difficulty string
	Raw        map[string]string
}

type filterState struct {
	mu       sync.RWMutex
	blocked  map[string]string
	keywords []string
	ips      []string
}

type counters struct {
	seen     int
	forward  int
	dropped  int
	matched  int
	parseErr int
}

func main() {
	keywordsFlag := flag.String("keywords", "", "comma-separated keywords matched against decoded GameDetailsServer fields")
	ipsFlag := flag.String("ips", "", "comma-separated IP or IP:port values to block immediately")
	dryRun := flag.Bool("dry-run", false, "log matches but forward every packet")
	verbose := flag.Bool("v", false, "print forwarded GameDetailsServer rows too")
	flag.Parse()

	state := &filterState{
		blocked:  map[string]string{},
		keywords: splitList(*keywordsFlag),
		ips:      splitList(*ipsFlag),
	}
	for _, ip := range state.ips {
		state.blocked[ip] = "configured ip"
	}
	if len(state.keywords) == 0 && len(state.ips) == 0 {
		fmt.Println("No keywords or IPs configured. Use -keywords \"多特,绕过Steam验证\" or -ips \"1.2.3.4:27015\".")
		fmt.Println("The tool will still run and only print decoded GameDetailsServer rows.")
	}

	filter := "ip and udp and (udp.SrcPort == 27005 or udp.DstPort == 27005)"
	handle, err := windivert.Open(filter, windivert.LayerNetwork, 0, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "WinDivert open failed: %v\n", err)
		os.Exit(1)
	}
	defer handle.Close()
	_ = handle.SetParam(windivert.QueueLength, 8192)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stop
		_ = handle.Shutdown(windivert.ShutdownRecv)
	}()

	fmt.Println("L4D2 GameDetailsServer filter running.")
	fmt.Println("Filter:", filter)
	fmt.Println("Dry run:", *dryRun)
	if len(state.keywords) > 0 {
		fmt.Println("Keywords:", strings.Join(state.keywords, ", "))
	}
	if len(state.ips) > 0 {
		fmt.Println("Initial IP blocks:", strings.Join(state.ips, ", "))
	}
	fmt.Println("Press Ctrl+C to stop.")

	var stats counters
	packet := make([]byte, 65535)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	go printStats(ticker.C, &stats)

	for {
		var addr windivert.Address
		n, err := handle.Recv(packet, &addr)
		if err != nil {
			break
		}
		current := append([]byte(nil), packet[:n]...)
		stats.seen++

		drop, reason := shouldDropPacket(current, state, &stats, *verbose)
		if drop {
			stats.dropped++
			fmt.Println("DROP", reason)
			if *dryRun {
				_, _ = handle.Send(current, &addr)
				stats.forward++
			}
			continue
		}
		if _, err := handle.Send(current, &addr); err != nil {
			fmt.Fprintf(os.Stderr, "send failed: %v\n", err)
			continue
		}
		stats.forward++
	}
	fmt.Printf("Stopped. seen=%d forwarded=%d dropped=%d matched=%d parse_errors=%d\n", stats.seen, stats.forward, stats.dropped, stats.matched, stats.parseErr)
}

func shouldDropPacket(packet []byte, state *filterState, stats *counters, verbose bool) (bool, string) {
	info, ok := parseIPv4UDPPacket(packet)
	if !ok {
		return false, ""
	}
	if reason, ok := state.isBlocked(info.remoteKey); ok {
		return true, fmt.Sprintf("%s blocked=%s", info.remoteKey, reason)
	}
	if reason, ok := state.isBlocked(stripPort(info.remoteKey)); ok {
		return true, fmt.Sprintf("%s blocked_ip=%s", info.remoteKey, reason)
	}
	details, ok := parseGameDetailsServer(info.payload)
	if !ok {
		return false, ""
	}
	stats.matched++
	if details.Online != "" {
		info.remoteKey = details.Online
	}
	matched, reason := state.matches(details)
	if matched {
		state.addBlock(info.remoteKey, reason)
		return true, fmt.Sprintf("%s name=%q reason=%s", info.remoteKey, details.Name, reason)
	}
	if verbose {
		fmt.Printf("ALLOW %s name=%q mode=%s campaign=%s difficulty=%s\n", info.remoteKey, details.Name, details.Mode, details.Campaign, details.Difficulty)
	}
	return false, ""
}

func parseIPv4UDPPacket(packet []byte) (packetInfo, bool) {
	if len(packet) < 28 || packet[0]>>4 != 4 {
		return packetInfo{}, false
	}
	ihl := int(packet[0]&0x0f) * 4
	if ihl < 20 || len(packet) < ihl+8 || packet[9] != protoUDP {
		return packetInfo{}, false
	}
	srcIP := net.IPv4(packet[12], packet[13], packet[14], packet[15]).String()
	dstIP := net.IPv4(packet[16], packet[17], packet[18], packet[19]).String()
	srcPort := int(binary.BigEndian.Uint16(packet[ihl : ihl+2]))
	dstPort := int(binary.BigEndian.Uint16(packet[ihl+2 : ihl+4]))
	remoteIP, remotePort := srcIP, srcPort
	if srcPort == 27005 {
		remoteIP, remotePort = dstIP, dstPort
	}
	return packetInfo{
		srcIP:     srcIP,
		dstIP:     dstIP,
		srcPort:   srcPort,
		dstPort:   dstPort,
		proto:     protoUDP,
		payload:   packet[ihl+8:],
		remoteKey: net.JoinHostPort(remoteIP, strconv.Itoa(remotePort)),
	}, true
}

func parseGameDetailsServer(payload []byte) (serverDetails, bool) {
	if !isBinaryKVPayload(payload) {
		return serverDetails{}, false
	}
	offset := 16
	if offset >= len(payload) || payload[offset] != 0x00 {
		return serverDetails{}, false
	}
	offset++
	root, ok := readCString(payload, &offset)
	if !ok || root != "GameDetailsServer" {
		return serverDetails{}, false
	}
	fields := map[string]string{}
	readBinaryKVFields(payload, &offset, root, 0, fields)
	d := serverDetails{
		Name:       fields["GameDetailsServer.Server.Name"],
		Online:     fields["GameDetailsServer.Server.adronline"],
		Local:      fields["GameDetailsServer.Server.adrlocal"],
		Mode:       fields["GameDetailsServer.game.Mode"],
		Campaign:   fields["GameDetailsServer.game.campaign"],
		Mission:    fields["GameDetailsServer.game.MissionInfo.MissionFile"],
		Difficulty: fields["GameDetailsServer.game.difficulty"],
		Raw:        fields,
	}
	return d, d.Name != "" || d.Online != ""
}

func readBinaryKVFields(data []byte, offset *int, prefix string, depth int, fields map[string]string) {
	if depth > 32 {
		return
	}
	for *offset < len(data) {
		typ := data[*offset]
		*offset = *offset + 1
		if typ == 0x08 || typ == 0x0b {
			return
		}
		key, ok := readCString(data, offset)
		if !ok {
			return
		}
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}
		switch typ {
		case 0x00:
			readBinaryKVFields(data, offset, path, depth+1, fields)
		case 0x01:
			value, ok := readCString(data, offset)
			if !ok {
				return
			}
			fields[path] = value
		case 0x02:
			if *offset+4 > len(data) {
				return
			}
			fields[path] = strconv.FormatUint(uint64(binary.BigEndian.Uint32(data[*offset:*offset+4])), 10)
			*offset += 4
		case 0x03:
			if *offset+4 > len(data) {
				return
			}
			fields[path] = strconv.FormatUint(uint64(binary.BigEndian.Uint32(data[*offset:*offset+4])), 10)
			*offset += 4
		case 0x05:
			value, ok := readUTF16CString(data, offset)
			if !ok {
				return
			}
			fields[path] = value
		case 0x07:
			if *offset+8 > len(data) {
				return
			}
			fields[path] = strconv.FormatUint(binary.BigEndian.Uint64(data[*offset:*offset+8]), 10)
			*offset += 8
		default:
			return
		}
	}
}

func (s *filterState) matches(details serverDetails) (bool, string) {
	values := []string{
		details.Name,
		details.Online,
		details.Local,
		details.Mode,
		details.Campaign,
		details.Mission,
		details.Difficulty,
	}
	for key, value := range details.Raw {
		if strings.Contains(strings.ToLower(key), "name") || strings.Contains(strings.ToLower(key), "title") {
			values = append(values, value)
		}
	}
	joined := strings.ToLower(strings.Join(values, "\n"))
	for _, keyword := range s.keywords {
		if keyword != "" && strings.Contains(joined, strings.ToLower(keyword)) {
			return true, "keyword=" + keyword
		}
	}
	for _, ip := range s.ips {
		if ip != "" && (strings.Contains(details.Online, ip) || strings.Contains(details.Local, ip)) {
			return true, "ip=" + ip
		}
	}
	return false, ""
}

func (s *filterState) addBlock(remote, reason string) {
	if remote == "" {
		return
	}
	s.mu.Lock()
	s.blocked[remote] = reason
	if ip := stripPort(remote); ip != remote {
		s.blocked[ip] = reason
	}
	s.mu.Unlock()
}

func (s *filterState) isBlocked(remote string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	reason, ok := s.blocked[remote]
	return reason, ok
}

func splitList(text string) []string {
	var out []string
	for _, item := range strings.Split(text, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func stripPort(endpoint string) string {
	host, _, err := net.SplitHostPort(endpoint)
	if err != nil {
		return endpoint
	}
	return host
}

func isBinaryKVPayload(payload []byte) bool {
	return len(payload) >= 17 &&
		payload[0] == 0xff &&
		payload[1] == 0xff &&
		payload[2] == 0xff &&
		payload[3] == 0xff &&
		payload[4] == 0x00
}

func readCString(data []byte, offset *int) (string, bool) {
	start := *offset
	for *offset < len(data) && data[*offset] != 0 {
		*offset++
	}
	if *offset >= len(data) {
		return "", false
	}
	raw := data[start:*offset]
	*offset = *offset + 1
	return decodeServerText(raw), true
}

func readUTF16CString(data []byte, offset *int) (string, bool) {
	start := *offset
	for *offset+1 < len(data) {
		if data[*offset] == 0 && data[*offset+1] == 0 {
			raw := data[start:*offset]
			*offset += 2
			if len(raw)%2 != 0 {
				return "", false
			}
			wide := make([]uint16, len(raw)/2)
			for i := range wide {
				wide[i] = binary.LittleEndian.Uint16(raw[i*2 : i*2+2])
			}
			return syscall.UTF16ToString(wide), true
		}
		*offset += 2
	}
	return "", false
}

func decodeServerText(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	if utf8.Valid(raw) {
		return string(raw)
	}
	if decoded := decodeCP936(raw); decoded != "" {
		return decoded
	}
	return string(raw)
}

func decodeCP936(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	n, _, _ := procMultiByteToWideChar.Call(
		cpGBK,
		0,
		uintptr(unsafe.Pointer(&raw[0])),
		uintptr(len(raw)),
		0,
		0,
	)
	if n == 0 {
		return ""
	}
	buf := make([]uint16, n)
	ret, _, _ := procMultiByteToWideChar.Call(
		cpGBK,
		0,
		uintptr(unsafe.Pointer(&raw[0])),
		uintptr(len(raw)),
		uintptr(unsafe.Pointer(&buf[0])),
		n,
	)
	if ret == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf)
}

func printStats(ticks <-chan time.Time, stats *counters) {
	for range ticks {
		fmt.Printf("stats seen=%d forwarded=%d dropped=%d matched=%d parse_errors=%d\n", stats.seen, stats.forward, stats.dropped, stats.matched, stats.parseErr)
	}
}
