package main

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
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
	cpGBK          = 936
	protoUDP       = 17
	defaultGameDir = `E:\SteamLibrary\steamapps\common\Left 4 Dead 2`

	moveFileReplaceExisting = 0x00000001
	moveFileWriteThrough    = 0x00000008

	WS_OVERLAPPEDWINDOW    = 0x00CF0000
	WS_VISIBLE             = 0x10000000
	WS_CHILD               = 0x40000000
	WS_TABSTOP             = 0x00010000
	WS_BORDER              = 0x00800000
	WS_VSCROLL             = 0x00200000
	WS_EX_CLIENTEDGE       = 0x00000200
	WS_EX_WINDOWEDGE       = 0x00000100
	ES_MULTILINE           = 0x0004
	ES_AUTOVSCROLL         = 0x0040
	ES_READONLY            = 0x0800
	ES_AUTOHSCROLL         = 0x0080
	BS_PUSHBUTTON          = 0x00000000
	BS_AUTOCHECKBOX        = 0x00000003
	BS_GROUPBOX            = 0x00000007
	SS_LEFT                = 0x00000000
	EM_GETFIRSTVISIBLELINE = 0x00CE
	EM_LINESCROLL          = 0x00B6

	SW_SHOW       = 5
	SW_SHOWNORMAL = 1

	WM_CREATE         = 0x0001
	WM_DESTROY        = 0x0002
	WM_SIZE           = 0x0005
	WM_ERASEBKGND     = 0x0014
	WM_COMMAND        = 0x0111
	WM_SETICON        = 0x0080
	WM_SETFONT        = 0x0030
	WM_TIMER          = 0x0113
	WM_GETTEXT        = 0x000D
	WM_GETTEXTLENGTH  = 0x000E
	WM_CTLCOLOREDIT   = 0x0133
	WM_CTLCOLORBTN    = 0x0135
	WM_CTLCOLORSTATIC = 0x0138
	BM_GETCHECK       = 0x00F0
	BM_SETCHECK       = 0x00F1
	BST_CHECKED       = 1
	WM_APP            = 0x8000

	ICON_SMALL     = 0
	ICON_BIG       = 1
	IMAGE_ICON     = 1
	LR_DEFAULTSIZE = 0x00000040

	ID_START        = 1001
	ID_STOP         = 1002
	ID_CLEAR        = 1003
	ID_DRYRUN       = 1004
	ID_VERBOSE      = 1005
	ID_BLOCK_EP     = 1006
	ID_RPG_PRESET   = 1007
	ID_FETCH_XY     = 1008
	ID_KNOWN_IPS    = 1009
	ID_CLEAR_CACHE  = 1010
	ID_SAVE_IPS     = 1011
	ID_VERIFY_GD    = 1012
	ID_OPEN_GD_RPT  = 1013
	ID_STATUS_TIMER = 2001

	WM_WORKER_EVENT = WM_APP + 1
	WM_WORKER_DONE  = WM_APP + 2
	WM_IMPORT_DONE  = WM_APP + 3
	WM_CACHE_DONE   = WM_APP + 4
	WM_VERIFY_DONE  = WM_APP + 5
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")

	procDefWindowProc       = user32.NewProc("DefWindowProcW")
	procDispatchMessage     = user32.NewProc("DispatchMessageW")
	procGetMessage          = user32.NewProc("GetMessageW")
	procLoadIcon            = user32.NewProc("LoadIconW")
	procLoadImage           = user32.NewProc("LoadImageW")
	procLoadCursor          = user32.NewProc("LoadCursorW")
	procRegisterClassEx     = user32.NewProc("RegisterClassExW")
	procCreateWindowEx      = user32.NewProc("CreateWindowExW")
	procShowWindow          = user32.NewProc("ShowWindow")
	procUpdateWindow        = user32.NewProc("UpdateWindow")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procSendMessage         = user32.NewProc("SendMessageW")
	procPostMessage         = user32.NewProc("PostMessageW")
	procPostQuitMessage     = user32.NewProc("PostQuitMessage")
	procSetWindowText       = user32.NewProc("SetWindowTextW")
	procSetTimer            = user32.NewProc("SetTimer")
	procKillTimer           = user32.NewProc("KillTimer")
	procGetClientRect       = user32.NewProc("GetClientRect")
	procMoveWindow          = user32.NewProc("MoveWindow")
	procEnableWindow        = user32.NewProc("EnableWindow")
	procMessageBox          = user32.NewProc("MessageBoxW")
	procFillRect            = user32.NewProc("FillRect")
	procGetModuleHandle     = kernel32.NewProc("GetModuleHandleW")
	procMultiByteToWideChar = kernel32.NewProc("MultiByteToWideChar")
	procMoveFileEx          = kernel32.NewProc("MoveFileExW")
	procGetStockObject      = gdi32.NewProc("GetStockObject")
	procCreateSolidBrush    = gdi32.NewProc("CreateSolidBrush")
	procDeleteObject        = gdi32.NewProc("DeleteObject")
	procSetTextColor        = gdi32.NewProc("SetTextColor")
	procSetBkColor          = gdi32.NewProc("SetBkColor")
	procShellExecute        = shell32.NewProc("ShellExecuteW")
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

type darkTheme struct {
	bgBrush    uintptr
	panelBrush uintptr
	editBrush  uintptr
	bgColor    uint32
	panelColor uint32
	editColor  uint32
	textColor  uint32
	mutedColor uint32
}

type uiState struct {
	hwnd uintptr
	font uintptr

	ruleGroup   uintptr
	statusGroup uintptr
	hintText    uintptr
	logLabel    uintptr

	keywordEdit uintptr
	ipEdit      uintptr
	dryRun      uintptr
	verbose     uintptr
	blockEp     uintptr
	startBtn    uintptr
	stopBtn     uintptr
	clearBtn    uintptr
	presetBtn   uintptr
	fetchXYBtn  uintptr
	knownIPBtn  uintptr
	cacheBtn    uintptr
	saveIPBtn   uintptr
	verifyBtn   uintptr
	reportBtn   uintptr
	status      uintptr
	stats       uintptr
	logEdit     uintptr

	mu            sync.Mutex
	running       bool
	closing       bool
	cancel        context.CancelFunc
	handle        windivert.Handle
	events        []string
	logs          []string
	pendingIPs    []string
	hitConfig     hitConfig
	hitDirty      bool
	runStarted    string
	runHitCount   int
	runEndpoints  map[string]bool
	statsSnapshot filterCounters
	verifyRunning bool
	verifyLog     string
	verifyReport  string
}

type filterCounters struct {
	Seen    int
	Forward int
	Dropped int
	Matched int
	Errors  int
	Blocked int
	Last    string
}

type filterConfig struct {
	Keywords       []string
	IPs            []string
	DryRun         bool
	Verbose        bool
	BlockEndpoints bool
}

type packetInfo struct {
	srcIP     string
	dstIP     string
	srcPort   int
	dstPort   int
	payload   []byte
	remoteKey string
}

type serverDetails struct {
	Kind       string
	Name       string
	Online     string
	Local      string
	Mode       string
	Campaign   string
	Mission    string
	Difficulty string
	Tags       string
	Players    int
	MaxPlayers int
	Raw        map[string]string
}

type hitConfig struct {
	Version   int                   `json:"version"`
	UpdatedAt string                `json:"updated_at"`
	ManualIPs []string              `json:"manual_ips,omitempty"`
	Entries   map[string]*hitRecord `json:"entries"`
	Runs      []hitRunSummary       `json:"runs,omitempty"`
}

type hitRecord struct {
	Endpoint   string            `json:"endpoint"`
	Source     string            `json:"source,omitempty"`
	LastName   string            `json:"last_name"`
	LastMap    string            `json:"last_map"`
	LastKind   string            `json:"last_kind"`
	LastReason string            `json:"last_reason"`
	LastOnline string            `json:"last_online,omitempty"`
	LastLocal  string            `json:"last_local,omitempty"`
	LastTags   string            `json:"last_tags,omitempty"`
	RawFields  map[string]string `json:"raw_fields,omitempty"`
	Players    int               `json:"players"`
	MaxPlayers int               `json:"max_players"`
	FirstSeen  string            `json:"first_seen"`
	LastSeen   string            `json:"last_seen"`
	HitCount   int               `json:"hit_count"`
}

type hitRunSummary struct {
	StartedAt string   `json:"started_at"`
	EndedAt   string   `json:"ended_at"`
	Hits      int      `json:"hits"`
	Endpoints []string `json:"endpoints"`
}

