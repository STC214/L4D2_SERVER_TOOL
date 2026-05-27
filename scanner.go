package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

const appIDLeft4Dead2 = 550

var masterServers = []string{
	"hl2master.steampowered.com:27011",
	"208.64.200.39:27011",
	"208.64.200.52:27011",
	"208.64.200.65:27011",
	"208.64.200.65:27015",
}

type Config struct {
	NameKeywords []string `json:"name_keywords"`
	TagKeywords  []string `json:"tag_keywords"`
	MapKeywords  []string `json:"map_keywords"`
	IPExact      []string `json:"ip_exact"`
	IPCIDR       []string `json:"ip_cidr"`
	HideEmpty    bool     `json:"hide_empty"`
	HideFull     bool     `json:"hide_full"`
	HidePassword bool     `json:"hide_password"`
	MaxPingMS    int      `json:"max_ping_ms"`
	MaxPlayers   int      `json:"max_players"`
	Region       byte     `json:"region"`
	QueryWorkers int      `json:"query_workers"`
	MasterLimit  int      `json:"master_limit"`
	SteamAPIKey  string   `json:"steam_api_key"`
}

type ServerInfo struct {
	Address         string
	PreviousAddress string
	IPChanged       bool
	Host            string
	Map             string
	Folder          string
	Game            string
	Keywords        string
	Players         int
	MaxPlayers      int
	Bots            int
	Password        bool
	VAC             bool
	PingMS          int
	Blocked         bool
	BlockReasons    []string
	LastSeen        time.Time
	Error           string
}

type ScanProgress struct {
	Phase     string
	Done      int
	Total     int
	Server    *ServerInfo
	Finished  bool
	Message   string
	All       []ServerInfo
	BlockedIP []string
}

func defaultConfig() Config {
	return Config{
		NameKeywords: []string{"rpg", "药抗", "多特", "QQ群", "vip", "shop", "无限", "变态"},
		TagKeywords:  []string{"modded"},
		HideEmpty:    true,
		HidePassword: true,
		MaxPingMS:    180,
		MaxPlayers:   8,
		Region:       0xFF,
		QueryWorkers: 96,
		MasterLimit:  2500,
	}
}

func scanServers(ctx context.Context, cfg Config, progress chan<- ScanProgress) {
	defer close(progress)
	if cfg.QueryWorkers <= 0 {
		cfg.QueryWorkers = 64
	}
	if cfg.MasterLimit <= 0 {
		cfg.MasterLimit = 2000
	}

	progress <- ScanProgress{Phase: "master", Message: "正在请求 Steam 主服务器列表..."}
	addrs, err := queryMaster(ctx, cfg.Region, cfg.MasterLimit)
	if err != nil {
		if cfg.SteamAPIKey != "" {
			progress <- ScanProgress{Phase: "web", Message: "UDP 主服务器不可用，正在尝试 Steam Web API..."}
			webInfos, webErr := querySteamWebServerList(ctx, cfg)
			if webErr == nil && len(webInfos) > 0 {
				for i := range webInfos {
					applyRules(&webInfos[i], cfg)
				}
				ips := blockedIPsFrom(webInfos)
				progress <- ScanProgress{Phase: "done", Done: len(webInfos), Total: len(webInfos), Finished: true, All: webInfos, BlockedIP: ips, Message: "扫描完成"}
				return
			}
			progress <- ScanProgress{Phase: "error", Message: fmt.Sprintf("%v；Steam Web API 也失败：%v", err, webErr), Finished: true}
			return
		}
		progress <- ScanProgress{Phase: "error", Message: err.Error() + "；注意：L4D2 游戏内能刷新不代表外部 exe 也能直连 Steam master。很多加速器只代理 steam.exe/left4dead2.exe。请把本工具加入加速器/代理规则，或在 config.json 填入 steam_api_key 启用 HTTPS 备用扫描。", Finished: true}
		return
	}
	progress <- ScanProgress{Phase: "query", Total: len(addrs), Message: fmt.Sprintf("拿到 %d 个候选服务器，开始查询详情...", len(addrs))}

	jobs := make(chan string)
	results := make(chan ServerInfo)
	var wg sync.WaitGroup
	workers := cfg.QueryWorkers
	if workers > len(addrs) && len(addrs) > 0 {
		workers = len(addrs)
	}
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for addr := range jobs {
				info := queryServer(ctx, addr, 1300*time.Millisecond)
				applyRules(&info, cfg)
				results <- info
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, addr := range addrs {
			select {
			case <-ctx.Done():
				return
			case jobs <- addr:
			}
		}
	}()
	go func() {
		wg.Wait()
		close(results)
	}()

	all := make([]ServerInfo, 0, len(addrs))
	blockedIPs := map[string]bool{}
	done := 0
	for info := range results {
		done++
		all = append(all, info)
		if info.Blocked && info.Error == "" {
			host, _, err := net.SplitHostPort(info.Address)
			if err == nil {
				blockedIPs[host] = true
			}
		}
		progress <- ScanProgress{Phase: "query", Done: done, Total: len(addrs), Server: &info}
		if done == 1 || done%50 == 0 || done == len(addrs) {
			progress <- ScanProgress{Phase: "query", Done: done, Total: len(addrs)}
		}
	}

	sort.SliceStable(all, func(i, j int) bool {
		if all[i].Blocked != all[j].Blocked {
			return !all[i].Blocked
		}
		return all[i].PingMS < all[j].PingMS
	})
	ips := make([]string, 0, len(blockedIPs))
	for ip := range blockedIPs {
		ips = append(ips, ip)
	}
	sort.Strings(ips)
	progress <- ScanProgress{Phase: "done", Done: len(all), Total: len(addrs), Finished: true, All: all, BlockedIP: ips, Message: "扫描完成"}
}

