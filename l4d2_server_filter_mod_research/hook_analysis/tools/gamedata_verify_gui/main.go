package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

const (
	WS_OVERLAPPEDWINDOW = 0x00CF0000
	WS_VISIBLE          = 0x10000000
	WS_CHILD            = 0x40000000
	WS_TABSTOP          = 0x00010000
	WS_BORDER           = 0x00800000
	WS_VSCROLL          = 0x00200000
	WS_EX_CLIENTEDGE    = 0x00000200

	ES_MULTILINE   = 0x0004
	ES_AUTOVSCROLL = 0x0040
	ES_READONLY    = 0x0800
	ES_AUTOHSCROLL = 0x0080
	BS_PUSHBUTTON  = 0x00000000
	BS_GROUPBOX    = 0x00000007
	SS_LEFT        = 0x00000000

	SW_SHOW       = 5
	SW_SHOWNORMAL = 1

	WM_CREATE         = 0x0001
	WM_DESTROY        = 0x0002
	WM_SIZE           = 0x0005
	WM_COMMAND        = 0x0111
	WM_SETICON        = 0x0080
	WM_SETFONT        = 0x0030
	WM_GETTEXT        = 0x000D
	WM_GETTEXTLENGTH  = 0x000E
	WM_CTLCOLOREDIT   = 0x0133
	WM_CTLCOLORBTN    = 0x0135
	WM_CTLCOLORSTATIC = 0x0138
	WM_ERASEBKGND     = 0x0014
	WM_APP            = 0x8000

	ICON_SMALL     = 0
	ICON_BIG       = 1
	IMAGE_ICON     = 1
	LR_DEFAULTSIZE = 0x00000040

	ID_VERIFY      = 1001
	ID_OPEN_REPORT = 1002
	ID_OPEN_FOLDER = 1003
	ID_CLEAR       = 1004

	WM_VERIFY_DONE = WM_APP + 1
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")

	procDefWindowProc    = user32.NewProc("DefWindowProcW")
	procDispatchMessage  = user32.NewProc("DispatchMessageW")
	procGetMessage       = user32.NewProc("GetMessageW")
	procLoadIcon         = user32.NewProc("LoadIconW")
	procLoadImage        = user32.NewProc("LoadImageW")
	procLoadCursor       = user32.NewProc("LoadCursorW")
	procRegisterClassEx  = user32.NewProc("RegisterClassExW")
	procCreateWindowEx   = user32.NewProc("CreateWindowExW")
	procShowWindow       = user32.NewProc("ShowWindow")
	procUpdateWindow     = user32.NewProc("UpdateWindow")
	procTranslateMessage = user32.NewProc("TranslateMessage")
	procSendMessage      = user32.NewProc("SendMessageW")
	procPostMessage      = user32.NewProc("PostMessageW")
	procPostQuitMessage  = user32.NewProc("PostQuitMessage")
	procSetWindowText    = user32.NewProc("SetWindowTextW")
	procGetClientRect    = user32.NewProc("GetClientRect")
	procMoveWindow       = user32.NewProc("MoveWindow")
	procEnableWindow     = user32.NewProc("EnableWindow")
	procFillRect         = user32.NewProc("FillRect")
	procMessageBox       = user32.NewProc("MessageBoxW")

	procGetModuleHandle  = kernel32.NewProc("GetModuleHandleW")
	procGetStockObject   = gdi32.NewProc("GetStockObject")
	procCreateSolidBrush = gdi32.NewProc("CreateSolidBrush")
	procDeleteObject     = gdi32.NewProc("DeleteObject")
	procSetTextColor     = gdi32.NewProc("SetTextColor")
	procSetBkColor       = gdi32.NewProc("SetBkColor")

	procShellExecute = shell32.NewProc("ShellExecuteW")
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

type theme struct {
	bgBrush    uintptr
	panelBrush uintptr
	editBrush  uintptr
	bg         uint32
	panel      uint32
	edit       uint32
	text       uint32
	muted      uint32
}

type appState struct {
	hwnd uintptr
	font uintptr

	gameEdit     uintptr
	gamedataEdit uintptr
	reportEdit   uintptr
	statusText   uintptr
	logEdit      uintptr
	verifyBtn    uintptr
	openBtn      uintptr
	folderBtn    uintptr
	clearBtn     uintptr

	running    bool
	reportPath string
	lastLog    string
	mu         sync.Mutex
}

type GameData struct {
	Schema       int      `json:"schema"`
	Game         string   `json:"game"`
	Module       string   `json:"module"`
	ModuleName   string   `json:"module_name"`
	ImageBase    string   `json:"image_base"`
	SourceReport string   `json:"source_report"`
	Targets      []Target `json:"targets"`
}

type Target struct {
	Name         string `json:"name"`
	RVA          string `json:"rva"`
	Role         string `json:"role"`
	Pattern      string `json:"pattern"`
	Mask         string `json:"mask"`
	ExpectedHits int    `json:"expected_hits"`
}

type Result struct {
	Name         string
	Role         string
	RVA          uint64
	ExpectedHits int
	Hits         []int
	Error        string
}

var (
	app     appState
	uiTheme theme
)

func main() {
	runtime.LockOSThread()
	initTheme()
	defer destroyTheme()
	runGUI()
}

func runGUI() {
	hInstance, _, _ := procGetModuleHandle.Call(0)
	className := utf16Ptr("L4D2GamedataVerifyGUI")
	appIcon := loadAppIcon(hInstance)
	wndClass := wndclassex{
		Size:       uint32(unsafe.Sizeof(wndclassex{})),
		WndProc:    syscall.NewCallback(wndProc),
		Instance:   hInstance,
		Icon:       appIcon,
		Cursor:     loadCursor(32512),
		Background: uiTheme.bgBrush,
		ClassName:  className,
		IconSm:     appIcon,
	}
	procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wndClass)))

	app.hwnd = createWindowEx(0, "L4D2GamedataVerifyGUI", "L4D2 Gamedata Verify", WS_OVERLAPPEDWINDOW|WS_VISIBLE, 180, 140, 980, 620, 0, 0, hInstance)
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

func wndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case WM_CREATE:
		createControls(hwnd)
		return 0
	case WM_SIZE:
		layoutControls(hwnd)
		return 0
	case WM_COMMAND:
		handleCommand(uint16(wParam & 0xffff))
		return 0
	case WM_VERIFY_DONE:
		onVerifyDone()
		return 0
	case WM_CTLCOLOREDIT:
		setTextColors(wParam, uiTheme.text, uiTheme.edit)
		return uiTheme.editBrush
	case WM_CTLCOLORSTATIC, WM_CTLCOLORBTN:
		setTextColors(wParam, uiTheme.text, uiTheme.panel)
		return uiTheme.panelBrush
	case WM_ERASEBKGND:
		var rc rect
		procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
		procFillRect.Call(wParam, uintptr(unsafe.Pointer(&rc)), uiTheme.bgBrush)
		return 1
	case WM_DESTROY:
		procPostQuitMessage.Call(0)
		return 0
	}
	ret, _, _ := procDefWindowProc.Call(hwnd, uintptr(msg), wParam, lParam)
	return ret
}

func createControls(hwnd uintptr) {
	app.font, _, _ = procGetStockObject.Call(17)
	createChild(0, "STATIC", "Game directory", SS_LEFT, 20, 18, 120, 22, hwnd, 0)
	app.gameEdit = createChild(WS_EX_CLIENTEDGE, "EDIT", `E:\SteamLibrary\steamapps\common\Left 4 Dead 2`, WS_BORDER|ES_AUTOHSCROLL|WS_TABSTOP, 150, 14, 620, 26, hwnd, 0)
	app.verifyBtn = createChild(0, "BUTTON", "Verify", BS_PUSHBUTTON|WS_TABSTOP, 790, 12, 82, 30, hwnd, ID_VERIFY)
	app.openBtn = createChild(0, "BUTTON", "Report", BS_PUSHBUTTON|WS_TABSTOP, 880, 12, 82, 30, hwnd, ID_OPEN_REPORT)

	createChild(0, "STATIC", "Gamedata", SS_LEFT, 20, 54, 120, 22, hwnd, 0)
	app.gamedataEdit = createChild(WS_EX_CLIENTEDGE, "EDIT", defaultGamedataPath(), WS_BORDER|ES_AUTOHSCROLL|WS_TABSTOP, 150, 50, 620, 26, hwnd, 0)
	app.folderBtn = createChild(0, "BUTTON", "Folder", BS_PUSHBUTTON|WS_TABSTOP, 790, 48, 82, 30, hwnd, ID_OPEN_FOLDER)
	app.clearBtn = createChild(0, "BUTTON", "Clear", BS_PUSHBUTTON|WS_TABSTOP, 880, 48, 82, 30, hwnd, ID_CLEAR)

	createChild(0, "STATIC", "Report path", SS_LEFT, 20, 90, 120, 22, hwnd, 0)
	app.reportEdit = createChild(WS_EX_CLIENTEDGE, "EDIT", defaultReportPath(), WS_BORDER|ES_AUTOHSCROLL|WS_TABSTOP, 150, 86, 812, 26, hwnd, 0)

	app.statusText = createChild(0, "STATIC", "Idle", SS_LEFT, 20, 126, 940, 24, hwnd, 0)
	app.logEdit = createChild(WS_EX_CLIENTEDGE, "EDIT", "", WS_BORDER|ES_MULTILINE|ES_AUTOVSCROLL|ES_READONLY|WS_VSCROLL, 20, 156, 942, 400, hwnd, 0)
	setFontAll()
	appendLog("Ready. Click Verify to check the current matchmaking.dll signatures.")
}

