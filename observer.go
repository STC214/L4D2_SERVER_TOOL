package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	windivert "github.com/xjasonlyu/windivert-go"
)

var procWinDivertHelperFormatIPv4Address = syscall.NewLazyDLL("WinDivert.dll").NewProc("WinDivertHelperFormatIPv4Address")
var observeRawLogCount int

func observeL4D2Flows(ctx context.Context, cfg Config, progress chan<- ScanProgress) {
	defer close(progress)
	observeRawLogCount = 0
	pid, ok := findProcessID("left4dead2.exe")
	if !ok {
		progress <- ScanProgress{Phase: "observe", Message: "未找到 left4dead2.exe 进程。请先启动求生之路2。", Finished: true}
		return
	}

	flowFilter := fmt.Sprintf("udp and processId == %d", pid)
	flowHandle, err := windivert.Open(flowFilter, windivert.LayerFlow, 0, windivert.FlagSniff|windivert.FlagRecvOnly)
	if err != nil {
		progress <- ScanProgress{Phase: "observe", Message: "WinDivert 启动失败。请确认程序已用管理员权限启动，并且 WinDivert.dll/WinDivert64.sys 与 exe 在同一目录。错误：" + err.Error(), Finished: true}
		return
	}
	defer flowHandle.Close()
	_ = flowHandle.SetParam(windivert.QueueLength, 2048)

	packetHandle, err := windivert.Open("udp", windivert.LayerNetwork, 0, windivert.FlagSniff|windivert.FlagRecvOnly)
	if err != nil {
		progress <- ScanProgress{Phase: "observe", Message: "WinDivert 网络层监听启动失败：" + err.Error(), Finished: true}
		return
	}
	defer packetHandle.Close()
	_ = packetHandle.SetParam(windivert.QueueLength, 8192)

	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = flowHandle.Shutdown(windivert.ShutdownRecv)
			_ = packetHandle.Shutdown(windivert.ShutdownRecv)
		case <-done:
		}
	}()

	progress <- ScanProgress{Phase: "observe", Message: fmt.Sprintf("正在观察 left4dead2.exe，PID=%d。正在记录游戏 UDP 端口并从网络包提取服务器 IP。", pid)}

	candidates := make(chan string, 512)
	defer close(candidates)
	go observeInfoWorker(ctx, cfg, candidates, progress)

	ports := &observedPorts{ports: map[int]bool{}}
	seen := &candidateSet{seen: map[string]bool{}}
	errCh := make(chan string, 2)
	go observeGameUDPPorts(ctx, flowHandle, pid, ports, errCh)
	go observeUDPPackets(ctx, packetHandle, ports, seen, candidates, errCh)

	select {
	case <-ctx.Done():
		progress <- ScanProgress{Phase: "observe", Message: "观察已停止。", Finished: true}
	case msg := <-errCh:
		if ctx.Err() != nil {
			progress <- ScanProgress{Phase: "observe", Message: "观察已停止。", Finished: true}
		} else {
			progress <- ScanProgress{Phase: "observe", Message: msg, Finished: true}
		}
	}
}

type observedPorts struct {
	mu    sync.RWMutex
	ports map[int]bool
}

func (p *observedPorts) add(port int) {
	if port <= 0 {
		return
	}
	p.mu.Lock()
	p.ports[port] = true
	p.mu.Unlock()
}

func (p *observedPorts) has(port int) bool {
	p.mu.RLock()
	ok := p.ports[port]
	p.mu.RUnlock()
	return ok
}

type candidateSet struct {
	mu   sync.Mutex
	seen map[string]bool
}

func (s *candidateSet) add(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.seen[key] {
		return false
	}
	s.seen[key] = true
	return true
}

func observeGameUDPPorts(ctx context.Context, handle windivert.Handle, pid uint32, ports *observedPorts, errCh chan<- string) {
	packet := make([]byte, 64)
	for {
		var addr windivert.Address
		_, err := handle.Recv(packet, &addr)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			errCh <- "WinDivert Flow 层读取失败：" + err.Error()
			return
		}
		if addr.Event() != windivert.EventFlowEstablished {
			continue
		}
		flow := addr.Flow()
		if flow == nil || flow.ProcessID != pid || (flow.Protocol != 17 && flow.Protocol != 6) {
			continue
		}
		localPort := normalizeObservedPort(flow.LocalPort)
		ports.add(localPort)
		if observeRawLogCount < 20 {
			logObservedRawAddress(fmt.Sprintf("game-local-port %d", localPort), flow.Protocol, flow.RemotePort, flow.RemoteAddress)
		}
	}
}