func blockedIPsFrom(infos []ServerInfo) []string {
	seen := map[string]bool{}
	for _, info := range infos {
		if !info.Blocked || info.Error != "" {
			continue
		}
		host, _, err := net.SplitHostPort(info.Address)
		if err == nil {
			seen[host] = true
		}
	}
	ips := make([]string, 0, len(seen))
	for ip := range seen {
		ips = append(ips, ip)
	}
	sort.Strings(ips)
	return ips
}

type steamWebServerList struct {
	Response struct {
		Servers []steamWebServer `json:"servers"`
	} `json:"response"`
}

type steamWebServer struct {
	Addr       string `json:"addr"`
	Name       string `json:"name"`
	Map        string `json:"map"`
	GameDir    string `json:"gamedir"`
	Product    string `json:"product"`
	GameType   string `json:"gametype"`
	Players    int    `json:"players"`
	MaxPlayers int    `json:"max_players"`
	Bots       int    `json:"bots"`
	Secure     bool   `json:"secure"`
}

func querySteamWebServerList(ctx context.Context, cfg Config) ([]ServerInfo, error) {
	limit := cfg.MasterLimit
	if limit <= 0 {
		limit = 2000
	}
	values := url.Values{}
	values.Set("key", cfg.SteamAPIKey)
	values.Set("filter", "\\appid\\550")
	values.Set("limit", fmt.Sprintf("%d", limit))
	endpoint := "https://api.steampowered.com/IGameServersService/GetServerList/v1/?" + values.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %s", resp.Status)
	}
	var payload steamWebServerList
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	out := make([]ServerInfo, 0, len(payload.Response.Servers))
	now := time.Now()
	for _, s := range payload.Response.Servers {
		if s.Addr == "" {
			continue
		}
		out = append(out, ServerInfo{
			Address:    s.Addr,
			Host:       s.Name,
			Map:        s.Map,
			Folder:     s.GameDir,
			Game:       s.Product,
			Keywords:   s.GameType,
			Players:    s.Players,
			MaxPlayers: s.MaxPlayers,
			Bots:       s.Bots,
			VAC:        s.Secure,
			PingMS:     0,
			LastSeen:   now,
		})
	}
	return out, nil
}

func queryMaster(ctx context.Context, region byte, limit int) ([]string, error) {
	var errs []string
	for _, server := range masterServers {
		addrs, err := queryMasterOne(ctx, server, region, limit)
		if err == nil && len(addrs) > 0 {
			return addrs, nil
		}
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", server, err))
		} else {
			errs = append(errs, fmt.Sprintf("%s: 返回空列表", server))
		}
	}
	return nil, fmt.Errorf("无法从 Steam 主服务器获取列表。请检查代理/加速器是否拦截 UDP 27011。已尝试：%s", strings.Join(errs, "；"))
}

