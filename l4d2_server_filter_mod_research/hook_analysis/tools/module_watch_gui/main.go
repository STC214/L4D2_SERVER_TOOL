package main

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
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
	defaultGameDir = `E:\SteamLibrary\steamapps\common\Left 4 Dead 2`
	processName    = "left4dead2.exe"

	TH32CS_SNAPPROCESS      = 0x00000002
	TH32CS_SNAPMODULE       = 0x00000008
	TH32CS_SNAPMODULE32     = 0x00000010
	MAX_PATH                = 260
	AF_INET                 = 2
	TCP_TABLE_OWNER_PID_ALL = 5
	UDP_TABLE_OWNER_PID     = 1

	WS_OVERLAPPEDWINDOW = 0x00CF0000
	WS_VISIBLE          = 0x10000000
	WS_CHILD            = 0x40000000
	WS_TABSTOP          = 0x00010000
	WS_BORDER           = 0x00800000
	WS_VSCROLL          = 0x00200000
	WS_EX_CLIENTEDGE    = 0x00000200
	ES_MULTILINE        = 0x0004
	ES_AUTOVSCROLL      = 0x0040
	ES_READONLY         = 0x0800
	ES_AUTOHSCROLL      = 0x0080
	BS_PUSHBUTTON       = 0x00000000
	SS_LEFT             = 0x00000000

	SW_SHOW = 5
	CP_GBK  = 936

	WM_DESTROY = 0x0002
	WM_CREATE  = 0x0001
	WM_SIZE    = 0x0005
	WM_COMMAND = 0x0111
	WM_SETICON = 0x0080
	WM_SETFONT = 0x0030
	WM_TIMER   = 0x0113
	WM_APP     = 0x8000

	ICON_SMALL     = 0
	ICON_BIG       = 1
	IMAGE_ICON     = 1
	LR_DEFAULTSIZE = 0x00000040

	ID_LAUNCH       = 1001
	ID_START        = 1002
	ID_STOP         = 1003
	ID_STATUS_TIMER = 2001

	WM_CAPTURE_EVENT = WM_APP + 1
	WM_CAPTURE_DONE  = WM_APP + 2
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	iphlpapi = syscall.NewLazyDLL("iphlpapi.dll")

	procDefWindowProc            = user32.NewProc("DefWindowProcW")
	procDispatchMessage          = user32.NewProc("DispatchMessageW")
	procGetMessage               = user32.NewProc("GetMessageW")
	procLoadIcon                 = user32.NewProc("LoadIconW")
	procLoadImage                = user32.NewProc("LoadImageW")
	procLoadCursor               = user32.NewProc("LoadCursorW")
	procRegisterClassEx          = user32.NewProc("RegisterClassExW")
	procCreateWindowEx           = user32.NewProc("CreateWindowExW")
	procShowWindow               = user32.NewProc("ShowWindow")
	procUpdateWindow             = user32.NewProc("UpdateWindow")
	procTranslateMessage         = user32.NewProc("TranslateMessage")
	procSendMessage              = user32.NewProc("SendMessageW")
	procPostMessage              = user32.NewProc("PostMessageW")
	procPostQuitMessage          = user32.NewProc("PostQuitMessage")
	procSetWindowText            = user32.NewProc("SetWindowTextW")
	procSetTimer                 = user32.NewProc("SetTimer")
	procKillTimer                = user32.NewProc("KillTimer")
	procGetWindowText            = user32.NewProc("GetWindowTextW")
	procGetWindowTextLength      = user32.NewProc("GetWindowTextLengthW")
	procGetClientRect            = user32.NewProc("GetClientRect")
	procMoveWindow               = user32.NewProc("MoveWindow")
	procEnableWindow             = user32.NewProc("EnableWindow")
	procMessageBox               = user32.NewProc("MessageBoxW")
	procGetModuleHandle          = kernel32.NewProc("GetModuleHandleW")
	procCreateToolhelp32Snapshot = kernel32.NewProc("CreateToolhelp32Snapshot")
	procProcess32First           = kernel32.NewProc("Process32FirstW")
	procProcess32Next            = kernel32.NewProc("Process32NextW")
	procModule32First            = kernel32.NewProc("Module32FirstW")
	procModule32Next             = kernel32.NewProc("Module32NextW")
	procMultiByteToWideChar      = kernel32.NewProc("MultiByteToWideChar")
	procGetStockObject           = gdi32.NewProc("GetStockObject")
	procGetExtendedTcpTable      = iphlpapi.NewProc("GetExtendedTcpTable")
	procGetExtendedUdpTable      = iphlpapi.NewProc("GetExtendedUdpTable")
)

type point struct{ X, Y int32 }
type rect struct{ Left, Top, Right, Bottom int32 }

type msg struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}

type wndclassex struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

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

type mibTCPRowOwnerPID struct {
	State      uint32
	LocalAddr  uint32
	LocalPort  uint32
	RemoteAddr uint32
	RemotePort uint32
	OwningPID  uint32
}

type mibUDPRowOwnerPID struct {
	LocalAddr uint32
	LocalPort uint32
	OwningPID uint32
}

type uiState struct {
	hwnd      uintptr
	gameEdit  uintptr
	phaseText uintptr
	status    uintptr
	logEdit   uintptr
	launchBtn uintptr
	startBtn  uintptr
	stopBtn   uintptr
	font      uintptr

	mu             sync.Mutex
	cancel         context.CancelFunc
	running        bool
	closing        bool
	logLines       []string
	events         []captureEvent
	dones          []captureDone
	lastStatusPost time.Time
	stats          *captureStats
	currentPhase   string
}

type captureEvent struct {
	Line   string
	Status string
	Phase  string
}

type captureDone struct {
	ReportPath string
	Error      string
}

type phase struct {
	Name    string
	Seconds int
	Prompt  string
}

type moduleInfo struct {
	Base uintptr
	Path string
}

type endpointInfo struct {
	Phase  string
	Proto  string
	Local  string
	Remote string
	State  string
}

type packetObservation struct {
	Phase       string
	Proto       string
	Direction   string
	Local       string
	Remote      string
	PayloadHint string
	ServerName  string
	SampleHex   string
	PayloadText string
	Bytes       int
	Count       int
}

type phaseTracker struct {
	mu    sync.RWMutex
	phase string
}

func (p *phaseTracker) set(phase string) {
	p.mu.Lock()
	p.phase = phase
	p.mu.Unlock()
}

func (p *phaseTracker) get() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.phase == "" {
		return "unknown"
	}
	return p.phase
}

type observedPorts struct {
	mu    sync.RWMutex
	ports map[uint8]map[int]bool
}

func newObservedPorts() *observedPorts {
	return &observedPorts{ports: map[uint8]map[int]bool{}}
}