func setFontAll() {
	for _, h := range []uintptr{app.gameEdit, app.gamedataEdit, app.reportEdit, app.statusText, app.logEdit, app.verifyBtn, app.openBtn, app.folderBtn, app.clearBtn} {
		if h != 0 {
			procSendMessage.Call(h, WM_SETFONT, app.font, 1)
		}
	}
}

func layoutControls(hwnd uintptr) {
	var rc rect
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	w := rc.Right - rc.Left
	h := rc.Bottom - rc.Top
	if w < 760 {
		w = 760
	}
	if h < 480 {
		h = 480
	}
	editW := w - 360
	procMoveWindow.Call(app.gameEdit, 150, 14, uintptr(editW), 26, 1)
	procMoveWindow.Call(app.verifyBtn, uintptr(w-190), 12, 82, 30, 1)
	procMoveWindow.Call(app.openBtn, uintptr(w-100), 12, 82, 30, 1)
	procMoveWindow.Call(app.gamedataEdit, 150, 50, uintptr(editW), 26, 1)
	procMoveWindow.Call(app.folderBtn, uintptr(w-190), 48, 82, 30, 1)
	procMoveWindow.Call(app.clearBtn, uintptr(w-100), 48, 82, 30, 1)
	procMoveWindow.Call(app.reportEdit, 150, 86, uintptr(w-168), 26, 1)
	procMoveWindow.Call(app.statusText, 20, 126, uintptr(w-40), 24, 1)
	procMoveWindow.Call(app.logEdit, 20, 156, uintptr(w-40), uintptr(h-176), 1)
}

func handleCommand(id uint16) {
	switch id {
	case ID_VERIFY:
		startVerify()
	case ID_OPEN_REPORT:
		openPath(readText(app.reportEdit))
	case ID_OPEN_FOLDER:
		openPath(filepath.Dir(readText(app.reportEdit)))
	case ID_CLEAR:
		setText(app.logEdit, "")
	}
}

func startVerify() {
	app.mu.Lock()
	if app.running {
		app.mu.Unlock()
		return
	}
	app.running = true
	app.lastLog = ""
	app.mu.Unlock()
	enable(app.verifyBtn, false)
	setText(app.statusText, "Running verification...")
	setText(app.logEdit, "")

	gameDir := readText(app.gameEdit)
	gamedataPath := readText(app.gamedataEdit)
	reportPath := readText(app.reportEdit)
	appendLog("Game: " + gameDir)
	appendLog("Gamedata: " + gamedataPath)
	appendLog("Report: " + reportPath)

	go func() {
		log, err := runVerification(gameDir, gamedataPath, reportPath)
		if err != nil {
			log += "\r\nERROR: " + err.Error()
		}
		app.mu.Lock()
		app.lastLog = log
		app.reportPath = reportPath
		app.running = false
		app.mu.Unlock()
		procPostMessage.Call(app.hwnd, WM_VERIFY_DONE, 0, 0)
	}()
}

