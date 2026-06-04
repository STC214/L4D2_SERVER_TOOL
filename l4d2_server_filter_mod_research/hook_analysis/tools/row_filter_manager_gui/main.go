package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	WS_HSCROLL          = 0x00100000
	WS_EX_CLIENTEDGE    = 0x00000200
	ES_MULTILINE        = 0x0004
	ES_AUTOVSCROLL      = 0x0040
	ES_AUTOHSCROLL      = 0x0080
	ES_READONLY         = 0x0800
	BS_PUSHBUTTON       = 0x00000000
	BS_GROUPBOX         = 0x00000007
	SS_LEFT             = 0x00000000

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
	WM_CTLCOLORSTATIC = 0x0138
	WM_CTLCOLORBTN    = 0x0135
	WM_APP            = 0x8000

	ICON_SMALL     = 0
	ICON_BIG       = 1
	IMAGE_ICON     = 1
	LR_DEFAULTSIZE = 0x00000040

	ID_RESTORE = 1001
	ID_SAVE    = 1002
	ID_START   = 1003
	ID_INJECT  = 1004
	ID_REFRESH = 1005
	ID_OPENLOG = 1006
	ID_FOLDER  = 1007
	ID_ADD_IP  = 1008
	ID_ADD_KEY = 1009
	ID_TIMER   = 2001

	WM_APP_EVENT = WM_APP + 1

	MOVEFILE_REPLACE_EXISTING = 0x00000001
	MOVEFILE_WRITE_THROUGH    = 0x00000008
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")

	procDefWindowProc    = user32.NewProc("DefWindowProcW")
	procDispatchMessage  = user32.NewProc("DispatchMessageW")
	procGetMessage       = user32.NewProc("GetMessageW")
	procLoadCursor       = user32.NewProc("LoadCursorW")
	procLoadIcon         = user32.NewProc("LoadIconW")
	procLoadImage        = user32.NewProc("LoadImageW")
	procRegisterClassEx  = user32.NewProc("RegisterClassExW")
	procCreateWindowEx   = user32.NewProc("CreateWindowExW")
	procShowWindow       = user32.NewProc("ShowWindow")
	procUpdateWindow     = user32.NewProc("UpdateWindow")
	procTranslateMessage = user32.NewProc("TranslateMessage")
	procSendMessage      = user32.NewProc("SendMessageW")
	procPostMessage      = user32.NewProc("PostMessageW")
	procPostQuitMessage  = user32.NewProc("PostQuitMessage")
	procSetWindowText    = user32.NewProc("SetWindowTextW")
	procSetTimer         = user32.NewProc("SetTimer")
	procKillTimer        = user32.NewProc("KillTimer")
	procGetClientRect    = user32.NewProc("GetClientRect")
	procMoveWindow       = user32.NewProc("MoveWindow")
	procEnableWindow     = user32.NewProc("EnableWindow")
	procMessageBox       = user32.NewProc("MessageBoxW")
	procFillRect         = user32.NewProc("FillRect")
	procIsUserAnAdmin    = shell32.NewProc("IsUserAnAdmin")
	procShellExecute     = shell32.NewProc("ShellExecuteW")
	procGetModuleHandle  = kernel32.NewProc("GetModuleHandleW")
	procMoveFileEx       = kernel32.NewProc("MoveFileExW")
	procGetStockObject   = gdi32.NewProc("GetStockObject")
	procCreateSolidBrush = gdi32.NewProc("CreateSolidBrush")
	procDeleteObject     = gdi32.NewProc("DeleteObject")
	procSetTextColor     = gdi32.NewProc("SetTextColor")
	procSetBkColor       = gdi32.NewProc("SetBkColor")
	procCreateFont       = gdi32.NewProc("CreateFontW")
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
	bgBrush     uintptr
	panelBrush  uintptr
	editBrush   uintptr
	bgColor     uint32
	panelColor  uint32
	editColor   uint32
	textColor   uint32
	mutedColor  uint32
	accentColor uint32
}