func (p *observedPorts) add(proto uint8, port int) {
	if port <= 0 {
		return
	}
	p.mu.Lock()
	if p.ports[proto] == nil {
		p.ports[proto] = map[int]bool{}
	}
	p.ports[proto][port] = true
	p.mu.Unlock()
}

func (p *observedPorts) has(proto uint8, port int) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.ports[proto] != nil && p.ports[proto][port]
}

type packetInfo struct {
	srcIP   string
	dstIP   string
	srcPort int
	dstPort int
	proto   uint8
	payload []byte
}

type captureStats struct {
	mu         sync.RWMutex
	modules    int
	relevant   int
	endpoints  int
	packets    int
	packetErrs int
	lastPacket string
	message    string
}

func (s *captureStats) setModules(total, relevant int) {
	s.mu.Lock()
	s.modules = total
	s.relevant = relevant
	s.mu.Unlock()
}

func (s *captureStats) setEndpoints(count int) {
	s.mu.Lock()
	s.endpoints = count
	s.mu.Unlock()
}

func (s *captureStats) addPacket(obs packetObservation) {
	s.mu.Lock()
	s.packets++
	s.lastPacket = fmt.Sprintf("%s %s %s -> %s %s", obs.Phase, obs.Proto, obs.Local, obs.Remote, obs.PayloadHint)
	s.mu.Unlock()
}

func (s *captureStats) addPacketError() {
	s.mu.Lock()
	s.packetErrs++
	s.mu.Unlock()
}

func (s *captureStats) setMessage(message string) {
	s.mu.Lock()
	s.message = message
	s.mu.Unlock()
}

func (s *captureStats) status() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.message != "" && s.modules == 0 && s.packets == 0 {
		return s.message
	}
	msg := fmt.Sprintf("Capturing... modules=%d relevant=%d endpoints=%d packet_observations=%d packet_errors=%d", s.modules, s.relevant, s.endpoints, s.packets, s.packetErrs)
	if s.lastPacket != "" {
		msg += " | last: " + s.lastPacket
	}
	return msg
}

var app = &uiState{}

var phases = []phase{
	{Name: "Main menu", Seconds: 15, Prompt: "Keep the game at the main menu."},
	{Name: "Group refresh background", Seconds: 35, Prompt: "Stay on the main menu. The group server list refreshes in the background here."},
	{Name: "Group list inspect", Seconds: 20, Prompt: "Open the group server list only to inspect the latest visible results, then return to the main menu."},
	{Name: "Group refresh recheck", Seconds: 25, Prompt: "Stay on the main menu again, wait for another background refresh, then re-open the list if needed."},
	{Name: "Classic server browser", Seconds: 30, Prompt: "Open the classic server browser and refresh once."},
	{Name: "Quick/random match", Seconds: 30, Prompt: "Open quick/random match and trigger or wait on search."},
	{Name: "Back to menu", Seconds: 10, Prompt: "Return to menu or stop capture."},
}

func main() {
	runGUI()
}

func runGUI() {
	runtime.LockOSThread()
	hInstance := getModuleHandle()
	className := utf16Ptr("L4D2ModuleWatchGUI")
	appIcon := loadAppIcon(hInstance)
	wc := wndclassex{
		Size:       uint32(unsafe.Sizeof(wndclassex{})),
		WndProc:    syscall.NewCallback(wndProc),
		Instance:   hInstance,
		Icon:       appIcon,
		Cursor:     loadCursor(32512),
		Background: 6,
		ClassName:  className,
		IconSm:     appIcon,
	}
	procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))

	app.hwnd = createWindowEx(0, "L4D2ModuleWatchGUI", "L4D2 Hook Analysis - Module Watch", WS_OVERLAPPEDWINDOW|WS_VISIBLE, 100, 100, 920, 660, 0, 0, hInstance)
	setWindowIcon(app.hwnd, appIcon)
	procShowWindow.Call(app.hwnd, SW_SHOW)
	procUpdateWindow.Call(app.hwnd)

	var m msg
	for {
		ret, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(ret) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func wndProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	switch message {
	case WM_CREATE:
		createControls(hwnd)
		return 0
	case WM_DESTROY:
		app.mu.Lock()
		app.closing = true
		app.mu.Unlock()
		procKillTimer.Call(hwnd, ID_STATUS_TIMER)
		stopCapture()
		procPostQuitMessage.Call(0)
		return 0
	case WM_SIZE:
		layout()
		return 0
	case WM_COMMAND:
		switch loword(wParam) {
		case ID_LAUNCH:
			launchGame()
		case ID_START:
			startCapture()
		case ID_STOP:
			stopCapture()
		}
		return 0
	case WM_TIMER:
		if wParam == ID_STATUS_TIMER {
			refreshStatusFromTimer()
			return 0
		}
	case WM_CAPTURE_EVENT:
		for _, ev := range drainEvents() {
			if ev.Phase != "" {
				setText(app.phaseText, ev.Phase)
			}
			if ev.Status != "" {
				setText(app.status, ev.Status)
			}
			if ev.Line != "" {
				appendLog(ev.Line)
			}
		}
		return 0
	case WM_CAPTURE_DONE:
		for _, done := range drainDones() {
			app.mu.Lock()
			app.running = false
			app.cancel = nil
			app.stats = nil
			app.mu.Unlock()
			procKillTimer.Call(app.hwnd, ID_STATUS_TIMER)
			enableCaptureButtons(false)
			if done.Error != "" {
				setText(app.status, "Capture failed: "+done.Error)
				messageBox("Capture failed", done.Error)
			} else {
				setText(app.status, "Saved report: "+done.ReportPath)
				appendLog("Saved report: " + done.ReportPath)
			}
		}
		return 0
	}
	ret, _, _ := procDefWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
	return ret
}

func createControls(hwnd uintptr) {
	app.hwnd = hwnd
	app.font, _, _ = procGetStockObject.Call(17)

	createChild(0, "STATIC", "Game directory:", SS_LEFT, 16, 16, 120, 24, hwnd, 0)
	app.gameEdit = createChild(WS_EX_CLIENTEDGE, "EDIT", defaultGameDir, WS_BORDER|WS_TABSTOP|ES_AUTOHSCROLL, 136, 12, 560, 28, hwnd, 0)
	app.launchBtn = createChild(0, "BUTTON", "Launch Game", BS_PUSHBUTTON|WS_TABSTOP, 712, 12, 150, 30, hwnd, ID_LAUNCH)

	app.startBtn = createChild(0, "BUTTON", "Start Capture", BS_PUSHBUTTON|WS_TABSTOP, 16, 54, 150, 34, hwnd, ID_START)
	app.stopBtn = createChild(0, "BUTTON", "Stop And Save", BS_PUSHBUTTON|WS_TABSTOP, 178, 54, 150, 34, hwnd, ID_STOP)

	createChild(0, "STATIC", "Current step:", SS_LEFT, 16, 104, 120, 24, hwnd, 0)
	app.phaseText = createChild(WS_EX_CLIENTEDGE, "STATIC", "Not started. Click Launch Game or start L4D2 manually, then click Start Capture.", SS_LEFT, 136, 100, 720, 52, hwnd, 0)

	createChild(0, "STATIC", "Live log:", SS_LEFT, 16, 166, 120, 24, hwnd, 0)
	app.logEdit = createChild(WS_EX_CLIENTEDGE, "EDIT", "", WS_BORDER|WS_VSCROLL|ES_MULTILINE|ES_AUTOVSCROLL|ES_READONLY, 16, 192, 846, 350, hwnd, 0)
	app.status = createChild(0, "STATIC", "Ready. Buttons: Launch Game, Start Capture, Stop And Save.", SS_LEFT, 16, 570, 846, 28, hwnd, 0)

	enableCaptureButtons(false)
	layout()
}