func onVerifyDone() {
	app.mu.Lock()
	log := app.lastLog
	app.mu.Unlock()
	appendLog(log)
	if strings.Contains(log, "MISMATCH") || strings.Contains(log, "ERROR:") {
		setText(app.statusText, "Verification failed or has mismatches. Check the report.")
	} else {
		setText(app.statusText, "Verification OK. All configured targets are stable.")
	}
	enable(app.verifyBtn, true)
}

func runVerification(gameDir, gamedataPath, reportPath string) (string, error) {
	cfg, err := loadGameData(gamedataPath)
	if err != nil {
		return "", err
	}
	modulePath := filepath.Join(gameDir, filepath.FromSlash(cfg.Module))
	moduleBytes, err := os.ReadFile(modulePath)
	if err != nil {
		return "", err
	}
	var results []Result
	for _, target := range cfg.Targets {
		results = append(results, verifyTarget(moduleBytes, target))
	}
	if err := writeReport(reportPath, gameDir, modulePath, gamedataPath, cfg, results); err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("Verification finished.\r\n\r\n")
	for _, r := range results {
		status := "OK"
		if r.Error != "" {
			status = "ERROR: " + r.Error
		} else if len(r.Hits) != r.ExpectedHits {
			status = "MISMATCH"
		}
		b.WriteString(fmt.Sprintf("%-28s expected=%d hits=%d status=%s\r\n", r.Name, r.ExpectedHits, len(r.Hits), status))
	}
	b.WriteString("\r\nReport written: " + reportPath)
	if !allOK(results) {
		return b.String(), fmt.Errorf("one or more targets failed verification")
	}
	return b.String(), nil
}

func loadGameData(path string) (GameData, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return GameData{}, err
	}
	var cfg GameData
	if err := json.Unmarshal(data, &cfg); err != nil {
		return GameData{}, err
	}
	if cfg.Module == "" || len(cfg.Targets) == 0 {
		return GameData{}, fmt.Errorf("missing module or targets")
	}
	return cfg, nil
}

func verifyTarget(moduleBytes []byte, target Target) Result {
	result := Result{Name: target.Name, Role: target.Role, ExpectedHits: target.ExpectedHits}
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
	result.Hits = findPattern(moduleBytes, pattern, target.Mask)
	return result
}

func parseHexUint(s string) (uint64, error) {
	s = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(s)), "0x")
	return strconv.ParseUint(s, 16, 64)
}