type gameDataConfig struct {
	Schema       int              `json:"schema"`
	Game         string           `json:"game"`
	Module       string           `json:"module"`
	ModuleName   string           `json:"module_name"`
	ImageBase    string           `json:"image_base"`
	SourceReport string           `json:"source_report"`
	Targets      []gameDataTarget `json:"targets"`
}

type gameDataTarget struct {
	Name         string `json:"name"`
	RVA          string `json:"rva"`
	Role         string `json:"role"`
	Pattern      string `json:"pattern"`
	Mask         string `json:"mask"`
	ExpectedHits int    `json:"expected_hits"`
}

type gameDataVerifyResult struct {
	Name         string
	Role         string
	RVA          uint64
	ExpectedHits int
	Hits         []int
	Error        string
}

type filterState struct {
	mu       sync.RWMutex
	blocked  map[string]string
	keywords []string
	ips      []string
}

var (
	app   = &uiState{}
	theme darkTheme
)

var officialMapCodes = map[string]bool{
	"c1m1_hotel":               true,
	"c1m2_streets":             true,
	"c1m3_mall":                true,
	"c1m4_atrium":              true,
	"c2m1_highway":             true,
	"c2m2_fairgrounds":         true,
	"c2m3_coaster":             true,
	"c2m4_barns":               true,
	"c2m5_concert":             true,
	"c3m1_plankcountry":        true,
	"c3m2_swamp":               true,
	"c3m3_shantytown":          true,
	"c3m4_plantation":          true,
	"c4m1_milltown_a":          true,
	"c4m2_sugarmill_a":         true,
	"c4m3_sugarmill_b":         true,
	"c4m4_milltown_b":          true,
	"c4m5_milltown_escape":     true,
	"c5m1_waterfront_sndscape": true,
	"c5m1_waterfront":          true,
	"c5m2_park":                true,
	"c5m3_cemetery":            true,
	"c5m4_quarter":             true,
	"c5m5_bridge":              true,
	"c6m1_riverbank":           true,
	"c6m2_bedlam":              true,
	"c6m3_port":                true,
	"c7m1_docks":               true,
	"c7m2_barge":               true,
	"c7m3_port":                true,
	"c8m1_apartment":           true,
	"c8m2_subway":              true,
	"c8m3_sewers":              true,
	"c8m4_interior":            true,
	"c8m5_rooftop":             true,
	"c9m1_alleys":              true,
	"c9m2_lots":                true,
	"c10m1_caves":              true,
	"c10m2_drainage":           true,
	"c10m3_ranchhouse":         true,
	"c10m4_mainstreet":         true,
	"c10m5_houseboat":          true,
	"c11m1_greenhouse":         true,
	"c11m2_offices":            true,
	"c11m3_garage":             true,
	"c11m4_terminal":           true,
	"c11m5_runway":             true,
	"c12m1_hilltop":            true,
	"c12m2_traintunnel":        true,
	"c12m3_bridge":             true,
	"c12m4_barn":               true,
	"c12m5_cornfield":          true,
	"c13m1_alpinecreek":        true,
	"c13m2_southpinestream":    true,
	"c13m3_memorialbridge":     true,
	"c13m4_cutthroatcreek":     true,
	"c14m1_junkyard":           true,
	"c14m2_lighthouse":         true,
}

func main() {
	runGUI()
}

func runGUI() {
	runtime.LockOSThread()
	initTheme()
	loadHitConfig()
	hInstance := getModuleHandle()
	className := utf16Ptr("L4D2GameDetailsFilterGUI")
	appIcon := loadAppIcon(hInstance)
	wc := wndclassex{
		Size:       uint32(unsafe.Sizeof(wndclassex{})),
		WndProc:    syscall.NewCallback(wndProc),
		Instance:   hInstance,
		Icon:       appIcon,
		Cursor:     loadCursor(32512),
		Background: theme.bgBrush,
		ClassName:  className,
		IconSm:     appIcon,
	}
	procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	app.hwnd = createWindowEx(0, "L4D2GameDetailsFilterGUI", "L4D2 GameDetails Filter", WS_OVERLAPPEDWINDOW|WS_VISIBLE, 120, 120, 940, 640, 0, 0, hInstance)
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
	case WM_ERASEBKGND:
		var rc rect
		procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
		procFillRect.Call(wParam, uintptr(unsafe.Pointer(&rc)), theme.bgBrush)
		return 1
	case WM_SIZE:
		layout()
		return 0
	case WM_COMMAND:
		switch loword(wParam) {
		case ID_START:
			startFilter()
		case ID_STOP:
			stopFilter()
		case ID_CLEAR:
			clearLog()
		case ID_RPG_PRESET:
			addRPGPreset()
		case ID_FETCH_XY:
			fetchXYServers()
		case ID_KNOWN_IPS:
			importKnownEntryIPs()
		case ID_CLEAR_CACHE:
			clearGameServerCache()
		case ID_SAVE_IPS:
			saveManualIPs()
		case ID_VERIFY_GD:
			verifyGamedataTargets()
		case ID_OPEN_GD_RPT:
			openGamedataReport()
		}
		return 0
	case WM_TIMER:
		if wParam == ID_STATUS_TIMER {
			refreshStatus()
			return 0
		}
	case WM_WORKER_EVENT:
		for _, line := range drainEvents() {
			appendLog(line)
		}
		return 0
	case WM_WORKER_DONE:
		onWorkerDone()
		return 0
	case WM_IMPORT_DONE:
		applyPendingIPs()
		return 0
	case WM_CACHE_DONE:
		enable(app.cacheBtn, true)
		return 0
	case WM_VERIFY_DONE:
		onVerifyDone()
		return 0
	case WM_CTLCOLOREDIT:
		applyControlColors(wParam, theme.textColor, theme.editColor)
		return theme.editBrush
	case WM_CTLCOLORSTATIC:
		if lParam == app.logEdit {
			applyControlColors(wParam, theme.textColor, theme.editColor)
			return theme.editBrush
		}
		if lParam == app.hintText {
			applyControlColors(wParam, theme.mutedColor, theme.bgColor)
			return theme.bgBrush
		}
		applyControlColors(wParam, theme.textColor, theme.bgColor)
		return theme.bgBrush
	case WM_CTLCOLORBTN:
		applyControlColors(wParam, theme.textColor, theme.bgColor)
		return theme.bgBrush
	case WM_DESTROY:
		if app.ipEdit != 0 {
			persistCurrentIPList()
			finalizeRunAndSaveHits()
		}
		app.mu.Lock()
		app.closing = true
		app.mu.Unlock()
		procKillTimer.Call(hwnd, ID_STATUS_TIMER)
		stopFilter()
		deleteTheme()
		procPostQuitMessage.Call(0)
		return 0
	}
	ret, _, _ := procDefWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
	return ret
}