func createChild(exStyle uint32, className, text string, style uint32, x, y, w, h int32, parent uintptr, id uintptr) uintptr {
	ctrl := createWindowEx(exStyle, className, text, WS_CHILD|WS_VISIBLE|style, x, y, w, h, parent, id, 0)
	if app.font != 0 {
		procSendMessage.Call(ctrl, WM_SETFONT, app.font, 1)
	}
	return ctrl
}

func layout() {
	if app.gameEdit == 0 {
		return
	}
	var rc rect
	procGetClientRect.Call(app.hwnd, uintptr(unsafe.Pointer(&rc)))
	width := rc.Right - rc.Left
	height := rc.Bottom - rc.Top
	if width < 760 {
		width = 760
	}
	if height < 520 {
		height = 520
	}
	procMoveWindow.Call(app.gameEdit, 136, 12, uintptr(width-326), 28, 1)
	procMoveWindow.Call(app.launchBtn, uintptr(width-174), 12, 150, 30, 1)
	procMoveWindow.Call(app.startBtn, 16, 54, 150, 34, 1)
	procMoveWindow.Call(app.stopBtn, 178, 54, 150, 34, 1)
	procMoveWindow.Call(app.phaseText, 136, 100, uintptr(width-160), 52, 1)
	procMoveWindow.Call(app.logEdit, 16, 192, uintptr(width-32), uintptr(height-252), 1)
	procMoveWindow.Call(app.status, 16, uintptr(height-44), uintptr(width-32), 28, 1)
}

func launchGame() {
	gameDir := strings.TrimSpace(getText(app.gameEdit))
	exe := filepath.Join(gameDir, processName)
	if _, err := os.Stat(exe); err != nil {
		messageBox("Launch failed", err.Error())
		return
	}
	cmd := exec.Command(exe)
	cmd.Dir = gameDir
	if err := cmd.Start(); err != nil {
		messageBox("Launch failed", err.Error())
		return
	}
	appendLog("Launched game: " + exe)
	setText(app.status, "Game launched. Click Start Capture after the main menu appears.")
}