func findPattern(data, pattern []byte, mask string) []int {
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

func writeReport(path, gameDir, modulePath, gamedataPath string, cfg GameData, results []Result) error {
	var b strings.Builder
	b.WriteString("# Gamedata Verify GUI Report\r\n\r\n")
	b.WriteString(fmt.Sprintf("- Started: `%s`\r\n", time.Now().Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("- Game directory: `%s`\r\n", gameDir))
	b.WriteString(fmt.Sprintf("- Module: `%s`\r\n", modulePath))
	b.WriteString(fmt.Sprintf("- Gamedata: `%s`\r\n", gamedataPath))
	b.WriteString(fmt.Sprintf("- Source report: `%s`\r\n\r\n", cfg.SourceReport))
	b.WriteString("## Results\r\n\r\n")
	b.WriteString("| Target | RVA | Role | Expected | Hits | Status |\r\n")
	b.WriteString("|---|---:|---|---:|---:|---|\r\n")
	for _, r := range results {
		status := "OK"
		if r.Error != "" {
			status = "ERROR: " + r.Error
		} else if len(r.Hits) != r.ExpectedHits {
			status = "MISMATCH"
		}
		b.WriteString(fmt.Sprintf("| `%s` | `0x%X` | `%s` | `%d` | `%d` | `%s` |\r\n",
			escapeMD(r.Name), r.RVA, escapeMD(r.Role), r.ExpectedHits, len(r.Hits), escapeMD(status)))
	}
	b.WriteString("\r\n## Hit Offsets\r\n\r\n")
	for _, r := range results {
		b.WriteString(fmt.Sprintf("### %s\r\n\r\n", r.Name))
		if r.Error != "" {
			b.WriteString(fmt.Sprintf("- Error: `%s`\r\n\r\n", escapeMD(r.Error)))
			continue
		}
		if len(r.Hits) == 0 {
			b.WriteString("- No hits.\r\n\r\n")
			continue
		}
		for _, hit := range r.Hits {
			b.WriteString(fmt.Sprintf("- File offset: `0x%X`\r\n", hit))
		}
		b.WriteString("\r\n")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(b.String()), 0644)
}

func allOK(results []Result) bool {
	for _, r := range results {
		if r.Error != "" || len(r.Hits) != r.ExpectedHits {
			return false
		}
	}
	return true
}

func initTheme() {
	uiTheme = theme{
		bg:    rgb(18, 22, 28),
		panel: rgb(31, 37, 46),
		edit:  rgb(13, 17, 23),
		text:  rgb(224, 231, 242),
		muted: rgb(148, 163, 184),
	}
	uiTheme.bgBrush = createBrush(uiTheme.bg)
	uiTheme.panelBrush = createBrush(uiTheme.panel)
	uiTheme.editBrush = createBrush(uiTheme.edit)
}

func destroyTheme() {
	for _, h := range []uintptr{uiTheme.bgBrush, uiTheme.panelBrush, uiTheme.editBrush} {
		if h != 0 {
			procDeleteObject.Call(h)
		}
	}
}

func createBrush(color uint32) uintptr {
	ret, _, _ := procCreateSolidBrush.Call(uintptr(color))
	return ret
}

func rgb(r, g, b byte) uint32 {
	return uint32(r) | uint32(g)<<8 | uint32(b)<<16
}

func setTextColors(hdc uintptr, text, bg uint32) {
	procSetTextColor.Call(hdc, uintptr(text))
	procSetBkColor.Call(hdc, uintptr(bg))
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

func loadCursor(id uintptr) uintptr {
	ret, _, _ := procLoadCursor.Call(0, id)
	return ret
}

func enable(hwnd uintptr, enabled bool) {
	v := uintptr(0)
	if enabled {
		v = 1
	}
	procEnableWindow.Call(hwnd, v)
}

func setText(hwnd uintptr, text string) {
	procSetWindowText.Call(hwnd, uintptr(unsafe.Pointer(utf16Ptr(text))))
}

func readText(hwnd uintptr) string {
	n, _, _ := procSendMessage.Call(hwnd, WM_GETTEXTLENGTH, 0, 0)
	buf := make([]uint16, n+1)
	procSendMessage.Call(hwnd, WM_GETTEXT, n+1, uintptr(unsafe.Pointer(&buf[0])))
	return syscall.UTF16ToString(buf)
}

func appendLog(text string) {
	current := readText(app.logEdit)
	if current != "" && !strings.HasSuffix(current, "\r\n") {
		current += "\r\n"
	}
	text = strings.ReplaceAll(text, "\n", "\r\n")
	if len(current)+len(text) > 20000 {
		current = current[len(current)/2:]
	}
	setText(app.logEdit, current+text+"\r\n")
}

func openPath(path string) {
	if strings.TrimSpace(path) == "" {
		return
	}
	procShellExecute.Call(app.hwnd, uintptr(unsafe.Pointer(utf16Ptr("open"))), uintptr(unsafe.Pointer(utf16Ptr(path))), 0, 0, SW_SHOWNORMAL)
}

func defaultGamedataPath() string {
	return filepath.Clean(filepath.Join("..", "..", "gamedata", "matchmaking_targets.json"))
}

func defaultReportPath() string {
	return filepath.Clean(filepath.Join("..", "..", "reports", "gamedata_verify_gui_report.md"))
}

func escapeMD(s string) string {
	s = strings.ReplaceAll(s, "`", "'")
	s = strings.ReplaceAll(s, "|", "\\|")
	return s
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
