package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	TH32CS_SNAPPROCESS  = 0x00000002
	TH32CS_SNAPMODULE   = 0x00000008
	TH32CS_SNAPMODULE32 = 0x00000010
	MAX_PATH            = 260
)

var (
	kernel32                     = syscall.NewLazyDLL("kernel32.dll")
	procCreateToolhelp32Snapshot = kernel32.NewProc("CreateToolhelp32Snapshot")
	procProcess32First           = kernel32.NewProc("Process32FirstW")
	procProcess32Next            = kernel32.NewProc("Process32NextW")
	procModule32First            = kernel32.NewProc("Module32FirstW")
	procModule32Next             = kernel32.NewProc("Module32NextW")
)

type processEntry32 struct {
	Size            uint32
	CntUsage        uint32
	ProcessID       uint32
	DefaultHeapID   uintptr
	ModuleID        uint32
	CntThreads      uint32
	ParentProcessID uint32
	PriClassBase    int32
	Flags           uint32
	ExeFile         [MAX_PATH]uint16
}

type moduleEntry32 struct {
	Size         uint32
	ModuleID     uint32
	ProcessID    uint32
	GlblcntUsage uint32
	ProccntUsage uint32
	ModBaseAddr  uintptr
	ModBaseSize  uint32
	Module       uintptr
	ModuleName   [256]uint16
	ExePath      [MAX_PATH]uint16
}

func main() {
	procName := flag.String("proc", "left4dead2.exe", "process name")
	duration := flag.Duration("duration", 60*time.Second, "watch duration")
	interval := flag.Duration("interval", time.Second, "poll interval")
	out := flag.String("out", "", "output markdown report path")
	flag.Parse()

	if *out == "" {
		*out = filepath.Clean(filepath.Join("..", "..", "reports", "module_watch_report.md"))
	}

	report, err := watch(*procName, *duration, *interval)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0755); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, []byte(report), 0644); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Println(*out)
}

func watch(procName string, duration, interval time.Duration) (string, error) {
	deadline := time.Now().Add(duration)
	seen := map[string]uintptr{}
	var pid uint32
	var events []string

	for time.Now().Before(deadline) {
		currentPID, err := findProcess(procName)
		if err == nil && currentPID != 0 {
			if pid == 0 {
				pid = currentPID
				events = append(events, fmt.Sprintf("%s process found pid=%d", time.Now().Format(time.RFC3339), pid))
			}
			modules, err := listModules(pid)
			if err != nil {
				events = append(events, fmt.Sprintf("%s module enumeration failed: %s", time.Now().Format(time.RFC3339), err))
			} else {
				for _, m := range modules {
					key := strings.ToLower(m.Path)
					if _, ok := seen[key]; !ok {
						seen[key] = m.Base
						events = append(events, fmt.Sprintf("%s module base=0x%X path=%s", time.Now().Format(time.RFC3339), m.Base, m.Path))
					}
				}
			}
		}
		time.Sleep(interval)
	}

	var modules []string
	for path := range seen {
		modules = append(modules, path)
	}
	sort.Strings(modules)

	var b strings.Builder
	fmt.Fprintf(&b, "# Module Watch Report\n\n")
	fmt.Fprintf(&b, "- Process: `%s`\n", procName)
	fmt.Fprintf(&b, "- Duration: `%s`\n", duration)
	if pid == 0 {
		fmt.Fprintf(&b, "- Result: process was not found\n\n")
	} else {
		fmt.Fprintf(&b, "- PID: `%d`\n\n", pid)
	}

	fmt.Fprintf(&b, "## Events\n\n")
	for _, event := range events {
		fmt.Fprintf(&b, "- %s\n", event)
	}
	if len(events) == 0 {
		fmt.Fprintf(&b, "No events.\n")
	}

	fmt.Fprintf(&b, "\n## Relevant Modules\n\n")
	for _, path := range modules {
		if isRelevant(path) {
			fmt.Fprintf(&b, "- `%s`\n", path)
		}
	}
	return b.String(), nil
}

type moduleInfo struct {
	Base uintptr
	Path string
}

func findProcess(name string) (uint32, error) {
	snap, _, err := procCreateToolhelp32Snapshot.Call(TH32CS_SNAPPROCESS, 0)
	if snap == uintptr(syscall.InvalidHandle) {
		return 0, err
	}
	defer syscall.CloseHandle(syscall.Handle(snap))

	var pe processEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	ret, _, _ := procProcess32First.Call(snap, uintptr(unsafe.Pointer(&pe)))
	for ret != 0 {
		exe := syscall.UTF16ToString(pe.ExeFile[:])
		if strings.EqualFold(exe, name) {
			return pe.ProcessID, nil
		}
		ret, _, _ = procProcess32Next.Call(snap, uintptr(unsafe.Pointer(&pe)))
	}
	return 0, fmt.Errorf("%s not found", name)
}

func listModules(pid uint32) ([]moduleInfo, error) {
	snap, _, err := procCreateToolhelp32Snapshot.Call(TH32CS_SNAPMODULE|TH32CS_SNAPMODULE32, uintptr(pid))
	if snap == uintptr(syscall.InvalidHandle) {
		return nil, err
	}
	defer syscall.CloseHandle(syscall.Handle(snap))

	var out []moduleInfo
	var me moduleEntry32
	me.Size = uint32(unsafe.Sizeof(me))
	ret, _, _ := procModule32First.Call(snap, uintptr(unsafe.Pointer(&me)))
	for ret != 0 {
		out = append(out, moduleInfo{
			Base: me.ModBaseAddr,
			Path: syscall.UTF16ToString(me.ExePath[:]),
		})
		ret, _, _ = procModule32Next.Call(snap, uintptr(unsafe.Pointer(&me)))
	}
	return out, nil
}

func isRelevant(path string) bool {
	lower := strings.ToLower(path)
	for _, token := range []string{"serverbrowser", "matchmaking", "steam_api", "engine.dll", "client.dll", "vgui", "server.dll"} {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return false
}