type appState struct {
	hwnd uintptr
	font uintptr

	title       uintptr
	subtitle    uintptr
	status      uintptr
	statusGroup uintptr
	ruleGroup   uintptr
	liveGroup   uintptr
	logGroup    uintptr

	keywordsEdit uintptr
	blockedEdit  uintptr
	addKeyEdit   uintptr
	addIPEdit    uintptr
	learnedEdit  uintptr
	autoEdit     uintptr
	logEdit      uintptr
	statsText    uintptr

	restoreBtn uintptr
	saveBtn    uintptr
	startBtn   uintptr
	injectBtn  uintptr
	refreshBtn uintptr
	openLogBtn uintptr
	folderBtn  uintptr
	addIPBtn   uintptr
	addKeyBtn  uintptr

	mu                 sync.Mutex
	busy               bool
	events             []string
	logLines           []string
	pendingLoadConfigs bool
	pendingRefreshLog  bool
}

var app appState
var uiTheme theme

func main() {
	runtime.LockOSThread()
	if !isAdmin() {
		relaunchAsAdmin()
		return
	}
	initTheme()
	defer cleanupTheme()
	runUI()
}

func isAdmin() bool {
	ret, _, _ := procIsUserAnAdmin.Call()
	return ret != 0
}

func relaunchAsAdmin() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	verb := utf16Ptr("runas")
	file := utf16Ptr(exe)
	dir := utf16Ptr(filepath.Dir(exe))
	procShellExecute.Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(file)), 0, uintptr(unsafe.Pointer(dir)), SW_SHOWNORMAL)
}

func initTheme() {
	uiTheme.bgColor = rgb(18, 22, 28)
	uiTheme.panelColor = rgb(31, 37, 47)
	uiTheme.editColor = rgb(12, 15, 20)
	uiTheme.textColor = rgb(228, 234, 242)
	uiTheme.mutedColor = rgb(154, 166, 181)
	uiTheme.accentColor = rgb(80, 154, 255)
	uiTheme.bgBrush = createBrush(uiTheme.bgColor)
	uiTheme.panelBrush = createBrush(uiTheme.panelColor)
	uiTheme.editBrush = createBrush(uiTheme.editColor)
}

func cleanupTheme() {
	for _, h := range []uintptr{uiTheme.bgBrush, uiTheme.panelBrush, uiTheme.editBrush, app.font} {
		if h != 0 {
			procDeleteObject.Call(h)
		}
	}
}

func runUI() {
	className := utf16Ptr("L4D2RowFilterManagerWindow")
	hinst, _, _ := procGetModuleHandle.Call(0)
	appIcon := loadAppIcon(hinst)
	wc := wndclassex{
		Size:       uint32(unsafe.Sizeof(wndclassex{})),
		WndProc:    syscall.NewCallback(wndProc),
		Instance:   hinst,
		Icon:       appIcon,
		Cursor:     loadCursor(32512),
		Background: uiTheme.bgBrush,
		ClassName:  className,
		IconSm:     appIcon,
	}
	procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	hwnd, _, _ := procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(utf16Ptr("L4D2 组服务器过滤器"))),
		WS_OVERLAPPEDWINDOW|WS_VISIBLE,
		120, 80, 1180, 760,
		0, 0, hinst, 0,
	)
	app.hwnd = hwnd
	if appIcon != 0 {
		procSendMessage.Call(hwnd, WM_SETICON, ICON_BIG, appIcon)
		procSendMessage.Call(hwnd, WM_SETICON, ICON_SMALL, appIcon)
	}
	procShowWindow.Call(hwnd, SW_SHOW)
	procUpdateWindow.Call(hwnd)

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