func createControls(hwnd uintptr) {
	app.hwnd = hwnd
	app.font, _, _ = procGetStockObject.Call(17)

	app.ruleGroup = createChild(0, "BUTTON", "Filter rules", BS_GROUPBOX, 14, 10, 900, 132, hwnd, 0)
	createChild(0, "STATIC", "Keywords:", SS_LEFT, 30, 38, 88, 22, hwnd, 0)
	app.keywordEdit = createChild(WS_EX_CLIENTEDGE, "EDIT", "RPG,\u661f\u7f18,\u7834\u6653,\u6740\u622e,\u795e\u57df", WS_BORDER|WS_TABSTOP|ES_AUTOHSCROLL, 120, 34, 610, 28, hwnd, 0)
	createChild(0, "STATIC", "IP / IP:port:", SS_LEFT, 30, 74, 88, 22, hwnd, 0)
	app.ipEdit = createChild(WS_EX_CLIENTEDGE, "EDIT", "", WS_BORDER|WS_TABSTOP|ES_AUTOHSCROLL, 120, 70, 488, 28, hwnd, 0)
	if len(app.hitConfig.ManualIPs) > 0 {
		setText(app.ipEdit, strings.Join(app.hitConfig.ManualIPs, ","))
	}
	app.saveIPBtn = createChild(0, "BUTTON", "Save IPs", BS_PUSHBUTTON|WS_TABSTOP, 620, 70, 88, 28, hwnd, ID_SAVE_IPS)
	app.dryRun = createChild(0, "BUTTON", "Dry run only", BS_AUTOCHECKBOX|WS_TABSTOP, 748, 28, 156, 24, hwnd, ID_DRYRUN)
	app.verbose = createChild(0, "BUTTON", "Verbose allow log", BS_AUTOCHECKBOX|WS_TABSTOP, 748, 58, 156, 24, hwnd, ID_VERBOSE)
	app.blockEp = createChild(0, "BUTTON", "Also block endpoint", BS_AUTOCHECKBOX|WS_TABSTOP, 748, 88, 156, 24, hwnd, ID_BLOCK_EP)

	app.startBtn = createChild(0, "BUTTON", "Start", BS_PUSHBUTTON|WS_TABSTOP, 30, 106, 82, 30, hwnd, ID_START)
	app.stopBtn = createChild(0, "BUTTON", "Stop", BS_PUSHBUTTON|WS_TABSTOP, 122, 106, 70, 30, hwnd, ID_STOP)
	app.clearBtn = createChild(0, "BUTTON", "Clear Log", BS_PUSHBUTTON|WS_TABSTOP, 202, 106, 86, 30, hwnd, ID_CLEAR)
	app.presetBtn = createChild(0, "BUTTON", "RPG Preset", BS_PUSHBUTTON|WS_TABSTOP, 298, 106, 98, 30, hwnd, ID_RPG_PRESET)
	app.fetchXYBtn = createChild(0, "BUTTON", "XY IPs", BS_PUSHBUTTON|WS_TABSTOP, 406, 106, 76, 30, hwnd, ID_FETCH_XY)
	app.knownIPBtn = createChild(0, "BUTTON", "Known IPs", BS_PUSHBUTTON|WS_TABSTOP, 492, 106, 90, 30, hwnd, ID_KNOWN_IPS)
	app.cacheBtn = createChild(0, "BUTTON", "Cache", BS_PUSHBUTTON|WS_TABSTOP, 592, 106, 76, 30, hwnd, ID_CLEAR_CACHE)
	app.hintText = createChild(0, "STATIC", "", SS_LEFT, 30, 142, 860, 1, hwnd, 0)

	app.statusGroup = createChild(0, "BUTTON", "Status", BS_GROUPBOX, 14, 150, 900, 110, hwnd, 0)
	app.status = createChild(0, "STATIC", "Ready. This tool requires administrator privileges and uses WinDivert.", SS_LEFT, 30, 176, 860, 22, hwnd, 0)
	app.stats = createChild(0, "STATIC", "seen=0 forwarded=0 dropped=0 matched=0 errors=0 blocked=0", SS_LEFT, 30, 202, 860, 22, hwnd, 0)
	app.verifyBtn = createChild(0, "BUTTON", "Verify Targets", BS_PUSHBUTTON|WS_TABSTOP, 30, 226, 116, 28, hwnd, ID_VERIFY_GD)
	app.reportBtn = createChild(0, "BUTTON", "Target Report", BS_PUSHBUTTON|WS_TABSTOP, 156, 226, 112, 28, hwnd, ID_OPEN_GD_RPT)

	app.logLabel = createChild(0, "STATIC", "Log:", SS_LEFT, 16, 270, 80, 22, hwnd, 0)
	app.logEdit = createChild(WS_EX_CLIENTEDGE, "EDIT", "", WS_BORDER|WS_VSCROLL|ES_MULTILINE|ES_AUTOVSCROLL|ES_READONLY, 16, 296, 898, 290, hwnd, 0)

	enable(app.stopBtn, false)
	layout()
	app.mu.Lock()
	knownHits := len(app.hitConfig.Entries)
	app.mu.Unlock()
	if knownHits > 0 {
		appendLog(fmt.Sprintf("%s Loaded %d saved matched entries from %s.", time.Now().Format("15:04:05"), knownHits, hitConfigFilePath()))
	}
}

func startFilter() {
	app.mu.Lock()
	if app.running {
		app.mu.Unlock()
		return
	}
	app.mu.Unlock()

	cfg := filterConfig{
		Keywords:       splitList(getText(app.keywordEdit)),
		IPs:            splitList(getText(app.ipEdit)),
		DryRun:         isChecked(app.dryRun),
		Verbose:        isChecked(app.verbose),
		BlockEndpoints: isChecked(app.blockEp),
	}
	ctx, cancel := context.WithCancel(context.Background())

	persistIPList(cfg.IPs)

	app.mu.Lock()
	if app.running {
		app.mu.Unlock()
		cancel()
		return
	}
	app.running = true
	app.cancel = cancel
	app.logs = nil
	app.events = nil
	app.statsSnapshot = filterCounters{}
	app.runStarted = time.Now().Format(time.RFC3339)
	app.runHitCount = 0
	app.runEndpoints = map[string]bool{}
	app.mu.Unlock()

	setText(app.logEdit, "")
	enable(app.startBtn, false)
	enable(app.stopBtn, true)
	enable(app.keywordEdit, false)
	enable(app.ipEdit, false)
	enable(app.saveIPBtn, false)
	enable(app.dryRun, false)
	enable(app.verbose, false)
	enable(app.blockEp, false)
	enable(app.presetBtn, false)
	enable(app.fetchXYBtn, false)
	enable(app.knownIPBtn, false)
	enable(app.cacheBtn, false)
	setText(app.status, "Starting WinDivert filter...")
	procSetTimer.Call(app.hwnd, ID_STATUS_TIMER, 1000, 0)
	go func() {
		defer recoverWorker("filter worker")
		time.Sleep(150 * time.Millisecond)
		filterWorker(ctx, cfg)
	}()
}

func stopFilter() {
	app.mu.Lock()
	running := app.running
	cancel := app.cancel
	handle := app.handle
	app.mu.Unlock()
	if !running {
		return
	}
	setText(app.status, "Stopping filter...")
	enable(app.stopBtn, false)
	if cancel != nil {
		cancel()
	}
	if handle != 0 {
		_ = handle.Shutdown(windivert.ShutdownBoth)
		go func(h windivert.Handle) {
			time.Sleep(250 * time.Millisecond)
			_ = h.Close()
		}(handle)
	}
}

func filterWorker(ctx context.Context, cfg filterConfig) {
	state := &filterState{blocked: map[string]string{}, keywords: cfg.Keywords, ips: cfg.IPs}
	for _, ip := range cfg.IPs {
		state.blocked[ip] = "configured ip"
	}

	postEvent("Filter starting. keywords=" + strings.Join(cfg.Keywords, ", ") + " ips=" + strings.Join(cfg.IPs, ", "))
	filter := "ip and udp and (udp.SrcPort == 27005 or udp.DstPort == 27005 or (udp.SrcPort >= 2000 and udp.SrcPort <= 40000))"
	handle, err := windivert.Open(filter, windivert.LayerNetwork, 0, 0)
	if err != nil {
		postEvent("WinDivert open failed: " + err.Error())
		postDone()
		return
	}
	app.mu.Lock()
	app.handle = handle
	app.mu.Unlock()
	defer func() {
		_ = handle.Close()
		app.mu.Lock()
		app.handle = 0
		app.mu.Unlock()
		postDone()
	}()
	select {
	case <-ctx.Done():
		postEvent("Stopping filter...")
		return
	default:
	}
	_ = handle.SetParam(windivert.QueueLength, 8192)
	postEvent("Filter running. Matching GameDetailsServer and A2S_INFO responses.")

	packet := make([]byte, 65535)
	for {
		select {
		case <-ctx.Done():
			postEvent("Stopping filter...")
			return
		default:
		}
		var addr windivert.Address
		n, err := handle.Recv(packet, &addr)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			updateStats(func(s *filterCounters) { s.Errors++ })
			postEvent("recv failed: " + err.Error())
			return
		}
		current := append([]byte(nil), packet[:n]...)
		updateStats(func(s *filterCounters) { s.Seen++ })
		drop, line := shouldDropPacket(current, state, cfg)
		if drop {
			updateStats(func(s *filterCounters) {
				s.Dropped++
				s.Blocked = state.blockCount()
				s.Last = line
			})
			postEvent("DROP " + line)
			if cfg.DryRun {
				_, _ = handle.Send(current, &addr)
				updateStats(func(s *filterCounters) { s.Forward++ })
			}
			continue
		}
		if _, err := handle.Send(current, &addr); err != nil {
			updateStats(func(s *filterCounters) { s.Errors++ })
			postEvent("send failed: " + err.Error())
			continue
		}
		updateStats(func(s *filterCounters) { s.Forward++ })
	}
}