func queryMasterOne(ctx context.Context, server string, region byte, limit int) ([]string, error) {
	raddr, err := resolveMasterUDP(server)
	if err != nil {
		return nil, err
	}
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	filter := "\\appid\\550\\secure\\1"
	start := "0.0.0.0:0"
	seen := map[string]bool{}
	out := make([]string, 0, limit)
	buf := make([]byte, 1500)

	for len(out) < limit {
		packet := append([]byte{0x31, region}, []byte(start)...)
		packet = append(packet, 0)
		packet = append(packet, []byte(filter)...)
		packet = append(packet, 0)
		if _, err := conn.Write(packet); err != nil {
			return out, err
		}
		_ = conn.SetReadDeadline(time.Now().Add(2500 * time.Millisecond))
		n, err := conn.Read(buf)
		if err != nil {
			if len(out) > 0 {
				return out, nil
			}
			return nil, fmt.Errorf("读取超时或失败：%w", err)
		}
		addrs := parseMasterResponse(buf[:n])
		if len(addrs) == 0 {
			break
		}
		last := ""
		for _, addr := range addrs {
			if addr == "0.0.0.0:0" {
				return out, nil
			}
			last = addr
			if !seen[addr] {
				seen[addr] = true
				out = append(out, addr)
				if len(out) >= limit {
					return out, nil
				}
			}
		}
		if last == "" || last == start {
			break
		}
		start = last
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		default:
		}
	}
	return out, nil
}

func resolveMasterUDP(server string) (*net.UDPAddr, error) {
	host, port, err := net.SplitHostPort(server)
	if err != nil {
		return nil, err
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		if !isUsableMasterAddr(ip) {
			return nil, fmt.Errorf("地址不可用 %s", ip)
		}
		return net.ResolveUDPAddr("udp", net.JoinHostPort(ip.String(), port))
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return nil, err
	}
	var rejected []string
	for _, raw := range ips {
		addr, ok := netip.AddrFromSlice(raw)
		if !ok {
			continue
		}
		if isUsableMasterAddr(addr) {
			return net.ResolveUDPAddr("udp", net.JoinHostPort(addr.String(), port))
		}
		rejected = append(rejected, addr.String())
	}
	return nil, fmt.Errorf("DNS 只返回不可用地址：%s", strings.Join(rejected, ", "))
}

func isUsableMasterAddr(addr netip.Addr) bool {
	return addr.IsGlobalUnicast() &&
		!addr.IsPrivate() &&
		!addr.IsLoopback() &&
		!addr.IsLinkLocalUnicast() &&
		!addr.Is4In6() &&
		!strings.HasPrefix(addr.String(), "198.18.") &&
		!strings.HasPrefix(addr.String(), "198.19.")
}

func parseMasterResponse(b []byte) []string {
	start := 0
	if len(b) >= 6 && b[0] == 0xFF && b[1] == 0xFF && b[2] == 0xFF && b[3] == 0xFF && b[4] == 0x66 {
		start = 6
	} else if len(b) >= 2 && b[0] == 0x66 {
		start = 2
	}
	var out []string
	for i := start; i+6 <= len(b); i += 6 {
		ip := net.IPv4(b[i], b[i+1], b[i+2], b[i+3]).String()
		port := binary.BigEndian.Uint16(b[i+4 : i+6])
		out = append(out, fmt.Sprintf("%s:%d", ip, port))
	}
	return out
}

func queryServer(ctx context.Context, addr string, timeout time.Duration) ServerInfo {
	info := ServerInfo{Address: addr, PingMS: 9999, LastSeen: time.Now()}
	raddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		info.Error = err.Error()
		return info
	}
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		info.Error = err.Error()
		return info
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(timeout))
	start := time.Now()
	resp, err := a2sInfo(conn, nil)
	if err != nil && errors.Is(err, errChallenge) {
		resp, err = a2sInfo(conn, resp)
	}
	select {
	case <-ctx.Done():
		info.Error = "已取消"
		return info
	default:
	}
	if err != nil {
		info.Error = err.Error()
		return info
	}
	info.PingMS = int(time.Since(start).Milliseconds())
	if err := parseA2SInfo(resp, &info); err != nil {
		info.Error = err.Error()
	}
	return info
}