func wndProc(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	switch msg {
	case WM_CREATE:
		app.hwnd = hwnd
		createControls(hwnd)
		procSetTimer.Call(hwnd, ID_TIMER, 350, 0)
		loadConfigsToUI()
		refreshLogTail()
		return 0
	case WM_SIZE:
		layout(hwnd)
		return 0
	case WM_TIMER, WM_APP_EVENT:
		drainEventsToUI()
		return 0
	case WM_COMMAND:
		handleCommand(int(wparam & 0xffff))
		return 0
	case WM_ERASEBKGND:
		fillBackground(hwnd, wparam)
		return 1
	case WM_CTLCOLOREDIT:
		setControlColors(wparam, uiTheme.textColor, uiTheme.editColor)
		return uiTheme.editBrush
	case WM_CTLCOLORSTATIC, WM_CTLCOLORBTN:
		setControlColors(wparam, uiTheme.textColor, uiTheme.bgColor)
		return uiTheme.bgBrush
	case WM_DESTROY:
		procKillTimer.Call(hwnd, ID_TIMER)
		procPostQuitMessage.Call(0)
		return 0
	}
	ret, _, _ := procDefWindowProc.Call(hwnd, uintptr(msg), wparam, lparam)
	return ret
}

func createControls(hwnd uintptr) {
	app.font = createFont("Microsoft YaHei UI", 18, 400)
	app.title = label(hwnd, "L4D2 组服务器过滤器", 24, 18, 360, 28)
	app.subtitle = label(hwnd, "编辑规则、启动游戏并注入组服务器过滤核心。", 24, 48, 760, 24)
	app.status = label(hwnd, "状态：就绪", 820, 24, 320, 24)

	app.ruleGroup = group(hwnd, "过滤规则", 18, 84, 545, 490)
	app.liveGroup = group(hwnd, "运行状态", 575, 84, 565, 230)
	app.logGroup = group(hwnd, "日志尾部", 575, 324, 565, 250)
	app.statusGroup = group(hwnd, "操作", 18, 586, 1122, 92)

	label(hwnd, "关键字", 34, 116, 180, 22)
	app.addKeyEdit = singleLineEdit(hwnd, "", 34, 142, 150, 28)
	app.addKeyBtn = button(hwnd, "置顶添加", 194, 142, 80, 28, ID_ADD_KEY)
	app.keywordsEdit = edit(hwnd, "", 34, 178, 240, 134, false)
	label(hwnd, "永久 IP / IP:端口", 292, 116, 220, 22)
	app.addIPEdit = singleLineEdit(hwnd, "", 292, 142, 160, 28)
	app.addIPBtn = button(hwnd, "置顶添加", 462, 142, 80, 28, ID_ADD_IP)
	app.blockedEdit = edit(hwnd, "", 292, 178, 250, 134, false)

	label(hwnd, "学习地址", 34, 326, 180, 22)
	app.learnedEdit = edit(hwnd, "", 34, 352, 240, 195, false)
	label(hwnd, "派生地址", 292, 326, 180, 22)
	app.autoEdit = edit(hwnd, "", 292, 352, 250, 195, false)

	app.statsText = label(hwnd, "规则统计：-", 596, 118, 520, 60)
	app.logEdit = edit(hwnd, "", 596, 354, 520, 190, true)

	app.restoreBtn = button(hwnd, "恢复默认", 42, 626, 112, 32, ID_RESTORE)
	app.saveBtn = button(hwnd, "保存配置", 166, 626, 112, 32, ID_SAVE)
	app.startBtn = button(hwnd, "启动并注入", 308, 626, 124, 32, ID_START)
	app.injectBtn = button(hwnd, "注入运行中游戏", 444, 626, 132, 32, ID_INJECT)
	app.refreshBtn = button(hwnd, "刷新日志", 596, 626, 112, 32, ID_REFRESH)
	app.openLogBtn = button(hwnd, "打开日志", 720, 626, 112, 32, ID_OPENLOG)
	app.folderBtn = button(hwnd, "打开组件目录", 844, 626, 112, 32, ID_FOLDER)
}