func shouldDropPacket(packet []byte, state *filterState, cfg filterConfig) (bool, string) {
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
	details, ok := parseServerDetails(info.payload)
	if !ok {
		return false, ""
	}
	updateStats(func(s *filterCounters) { s.Matched++ })
	sourceKey := info.remoteKey
	if details.Online != "" {
		info.remoteKey = details.Online
	}
	matched, reason := state.matches(details)
	if matched {
		if cfg.BlockEndpoints {
			state.addBlock(info.remoteKey, reason)
			if details.Online != "" {
				state.addBlock(details.Online, "linked_online:"+reason)
			}
			if details.Local != "" {
				state.addBlock(details.Local, "linked_local:"+reason)
			}
		}
		rememberHit(info.remoteKey, sourceKey, details, reason)
		return true, fmt.Sprintf("%s type=%s name=%q map=%s players=%d/%d mode=%s online=%s local=%s tags=%q reason=%s", info.remoteKey, details.Kind, details.Name, details.Campaign, details.Players, details.MaxPlayers, details.Mode, details.Online, details.Local, details.Tags, reason)
	}
	if cfg.Verbose {
		postEvent(fmt.Sprintf("ALLOW %s type=%s name=%q map=%s players=%d/%d mode=%s online=%s local=%s tags=%q difficulty=%s", info.remoteKey, details.Kind, details.Name, details.Campaign, details.Players, details.MaxPlayers, details.Mode, details.Online, details.Local, details.Tags, details.Difficulty))
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
		payload:   packet[ihl+8:],
		remoteKey: net.JoinHostPort(remoteIP, strconv.Itoa(remotePort)),
	}, true
}

func parseServerDetails(payload []byte) (serverDetails, bool) {
	if details, ok := parseGameDetailsServer(payload); ok {
		return details, true
	}
	return parseA2SInfoResponse(payload)
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
		Kind:       "GameDetailsServer",
		Name:       fields["GameDetailsServer.Server.Name"],
		Online:     fields["GameDetailsServer.Server.adronline"],
		Local:      fields["GameDetailsServer.Server.adrlocal"],
		Mode:       fields["GameDetailsServer.game.Mode"],
		Campaign:   fields["GameDetailsServer.game.campaign"],
		Mission:    fields["GameDetailsServer.game.MissionInfo.MissionFile"],
		Difficulty: fields["GameDetailsServer.game.difficulty"],
		Players:    parseIntField(fields["GameDetailsServer.Members.numPlayers"]),
		MaxPlayers: parseIntField(fields["GameDetailsServer.Members.numSlots"]),
		Raw:        fields,
	}
	return d, d.Name != "" || d.Online != ""
}