var errChallenge = errors.New("server requested challenge")

func a2sInfo(conn *net.UDPConn, challenge []byte) ([]byte, error) {
	req := []byte{0xFF, 0xFF, 0xFF, 0xFF, 0x54}
	req = append(req, []byte("Source Engine Query")...)
	req = append(req, 0)
	if len(challenge) == 4 {
		req = append(req, challenge...)
	}
	if _, err := conn.Write(req); err != nil {
		return nil, err
	}
	buf := make([]byte, 1400)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, err
	}
	b := buf[:n]
	if len(b) >= 9 && bytes.Equal(b[:4], []byte{0xFF, 0xFF, 0xFF, 0xFF}) && b[4] == 0x41 {
		return b[5:9], errChallenge
	}
	return b, nil
}

func parseA2SInfo(b []byte, info *ServerInfo) error {
	if len(b) < 6 || !bytes.Equal(b[:4], []byte{0xFF, 0xFF, 0xFF, 0xFF}) || b[4] != 0x49 {
		return errors.New("不是有效的 A2S_INFO 响应")
	}
	r := bytes.NewReader(b[6:])
	info.Host = readCString(r)
	info.Map = readCString(r)
	info.Folder = readCString(r)
	info.Game = readCString(r)
	var app uint16
	_ = binary.Read(r, binary.LittleEndian, &app)
	info.Players = readByte(r)
	info.MaxPlayers = readByte(r)
	info.Bots = readByte(r)
	_, _ = r.ReadByte()
	_, _ = r.ReadByte()
	visibility := readByte(r)
	vac := readByte(r)
	info.Password = visibility == 1
	info.VAC = vac == 1
	_ = readCString(r)
	if r.Len() > 0 {
		flags := readByte(r)
		if flags&0x80 != 0 && r.Len() >= 2 {
			_, _ = r.Seek(2, 1)
		}
		if flags&0x10 != 0 && r.Len() >= 8 {
			_, _ = r.Seek(8, 1)
		}
		if flags&0x40 != 0 {
			_ = readCString(r)
			if r.Len() >= 2 {
				_, _ = r.Seek(2, 1)
			}
		}
		if flags&0x20 != 0 {
			info.Keywords = readCString(r)
		}
	}
	if app != appIDLeft4Dead2 {
		return fmt.Errorf("非 L4D2 AppID: %d", app)
	}
	return nil
}

func readCString(r *bytes.Reader) string {
	var out []byte
	for r.Len() > 0 {
		c, err := r.ReadByte()
		if err != nil || c == 0 {
			break
		}
		out = append(out, c)
	}
	return string(out)
}

func readByte(r *bytes.Reader) int {
	b, err := r.ReadByte()
	if err != nil {
		return 0
	}
	return int(b)
}

func applyRules(info *ServerInfo, cfg Config) {
	if info.Error != "" {
		return
	}
	name := normalizeText(info.Host)
	for _, kw := range cfg.NameKeywords {
		normalizedKW := normalizeText(kw)
		if normalizedKW != "" && strings.Contains(name, normalizedKW) {
			info.Blocked = true
			info.BlockReasons = append(info.BlockReasons, "名称:"+kw)
		}
	}
}

func applyIPRulesOnly(info *ServerInfo, cfg Config) {
	host, _, err := net.SplitHostPort(info.Address)
	if err == nil && ipBlocked(host, cfg) {
		info.Blocked = true
		info.BlockReasons = append(info.BlockReasons, "IP规则")
	}
}

func normalizeText(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	replacer := strings.NewReplacer("　", " ", "★", " ", "☆", " ", "丨", " ", "|", " ", "[", " ", "]", " ", "【", " ", "】", " ")
	s = replacer.Replace(s)
	return strings.Join(strings.Fields(s), " ")
}

func ipBlocked(host string, cfg Config) bool {
	for _, exact := range cfg.IPExact {
		if strings.TrimSpace(exact) == host {
			return true
		}
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	for _, cidr := range cfg.IPCIDR {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(cidr))
		if err == nil && prefix.Contains(ip) {
			return true
		}
	}
	return false
}