func handleCommand(id int) {
	switch id {
	case ID_RESTORE:
		runBackground("恢复默认配置", true, true, func(ctx context.Context) error {
			return runPowerShell(ctx, filepath.Join(dllDir(), "restore_default_configs.ps1"))
		})
	case ID_SAVE:
		saveConfigsFromUI()
	case ID_ADD_KEY:
		addManualKeywordToTop()
	case ID_ADD_IP:
		addManualIPToTop()
	case ID_START:
		runBackground("启动并注入", false, true, func(ctx context.Context) error {
			return runPowerShell(ctx, filepath.Join(dllDir(), "launch_row_filter_early_admin.ps1"))
		})
	case ID_INJECT:
		runBackground("注入运行中游戏", false, true, func(ctx context.Context) error {
			return runPowerShell(ctx, filepath.Join(dllDir(), "load_row_filter_admin.ps1"))
		})
	case ID_REFRESH:
		refreshLogTail()
	case ID_OPENLOG:
		openPath(filepath.Join(dllDir(), "matchmaking_row_filter.log"))
	case ID_FOLDER:
		openPath(dllDir())
	}
}

func loadConfigsToUI() {
	setText(app.keywordsEdit, readListFile(filepath.Join(dllDir(), "blocked_keywords.txt")))
	setText(app.blockedEdit, readListFile(filepath.Join(dllDir(), "blocked_connectstrings.txt")))
	setText(app.learnedEdit, readListFile(filepath.Join(dllDir(), "learned_connectstrings.txt")))
	setText(app.autoEdit, readListFile(filepath.Join(dllDir(), "auto_derived_connectstrings.txt")))
	updateStats()
	queueLog("配置已加载。")
}

func saveConfigsFromUI() {
	files := map[string]string{
		"blocked_keywords.txt":            getText(app.keywordsEdit),
		"blocked_connectstrings.txt":      getText(app.blockedEdit),
		"learned_connectstrings.txt":      getText(app.learnedEdit),
		"auto_derived_connectstrings.txt": getText(app.autoEdit),
	}
	for name, text := range files {
		if err := writeTextAtomic(filepath.Join(dllDir(), name), normalizeEditableList(text)); err != nil {
			showError("保存失败", err)
			return
		}
	}
	updateStats()
	queueLog("配置已保存。重新注入或重启游戏后生效。")
}

func addManualIPToTop() {
	value := strings.TrimSpace(getText(app.addIPEdit))
	if value == "" {
		return
	}
	current := splitRuleLines(getText(app.blockedEdit))
	var out []string
	seen := map[string]bool{strings.ToLower(value): true}
	out = append(out, value)
	for _, item := range current {
		key := strings.ToLower(item)
		if key == strings.ToLower(value) || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, item)
	}
	setText(app.blockedEdit, strings.Join(out, "\r\n")+"\r\n")
	setText(app.addIPEdit, "")
	updateStats()
	queueLog("已将手动 IP 置顶：" + value)
}

func addManualKeywordToTop() {
	value := strings.TrimSpace(getText(app.addKeyEdit))
	if value == "" {
		return
	}
	current := splitRuleLines(getText(app.keywordsEdit))
	var out []string
	seen := map[string]bool{strings.ToLower(value): true}
	out = append(out, value)
	for _, item := range current {
		key := strings.ToLower(item)
		if key == strings.ToLower(value) || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, item)
	}
	setText(app.keywordsEdit, strings.Join(out, "\r\n")+"\r\n")
	setText(app.addKeyEdit, "")
	updateStats()
	queueLog("已将手动关键字置顶：" + value)
}

func refreshLogTail() {
	text, err := readTail(filepath.Join(dllDir(), "matchmaking_row_filter.log"), 96*1024)
	if err != nil {
		text = "日志不可用：" + err.Error()
	}
	setText(app.logEdit, text)
	updateStats()
}

func updateStats() {
	stats := []string{
		fmt.Sprintf("关键字：%d", countRules(getText(app.keywordsEdit))),
		fmt.Sprintf("永久：%d", countRules(getText(app.blockedEdit))),
		fmt.Sprintf("学习：%d", countRules(getText(app.learnedEdit))),
		fmt.Sprintf("派生：%d", countRules(getText(app.autoEdit))),
	}
	setText(app.statsText, "规则统计：\r\n"+strings.Join(stats, "    "))
}