func parseA2SInfoResponse(payload []byte) (serverDetails, bool) {
	if len(payload) < 8 || payload[0] != 0xff || payload[1] != 0xff || payload[2] != 0xff || payload[3] != 0xff || payload[4] != 0x49 {
		return serverDetails{}, false
	}
	fields := map[string]string{
		"A2S_INFO.Header":   "0x49",
		"A2S_INFO.Protocol": strconv.Itoa(int(payload[5])),
	}
	offset := 6
	name, ok := readCString(payload, &offset)
	if !ok || name == "" {
		return serverDetails{}, false
	}
	mapName, _ := readCString(payload, &offset)
	folder, _ := readCString(payload, &offset)
	game, _ := readCString(payload, &offset)
	fields["A2S_INFO.Name"] = name
	fields["A2S_INFO.Map"] = mapName
	fields["A2S_INFO.Folder"] = folder
	fields["A2S_INFO.Game"] = game
	if offset+2 <= len(payload) {
		fields["A2S_INFO.AppID"] = strconv.Itoa(int(binary.LittleEndian.Uint16(payload[offset : offset+2])))
		offset += 2
	}
	if offset < len(payload) {
		fields["A2S_INFO.Players"] = strconv.Itoa(int(payload[offset]))
		offset++
	}
	if offset < len(payload) {
		fields["A2S_INFO.MaxPlayers"] = strconv.Itoa(int(payload[offset]))
		offset++
	}
	if offset < len(payload) {
		fields["A2S_INFO.Bots"] = strconv.Itoa(int(payload[offset]))
		offset++
	}
	if offset < len(payload) {
		fields["A2S_INFO.ServerType"] = string([]byte{payload[offset]})
		offset++
	}
	if offset < len(payload) {
		fields["A2S_INFO.Environment"] = string([]byte{payload[offset]})
		offset++
	}
	if offset < len(payload) {
		fields["A2S_INFO.Visibility"] = strconv.Itoa(int(payload[offset]))
		offset++
	}
	if offset < len(payload) {
		fields["A2S_INFO.VAC"] = strconv.Itoa(int(payload[offset]))
		offset++
	}
	version, _ := readCString(payload, &offset)
	if version != "" {
		fields["A2S_INFO.Version"] = version
	}
	tags := ""
	if offset < len(payload) {
		edf := payload[offset]
		fields["A2S_INFO.EDF"] = fmt.Sprintf("0x%02x", edf)
		offset++
		if edf&0x80 != 0 && offset+2 <= len(payload) {
			fields["A2S_INFO.GamePort"] = strconv.Itoa(int(binary.LittleEndian.Uint16(payload[offset : offset+2])))
			offset += 2
		}
		if edf&0x10 != 0 && offset+8 <= len(payload) {
			fields["A2S_INFO.SteamID"] = strconv.FormatUint(binary.LittleEndian.Uint64(payload[offset:offset+8]), 10)
			offset += 8
		}
		if edf&0x40 != 0 && offset+2 <= len(payload) {
			fields["A2S_INFO.SpectatorPort"] = strconv.Itoa(int(binary.LittleEndian.Uint16(payload[offset : offset+2])))
			offset += 2
			spectatorName, _ := readCString(payload, &offset)
			fields["A2S_INFO.SpectatorName"] = spectatorName
		}
		if edf&0x20 != 0 {
			tags, _ = readCString(payload, &offset)
			fields["A2S_INFO.Keywords"] = tags
		}
		if edf&0x01 != 0 && offset+8 <= len(payload) {
			fields["A2S_INFO.GameID"] = strconv.FormatUint(binary.LittleEndian.Uint64(payload[offset:offset+8]), 10)
		}
	}
	return serverDetails{
		Kind:       "A2S_INFO",
		Name:       name,
		Campaign:   mapName,
		Mode:       game,
		Mission:    folder,
		Tags:       tags,
		Players:    parseIntField(fields["A2S_INFO.Players"]),
		MaxPlayers: parseIntField(fields["A2S_INFO.MaxPlayers"]),
		Raw:        fields,
	}, true
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
		case 0x02, 0x03:
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
	if isDisguisedDefaultServer(details) {
		return true, fmt.Sprintf("default-name-official-map-maxplayers>8 map=%s players=%d/%d", details.Campaign, details.Players, details.MaxPlayers)
	}
	values := []string{details.Kind, details.Name, details.Online, details.Local, details.Mode, details.Campaign, details.Mission, details.Difficulty, details.Tags}
	for _, value := range details.Raw {
		if value != "" {
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

func isDisguisedDefaultServer(details serverDetails) bool {
	name := strings.TrimSpace(details.Name)
	mapCode := strings.ToLower(strings.TrimSpace(details.Campaign))
	if strings.EqualFold(name, "Left 4 Dead 2") && officialMapCodes[mapCode] && details.MaxPlayers > 8 {
		return true
	}
	if strings.Contains(strings.ToLower(name), "valve left4dead 2 hong kong server") && officialMapCodes[mapCode] {
		return true
	}
	return false
}

func parseIntField(value string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(value))
	return n
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

func (s *filterState) blockCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.blocked)
}

func updateStats(fn func(*filterCounters)) {
	app.mu.Lock()
	fn(&app.statsSnapshot)
	app.mu.Unlock()
}

func refreshStatus() {
	app.mu.Lock()
	running := app.running
	stats := app.statsSnapshot
	app.mu.Unlock()
	if running {
		setText(app.status, "Filtering GameDetailsServer and A2S_INFO responses. Keep this window open while testing.")
	} else {
		setText(app.status, "Ready. This tool requires administrator privileges and uses WinDivert.")
	}
	setText(app.stats, fmt.Sprintf("seen=%d forwarded=%d dropped=%d matched=%d errors=%d blocked=%d last=%s", stats.Seen, stats.Forward, stats.Dropped, stats.Matched, stats.Errors, stats.Blocked, stats.Last))
}

func postEvent(line string) {
	app.mu.Lock()
	if app.closing {
		app.mu.Unlock()
		return
	}
	shouldNotify := len(app.events) == 0
	app.events = append(app.events, time.Now().Format("15:04:05")+" "+line)
	hwnd := app.hwnd
	app.mu.Unlock()
	if hwnd != 0 && shouldNotify {
		procPostMessage.Call(hwnd, WM_WORKER_EVENT, 0, 0)
	}
}

func postDone() {
	app.mu.Lock()
	if app.closing {
		app.mu.Unlock()
		return
	}
	hwnd := app.hwnd
	app.mu.Unlock()
	if hwnd != 0 {
		procPostMessage.Call(hwnd, WM_WORKER_DONE, 0, 0)
	}
}

func recoverWorker(name string) {
	if r := recover(); r != nil {
		postEvent(fmt.Sprintf("%s recovered from panic: %v", name, r))
		postDone()
	}
}

func drainEvents() []string {
	app.mu.Lock()
	defer app.mu.Unlock()
	events := append([]string(nil), app.events...)
	app.events = nil
	return events
}

func rememberHit(endpoint, source string, details serverDetails, reason string) {
	if endpoint == "" {
		return
	}
	now := time.Now().Format(time.RFC3339)
	app.mu.Lock()
	defer app.mu.Unlock()
	if app.hitConfig.Version == 0 {
		app.hitConfig.Version = 1
	}
	if app.hitConfig.Entries == nil {
		app.hitConfig.Entries = map[string]*hitRecord{}
	}
	rec := app.hitConfig.Entries[endpoint]
	if rec == nil {
		rec = &hitRecord{Endpoint: endpoint, FirstSeen: now}
		app.hitConfig.Entries[endpoint] = rec
	}
	rec.Source = source
	rec.LastName = details.Name
	rec.LastMap = details.Campaign
	rec.LastKind = details.Kind
	rec.LastReason = reason
	rec.LastOnline = details.Online
	rec.LastLocal = details.Local
	rec.LastTags = details.Tags
	if len(details.Raw) > 0 {
		rec.RawFields = map[string]string{}
		for key, value := range details.Raw {
			rec.RawFields[key] = value
		}
	} else {
		rec.RawFields = nil
	}
	rec.Players = details.Players
	rec.MaxPlayers = details.MaxPlayers
	rec.LastSeen = now
	rec.HitCount++
	app.hitConfig.UpdatedAt = now
	app.hitDirty = true
	app.runHitCount++
	if app.runEndpoints == nil {
		app.runEndpoints = map[string]bool{}
	}
	app.runEndpoints[endpoint] = true
}

func finalizeRunAndSaveHits() {
	cfg, dirty := snapshotHitConfig(true)
	if !dirty {
		return
	}
	if err := saveHitConfig(cfg); err != nil {
		appendLog(time.Now().Format("15:04:05") + " Failed to save hit config: " + err.Error())
		return
	}
	appendLog(fmt.Sprintf("%s Saved %d matched entries to %s.", time.Now().Format("15:04:05"), len(cfg.Entries), hitConfigFilePath()))
}

func snapshotHitConfig(includeRun bool) (hitConfig, bool) {
	app.mu.Lock()
	defer app.mu.Unlock()
	dirty := app.hitDirty
	if includeRun && app.runHitCount > 0 {
		var endpoints []string
		for endpoint := range app.runEndpoints {
			endpoints = append(endpoints, endpoint)
		}
		sort.Strings(endpoints)
		app.hitConfig.Runs = append(app.hitConfig.Runs, hitRunSummary{
			StartedAt: app.runStarted,
			EndedAt:   time.Now().Format(time.RFC3339),
			Hits:      app.runHitCount,
			Endpoints: endpoints,
		})
		if len(app.hitConfig.Runs) > 20 {
			app.hitConfig.Runs = app.hitConfig.Runs[len(app.hitConfig.Runs)-20:]
		}
		app.runHitCount = 0
		app.runEndpoints = nil
		app.hitDirty = true
		dirty = true
	}
	if app.hitConfig.Entries == nil {
		app.hitConfig.Entries = map[string]*hitRecord{}
	}
	out := hitConfig{
		Version:   app.hitConfig.Version,
		UpdatedAt: app.hitConfig.UpdatedAt,
		ManualIPs: append([]string(nil), app.hitConfig.ManualIPs...),
		Entries:   map[string]*hitRecord{},
		Runs:      append([]hitRunSummary(nil), app.hitConfig.Runs...),
	}
	for key, rec := range app.hitConfig.Entries {
		cp := *rec
		if rec.RawFields != nil {
			cp.RawFields = map[string]string{}
			for fieldKey, fieldValue := range rec.RawFields {
				cp.RawFields[fieldKey] = fieldValue
			}
		}
		out.Entries[key] = &cp
	}
	app.hitDirty = false
	return out, dirty
}

func onWorkerDone() {
	app.mu.Lock()
	app.running = false
	app.cancel = nil
	app.handle = 0
	app.mu.Unlock()
	procKillTimer.Call(app.hwnd, ID_STATUS_TIMER)
	enable(app.startBtn, true)
	enable(app.stopBtn, false)
	enable(app.keywordEdit, true)
	enable(app.ipEdit, true)
	enable(app.saveIPBtn, true)
	enable(app.dryRun, true)
	enable(app.verbose, true)
	enable(app.blockEp, true)
	enable(app.presetBtn, true)
	enable(app.fetchXYBtn, true)
	enable(app.knownIPBtn, true)
	enable(app.cacheBtn, true)
	refreshStatus()
	appendLog(time.Now().Format("15:04:05") + " Filter stopped.")
	finalizeRunAndSaveHits()
}

func verifyGamedataTargets() {
	app.mu.Lock()
	if app.verifyRunning {
		app.mu.Unlock()
		return
	}
	app.verifyRunning = true
	app.verifyLog = ""
	app.verifyReport = gamedataVerifyReportPath()
	app.mu.Unlock()

	enable(app.verifyBtn, false)
	setText(app.status, "Verifying matchmaking target signatures...")
	appendLog(time.Now().Format("15:04:05") + " Verifying gamedata targets against local matchmaking.dll...")

	go func() {
		defer recoverVerifyWorker()
		log, err := runGamedataVerify(defaultGameDir, gamedataPath(), gamedataVerifyReportPath())
		if err != nil {
			log += "\r\nERROR: " + err.Error()
		}
		app.mu.Lock()
		app.verifyLog = log
		app.verifyReport = gamedataVerifyReportPath()
		app.verifyRunning = false
		hwnd := app.hwnd
		app.mu.Unlock()
		if hwnd != 0 {
			procPostMessage.Call(hwnd, WM_VERIFY_DONE, 0, 0)
		}
	}()
}

func recoverVerifyWorker() {
	if r := recover(); r != nil {
		app.mu.Lock()
		app.verifyLog = fmt.Sprintf("ERROR: gamedata verifier recovered from panic: %v", r)
		app.verifyRunning = false
		hwnd := app.hwnd
		app.mu.Unlock()
		if hwnd != 0 {
			procPostMessage.Call(hwnd, WM_VERIFY_DONE, 0, 0)
		}
	}
}

func onVerifyDone() {
	app.mu.Lock()
	log := app.verifyLog
	report := app.verifyReport
	app.mu.Unlock()
	for _, line := range strings.Split(strings.ReplaceAll(log, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(line) != "" {
			appendLog(time.Now().Format("15:04:05") + " " + line)
		}
	}
	if strings.Contains(log, "MISMATCH") || strings.Contains(log, "ERROR:") {
		setText(app.status, "Target verification failed or has mismatches. Check target report.")
	} else {
		setText(app.status, "Target verification OK. All matchmaking signatures are stable.")
	}
	if report != "" {
		appendLog(time.Now().Format("15:04:05") + " Target report: " + report)
	}
	enable(app.verifyBtn, true)
}

func openGamedataReport() {
	report := gamedataVerifyReportPath()
	if _, err := os.Stat(report); err != nil {
		appendLog(time.Now().Format("15:04:05") + " Target report does not exist yet. Click Verify Targets first.")
		return
	}
	openPath(report)
}

func runGamedataVerify(gameDir, gamedataFile, reportFile string) (string, error) {
	cfg, err := loadGameDataConfig(gamedataFile)
	if err != nil {
		return "", err
	}
	modulePath := filepath.Join(gameDir, filepath.FromSlash(cfg.Module))
	moduleBytes, err := os.ReadFile(modulePath)
	if err != nil {
		return "", err
	}
	var results []gameDataVerifyResult
	for _, target := range cfg.Targets {
		results = append(results, verifyGameDataTarget(moduleBytes, target))
	}
	if err := writeGamedataVerifyReport(reportFile, gameDir, modulePath, gamedataFile, cfg, results); err != nil {
		return "", err
	}
	var out strings.Builder
	out.WriteString("Target verification finished.")
	for _, result := range results {
		status := "OK"
		if result.Error != "" {
			status = "ERROR: " + result.Error
		} else if len(result.Hits) != result.ExpectedHits {
			status = "MISMATCH"
		}
		out.WriteString(fmt.Sprintf("\r\n%-28s expected=%d hits=%d status=%s", result.Name, result.ExpectedHits, len(result.Hits), status))
	}
	if !allGamedataOK(results) {
		return out.String(), fmt.Errorf("one or more gamedata targets failed verification")
	}
	return out.String(), nil
}

func loadGameDataConfig(path string) (gameDataConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return gameDataConfig{}, err
	}
	var cfg gameDataConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return gameDataConfig{}, err
	}
	if cfg.Module == "" || len(cfg.Targets) == 0 {
		return gameDataConfig{}, fmt.Errorf("missing module or targets in gamedata")
	}
	return cfg, nil
}