func startCapture() {
	app.mu.Lock()
	if app.running {
		app.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	stats := &captureStats{}
	app.cancel = cancel
	app.running = true
	app.logLines = nil
	app.stats = stats
	app.currentPhase = "Starting capture"
	app.mu.Unlock()

	setText(app.logEdit, "")
	enableCaptureButtons(true)
	setText(app.status, "Capture running.")
	procSetTimer.Call(app.hwnd, ID_STATUS_TIMER, 1000, 0)
	go captureWorker(ctx, getText(app.gameEdit), stats)
}

func stopCapture() {
	app.mu.Lock()
	cancel := app.cancel
	app.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func captureWorker(ctx context.Context, gameDir string, stats *captureStats) {
	seen := map[string]moduleInfo{}
	seenEndpoints := map[string]endpointInfo{}
	seenPackets := map[string]packetObservation{}
	var events []string
	var pid uint32
	start := time.Now()
	reportPath := defaultReportPath()
	tracker := &phaseTracker{}
	packetEvents := make(chan packetObservation, 1024)
	packetErrs := make(chan string, 8)
	var packetCancel context.CancelFunc
	packetStarted := false

	postEvent("Capture started.", "Waiting for "+processName, "")
	defer func() {
		if packetCancel != nil {
			packetCancel()
		}
		report, err := buildReport(gameDir, start, time.Now(), pid, events, seen, seenEndpoints, seenPackets)
		if err == nil {
			err = os.MkdirAll(filepath.Dir(reportPath), 0755)
		}
		if err == nil {
			err = os.WriteFile(reportPath, []byte(report), 0644)
		}
		done := captureDone{ReportPath: reportPath}
		if err != nil {
			done.Error = err.Error()
		}
		postDone(done)
	}()

	for _, ph := range phases {
		tracker.set(ph.Name)
		setCurrentPhase(ph.Name)
		for remaining := ph.Seconds; remaining > 0; remaining-- {
			select {
			case <-ctx.Done():
				events = append(events, stamp("capture stopped by user"))
				return
			default:
			}

			phaseText := fmt.Sprintf("%s (%ds left): %s", ph.Name, remaining, ph.Prompt)
			currentPID, err := findProcess(processName)
			if err != nil {
				stats.setMessage(processName + " not found yet")
			} else {
				if pid == 0 {
					pid = currentPID
					line := stamp(fmt.Sprintf("process found pid=%d", pid))
					events = append(events, line)
					postEvent(line, "", "")
				}
				if !packetStarted {
					packetStarted = true
					var packetCtx context.Context
					packetCtx, packetCancel = context.WithCancel(ctx)
					go observePackets(packetCtx, currentPID, tracker, packetEvents, packetErrs)
					line := stamp("WinDivert packet probe starting")
					events = append(events, line)
					postEvent(line, "", "")
				}
				modules, err := listModules(currentPID)
				if err != nil {
					line := stamp("module enumeration failed: " + err.Error())
					events = append(events, line)
					postEvent(line, "", "")
				} else {
					relevant := countRelevant(modules)
					stats.setModules(len(modules), relevant)
					for _, m := range modules {
						key := strings.ToLower(m.Path)
						if _, ok := seen[key]; ok {
							continue
						}
						seen[key] = m
						if isRelevant(m.Path) {
							line := stamp(fmt.Sprintf("module base=0x%X path=%s", m.Base, m.Path))
							events = append(events, line)
							postEvent(line, "", "")
						}
					}
				}
				endpoints, err := listProcessEndpoints(currentPID)
				if err != nil {
					line := stamp("endpoint enumeration failed: " + err.Error())
					events = append(events, line)
					postEvent(line, "", "")
				} else {
					for _, ep := range endpoints {
						ep.Phase = ph.Name
						key := ep.Proto + "|" + ep.Local + "|" + ep.Remote + "|" + ep.State
						if _, ok := seenEndpoints[key]; ok {
							continue
						}
						seenEndpoints[key] = ep
						stats.setEndpoints(len(seenEndpoints))
						line := stamp(fmt.Sprintf("endpoint phase=%s proto=%s local=%s remote=%s state=%s", ep.Phase, ep.Proto, ep.Local, ep.Remote, ep.State))
						events = append(events, line)
						postEvent(line, "", "")
					}
				}
			}
			drainPacketChannels(&events, seenPackets, packetEvents, packetErrs, stats)
			setCurrentPhase(phaseText)
			select {
			case <-ctx.Done():
				events = append(events, stamp("capture stopped by user"))
				drainPacketChannels(&events, seenPackets, packetEvents, packetErrs, stats)
				return
			case <-time.After(time.Second):
			}
		}
	}
	drainPacketChannels(&events, seenPackets, packetEvents, packetErrs, stats)
	events = append(events, stamp("capture finished all phases"))
}

func drainPacketChannels(events *[]string, seen map[string]packetObservation, packetEvents <-chan packetObservation, packetErrs <-chan string, stats *captureStats) {
	for {
		select {
		case err := <-packetErrs:
			stats.addPacketError()
			line := stamp("packet probe: " + err)
			*events = append(*events, line)
			postEvent(line, "", "")
		case obs := <-packetEvents:
			key := obs.Proto + "|" + obs.Direction + "|" + obs.Local + "|" + obs.Remote + "|" + obs.PayloadHint
			stats.addPacket(obs)
			if existing, ok := seen[key]; ok {
				existing.Count++
				if existing.ServerName == "" {
					existing.ServerName = obs.ServerName
				}
				if existing.PayloadText == "" {
					existing.PayloadText = obs.PayloadText
				}
				seen[key] = existing
				continue
			}
			seen[key] = obs
		default:
			return
		}
	}
}

func buildReport(gameDir string, started, ended time.Time, pid uint32, events []string, seen map[string]moduleInfo, endpoints map[string]endpointInfo, packets map[string]packetObservation) (string, error) {
	var modules []moduleInfo
	for _, m := range seen {
		if isRelevant(m.Path) {
			modules = append(modules, m)
		}
	}
	sort.Slice(modules, func(i, j int) bool {
		return strings.ToLower(modules[i].Path) < strings.ToLower(modules[j].Path)
	})
	var endpointList []endpointInfo
	for _, ep := range endpoints {
		endpointList = append(endpointList, ep)
	}
	sort.Slice(endpointList, func(i, j int) bool {
		if endpointList[i].Phase != endpointList[j].Phase {
			return endpointList[i].Phase < endpointList[j].Phase
		}
		if endpointList[i].Proto != endpointList[j].Proto {
			return endpointList[i].Proto < endpointList[j].Proto
		}
		if endpointList[i].Remote != endpointList[j].Remote {
			return endpointList[i].Remote < endpointList[j].Remote
		}
		return endpointList[i].Local < endpointList[j].Local
	})
	var packetList []packetObservation
	for _, packet := range packets {
		packetList = append(packetList, packet)
	}
	sort.Slice(packetList, func(i, j int) bool {
		if packetList[i].Phase != packetList[j].Phase {
			return packetList[i].Phase < packetList[j].Phase
		}
		if packetList[i].Proto != packetList[j].Proto {
			return packetList[i].Proto < packetList[j].Proto
		}
		if packetList[i].Remote != packetList[j].Remote {
			return packetList[i].Remote < packetList[j].Remote
		}
		return packetList[i].Local < packetList[j].Local
	})

	var b strings.Builder
	fmt.Fprintf(&b, "# Module Watch GUI Report\n\n")
	fmt.Fprintf(&b, "- Game directory: `%s`\n", gameDir)
	fmt.Fprintf(&b, "- Process: `%s`\n", processName)
	fmt.Fprintf(&b, "- Started: `%s`\n", started.Format(time.RFC3339))
	fmt.Fprintf(&b, "- Ended: `%s`\n", ended.Format(time.RFC3339))
	if pid == 0 {
		fmt.Fprintf(&b, "- Result: process was not found\n\n")
	} else {
		fmt.Fprintf(&b, "- PID: `%d`\n\n", pid)
	}

	fmt.Fprintf(&b, "## Guided Phases\n\n")
	for _, ph := range phases {
		fmt.Fprintf(&b, "- `%s`: %d seconds, %s\n", ph.Name, ph.Seconds, ph.Prompt)
	}

	fmt.Fprintf(&b, "\n## Events\n\n")
	if len(events) == 0 {
		fmt.Fprintf(&b, "No events.\n")
	} else {
		for _, event := range events {
			fmt.Fprintf(&b, "- %s\n", event)
		}
	}

	fmt.Fprintf(&b, "\n## Relevant Modules\n\n")
	if len(modules) == 0 {
		fmt.Fprintf(&b, "No relevant modules observed.\n")
	} else {
		for _, m := range modules {
			fmt.Fprintf(&b, "- `0x%X` `%s`\n", m.Base, m.Path)
		}
	}

	fmt.Fprintf(&b, "\n## First-Seen Endpoints\n\n")
	if len(endpointList) == 0 {
		fmt.Fprintf(&b, "No process endpoints observed.\n")
	} else {
		fmt.Fprintf(&b, "| Phase | Proto | Local | Remote | State |\n")
		fmt.Fprintf(&b, "|---|---|---|---|---|\n")
		for _, ep := range endpointList {
			fmt.Fprintf(&b, "| %s | %s | `%s` | `%s` | %s |\n", ep.Phase, ep.Proto, ep.Local, ep.Remote, ep.State)
		}
	}
	fmt.Fprintf(&b, "\nNote: Windows UDP owner tables expose local UDP sockets, not per-packet remote UDP destinations. If group server refresh uses unconnected UDP sockets, use the next WinDivert packet probe to capture remote server IPs.\n")

	fmt.Fprintf(&b, "\n## Packet Observations\n\n")
	if len(packetList) == 0 {
		fmt.Fprintf(&b, "No packet-level observations captured. Run as administrator and make sure `WinDivert.dll` and `WinDivert64.sys` are beside the exe.\n")
	} else {
		writePacketSummary(&b, packetList)
	}

	fmt.Fprintf(&b, "\n## Interpretation Checklist\n\n")
	fmt.Fprintf(&b, "- If `left4dead2\\bin\\matchmaking.dll` appears during group-server or quick-match phases, prioritize matchmaking candidate hooks.\n")
	fmt.Fprintf(&b, "- If `platform\\servers\\serverbrowser.dll` appears only during classic browser phase, treat visible browser filtering separately.\n")
	fmt.Fprintf(&b, "- If both appear before interaction, use endpoint or packet logging to separate active code paths.\n")
	return b.String(), nil
}

func writePacketSummary(b *strings.Builder, packets []packetObservation) {
	total := 0
	for _, packet := range packets {
		total += packet.Count
	}
	fmt.Fprintf(b, "Unique packet observations: `%d`\n\n", len(packets))
	fmt.Fprintf(b, "Total packet events represented: `%d`\n\n", total)

	writePacketGroupTable(b, "By Phase", packets, func(p packetObservation) string {
		return p.Phase
	}, 20)
	writePacketGroupTable(b, "By Phase / Hint", packets, func(p packetObservation) string {
		return p.Phase + " / " + p.PayloadHint
	}, 30)
	writePacketGroupTable(b, "Top Remote Endpoints", packets, func(p packetObservation) string {
		return p.Remote
	}, 30)

	fmt.Fprintf(b, "\n### A2S Server Name Samples\n\n")
	fmt.Fprintf(b, "| Phase | Remote | Server Name | Count |\n")
	fmt.Fprintf(b, "|---|---|---|---:|\n")
	written := 0
	for _, packet := range packets {
		if packet.ServerName == "" {
			continue
		}
		fmt.Fprintf(b, "| %s | `%s` | %s | %d |\n", packet.Phase, packet.Remote, packet.ServerName, packet.Count)
		written++
		if written >= 80 {
			break
		}
	}
	if written == 0 {
		fmt.Fprintf(b, "| - | - | No A2S names parsed | 0 |\n")
	}

	fmt.Fprintf(b, "\n### Unusual Payload Samples\n\n")
	fmt.Fprintf(b, "| Phase | Direction | Local | Remote | Hint | Bytes | Count | Payload Summary | Sample Hex |\n")
	fmt.Fprintf(b, "|---|---|---|---|---|---:|---:|---|---|\n")
	written = 0
	for _, packet := range packets {
		if strings.HasPrefix(packet.PayloadHint, "A2S_") {
			continue
		}
		fmt.Fprintf(b, "| %s | %s | `%s` | `%s` | %s | %d | %d | %s | `%s` |\n", packet.Phase, packet.Direction, packet.Local, packet.Remote, packet.PayloadHint, packet.Bytes, packet.Count, escapeTableText(packet.PayloadText), packet.SampleHex)
		written++
		if written >= 80 {
			break
		}
	}
	if written == 0 {
		fmt.Fprintf(b, "| - | - | - | - | No unusual payloads | 0 | 0 | - | - |\n")
	}
}

func writePacketGroupTable(b *strings.Builder, title string, packets []packetObservation, keyFn func(packetObservation) string, limit int) {
	counts := map[string]int{}
	for _, packet := range packets {
		counts[keyFn(packet)] += packet.Count
	}
	type row struct {
		Key   string
		Count int
	}
	var rows []row
	for key, count := range counts {
		rows = append(rows, row{Key: key, Count: count})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Count == rows[j].Count {
			return rows[i].Key < rows[j].Key
		}
		return rows[i].Count > rows[j].Count
	})
	fmt.Fprintf(b, "### %s\n\n", title)
	fmt.Fprintf(b, "| Key | Count |\n")
	fmt.Fprintf(b, "|---|---:|\n")
	for i, row := range rows {
		if i >= limit {
			break
		}
		fmt.Fprintf(b, "| `%s` | %d |\n", row.Key, row.Count)
	}
	if len(rows) == 0 {
		fmt.Fprintf(b, "| - | 0 |\n")
	}
	fmt.Fprintf(b, "\n")
}

func postEvent(line, status, phase string) {
	app.mu.Lock()
	if app.closing {
		app.mu.Unlock()
		return
	}
	app.events = append(app.events, captureEvent{Line: line, Status: status, Phase: phase})
	hwnd := app.hwnd
	app.mu.Unlock()
	if hwnd != 0 {
		procPostMessage.Call(hwnd, WM_CAPTURE_EVENT, 0, 0)
	}
}

func postEventRateLimited(line, status, phase string) {
	app.mu.Lock()
	if app.closing {
		app.mu.Unlock()
		return
	}
	now := time.Now()
	if line == "" && now.Sub(app.lastStatusPost) < 900*time.Millisecond {
		app.mu.Unlock()
		return
	}
	app.lastStatusPost = now
	app.events = append(app.events, captureEvent{Line: line, Status: status, Phase: phase})
	hwnd := app.hwnd
	app.mu.Unlock()
	if hwnd != 0 {
		procPostMessage.Call(hwnd, WM_CAPTURE_EVENT, 0, 0)
	}
}

func setCurrentPhase(phase string) {
	app.mu.Lock()
	app.currentPhase = phase
	app.mu.Unlock()
}

func refreshStatusFromTimer() {
	app.mu.Lock()
	running := app.running
	stats := app.stats
	phase := app.currentPhase
	app.mu.Unlock()
	if !running || stats == nil {
		return
	}
	if phase != "" {
		setText(app.phaseText, phase)
	}
	setText(app.status, stats.status())
}

func postDone(done captureDone) {
	app.mu.Lock()
	if app.closing {
		app.mu.Unlock()
		return
	}
	app.dones = append(app.dones, done)
	hwnd := app.hwnd
	app.mu.Unlock()
	if hwnd != 0 {
		procPostMessage.Call(hwnd, WM_CAPTURE_DONE, 0, 0)
	}
}

func drainEvents() []captureEvent {
	app.mu.Lock()
	defer app.mu.Unlock()
	events := append([]captureEvent(nil), app.events...)
	app.events = nil
	return events
}

func drainDones() []captureDone {
	app.mu.Lock()
	defer app.mu.Unlock()
	dones := append([]captureDone(nil), app.dones...)
	app.dones = nil
	return dones
}

func appendLog(line string) {
	app.mu.Lock()
	app.logLines = append(app.logLines, line)
	if len(app.logLines) > 500 {
		app.logLines = app.logLines[len(app.logLines)-500:]
	}
	text := strings.Join(app.logLines, "\r\n")
	app.mu.Unlock()
	setText(app.logEdit, text)
}

func enableCaptureButtons(running bool) {
	enable(app.startBtn, !running)
	enable(app.stopBtn, running)
	enable(app.launchBtn, !running)
}

func enable(hwnd uintptr, enabled bool) {
	value := uintptr(0)
	if enabled {
		value = 1
	}
	procEnableWindow.Call(hwnd, value)
}

func defaultReportPath() string {
	return filepath.Clean(filepath.Join("..", "..", "reports", "module_watch_gui_report.md"))
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

func countRelevant(modules []moduleInfo) int {
	count := 0
	for _, m := range modules {
		if isRelevant(m.Path) {
			count++
		}
	}
	return count
}

func listProcessEndpoints(pid uint32) ([]endpointInfo, error) {
	var out []endpointInfo
	tcp, tcpErr := listTCP4(pid)
	if tcpErr == nil {
		out = append(out, tcp...)
	}
	udp, udpErr := listUDP4(pid)
	if udpErr == nil {
		out = append(out, udp...)
	}
	if tcpErr != nil && udpErr != nil {
		return nil, fmt.Errorf("tcp: %v; udp: %v", tcpErr, udpErr)
	}
	return out, nil
}

func observePackets(ctx context.Context, pid uint32, tracker *phaseTracker, events chan<- packetObservation, errs chan<- string) {
	flowFilter := fmt.Sprintf("processId == %d", pid)
	flowHandle, err := windivert.Open(flowFilter, windivert.LayerFlow, 0, windivert.FlagSniff|windivert.FlagRecvOnly)
	if err != nil {
		sendPacketErr(ctx, errs, "flow open failed: "+err.Error())
		return
	}
	defer flowHandle.Close()
	_ = flowHandle.SetParam(windivert.QueueLength, 2048)

	packetHandle, err := windivert.Open("ip and (tcp or udp)", windivert.LayerNetwork, 0, windivert.FlagSniff|windivert.FlagRecvOnly)
	if err != nil {
		sendPacketErr(ctx, errs, "network open failed: "+err.Error())
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

	ports := newObservedPorts()
	go observeGamePorts(ctx, pid, flowHandle, ports, errs)
	observeNetworkPackets(ctx, packetHandle, ports, tracker, events, errs)
}

func observeGamePorts(ctx context.Context, pid uint32, handle windivert.Handle, ports *observedPorts, errs chan<- string) {
	packet := make([]byte, 64)
	for {
		var addr windivert.Address
		_, err := handle.Recv(packet, &addr)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			sendPacketErr(ctx, errs, "flow recv failed: "+err.Error())
			return
		}
		if addr.Event() != windivert.EventFlowEstablished {
			continue
		}
		flow := addr.Flow()
		if flow == nil || flow.ProcessID != pid || (flow.Protocol != 17 && flow.Protocol != 6) {
			continue
		}
		ports.add(flow.Protocol, normalizePort(flow.LocalPort))
	}
}

func observeNetworkPackets(ctx context.Context, handle windivert.Handle, ports *observedPorts, tracker *phaseTracker, events chan<- packetObservation, errs chan<- string) {
	packet := make([]byte, 65535)
	for {
		var addr windivert.Address
		n, err := handle.Recv(packet, &addr)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			sendPacketErr(ctx, errs, "network recv failed: "+err.Error())
			return
		}
		if addr.Event() != windivert.EventNetworkPacket {
			continue
		}
		pkt, ok := parseIPv4Packet(packet[:n])
		if !ok {
			continue
		}
		obs, ok := packetToObservation(pkt, ports, tracker.get())
		if !ok {
			continue
		}
		select {
		case events <- obs:
		case <-ctx.Done():
			return
		default:
		}
	}
}

func packetToObservation(pkt packetInfo, ports *observedPorts, phase string) (packetObservation, bool) {
	var localIP string
	var localPort int
	var remoteIP string
	var remotePort int
	direction := ""
	if ports.has(pkt.proto, pkt.srcPort) {
		localIP, localPort = pkt.srcIP, pkt.srcPort
		remoteIP, remotePort = pkt.dstIP, pkt.dstPort
		direction = "out"
	} else if ports.has(pkt.proto, pkt.dstPort) {
		localIP, localPort = pkt.dstIP, pkt.dstPort
		remoteIP, remotePort = pkt.srcIP, pkt.srcPort
		direction = "in"
	} else {
		return packetObservation{}, false
	}
	if remoteIP == "" || remoteIP == "0.0.0.0" || remoteIP == "255.255.255.255" {
		return packetObservation{}, false
	}
	proto := "TCP"
	if pkt.proto == 17 {
		proto = "UDP"
	}
	return packetObservation{
		Phase:       phase,
		Proto:       proto,
		Direction:   direction,
		Local:       net.JoinHostPort(localIP, strconv.Itoa(localPort)),
		Remote:      net.JoinHostPort(remoteIP, strconv.Itoa(remotePort)),
		PayloadHint: payloadHint(pkt.payload),
		ServerName:  parseA2SInfoName(pkt.payload),
		SampleHex:   payloadSampleHex(pkt.payload),
		PayloadText: payloadSummary(pkt.payload),
		Bytes:       len(pkt.payload),
		Count:       1,
	}, true
}

func parseIPv4Packet(packet []byte) (packetInfo, bool) {
	if len(packet) < 28 || packet[0]>>4 != 4 {
		return packetInfo{}, false
	}
	ihl := int(packet[0]&0x0f) * 4
	if ihl < 20 || len(packet) < ihl+8 {
		return packetInfo{}, false
	}
	proto := packet[9]
	if proto != 17 && proto != 6 {
		return packetInfo{}, false
	}
	headerLen := 8
	if proto == 6 {
		if len(packet) < ihl+20 {
			return packetInfo{}, false
		}
		headerLen = int(packet[ihl+12]>>4) * 4
		if headerLen < 20 || len(packet) < ihl+headerLen {
			return packetInfo{}, false
		}
	}
	return packetInfo{
		srcIP:   net.IPv4(packet[12], packet[13], packet[14], packet[15]).String(),
		dstIP:   net.IPv4(packet[16], packet[17], packet[18], packet[19]).String(),
		srcPort: int(binary.BigEndian.Uint16(packet[ihl : ihl+2])),
		dstPort: int(binary.BigEndian.Uint16(packet[ihl+2 : ihl+4])),
		proto:   proto,
		payload: packet[ihl+headerLen:],
	}, true
}

func payloadHint(payload []byte) string {
	if len(payload) == 0 {
		return "empty"
	}
	if len(payload) >= 5 && payload[0] == 0xff && payload[1] == 0xff && payload[2] == 0xff && payload[3] == 0xff {
		switch payload[4] {
		case 0x49:
			return "A2S_INFO response"
		case 0x54:
			return "A2S_INFO query"
		case 0x41:
			return "A2S challenge"
		default:
			return fmt.Sprintf("Source packet 0x%02x", payload[4])
		}
	}
	if len(payload) >= 6 && payload[0] == 0xff && payload[1] == 0xff && payload[2] == 0xff && payload[3] == 0xff && payload[4] == 0x66 {
		return "Steam master response"
	}
	if len(payload) >= 2 && payload[0] == 0x31 {
		return "Steam master query-like"
	}
	if len(payload) > 32 {
		return fmt.Sprintf("payload %d bytes", len(payload))
	}
	return fmt.Sprintf("payload %d bytes", len(payload))
}

func parseA2SInfoName(payload []byte) string {
	if len(payload) < 7 || payload[0] != 0xff || payload[1] != 0xff || payload[2] != 0xff || payload[3] != 0xff || payload[4] != 0x49 {
		return ""
	}
	start := 6
	end := start
	for end < len(payload) && payload[end] != 0 {
		end++
	}
	if end == start || end >= len(payload) {
		return ""
	}
	name := strings.TrimSpace(decodeServerText(payload[start:end]))
	name = strings.ReplaceAll(name, "|", "/")
	name = strings.ReplaceAll(name, "\r", " ")
	name = strings.ReplaceAll(name, "\n", " ")
	return name
}

func payloadSummary(payload []byte) string {
	if summary := parseBinaryKVPayload(payload); summary != "" {
		return summary
	}
	if len(payload) == 0 {
		return ""
	}
	if len(payload) <= 96 && mostlyPrintable(payload) {
		return sanitizeTableText(string(payload))
	}
	return ""
}

func parseBinaryKVPayload(payload []byte) string {
	if !isBinaryKVPayload(payload) {
		return ""
	}
	offset := 16
	if offset >= len(payload) || payload[offset] != 0x00 {
		return ""
	}
	offset++
	root, ok := readCString(payload, &offset)
	if !ok || root == "" {
		return ""
	}
	fields := []string{"root=" + root}
	readBinaryKVFields(payload, &offset, root, 0, &fields)
	offset = skipBinaryKVTail(payload, offset)
	if offset < len(payload) {
		fields = append(fields, fmt.Sprintf("unparsed_tail=%d", len(payload)-offset))
	}
	return strings.Join(fields, "; ")
}

func readBinaryKVFields(data []byte, offset *int, prefix string, depth int, fields *[]string) {
	if depth > 32 {
		*fields = append(*fields, fmt.Sprintf("%s.<depth_limit>=offset_%d", prefix, *offset))
		return
	}
	for *offset < len(data) {
		before := *offset
		typ := data[*offset]
		*offset = *offset + 1
		if typ == 0x08 || typ == 0x0b {
			return
		}
		key, ok := readCString(data, offset)
		if !ok {
			*fields = append(*fields, fmt.Sprintf("%s.<bad_key_type_0x%02x>=offset_%d", prefix, typ, before))
			return
		}
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}
		switch typ {
		case 0x00:
			*fields = append(*fields, path+"={}")
			readBinaryKVFields(data, offset, path, depth+1, fields)
		case 0x01:
			value, ok := readCString(data, offset)
			if !ok {
				*fields = append(*fields, path+"=<bad_string>")
				return
			}
			*fields = append(*fields, path+"="+sanitizeTableText(value))
		case 0x02, 0x03:
			if *offset+4 > len(data) {
				*fields = append(*fields, path+"=<truncated_4>")
				return
			}
			raw := data[*offset : *offset+4]
			*offset += 4
			if typ == 0x03 {
				be := binary.BigEndian.Uint32(raw)
				le := binary.LittleEndian.Uint32(raw)
				*fields = append(*fields, fmt.Sprintf("%s=be:%d/%.2f le:%d/%.2f", path, be, math.Float32frombits(be), le, math.Float32frombits(le)))
			} else {
				value := binary.BigEndian.Uint32(raw)
				*fields = append(*fields, fmt.Sprintf("%s=%d", path, value))
			}
		case 0x04:
			if *offset+4 > len(data) {
				*fields = append(*fields, path+"=<truncated_ptr>")
				return
			}
			value := binary.LittleEndian.Uint32(data[*offset : *offset+4])
			*offset += 4
			*fields = append(*fields, fmt.Sprintf("%s=ptr_0x%08x", path, value))
		case 0x05:
			value, ok := readUTF16CString(data, offset)
			if !ok {
				*fields = append(*fields, path+"=<bad_wstring>")
				return
			}
			*fields = append(*fields, path+"="+sanitizeTableText(value))
		case 0x06:
			if *offset+4 > len(data) {
				*fields = append(*fields, path+"=<truncated_color>")
				return
			}
			r, g, b, a := data[*offset], data[*offset+1], data[*offset+2], data[*offset+3]
			*offset += 4
			*fields = append(*fields, fmt.Sprintf("%s=rgba(%d,%d,%d,%d)", path, r, g, b, a))
		case 0x07:
			if *offset+8 > len(data) {
				*fields = append(*fields, path+"=<truncated_u64>")
				return
			}
			value := binary.LittleEndian.Uint64(data[*offset : *offset+8])
			*offset += 8
			*fields = append(*fields, fmt.Sprintf("%s=%d", path, value))
		default:
			*fields = append(*fields, fmt.Sprintf("%s=<unknown_type_0x%02x_at_%d>", path, typ, before))
			return
		}
		if *offset <= before {
			*fields = append(*fields, fmt.Sprintf("%s.<no_progress>=offset_%d", prefix, *offset))
			return
		}
	}
}