func runBackground(name string, reloadConfigs bool, refreshLog bool, fn func(context.Context) error) {
	app.mu.Lock()
	if app.busy {
		app.mu.Unlock()
		queueLog("已有任务正在运行。")
		return
	}
	app.busy = true
	app.mu.Unlock()
	setBusy(true)
	queueLog(name + " 已开始。")

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
		defer cancel()
		err := fn(ctx)
		app.mu.Lock()
		app.busy = false
		app.mu.Unlock()
		if err != nil {
			queueLog(name + " 失败：" + err.Error())
		} else {
			queueLog(name + " 完成。")
		}
		app.mu.Lock()
		app.pendingLoadConfigs = app.pendingLoadConfigs || reloadConfigs
		app.pendingRefreshLog = app.pendingRefreshLog || refreshLog
		app.mu.Unlock()
		procPostMessage.Call(app.hwnd, WM_APP_EVENT, 0, 0)
	}()
}

func runPowerShell(ctx context.Context, script string) error {
	if _, err := os.Stat(script); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script)
	cmd.Dir = filepath.Dir(script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	trimmed := strings.TrimSpace(string(out))
	if trimmed != "" {
		queueLog(trimmed)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return fmt.Errorf("%w: %s", err, trimmed)
	}
	return nil
}

func queueLog(line string) {
	app.mu.Lock()
	app.logLines = append(app.logLines, time.Now().Format("15:04:05")+"  "+line)
	if len(app.logLines) > 80 {
		app.logLines = app.logLines[len(app.logLines)-80:]
	}
	app.mu.Unlock()
	procPostMessage.Call(app.hwnd, WM_APP_EVENT, 0, 0)
}

func drainEventsToUI() {
	app.mu.Lock()
	lines := append([]string(nil), app.logLines...)
	busy := app.busy
	loadConfigs := app.pendingLoadConfigs
	refreshLog := app.pendingRefreshLog
	app.pendingLoadConfigs = false
	app.pendingRefreshLog = false
	app.mu.Unlock()
	if busy {
		setText(app.status, "状态：运行中")
	} else {
		setText(app.status, "状态：就绪")
	}
	setBusy(busy)
	if len(lines) > 0 {
		current := getText(app.logEdit)
		if !strings.Contains(current, "matchmaking_row_filter loaded") {
			setText(app.logEdit, strings.Join(lines, "\r\n"))
		}
	}
	if loadConfigs {
		loadConfigsToUI()
	}
	if refreshLog {
		refreshLogTail()
	}
}

func setBusy(busy bool) {
	for _, h := range []uintptr{app.restoreBtn, app.saveBtn, app.startBtn, app.injectBtn} {
		enable(h, !busy)
	}
}

func readListFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return normalizeEditableList(string(data))
}

func normalizeEditableList(text string) string {
	out := splitRuleLines(text)
	if len(out) == 0 {
		return ""
	}
	return strings.Join(out, "\r\n") + "\r\n"
}

func splitRuleLines(text string) []string {
	lines := strings.Split(normalizeNewlines(text), "\n")
	var out []string
	seen := map[string]bool{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key := strings.ToLower(line)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, line)
	}
	return out
}

func countRules(text string) int {
	n := 0
	for _, line := range strings.Split(normalizeNewlines(text), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			n++
		}
	}
	return n
}

func readTail(path string, maxBytes int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	start := int64(0)
	if info.Size() > maxBytes {
		start = info.Size() - maxBytes
	}
	if _, err := f.Seek(start, 0); err != nil {
		return "", err
	}
	buf := bytes.Buffer{}
	if _, err := buf.ReadFrom(f); err != nil {
		return "", err
	}
	return normalizeNewlines(buf.String()), nil
}