func verifyGameDataTarget(moduleBytes []byte, target gameDataTarget) gameDataVerifyResult {
	result := gameDataVerifyResult{Name: target.Name, Role: target.Role, ExpectedHits: target.ExpectedHits}
	if result.ExpectedHits == 0 {
		result.ExpectedHits = 1
	}
	rva, err := parseHexUint(target.RVA)
	if err != nil {
		result.Error = "invalid rva: " + err.Error()
		return result
	}
	result.RVA = rva
	pattern, err := hex.DecodeString(strings.TrimSpace(target.Pattern))
	if err != nil {
		result.Error = "invalid pattern: " + err.Error()
		return result
	}
	if len(pattern) != len(target.Mask) {
		result.Error = fmt.Sprintf("pattern length %d != mask length %d", len(pattern), len(target.Mask))
		return result
	}
	for _, ch := range target.Mask {
		if ch != 'x' && ch != '?' {
			result.Error = "mask may only contain x and ?"
			return result
		}
	}
	result.Hits = findMaskedPattern(moduleBytes, pattern, target.Mask)
	return result
}

func parseHexUint(s string) (uint64, error) {
	s = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(s)), "0x")
	return strconv.ParseUint(s, 16, 64)
}

func findMaskedPattern(data, pattern []byte, mask string) []int {
	if len(pattern) == 0 || len(pattern) != len(mask) || len(data) < len(pattern) {
		return nil
	}
	var hits []int
	for i := 0; i <= len(data)-len(pattern); i++ {
		ok := true
		for j := range pattern {
			if mask[j] == 'x' && data[i+j] != pattern[j] {
				ok = false
				break
			}
		}
		if ok {
			hits = append(hits, i)
		}
	}
	return hits
}

func writeGamedataVerifyReport(path, gameDir, modulePath, gamedataFile string, cfg gameDataConfig, results []gameDataVerifyResult) error {
	var b strings.Builder
	b.WriteString("# Gamedata Verify Report\r\n\r\n")
	b.WriteString(fmt.Sprintf("- Started: `%s`\r\n", time.Now().Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("- Game directory: `%s`\r\n", gameDir))
	b.WriteString(fmt.Sprintf("- Module: `%s`\r\n", modulePath))
	b.WriteString(fmt.Sprintf("- Gamedata: `%s`\r\n", gamedataFile))
	b.WriteString(fmt.Sprintf("- Source report: `%s`\r\n\r\n", cfg.SourceReport))
	b.WriteString("## Results\r\n\r\n")
	b.WriteString("| Target | RVA | Role | Expected | Hits | Status |\r\n")
	b.WriteString("|---|---:|---|---:|---:|---|\r\n")
	for _, result := range results {
		status := "OK"
		if result.Error != "" {
			status = "ERROR: " + result.Error
		} else if len(result.Hits) != result.ExpectedHits {
			status = "MISMATCH"
		}
		b.WriteString(fmt.Sprintf("| `%s` | `0x%X` | `%s` | `%d` | `%d` | `%s` |\r\n", escapeMD(result.Name), result.RVA, escapeMD(result.Role), result.ExpectedHits, len(result.Hits), escapeMD(status)))
	}
	b.WriteString("\r\n## Hit Offsets\r\n\r\n")
	for _, result := range results {
		b.WriteString(fmt.Sprintf("### %s\r\n\r\n", result.Name))
		if result.Error != "" {
			b.WriteString(fmt.Sprintf("- Error: `%s`\r\n\r\n", escapeMD(result.Error)))
			continue
		}
		if len(result.Hits) == 0 {
			b.WriteString("- No hits.\r\n\r\n")
			continue
		}
		for _, hit := range result.Hits {
			b.WriteString(fmt.Sprintf("- File offset: `0x%X`\r\n", hit))
		}
		b.WriteString("\r\n")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(b.String()), 0644)
}

func allGamedataOK(results []gameDataVerifyResult) bool {
	for _, result := range results {
		if result.Error != "" || len(result.Hits) != result.ExpectedHits {
			return false
		}
	}
	return true
}

func appendLog(line string) {
	firstLine, _, _ := procSendMessage.Call(app.logEdit, EM_GETFIRSTVISIBLELINE, 0, 0)
	app.mu.Lock()
	app.logs = append(app.logs, line)
	if len(app.logs) > 500 {
		app.logs = app.logs[len(app.logs)-500:]
	}
	text := strings.Join(app.logs, "\r\n")
	app.mu.Unlock()
	setText(app.logEdit, text)
	if firstLine > 0 {
		procSendMessage.Call(app.logEdit, EM_LINESCROLL, 0, firstLine)
	}
}

func clearLog() {
	app.mu.Lock()
	app.logs = nil
	app.events = nil
	app.mu.Unlock()
	setText(app.logEdit, "")
}

func saveManualIPs() {
	items := splitList(getText(app.ipEdit))
	persistIPList(items)
	appendLog(fmt.Sprintf("%s Saved %d manual IP/IP:port entries.", time.Now().Format("15:04:05"), len(items)))
}

func addRPGPreset() {
	preset := []string{
		"RPG",
		"rpg",
		"入口",
		"开服",
		"正在开启服务器",
		"正在创建游戏",
		"创建游戏",
		"创建服务器",
		"房间",
		"星缘天空",
		"破晓",
		"大厅",
		"排队",
		"挂机",
	}
	current := splitList(getText(app.keywordEdit))
	seen := map[string]bool{}
	for _, item := range current {
		seen[strings.ToLower(item)] = true
	}
	for _, item := range preset {
		if !seen[strings.ToLower(item)] {
			current = append(current, item)
			seen[strings.ToLower(item)] = true
		}
	}
	sort.Strings(current)
	setText(app.keywordEdit, strings.Join(current, ","))
	appendLog(time.Now().Format("15:04:05") + " Added RPG entry-server keyword preset.")
}

func fetchXYServers() {
	enable(app.fetchXYBtn, false)
	appendLog(time.Now().Format("15:04:05") + " Fetching XY server list from l4d2.xygamers.com...")
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		ips, err := downloadXYServers(ctx)
		if err != nil {
			postEvent("XY import failed: " + err.Error())
		} else {
			app.mu.Lock()
			app.pendingIPs = ips
			app.mu.Unlock()
			postEvent(fmt.Sprintf("XY import parsed %d endpoints.", len(ips)))
		}
		procPostMessage.Call(app.hwnd, WM_IMPORT_DONE, 0, 0)
	}()
}

func downloadXYServers(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://l4d2.xygamers.com/?page=1", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "L4D2GameDetailsFilterGUI/1.0")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("http status %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	return parseXYServerEndpoints(string(body)), nil
}

func parseXYServerEndpoints(page string) []string {
	re := regexp.MustCompile(`\["([0-9.]+)"\s*,\s*([0-9]+)\s*,\s*"main_list_[^"]+"\]`)
	seen := map[string]bool{}
	var out []string
	for _, match := range re.FindAllStringSubmatch(page, -1) {
		endpoint := net.JoinHostPort(match[1], match[2])
		if !seen[endpoint] {
			out = append(out, endpoint)
			seen[endpoint] = true
		}
	}
	sort.Strings(out)
	return out
}