func skipBinaryKVTail(data []byte, offset int) int {
	for offset < len(data) {
		switch data[offset] {
		case 0x00, 0x08, 0x0b:
			offset++
		default:
			return offset
		}
	}
	return offset
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
		CP_GBK,
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
		CP_GBK,
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

func sampleHex(payload []byte, limit int) string {
	if len(payload) == 0 {
		return ""
	}
	if len(payload) > limit {
		payload = payload[:limit]
	}
	return hex.EncodeToString(payload)
}

func payloadSampleHex(payload []byte) string {
	if isBinaryKVPayload(payload) {
		return sampleHex(payload, 2048)
	}
	return sampleHex(payload, 64)
}

func isBinaryKVPayload(payload []byte) bool {
	return len(payload) >= 17 &&
		payload[0] == 0xff &&
		payload[1] == 0xff &&
		payload[2] == 0xff &&
		payload[3] == 0xff &&
		payload[4] == 0x00
}

func mostlyPrintable(payload []byte) bool {
	printable := 0
	for _, b := range payload {
		if b == '\r' || b == '\n' || b == '\t' || (b >= 32 && b < 127) {
			printable++
		}
	}
	return printable*100/len(payload) >= 85
}

func escapeTableText(text string) string {
	if text == "" {
		return "-"
	}
	return sanitizeTableText(text)
}

func sanitizeTableText(text string) string {
	text = strings.ReplaceAll(text, "|", "/")
	text = strings.ReplaceAll(text, "\r", " ")
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.ReplaceAll(text, "\t", " ")
	return text
}

func sendPacketErr(ctx context.Context, errs chan<- string, err string) {
	select {
	case errs <- err:
	case <-ctx.Done():
	default:
	}
}

func listTCP4(pid uint32) ([]endpointInfo, error) {
	var size uint32
	ret, _, _ := procGetExtendedTcpTable.Call(0, uintptr(unsafe.Pointer(&size)), 0, AF_INET, TCP_TABLE_OWNER_PID_ALL, 0)
	if size == 0 {
		return nil, fmt.Errorf("GetExtendedTcpTable size query failed: %d", ret)
	}
	buf := make([]byte, size)
	ret, _, _ = procGetExtendedTcpTable.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), 0, AF_INET, TCP_TABLE_OWNER_PID_ALL, 0)
	if ret != 0 {
		return nil, fmt.Errorf("GetExtendedTcpTable failed: %d", ret)
	}
	if len(buf) < 4 {
		return nil, fmt.Errorf("tcp table too small")
	}
	count := *(*uint32)(unsafe.Pointer(&buf[0]))
	rowSize := int(unsafe.Sizeof(mibTCPRowOwnerPID{}))
	base := 4
	var out []endpointInfo
	for i := 0; i < int(count); i++ {
		offset := base + i*rowSize
		if offset+rowSize > len(buf) {
			break
		}
		row := (*mibTCPRowOwnerPID)(unsafe.Pointer(&buf[offset]))
		if row.OwningPID != pid {
			continue
		}
		out = append(out, endpointInfo{
			Proto:  "TCP",
			Local:  formatEndpoint(row.LocalAddr, row.LocalPort),
			Remote: formatEndpoint(row.RemoteAddr, row.RemotePort),
			State:  tcpState(row.State),
		})
	}
	return out, nil
}

