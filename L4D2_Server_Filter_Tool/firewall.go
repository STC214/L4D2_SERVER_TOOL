package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

const firewallRuleBase = "L4D2 Server Tool Block"
const firewallRuleOutBase = firewallRuleBase + " OUT"
const firewallRuleInBase = firewallRuleBase + " IN"

func exportBlockedIPs(path string, ips []string) error {
	var b strings.Builder
	for _, ip := range ips {
		if strings.TrimSpace(ip) != "" {
			b.WriteString(ip)
			b.WriteByte('\n')
		}
	}
	return os.WriteFile(path, []byte(b.String()), 0644)
}

type huorongIPBlacklistFile struct {
	Ver  string                   `json:"ver"`
	Tag  string                   `json:"tag"`
	Data []huorongIPBlacklistRule `json:"data"`
}

type huorongIPBlacklistRule struct {
	ID          int    `json:"id"`
	TmpFieldSel bool   `json:"tmp_field_sel"`
	RAddr       string `json:"raddr"`
	Memo        string `json:"memo"`
}

func exportHuorongIPBlacklist(dir string, infos []ServerInfo, ips []string) (string, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "huorong_ip_blacklist.json")

	rules := buildHuorongRules(infos, ips)
	payload := huorongIPBlacklistFile{
		Ver:  "6.0",
		Tag:  "ipblacklist",
		Data: rules,
	}
	b, err := json.MarshalIndent(payload, "", "    ")
	if err != nil {
		return "", err
	}
	return path, os.WriteFile(path, b, 0644)
}

func buildHuorongRules(infos []ServerInfo, ips []string) []huorongIPBlacklistRule {
	memos := map[string]string{}
	for _, info := range infos {
		if !info.Blocked {
			continue
		}
		host, _, err := netSplitHostPort(info.Address)
		if err != nil || host == "" {
			continue
		}
		memo := info.Host
		if len(info.BlockReasons) > 0 {
			if memo != "" {
				memo += " | "
			}
			memo += strings.Join(info.BlockReasons, " ")
		}
		if memo == "" {
			memo = "L4D2 Server Tool"
		}
		memos[host] = memo
	}

	unique := uniqueStrings(ips)
	if len(unique) == 0 {
		for ip := range memos {
			unique = append(unique, ip)
		}
	}
	rules := make([]huorongIPBlacklistRule, 0, len(unique))
	for i, ip := range unique {
		rules = append(rules, huorongIPBlacklistRule{
			ID:          i + 1,
			TmpFieldSel: true,
			RAddr:       ip,
			Memo:        memos[ip],
		})
	}
	return rules
}

func netSplitHostPort(address string) (string, string, error) {
	if strings.Count(address, ":") == 0 {
		return address, "", nil
	}
	return net.SplitHostPort(address)
}

func makeFirewallScript(dir string, ips []string) (string, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	script := filepath.Join(dir, "apply_l4d2_firewall.ps1")
	var b strings.Builder
	b.WriteString("$ErrorActionPreference = 'Continue'\r\n")
	writeFirewallDelete(&b, firewallRuleBase, "Out-Null")
	writeFirewallDelete(&b, firewallRuleOutBase, "Out-Null")
	writeFirewallDelete(&b, firewallRuleInBase, "Out-Null")
	chunks := chunkStrings(uniqueStrings(ips), 180)
	if len(chunks) == 0 {
		b.WriteString("Write-Host 'No blocked IPs to apply.'\r\n")
	} else {
		for i, chunk := range chunks {
			remote := strings.Join(chunk, ",")
			writeFirewallAdd(&b, fmt.Sprintf("%s %02d", firewallRuleOutBase, i+1), "out", remote)
			writeFirewallAdd(&b, fmt.Sprintf("%s %02d", firewallRuleInBase, i+1), "in", remote)
		}
	}
	b.WriteString("Write-Host ''\r\n")
	b.WriteString("Write-Host 'L4D2 inbound/outbound firewall rules updated. You can close this window.'\r\n")
	b.WriteString("Read-Host 'Press Enter'\r\n")
	return script, os.WriteFile(script, []byte(b.String()), 0644)
}

func writeFirewallAdd(b *strings.Builder, name, direction, remote string) {
	b.WriteString("netsh advfirewall firewall add rule ")
	b.WriteString("name=\"")
	b.WriteString(name)
	b.WriteString("\" dir=")
	b.WriteString(direction)
	b.WriteString(" action=block enable=yes profile=any protocol=UDP remoteip=\"")
	b.WriteString(remote)
	b.WriteString("\" | Out-Host\r\n")
}

func writeFirewallDelete(b *strings.Builder, name, sink string) {
	b.WriteString("netsh advfirewall firewall delete rule name=\"")
	b.WriteString(name)
	b.WriteString("\" | ")
	b.WriteString(sink)
	b.WriteString("\r\n")
}

func makeFirewallRemoveScript(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	script := filepath.Join(dir, "remove_l4d2_firewall.ps1")
	var b strings.Builder
	b.WriteString("$ErrorActionPreference = 'Continue'\r\n")
	writeFirewallDelete(&b, firewallRuleBase, "Out-Host")
	writeFirewallDelete(&b, firewallRuleOutBase, "Out-Host")
	writeFirewallDelete(&b, firewallRuleInBase, "Out-Host")
	b.WriteString("Write-Host 'L4D2 inbound/outbound firewall rules removed. You can close this window.'\r\n")
	b.WriteString("Read-Host 'Press Enter'\r\n")
	return script, os.WriteFile(script, []byte(b.String()), 0644)
}

func runPowerShellElevated(scriptPath string) error {
	verb, _ := syscall.UTF16PtrFromString("runas")
	exe, _ := syscall.UTF16PtrFromString("powershell.exe")
	args, _ := syscall.UTF16PtrFromString("-NoProfile -ExecutionPolicy Bypass -File \"" + scriptPath + "\"")
	dir, _ := syscall.UTF16PtrFromString(filepath.Dir(scriptPath))
	shell32 := syscall.NewLazyDLL("shell32.dll")
	shellExecute := shell32.NewProc("ShellExecuteW")
	r, _, err := shellExecute.Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(exe)), uintptr(unsafe.Pointer(args)), uintptr(unsafe.Pointer(dir)), 1)
	if r <= 32 {
		return err
	}
	return nil
}

func openFolder(path string) error {
	cmd := exec.Command("explorer.exe", path)
	return cmd.Start()
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func chunkStrings(in []string, size int) [][]string {
	if size <= 0 {
		size = 100
	}
	var chunks [][]string
	for len(in) > 0 {
		n := size
		if len(in) < n {
			n = len(in)
		}
		chunks = append(chunks, in[:n])
		in = in[n:]
	}
	return chunks
}