func applyPendingIPs() {
	app.mu.Lock()
	ips := append([]string(nil), app.pendingIPs...)
	app.pendingIPs = nil
	running := app.running
	app.mu.Unlock()
	if !running {
		enable(app.fetchXYBtn, true)
	}
	if len(ips) == 0 {
		appendLog(time.Now().Format("15:04:05") + " XY import produced no endpoints.")
		return
	}
	added := mergeIPList(ips)
	appendLog(fmt.Sprintf("%s Imported %d new XY endpoints into IP / IP:port.", time.Now().Format("15:04:05"), added))
}

func importKnownEntryIPs() {
	known := []string{
		"114.66.17.54:27017",
		"103.28.54.214:27199",
	}
	added := mergeIPList(known)
	appendLog(fmt.Sprintf("%s Imported %d known entry endpoints into IP / IP:port.", time.Now().Format("15:04:05"), added))
}

func clearGameServerCache() {
	enable(app.cacheBtn, false)
	appendLog(time.Now().Format("15:04:05") + " Clearing server browser cache candidates...")
	go func() {
		moved, backupDir, errs := backupAndRemoveCacheFiles()
		for _, err := range errs {
			postEvent("Cache clear warning: " + err.Error())
		}
		if moved == 0 {
			postEvent("Cache clear finished: no known cache files found.")
		} else {
			postEvent(fmt.Sprintf("Cache clear finished: moved %d file(s) to %s.", moved, backupDir))
		}
		app.mu.Lock()
		running := app.running
		app.mu.Unlock()
		if !running {
			procPostMessage.Call(app.hwnd, WM_CACHE_DONE, 0, 0)
		}
	}()
}

func backupAndRemoveCacheFiles() (int, string, []error) {
	targets := findCacheFiles()
	backupDir := filepath.Join(toolDir(), "cache_backups", time.Now().Format("20060102_150405"))
	var errs []error
	moved := 0
	for _, src := range targets {
		if _, err := os.Stat(src); err != nil {
			continue
		}
		if err := os.MkdirAll(backupDir, 0755); err != nil {
			return moved, backupDir, append(errs, err)
		}
		dst := filepath.Join(backupDir, safeBackupName(src))
		if err := os.Rename(src, dst); err != nil {
			data, readErr := os.ReadFile(src)
			if readErr != nil {
				errs = append(errs, fmt.Errorf("%s: %w", src, err))
				continue
			}
			if writeErr := os.WriteFile(dst, data, 0644); writeErr != nil {
				errs = append(errs, fmt.Errorf("%s: %w", dst, writeErr))
				continue
			}
			if removeErr := os.Remove(src); removeErr != nil {
				errs = append(errs, fmt.Errorf("%s: %w", src, removeErr))
				continue
			}
		}
		moved++
	}
	return moved, backupDir, errs
}

func findCacheFiles() []string {
	patterns := []string{
		`D:\Program Files (x86)\Steam\config\serverbrowser.vdf`,
		`D:\Program Files (x86)\Steam\userdata\*\7\remote\serverbrowser_hist.vdf`,
		`D:\Program Files (x86)\Steam\userdata\*\7\remote\serverbrowser.vdf`,
		`E:\SteamLibrary\steamapps\common\Left 4 Dead 2\platform\config\serverbrowser.vdf`,
		`E:\SteamLibrary\steamapps\common\Left 4 Dead 2\left4dead2\cfg\serverbrowser.vdf`,
	}
	seen := map[string]bool{}
	var out []string
	for _, pattern := range patterns {
		matches, _ := filepath.Glob(pattern)
		for _, item := range matches {
			if !seen[item] {
				seen[item] = true
				out = append(out, item)
			}
		}
	}
	sort.Strings(out)
	return out
}