func listUDP4(pid uint32) ([]endpointInfo, error) {
	var size uint32
	ret, _, _ := procGetExtendedUdpTable.Call(0, uintptr(unsafe.Pointer(&size)), 0, AF_INET, UDP_TABLE_OWNER_PID, 0)
	if size == 0 {
		return nil, fmt.Errorf("GetExtendedUdpTable size query failed: %d", ret)
	}
	buf := make([]byte, size)
	ret, _, _ = procGetExtendedUdpTable.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), 0, AF_INET, UDP_TABLE_OWNER_PID, 0)
	if ret != 0 {
		return nil, fmt.Errorf("GetExtendedUdpTable failed: %d", ret)
	}
	if len(buf) < 4 {
		return nil, fmt.Errorf("udp table too small")
	}
	count := *(*uint32)(unsafe.Pointer(&buf[0]))
	rowSize := int(unsafe.Sizeof(mibUDPRowOwnerPID{}))
	base := 4
	var out []endpointInfo
	for i := 0; i < int(count); i++ {
		offset := base + i*rowSize
		if offset+rowSize > len(buf) {
			break
		}
		row := (*mibUDPRowOwnerPID)(unsafe.Pointer(&buf[offset]))
		if row.OwningPID != pid {
			continue
		}
		out = append(out, endpointInfo{
			Proto:  "UDP",
			Local:  formatEndpoint(row.LocalAddr, row.LocalPort),
			Remote: "(remote not exposed by UDP owner table)",
			State:  "LISTEN",
		})
	}
	return out, nil
}