func writeTextAtomic(path, text string) error {
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
	if _, err := tmp.WriteString(text); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	oldName := utf16Ptr(tmpName)
	newName := utf16Ptr(path)
	ret, _, callErr := procMoveFileEx.Call(uintptr(unsafe.Pointer(oldName)), uintptr(unsafe.Pointer(newName)), MOVEFILE_REPLACE_EXISTING|MOVEFILE_WRITE_THROUGH)
	if ret == 0 {
		if callErr != syscall.Errno(0) {
			return callErr
		}
		return errors.New("MoveFileExW failed")
	}
	cleanup = false
	return nil
}

func normalizeNewlines(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return text
}

func dllDir() string {
	exe, err := os.Executable()
	if err != nil {
		return filepath.Clean(`F:\Project\03_Game_Tools\L4D2_SERVER_TOOL\l4d2_server_filter_mod_research\hook_analysis\tools\matchmaking_row_filter_dll`)
	}
	exeDir := filepath.Dir(exe)
	candidates := []string{
		filepath.Join(exeDir, "runtime", "matchmaking_row_filter_dll"),
		filepath.Join(exeDir, "..", "matchmaking_row_filter_dll"),
		filepath.Join(exeDir, "..", "runtime", "matchmaking_row_filter_dll"),
	}
	for _, candidate := range candidates {
		if _, statErr := os.Stat(filepath.Join(candidate, "matchmaking_row_filter.dll")); statErr == nil {
			return filepath.Clean(candidate)
		}
	}
	return filepath.Clean(candidates[0])
}

func openPath(path string) {
	procShellExecute.Call(0, uintptr(unsafe.Pointer(utf16Ptr("open"))), uintptr(unsafe.Pointer(utf16Ptr(path))), 0, 0, SW_SHOWNORMAL)
}

func createFont(name string, height int32, weight int32) uintptr {
	ret, _, _ := procCreateFont.Call(
		uintptr(height), 0, 0, 0, uintptr(weight), 0, 0, 0,
		1, 0, 0, 5, 0,
		uintptr(unsafe.Pointer(utf16Ptr(name))),
	)
	return ret
}

func label(hwnd uintptr, text string, x, y, w, h int32) uintptr {
	return createChild(0, "STATIC", text, SS_LEFT, x, y, w, h, hwnd, 0)
}

func group(hwnd uintptr, text string, x, y, w, h int32) uintptr {
	return createChild(0, "BUTTON", text, BS_GROUPBOX, x, y, w, h, hwnd, 0)
}

func button(hwnd uintptr, text string, x, y, w, h int32, id int) uintptr {
	return createChild(0, "BUTTON", text, BS_PUSHBUTTON|WS_TABSTOP, x, y, w, h, hwnd, uintptr(id))
}

func edit(hwnd uintptr, text string, x, y, w, h int32, readonly bool) uintptr {
	style := WS_BORDER | WS_TABSTOP | ES_MULTILINE | ES_AUTOVSCROLL | ES_AUTOHSCROLL | WS_VSCROLL
	if readonly {
		style |= ES_READONLY | WS_HSCROLL
	}
	return createChild(WS_EX_CLIENTEDGE, "EDIT", text, uint32(style), x, y, w, h, hwnd, 0)
}

func singleLineEdit(hwnd uintptr, text string, x, y, w, h int32) uintptr {
	style := WS_BORDER | WS_TABSTOP | ES_AUTOHSCROLL
	return createChild(WS_EX_CLIENTEDGE, "EDIT", text, uint32(style), x, y, w, h, hwnd, 0)
}

func createChild(exStyle uint32, class, text string, style uint32, x, y, w, h int32, parent, id uintptr) uintptr {
	ret, _, _ := procCreateWindowEx.Call(
		uintptr(exStyle),
		uintptr(unsafe.Pointer(utf16Ptr(class))),
		uintptr(unsafe.Pointer(utf16Ptr(text))),
		uintptr(WS_CHILD|WS_VISIBLE|style),
		uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		parent, id, 0, 0,
	)
	if app.font != 0 {
		procSendMessage.Call(ret, WM_SETFONT, app.font, 1)
	}
	return ret
}