func observeUDPPackets(ctx context.Context, handle windivert.Handle, ports *observedPorts, seen *candidateSet, candidates chan<- string, errCh chan<- string) {
	packet := make([]byte, 65535)
	for {
		var addr windivert.Address
		n, err := handle.Recv(packet, &addr)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			errCh <- "WinDivert 网络层读取失败：" + err.Error()
			return
		}
		if addr.Event() != windivert.EventNetworkPacket {
			continue
		}
		pkt, ok := parseIPv4UDPPacket(packet[:n])
		if !ok {
			continue
		}
		var remoteIP string
		var remotePort int
		if ports.has(pkt.srcPort) && looksLikeSourceServerPort(pkt.dstPort) {
			remoteIP = pkt.dstIP
			remotePort = pkt.dstPort
		} else if ports.has(pkt.dstPort) && looksLikeSourceServerPort(pkt.srcPort) {
			remoteIP = pkt.srcIP
			remotePort = pkt.srcPort
		} else {
			continue
		}
		if remoteIP == "" || remoteIP == "0.0.0.0" || remoteIP == "255.255.255.255" {
			continue
		}
		key := net.JoinHostPort(remoteIP, strconv.Itoa(remotePort))
		if !seen.add(key) {
			continue
		}
		select {
		case candidates <- key:
		default:
		}
	}
}

type udpPacketInfo struct {
	srcIP   string
	dstIP   string
	srcPort int
	dstPort int
}

func parseIPv4UDPPacket(packet []byte) (udpPacketInfo, bool) {
	if len(packet) < 28 {
		return udpPacketInfo{}, false
	}
	version := packet[0] >> 4
	if version != 4 {
		return udpPacketInfo{}, false
	}
	ihl := int(packet[0]&0x0f) * 4
	if ihl < 20 || len(packet) < ihl+8 {
		return udpPacketInfo{}, false
	}
	if packet[9] != 17 {
		return udpPacketInfo{}, false
	}
	return udpPacketInfo{
		srcIP:   net.IPv4(packet[12], packet[13], packet[14], packet[15]).String(),
		dstIP:   net.IPv4(packet[16], packet[17], packet[18], packet[19]).String(),
		srcPort: int(binary.BigEndian.Uint16(packet[ihl : ihl+2])),
		dstPort: int(binary.BigEndian.Uint16(packet[ihl+2 : ihl+4])),
	}, true
}

func observeInfoWorker(ctx context.Context, cfg Config, candidates <-chan string, progress chan<- ScanProgress) {
	for {
		select {
		case <-ctx.Done():
			return
		case key, ok := <-candidates:
			if !ok {
				return
			}
			info := queryServer(ctx, key, 1600*time.Millisecond)
			if info.Error != "" {
				info = ServerInfo{
					Address:      key,
					Host:         "候选服务器，暂未获取名称",
					Players:      -1,
					MaxPlayers:   -1,
					PingMS:       0,
					LastSeen:     time.Now(),
					BlockReasons: []string{"观察到 L4D2 UDP 流量"},
				}
				applyIPRulesOnly(&info, cfg)
			} else {
				applyRules(&info, cfg)
			}
			select {
			case progress <- ScanProgress{Phase: "observe", Server: &info, Message: "观察到候选服务器：" + key}:
			case <-ctx.Done():
				return
			}
		}
	}
}

func flowAddressToIP(raw [16]uint8) string {
	if bytesAllZero(raw[4:16]) && !bytesAllZero(raw[0:4]) {
		return formatWinDivertIPv4(raw)
	}
	if raw[10] == 0xff && raw[11] == 0xff {
		return net.IPv4(raw[12], raw[13], raw[14], raw[15]).String()
	}
	if bytesAllZero(raw[0:12]) && !bytesAllZero(raw[12:16]) {
		return net.IPv4(raw[12], raw[13], raw[14], raw[15]).String()
	}
	for _, offset := range []int{12, 8, 4, 0} {
		if !bytesAllZero(raw[offset : offset+4]) {
			ip := formatWinDivertIPv4At(raw, offset)
			if ip != "" && ip != "0.0.0.0" {
				return ip
			}
		}
	}
	if !bytesAllZero(raw[:]) {
		return net.IP(raw[:]).String()
	}
	return ""
}

func formatWinDivertIPv4(raw [16]uint8) string {
	return formatWinDivertIPv4At(raw, 0)
}

func formatWinDivertIPv4At(raw [16]uint8, offset int) string {
	var buf [64]byte
	addr := *(*uint32)(unsafe.Pointer(&raw[offset]))
	r, _, _ := procWinDivertHelperFormatIPv4Address.Call(
		uintptr(addr),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
	)
	if r == 0 {
		return net.IPv4(raw[offset], raw[offset+1], raw[offset+2], raw[offset+3]).String()
	}
	n := 0
	for n < len(buf) && buf[n] != 0 {
		n++
	}
	return string(buf[:n])
}

func logObservedRawAddress(label string, proto uint8, port uint16, raw [16]uint8) {
	if observeRawLogCount >= 20 {
		return
	}
	observeRawLogCount++
	debugLog(fmt.Sprintf("observe raw %s proto=%d port=%d bytes=% x", label, proto, port, raw))
}

func bytesAllZero(b []byte) bool {
	for _, v := range b {
		if v != 0 {
			return false
		}
	}
	return true
}

func normalizeObservedPort(port uint16) int {
	p := int(port)
	swapped := int((port>>8)&0xff) | int(port&0xff)<<8
	if looksLikeSourceServerPort(swapped) && !looksLikeSourceServerPort(p) {
		return swapped
	}
	return p
}

func looksLikeSourceServerPort(port int) bool {
	return (port >= 20000 && port <= 30000) || port == 27011 || strings.HasPrefix(strconv.Itoa(port), "27")
}