func safeBackupName(path string) string {
	replacer := strings.NewReplacer(`:`, `_`, `\`, `_`, `/`, `_`, " ", "_")
	return replacer.Replace(path)
}

func mergeIPList(items []string) int {
	current := splitList(getText(app.ipEdit))
	seen := map[string]bool{}
	for _, item := range current {
		seen[item] = true
	}
	added := 0
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		current = append(current, item)
		seen[item] = true
		added++
	}
	sort.Strings(current)
	setText(app.ipEdit, strings.Join(current, ","))
	persistIPList(current)
	return added
}

func persistCurrentIPList() {
	if app.ipEdit == 0 {
		return
	}
	persistIPList(splitList(getText(app.ipEdit)))
}

func persistIPList(items []string) {
	app.mu.Lock()
	if app.hitConfig.Version == 0 {
		app.hitConfig.Version = 1
	}
	app.hitConfig.ManualIPs = append([]string(nil), items...)
	app.hitConfig.UpdatedAt = time.Now().Format(time.RFC3339)
	app.hitDirty = true
	app.mu.Unlock()
	cfg, dirty := snapshotHitConfig(false)
	if dirty {
		_ = saveHitConfig(cfg)
	}
}

func loadHitConfig() {
	path := hitConfigFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		app.hitConfig = hitConfig{Version: 1, Entries: map[string]*hitRecord{}}
		return
	}
	var cfg hitConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		app.hitConfig = hitConfig{Version: 1, Entries: map[string]*hitRecord{}}
		return
	}
	if cfg.Version == 0 {
		cfg.Version = 1
	}
	if cfg.Entries == nil {
		cfg.Entries = map[string]*hitRecord{}
	}
	cfg.ManualIPs = normalizeList(cfg.ManualIPs)
	app.hitConfig = cfg
}

func saveHitConfig(cfg hitConfig) error {
	if cfg.Version == 0 {
		cfg.Version = 1
	}
	if cfg.Entries == nil {
		cfg.Entries = map[string]*hitRecord{}
	}
	cfg.ManualIPs = normalizeList(cfg.ManualIPs)
	cfg.UpdatedAt = time.Now().Format(time.RFC3339)
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := writeFileAtomic(hitConfigFilePath(), append(data, '\n'), 0644); err != nil {
		return err
	}
	return exportLearnedConnectstrings(cfg)
}

func exportLearnedConnectstrings(cfg hitConfig) error {
	items := map[string]string{}
	for _, item := range cfg.ManualIPs {
		addLearnedFilter(items, item, "manual_ip")
	}

	for _, rec := range cfg.Entries {
		if rec == nil || !hitRecordMatchesBlockedIdentity(rec) {
			continue
		}
		for _, endpoint := range []string{rec.Endpoint, rec.Source, rec.LastOnline, rec.LastLocal} {
			host := endpointHostOrSelf(endpoint)
			if host == "" {
				continue
			}
			addLearnedFilter(items, host, "learned_identity")
		}
	}

	var keys []string
	for item := range items {
		keys = append(keys, item)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString("# Auto-generated by L4D2GameDetailsFilterGUI from matched_entries.json.\r\n")
	b.WriteString("# The row-filter DLL reads this file at inject time.\r\n")
	b.WriteString("# Edit blocked_connectstrings.txt for permanent manual rules.\r\n\r\n")
	for _, item := range keys {
		if reason := items[item]; reason != "" {
			b.WriteString("# ")
			b.WriteString(reason)
			b.WriteString("\r\n")
		}
		b.WriteString(item)
		b.WriteString("\r\n")
	}

	path := learnedConnectstringsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return writeFileAtomic(path, []byte(b.String()), 0644)
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp.")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	oldName, err := syscall.UTF16PtrFromString(tmpName)
	if err != nil {
		return err
	}
	newName, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	flags := uintptr(moveFileReplaceExisting | moveFileWriteThrough)
	if ret, _, callErr := procMoveFileEx.Call(uintptr(unsafe.Pointer(oldName)), uintptr(unsafe.Pointer(newName)), flags); ret == 0 {
		return callErr
	}
	cleanup = false
	return nil
}

func hitRecordMatchesBlockedIdentity(rec *hitRecord) bool {
	var values []string
	values = append(values, rec.LastName, rec.LastReason, rec.LastTags)
	for _, value := range rec.RawFields {
		values = append(values, value)
	}
	joined := strings.ToLower(strings.Join(values, "\n"))
	for _, keyword := range learnedIdentityKeywords() {
		if strings.Contains(joined, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}

func learnedIdentityKeywords() []string {
	return []string{
		"rpg",
		"星缘",
		"鏄熺紭",
		"破晓",
		"鐮存檽",
		"杀戮",
		"神域",
	}
}

func addLearnedFilter(items map[string]string, value, reason string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	if _, exists := items[value]; !exists {
		items[value] = reason
	}
}

func endpointHostOrSelf(endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(endpoint); err == nil {
		return strings.Trim(host, "[]")
	}
	if strings.Count(endpoint, ":") == 1 {
		host, _, ok := strings.Cut(endpoint, ":")
		if ok {
			return strings.TrimSpace(host)
		}
	}
	return endpoint
}

func normalizeList(items []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

func hitConfigFilePath() string {
	return filepath.Join(toolDir(), "matched_entries.json")
}

func learnedConnectstringsPath() string {
	return filepath.Clean(filepath.Join(toolDir(), "..", "matchmaking_row_filter_dll", "learned_connectstrings.txt"))
}

func gamedataPath() string {
	return filepath.Clean(filepath.Join(toolDir(), "..", "..", "gamedata", "matchmaking_targets.json"))
}

func gamedataVerifyReportPath() string {
	return filepath.Clean(filepath.Join(toolDir(), "..", "..", "reports", "gamedata_verify_combined_report.md"))
}

func toolDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

func layout() {
	if app.keywordEdit == 0 {
		return
	}
	var rc rect
	procGetClientRect.Call(app.hwnd, uintptr(unsafe.Pointer(&rc)))
	width := rc.Right - rc.Left
	height := rc.Bottom - rc.Top
	if width < 760 {
		width = 740
	}
	if height < 520 {
		height = 520
	}
	procMoveWindow.Call(app.ruleGroup, 14, 10, uintptr(width-28), 132, 1)
	procMoveWindow.Call(app.keywordEdit, 120, 34, uintptr(width-330), 28, 1)
	procMoveWindow.Call(app.ipEdit, 120, 70, uintptr(width-420), 28, 1)
	procMoveWindow.Call(app.saveIPBtn, uintptr(width-292), 70, 98, 28, 1)
	procMoveWindow.Call(app.dryRun, uintptr(width-186), 28, 170, 24, 1)
	procMoveWindow.Call(app.verbose, uintptr(width-186), 58, 170, 24, 1)
	procMoveWindow.Call(app.blockEp, uintptr(width-186), 88, 170, 24, 1)
	procMoveWindow.Call(app.startBtn, 30, 106, 82, 30, 1)
	procMoveWindow.Call(app.stopBtn, 122, 106, 70, 30, 1)
	procMoveWindow.Call(app.clearBtn, 202, 106, 86, 30, 1)
	procMoveWindow.Call(app.presetBtn, 298, 106, 98, 30, 1)
	procMoveWindow.Call(app.fetchXYBtn, 406, 106, 76, 30, 1)
	procMoveWindow.Call(app.knownIPBtn, 492, 106, 90, 30, 1)
	procMoveWindow.Call(app.cacheBtn, 592, 106, 76, 30, 1)
	procMoveWindow.Call(app.hintText, 30, 142, uintptr(width-60), 1, 1)
	procMoveWindow.Call(app.statusGroup, 14, 150, uintptr(width-28), 110, 1)
	procMoveWindow.Call(app.status, 30, 176, uintptr(width-60), 22, 1)
	procMoveWindow.Call(app.stats, 30, 202, uintptr(width-60), 22, 1)
	procMoveWindow.Call(app.verifyBtn, 30, 226, 116, 28, 1)
	procMoveWindow.Call(app.reportBtn, 156, 226, 112, 28, 1)
	procMoveWindow.Call(app.logLabel, 16, 270, 80, 22, 1)
	procMoveWindow.Call(app.logEdit, 16, 296, uintptr(width-32), uintptr(height-326), 1)
}

func splitList(text string) []string {
	var out []string
	for _, item := range strings.Split(text, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	sort.Strings(out)
	return out
}

func openPath(path string) {
	if strings.TrimSpace(path) == "" {
		return
	}
	procShellExecute.Call(app.hwnd, uintptr(unsafe.Pointer(utf16Ptr("open"))), uintptr(unsafe.Pointer(utf16Ptr(path))), 0, 0, SW_SHOWNORMAL)
}

func escapeMD(s string) string {
	s = strings.ReplaceAll(s, "`", "'")
	s = strings.ReplaceAll(s, "|", "\\|")
	return s
}

func stripPort(endpoint string) string {
	host, _, err := net.SplitHostPort(endpoint)
	if err != nil {
		return endpoint
	}
	return host
}

func isBinaryKVPayload(payload []byte) bool {
	return len(payload) >= 17 && payload[0] == 0xff && payload[1] == 0xff && payload[2] == 0xff && payload[3] == 0xff && payload[4] == 0x00
}

func readCString(data []byte, offset *int) (string, bool) {
	start := *offset
	for *offset < len(data) && data[*offset] != 0 {
		*offset = *offset + 1
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
	n, _, _ := procMultiByteToWideChar.Call(cpGBK, 0, uintptr(unsafe.Pointer(&raw[0])), uintptr(len(raw)), 0, 0)
	if n == 0 {
		return ""
	}
	buf := make([]uint16, n)
	ret, _, _ := procMultiByteToWideChar.Call(cpGBK, 0, uintptr(unsafe.Pointer(&raw[0])), uintptr(len(raw)), uintptr(unsafe.Pointer(&buf[0])), n)
	if ret == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf)
}

func initTheme() {
	theme = darkTheme{
		bgColor:    rgb(22, 25, 29),
		panelColor: rgb(32, 37, 43),
		editColor:  rgb(12, 15, 18),
		textColor:  rgb(232, 238, 244),
		mutedColor: rgb(164, 176, 188),
	}
	theme.bgBrush = createSolidBrush(theme.bgColor)
	theme.panelBrush = createSolidBrush(theme.panelColor)
	theme.editBrush = createSolidBrush(theme.editColor)
}

func deleteTheme() {
	for _, brush := range []uintptr{theme.bgBrush, theme.panelBrush, theme.editBrush} {
		if brush != 0 {
			procDeleteObject.Call(brush)
		}
	}
}

func applyControlColors(hdc uintptr, textColor, bgColor uint32) {
	procSetTextColor.Call(hdc, uintptr(textColor))
	procSetBkColor.Call(hdc, uintptr(bgColor))
}

func createSolidBrush(color uint32) uintptr {
	ret, _, _ := procCreateSolidBrush.Call(uintptr(color))
	return ret
}

func rgb(r, g, b byte) uint32 {
	return uint32(r) | uint32(g)<<8 | uint32(b)<<16
}

func createChild(exStyle uint32, className, text string, style uint32, x, y, w, h int32, parent uintptr, id uintptr) uintptr {
	ctrl := createWindowEx(exStyle, className, text, WS_CHILD|WS_VISIBLE|style, x, y, w, h, parent, id, 0)
	if app.font != 0 {
		procSendMessage.Call(ctrl, WM_SETFONT, app.font, 1)
	}
	return ctrl
}

func createWindowEx(exStyle uint32, className, title string, style uint32, x, y, w, h int32, parent, menu, instance uintptr) uintptr {
	ret, _, _ := procCreateWindowEx.Call(uintptr(exStyle), uintptr(unsafe.Pointer(utf16Ptr(className))), uintptr(unsafe.Pointer(utf16Ptr(title))), uintptr(style), uintptr(x), uintptr(y), uintptr(w), uintptr(h), parent, menu, instance, 0)
	return ret
}

func getText(hwnd uintptr) string {
	n, _, _ := procSendMessage.Call(hwnd, WM_GETTEXTLENGTH, 0, 0)
	buf := make([]uint16, n+1)
	procSendMessage.Call(hwnd, WM_GETTEXT, n+1, uintptr(unsafe.Pointer(&buf[0])))
	return syscall.UTF16ToString(buf)
}

func setText(hwnd uintptr, text string) {
	procSetWindowText.Call(hwnd, uintptr(unsafe.Pointer(utf16Ptr(text))))
}

func isChecked(hwnd uintptr) bool {
	ret, _, _ := procSendMessage.Call(hwnd, BM_GETCHECK, 0, 0)
	return ret == BST_CHECKED
}

func enable(hwnd uintptr, enabled bool) {
	value := uintptr(0)
	if enabled {
		value = 1
	}
	procEnableWindow.Call(hwnd, value)
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