func layout(hwnd uintptr) {
	var r rect
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	width := r.Right - r.Left
	height := r.Bottom - r.Top
	if width < 980 {
		width = 980
	}
	if height < 680 {
		height = 680
	}
	leftW := (width - 58) / 2
	rightX := 36 + leftW
	rightW := width - rightX - 26
	top := int32(84)
	mainH := height - 194
	move(app.ruleGroup, 18, top, leftW, mainH)
	move(app.liveGroup, rightX, top, rightW, 218)
	move(app.logGroup, rightX, top+230, rightW, mainH-230)
	move(app.statusGroup, 18, height-98, width-36, 76)
	move(app.status, width-350, 24, 320, 24)

	half := (leftW - 70) / 2
	move(app.addKeyEdit, 34, 142, half-90, 28)
	move(app.addKeyBtn, 34+half-82, 142, 80, 28)
	move(app.keywordsEdit, 34, 178, half, 134)
	move(app.addIPEdit, 52+half, 142, half-90, 28)
	move(app.addIPBtn, 52+half+half-82, 142, 80, 28)
	move(app.blockedEdit, 52+half, 178, half, 134)
	move(app.learnedEdit, 34, 352, half, mainH-250)
	move(app.autoEdit, 52+half, 352, half, mainH-250)

	move(app.statsText, rightX+22, top+34, rightW-44, 70)
	move(app.logEdit, rightX+22, top+270, rightW-44, mainH-288)
	buttonY := height - 58
	x := int32(42)
	for _, item := range []struct {
		h uintptr
		w int32
	}{
		{app.restoreBtn, 104}, {app.saveBtn, 104}, {app.startBtn, 116}, {app.injectBtn, 140},
		{app.refreshBtn, 104}, {app.openLogBtn, 104}, {app.folderBtn, 128},
	} {
		move(item.h, x, buttonY, item.w, 32)
		x += item.w + 14
	}
}

func move(hwnd uintptr, x, y, w, h int32) {
	if hwnd != 0 {
		procMoveWindow.Call(hwnd, uintptr(x), uintptr(y), uintptr(w), uintptr(h), 1)
	}
}

func fillBackground(hwnd, hdc uintptr) {
	var r rect
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	procFillRect.Call(hdc, uintptr(unsafe.Pointer(&r)), uiTheme.bgBrush)
}

func setControlColors(hdc uintptr, fg, bg uint32) {
	procSetTextColor.Call(hdc, uintptr(fg))
	procSetBkColor.Call(hdc, uintptr(bg))
}

func setText(hwnd uintptr, text string) {
	procSetWindowText.Call(hwnd, uintptr(unsafe.Pointer(utf16Ptr(text))))
}

func getText(hwnd uintptr) string {
	n, _, _ := procSendMessage.Call(hwnd, WM_GETTEXTLENGTH, 0, 0)
	buf := make([]uint16, n+1)
	procSendMessage.Call(hwnd, WM_GETTEXT, n+1, uintptr(unsafe.Pointer(&buf[0])))
	return syscall.UTF16ToString(buf)
}

func enable(hwnd uintptr, ok bool) {
	v := uintptr(0)
	if ok {
		v = 1
	}
	procEnableWindow.Call(hwnd, v)
}

func showError(title string, err error) {
	procMessageBox.Call(app.hwnd, uintptr(unsafe.Pointer(utf16Ptr(err.Error()))), uintptr(unsafe.Pointer(utf16Ptr(title))), 0x10)
}

func loadCursor(id uintptr) uintptr {
	ret, _, _ := procLoadCursor.Call(0, id)
	return ret
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

func createBrush(color uint32) uintptr {
	ret, _, _ := procCreateSolidBrush.Call(uintptr(color))
	return ret
}

func rgb(r, g, b byte) uint32 {
	return uint32(r) | uint32(g)<<8 | uint32(b)<<16
}

func utf16Ptr(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}