func formatEndpoint(addr, port uint32) string {
	ip := formatIPv4(addr)
	return fmt.Sprintf("%s:%d", ip, ntohs32(port))
}

func formatIPv4(addr uint32) string {
	return fmt.Sprintf("%d.%d.%d.%d", byte(addr), byte(addr>>8), byte(addr>>16), byte(addr>>24))
}

func ntohs32(v uint32) uint16 {
	return uint16((v&0xff)<<8 | (v>>8)&0xff)
}

func normalizePort(port uint16) int {
	p := int(port)
	swapped := int((port>>8)&0xff) | int(port&0xff)<<8
	if looksLikeGamePort(swapped) && !looksLikeGamePort(p) {
		return swapped
	}
	return p
}

func looksLikeGamePort(port int) bool {
	return port > 0 && port <= 65535
}

func tcpState(state uint32) string {
	switch state {
	case 1:
		return "CLOSED"
	case 2:
		return "LISTEN"
	case 3:
		return "SYN_SENT"
	case 4:
		return "SYN_RCVD"
	case 5:
		return "ESTABLISHED"
	case 6:
		return "FIN_WAIT1"
	case 7:
		return "FIN_WAIT2"
	case 8:
		return "CLOSE_WAIT"
	case 9:
		return "CLOSING"
	case 10:
		return "LAST_ACK"
	case 11:
		return "TIME_WAIT"
	case 12:
		return "DELETE_TCB"
	default:
		return fmt.Sprintf("STATE_%d", state)
	}
}

func isRelevant(path string) bool {
	lower := strings.ToLower(path)
	for _, token := range []string{"serverbrowser", "matchmaking", "steam_api", "engine.dll", "client.dll", "vgui", "server.dll", "filesystem_stdio"} {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return false
}

func stamp(text string) string {
	return time.Now().Format(time.RFC3339) + " " + text
}

func getText(hwnd uintptr) string {
	n, _, _ := procGetWindowTextLength.Call(hwnd)
	buf := make([]uint16, n+1)
	procGetWindowText.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), n+1)
	return syscall.UTF16ToString(buf)
}

func setText(hwnd uintptr, text string) {
	procSetWindowText.Call(hwnd, uintptr(unsafe.Pointer(utf16Ptr(text))))
}

func messageBox(title, text string) {
	procMessageBox.Call(app.hwnd, uintptr(unsafe.Pointer(utf16Ptr(text))), uintptr(unsafe.Pointer(utf16Ptr(title))), 0x10)
}

func getModuleHandle() uintptr {
	ret, _, _ := procGetModuleHandle.Call(0)
	return ret
}

func loadCursor(id uintptr) uintptr {
	ret, _, _ := procLoadCursor.Call(0, id)
	return ret
}

func createWindowEx(exStyle uint32, className, title string, style uint32, x, y, w, h int32, parent, menu, instance uintptr) uintptr {
	ret, _, _ := procCreateWindowEx.Call(
		uintptr(exStyle),
		uintptr(unsafe.Pointer(utf16Ptr(className))),
		uintptr(unsafe.Pointer(utf16Ptr(title))),
		uintptr(style),
		uintptr(x),
		uintptr(y),
		uintptr(w),
		uintptr(h),
		parent,
		menu,
		instance,
		0,
	)
	return ret
}

func setWindowIcon(hwnd, icon uintptr) {
	if hwnd != 0 && icon != 0 {
		procSendMessage.Call(hwnd, WM_SETICON, ICON_BIG, icon)
		procSendMessage.Call(hwnd, WM_SETICON, ICON_SMALL, icon)
	}
}

func loadIcon(instance uintptr, id uintptr) uintptr {
	ret, _, _ := procLoadIcon.Call(instance, id)
	return ret
}

func loadImageIcon(instance uintptr, id uintptr) uintptr {
	ret, _, _ := procLoadImage.Call(instance, id, IMAGE_ICON, 0, 0, LR_DEFAULTSIZE)
	return ret
}

func loadAppIcon(instance uintptr) uintptr {
	for _, id := range []uintptr{1, 2, 3, 4, 5, 101, 102} {
		if h := loadImageIcon(instance, id); h != 0 {
			return h
		}
		if h := loadIcon(instance, id); h != 0 {
			return h
		}
	}
	return loadIcon(0, 32512)
}

func utf16Ptr(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

func loword(v uintptr) uintptr {
	return v & 0xffff
}
