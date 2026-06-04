package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	comctl32 = syscall.NewLazyDLL("comctl32.dll")

	procDefWindowProc       = user32.NewProc("DefWindowProcW")
	procDispatchMessage     = user32.NewProc("DispatchMessageW")
	procGetMessage          = user32.NewProc("GetMessageW")
	procLoadCursor          = user32.NewProc("LoadCursorW")
	procLoadIcon            = user32.NewProc("LoadIconW")
	procPostQuitMessage     = user32.NewProc("PostQuitMessage")
	procRegisterClassEx     = user32.NewProc("RegisterClassExW")
	procCreateWindowEx      = user32.NewProc("CreateWindowExW")
	procShowWindow          = user32.NewProc("ShowWindow")
	procUpdateWindow        = user32.NewProc("UpdateWindow")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procSendMessage         = user32.NewProc("SendMessageW")
	procPostMessage         = user32.NewProc("PostMessageW")
	procSetWindowText       = user32.NewProc("SetWindowTextW")
	procGetWindowText       = user32.NewProc("GetWindowTextW")
	procGetWindowTextLength = user32.NewProc("GetWindowTextLengthW")
	procMoveWindow          = user32.NewProc("MoveWindow")
	procEnableWindow        = user32.NewProc("EnableWindow")
	procMessageBox          = user32.NewProc("MessageBoxW")
	procSetWindowLongPtr    = user32.NewProc("SetWindowLongPtrW")
	procGetStockObject      = gdi32.NewProc("GetStockObject")
	procCreateFont          = gdi32.NewProc("CreateFontW")
	procCreateSolidBrush    = gdi32.NewProc("CreateSolidBrush")
	procDeleteObject        = gdi32.NewProc("DeleteObject")
	procSetBkColor          = gdi32.NewProc("SetBkColor")
	procSetTextColor        = gdi32.NewProc("SetTextColor")
	procInvalidateRect      = user32.NewProc("InvalidateRect")
	procGetModuleHandle     = kernel32.NewProc("GetModuleHandleW")
	procInitCommonControls  = comctl32.NewProc("InitCommonControls")
)

const (
	WS_OVERLAPPEDWINDOW  = 0x00CF0000
	WS_VISIBLE           = 0x10000000
	WS_CHILD             = 0x40000000
	WS_TABSTOP           = 0x00010000
	WS_BORDER            = 0x00800000
	WS_VSCROLL           = 0x00200000
	WS_HSCROLL           = 0x00100000
	WS_EX_CLIENTEDGE     = 0x00000200
	WS_EX_APPWINDOW      = 0x00040000
	ES_MULTILINE         = 0x0004
	ES_AUTOHSCROLL       = 0x0080
	ES_AUTOVSCROLL       = 0x0040
	ES_WANTRETURN        = 0x1000
	BS_PUSHBUTTON        = 0x00000000
	BS_AUTOCHECKBOX      = 0x00000003
	SS_LEFT              = 0x00000000
	LBS_NOINTEGRALHEIGHT = 0x0100

	SW_SHOW = 5

	WM_DESTROY         = 0x0002
	WM_SIZE            = 0x0005
	WM_SETREDRAW       = 0x000B
	WM_COMMAND         = 0x0111
	WM_SETFONT         = 0x0030
	WM_SETICON         = 0x0080
	WM_CTLCOLOREDIT    = 0x0133
	WM_CTLCOLORLISTBOX = 0x0134
	WM_CTLCOLORBTN     = 0x0135
	WM_CTLCOLORSTATIC  = 0x0138
	WM_APP             = 0x8000
	EN_SETFOCUS        = 0x0100
	EN_KILLFOCUS       = 0x0200
	EN_CHANGE          = 0x0300

	LVM_FIRST                    = 0x1000
	LVM_SETBKCOLOR               = LVM_FIRST + 1
	LVM_INSERTCOLUMN             = LVM_FIRST + 97
	LVM_INSERTITEM               = LVM_FIRST + 77
	LVM_SETITEM                  = LVM_FIRST + 76
	LVM_SETITEMTEXT              = LVM_FIRST + 116
	LVM_DELETEALLITEMS           = LVM_FIRST + 9
	LVM_GETITEMCOUNT             = LVM_FIRST + 4
	LVM_GETTOPINDEX              = LVM_FIRST + 39
	LVM_ENSUREVISIBLE            = LVM_FIRST + 19
	LVM_SETTEXTCOLOR             = LVM_FIRST + 36
	LVM_SETTEXTBKCOLOR           = LVM_FIRST + 38
	LVM_SETEXTENDEDLISTVIEWSTYLE = LVM_FIRST + 54
	LVS_EX_FULLROWSELECT         = 0x00000020
	LVS_EX_GRIDLINES             = 0x00000001
	LVS_EX_DOUBLEBUFFER          = 0x00010000
	LVS_REPORT                   = 0x0001
	LVS_SHOWSELALWAYS            = 0x0008
	LVS_SINGLESEL                = 0x0004

	LVCF_TEXT   = 0x0004
	LVCF_WIDTH  = 0x0002
	LVCF_FMT    = 0x0001
	LVIF_TEXT   = 0x0001
	BM_GETCHECK = 0x00F0
	BST_CHECKED = 1

	ID_SCAN           = 1001
	ID_CANCEL         = 1002
	ID_APPLY          = 1003
	ID_REMOVE         = 1004
	ID_EXPORT         = 1005
	ID_OPEN           = 1006
	ID_CHECK          = 1007
	ID_HUORONG        = 1008
	ID_STOP_OBSERVE   = 1009
	ID_FILTER_MATCHED = 1010
	ID_SEARCH         = 1011
	ID_KEYWORDS       = 1012

	IDI_APP = 1

	WM_PROGRESS = WM_APP + 1
	WM_POPULATE = WM_APP + 2
	WM_INITUI   = WM_APP + 3
	WM_REFRESH  = WM_APP + 4
)

type rect struct{ Left, Top, Right, Bottom int32 }
type point struct{ X, Y int32 }
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
type lvcolumn struct {
	Mask      uint32
	Fmt       int32
	Cx        int32
	Text      *uint16
	TextMax   int32
	SubItem   int32
	Image     int32
	Order     int32
	MinWidth  int32
	DefaultCX int32
	IdealCX   int32
}
type lvitem struct {
	Mask      uint32
	Item      int32
	SubItem   int32
	State     uint32
	StateMask uint32
	Text      *uint16
	TextMax   int32
	Image     int32
	LParam    uintptr
	Indent    int32
	GroupID   int32
	Columns   uint32
	PUColumns uintptr
}

var app = &uiState{}

type uiState struct {
	hwnd        uintptr
	list        uintptr
	resultTitle uintptr
	resultMeta  uintptr
	searchLabel uintptr
	searchEdit  uintptr
	edit        uintptr
	status      uintptr
	btnScan     uintptr
	btnCancel   uintptr
	btnApply    uintptr
	btnRemove   uintptr
	btnExport   uintptr
	btnOpen     uintptr
	btnCheck    uintptr
	btnHuorong  uintptr
	btnStopObs  uintptr
	chkMatched  uintptr
	font        uintptr
	listFont    uintptr
	bgBrush     uintptr
	panelBrush  uintptr
	inputBrush  uintptr
	buttonBrush uintptr

	cfg               Config
	cancel            context.CancelFunc
	refreshStop       context.CancelFunc
	scanning          bool
	observing         bool
	mode              string
	all               []ServerInfo
	blockedIPs        []string
	savedNameIPs      map[string]string
	rendering         bool
	mu                sync.Mutex
	queue             []ScanProgress
	renderRows        []ServerInfo
	renderedLines     []string
	renderIndex       int
	observeSeen       int
	progressDone      int
	progressTotal     int
	lastKeywordsText  string
	searchPlaceholder bool
	updatingSearch    bool
}

func main() {
	runtime.LockOSThread()
	debugLog("main: start")
	_ = loadConfig()
	debugLog("main: config loaded")
	_ = loadSavedResults()
	debugLog("main: saved results loaded")
	procInitCommonControls.Call()
	hInst, _, _ := procGetModuleHandle.Call(0)
	app.bgBrush = createSolidBrush(rgb(18, 20, 24))
	app.panelBrush = createSolidBrush(rgb(28, 31, 36))
	app.inputBrush = createSolidBrush(rgb(24, 26, 31))
	app.buttonBrush = createSolidBrush(rgb(43, 47, 55))
	className := utf16("L4D2ServerToolWindow")
	wc := wndclassex{
		Size:       uint32(unsafe.Sizeof(wndclassex{})),
		WndProc:    syscall.NewCallback(wndProc),
		Instance:   hInst,
		Icon:       loadIcon(hInst, IDI_APP),
		Cursor:     loadCursor(32512),
		Background: app.bgBrush,
		ClassName:  className,
		IconSm:     loadIcon(hInst, IDI_APP),
	}
	debugLog("main: registering class")
	procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	debugLog("main: creating window")
	app.hwnd = createWindowEx(WS_EX_APPWINDOW, "L4D2ServerToolWindow", "求生之路2服务器筛选工具", WS_OVERLAPPEDWINDOW|WS_VISIBLE, 120, 90, 1180, 720, 0, 0, hInst, 0)
	debugLog("main: window created")
	procShowWindow.Call(app.hwnd, SW_SHOW)
	procUpdateWindow.Call(app.hwnd)
	debugLog("main: entering message loop")
	var m msg
	for {
		r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func wndProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	switch message {
	case WM_DESTROY:
		if app.cancel != nil {
			app.cancel()
		}
		stopAutoRefresh()
		saveKeywordsFromEdit()
		saveSavedResults()
		releaseUIResources()
		procPostQuitMessage.Call(0)
		return 0
	case WM_SIZE:
		layout(hwnd, int32(lParam&0xFFFF), int32((lParam>>16)&0xFFFF))
		return 0
	case WM_COMMAND:
		switch int(wParam & 0xFFFF) {
		case ID_SCAN:
			startScan()
		case ID_CANCEL:
			cancelScan()
		case ID_APPLY:
			applyFirewall()
		case ID_REMOVE:
			removeFirewall()
		case ID_EXPORT:
			exportIPs()
		case ID_OPEN:
			_ = openFolder(appDir())
		case ID_CHECK:
			startObserve()
		case ID_HUORONG:
			exportHuorong()
		case ID_STOP_OBSERVE:
			stopObserve()
		case ID_FILTER_MATCHED:
			refreshResults()
		case ID_SEARCH:
			handleSearchCommand(int((wParam >> 16) & 0xFFFF))
		case ID_KEYWORDS:
			if int((wParam>>16)&0xFFFF) == EN_CHANGE && saveKeywordsFromEdit() {
				refreshResults()
			}
		}
		return 0
	case WM_PROGRESS:
		drainProgress()
		return 0
	case WM_POPULATE:
		populateListBatch()
		return 0
	case WM_INITUI:
		if app.list == 0 {
			debugLog("wnd: init ui start")
			createControls(hwnd)
			debugLog("wnd: init ui done")
		}
		return 0
	case WM_REFRESH:
		refreshResults()
		return 0
	case WM_CTLCOLORSTATIC, WM_CTLCOLOREDIT, WM_CTLCOLORLISTBOX, WM_CTLCOLORBTN:
		return colorControl(message, wParam, lParam)
	}
	if message == 0x0001 {
		debugLog("wnd: create")
		app.hwnd = hwnd
		hInst, _, _ := procGetModuleHandle.Call(0)
		icon := loadIcon(hInst, IDI_APP)
		if icon != 0 {
			send(hwnd, WM_SETICON, 0, icon)
			send(hwnd, WM_SETICON, 1, icon)
		}
		procPostMessage.Call(hwnd, WM_INITUI, 0, 0)
		return 0
	}
	ret, _, _ := procDefWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
	return ret
}

func createControls(hwnd uintptr) {
	debugLog("controls: start")
	app.font = createFont("Microsoft YaHei UI", 18, 500)
	app.listFont = createFont("Consolas", 16, 500)
	app.btnScan = button(hwnd, "扫描服务器", ID_SCAN)
	debugLog("controls: buttons")
	app.btnCancel = button(hwnd, "取消扫描", ID_CANCEL)
	app.btnApply = button(hwnd, "应用屏蔽", ID_APPLY)
	app.btnRemove = button(hwnd, "恢复屏蔽", ID_REMOVE)
	app.btnExport = button(hwnd, "导出IP", ID_EXPORT)
	app.btnOpen = button(hwnd, "打开目录", ID_OPEN)
	app.btnCheck = button(hwnd, "观察游戏", ID_CHECK)
	app.btnStopObs = button(hwnd, "停止观察", ID_STOP_OBSERVE)
	app.btnHuorong = button(hwnd, "导出火绒", ID_HUORONG)
	app.chkMatched = child("BUTTON", "仅显示命中结果", WS_CHILD|WS_VISIBLE|WS_TABSTOP|BS_AUTOCHECKBOX, ID_FILTER_MATCHED)
	app.status = child("STATIC", "就绪。左侧每行一个关键词；扫描和观察会在后台运行，结果每 3 秒刷新到列表。", WS_CHILD|WS_VISIBLE|SS_LEFT, 0)
	app.edit = childEx(WS_EX_CLIENTEDGE, "EDIT", strings.Join(app.cfg.NameKeywords, "\r\n"), WS_CHILD|WS_VISIBLE|WS_BORDER|WS_VSCROLL|ES_MULTILINE|ES_AUTOVSCROLL|ES_WANTRETURN|WS_TABSTOP, ID_KEYWORDS)
	app.lastKeywordsText = getText(app.edit)
	debugLog("controls: edit")
	app.resultTitle = child("STATIC", "扫描结果", WS_CHILD|WS_VISIBLE|SS_LEFT, 0)
	app.resultMeta = child("STATIC", "暂无结果", WS_CHILD|WS_VISIBLE|SS_LEFT, 0)
	app.searchEdit = childEx(WS_EX_CLIENTEDGE, "EDIT", "", WS_CHILD|WS_VISIBLE|WS_BORDER|ES_AUTOHSCROLL|WS_TABSTOP, ID_SEARCH)
	setSearchPlaceholder(true)
	app.list = childEx(WS_EX_CLIENTEDGE, "SysListView32", "", WS_CHILD|WS_VISIBLE|WS_VSCROLL|WS_HSCROLL|WS_TABSTOP|LVS_REPORT|LVS_SHOWSELALWAYS|LVS_SINGLESEL, 0)
	debugLog("controls: listview")
	for _, h := range []uintptr{app.btnScan, app.btnCancel, app.btnApply, app.btnRemove, app.btnExport, app.btnOpen, app.btnCheck, app.btnStopObs, app.btnHuorong, app.chkMatched, app.status, app.edit, app.resultTitle, app.resultMeta, app.searchEdit} {
		send(h, WM_SETFONT, app.font, 1)
	}
	send(app.list, WM_SETFONT, app.font, 1)
	initResultListView()
	updateResultSummary(0, 0, 0)
	if len(app.all) > 0 {
		refreshResults()
		setStatus(fmt.Sprintf("已读取上次保存的 %d 条结果。", len(app.all)))
	}
	enable(app.btnCancel, false)
	enable(app.btnStopObs, false)
	layout(hwnd, 1180, 720)
	debugLog("controls: done")
}

func colorControl(message uint32, hdc, hwnd uintptr) uintptr {
	procSetTextColor.Call(hdc, uintptr(rgb(229, 233, 240)))
	switch message {
	case WM_CTLCOLOREDIT, WM_CTLCOLORLISTBOX:
		if hwnd == app.searchEdit && app.searchPlaceholder {
			procSetTextColor.Call(hdc, uintptr(rgb(132, 144, 160)))
		}
		procSetBkColor.Call(hdc, uintptr(rgb(24, 26, 31)))
		return app.inputBrush
	case WM_CTLCOLORBTN:
		procSetBkColor.Call(hdc, uintptr(rgb(43, 47, 55)))
		return app.buttonBrush
	default:
		if hwnd == app.status || hwnd == app.chkMatched {
			procSetBkColor.Call(hdc, uintptr(rgb(18, 20, 24)))
			return app.bgBrush
		}
		if hwnd == app.resultTitle {
			procSetTextColor.Call(hdc, uintptr(rgb(160, 211, 255)))
			procSetBkColor.Call(hdc, uintptr(rgb(18, 20, 24)))
			return app.bgBrush
		}
		if hwnd == app.resultMeta {
			procSetTextColor.Call(hdc, uintptr(rgb(166, 176, 190)))
			procSetBkColor.Call(hdc, uintptr(rgb(18, 20, 24)))
			return app.bgBrush
		}
		procSetBkColor.Call(hdc, uintptr(rgb(28, 31, 36)))
		return app.panelBrush
	}
}

func layout(hwnd uintptr, w, h int32) {
	if app.list == 0 || w <= 0 || h <= 0 {
		return
	}
	margin := int32(14)
	top := int32(14)
	btnWidths := []int32{116, 116, 116, 116, 116, 116}
	btnH := int32(34)
	x := margin
	for i, hb := range []uintptr{app.btnCheck, app.btnStopObs, app.btnScan, app.btnCancel, app.btnApply, app.btnRemove} {
		move(hb, x, top, btnWidths[i], btnH)
		x += btnWidths[i] + 8
	}
	x = margin
	for i, hb := range []uintptr{app.btnHuorong, app.btnExport, app.btnOpen} {
		row2Widths := []int32{116, 116, 116}
		move(hb, x, top+42, row2Widths[i], btnH)
		x += row2Widths[i] + 8
	}
	chkW := int32(148)
	searchW := int32(208)
	move(app.chkMatched, x, top+46, chkW, 28)
	move(app.searchEdit, x+chkW+8, top+42, searchW, btnH)
	move(app.status, margin, top+84, w-margin*2, 26)
	leftW := int32(250)
	move(app.edit, margin, top+118, leftW, h-top-136)
	resultX := margin + leftW + 12
	resultW := w - leftW - margin*2 - 12
	move(app.resultTitle, resultX, top+116, 180, 24)
	move(app.resultMeta, resultX+190, top+116, resultW-190, 24)
	move(app.list, resultX, top+146, resultW, h-top-164)
}

func startScan() {
	if app.scanning || app.observing || app.rendering {
		return
	}
	app.cfg.NameKeywords = splitLines(getText(app.edit))
	saveConfig()
	clearResults()
	app.all = nil
	app.blockedIPs = nil
	app.renderRows = nil
	app.renderedLines = nil
	app.renderIndex = 0
	app.observeSeen = 0
	app.progressDone = 0
	app.progressTotal = 0
	ctx, cancel := context.WithCancel(context.Background())
	app.cancel = cancel
	app.scanning = true
	app.mode = "scan"
	enable(app.btnScan, false)
	enable(app.btnCheck, false)
	enable(app.btnCancel, true)
	enable(app.btnStopObs, false)
	setStatus("正在扫描：请求 Steam 主服务器列表...")
	startAutoRefresh()
	ch := make(chan ScanProgress, 256)
	go scanServers(ctx, app.cfg, ch)
	go func() {
		for p := range ch {
			app.mu.Lock()
			wasEmpty := len(app.queue) == 0
			app.queue = append(app.queue, p)
			app.mu.Unlock()
			if wasEmpty {
				procPostMessage.Call(app.hwnd, WM_PROGRESS, 0, 0)
			}
		}
	}()
}

func cancelScan() {
	if app.cancel != nil {
		app.cancel()
	}
	if app.observing {
		setStatus("正在停止观察...")
	} else {
		setStatus("正在取消扫描...")
	}
}

func stopObserve() {
	if !app.observing {
		setStatus("当前没有正在进行的观察任务。")
		return
	}
	if app.cancel != nil {
		app.cancel()
	}
	setStatus("正在停止观察...")
}

func startObserve() {
	if app.scanning || app.observing || app.rendering {
		return
	}
	app.cfg.NameKeywords = splitLines(getText(app.edit))
	saveConfig()
	clearResults()
	app.all = nil
	app.blockedIPs = nil
	app.renderRows = nil
	app.renderedLines = nil
	app.renderIndex = 0
	app.observeSeen = 0
	app.progressDone = 0
	app.progressTotal = 0
	ctx, cancel := context.WithCancel(context.Background())
	app.cancel = cancel
	app.observing = true
	app.mode = "observe"
	enable(app.btnScan, false)
	enable(app.btnCheck, false)
	enable(app.btnCancel, true)
	enable(app.btnStopObs, true)
	setStatus("正在启动 WinDivert 观察...")
	startAutoRefresh()
	ch := make(chan ScanProgress, 256)
	go observeL4D2Flows(ctx, app.cfg, ch)
	go func() {
		for p := range ch {
			app.mu.Lock()
			wasEmpty := len(app.queue) == 0
			app.queue = append(app.queue, p)
			app.mu.Unlock()
			if wasEmpty {
				procPostMessage.Call(app.hwnd, WM_PROGRESS, 0, 0)
			}
		}
	}()
}

func drainProgress() {
	app.mu.Lock()
	n := len(app.queue)
	if n > 40 {
		n = 40
	}
	q := append([]ScanProgress(nil), app.queue[:n]...)
	app.queue = app.queue[n:]
	remaining := len(app.queue)
	app.mu.Unlock()
	if remaining > 0 {
		defer procPostMessage.Call(app.hwnd, WM_PROGRESS, 0, 0)
	}
	for _, p := range q {
		if p.Phase == "observe" && p.Server != nil {
			markIPChange(p.Server)
			upsertServer(*p.Server)
			app.observeSeen++
			addBlockedIPFromServer(*p.Server)
			if app.observeSeen == 1 || app.observeSeen%10 == 0 {
				setStatus(fmt.Sprintf("正在观察：已识别 %d 个候选服务器，命中 %d 个 IP。", len(app.all), len(app.blockedIPs)))
			}
		}
		if p.Phase != "observe" && p.Server != nil {
			markIPChange(p.Server)
			upsertServer(*p.Server)
			addBlockedIPFromServer(*p.Server)
		}
		if p.Total > 0 && !p.Finished {
			app.progressDone = p.Done
			app.progressTotal = p.Total
			setStatus(fmt.Sprintf("正在扫描：%d/%d，列表每 3 秒刷新。", p.Done, p.Total))
		} else if p.Message != "" {
			setStatus(p.Message)
		}
		if p.Finished {
			app.scanning = false
			app.observing = false
			stopAutoRefresh()
			enable(app.btnCancel, false)
			enable(app.btnStopObs, false)
			enable(app.btnCheck, true)
			if p.Phase == "observe" && len(app.all) > 0 {
				beginListPopulate(app.all)
			} else if p.Phase == "observe" {
				enable(app.btnScan, true)
				setStatus(p.Message)
			} else if p.Phase == "error" {
				enable(app.btnScan, true)
				message("扫描失败", p.Message)
			} else {
				app.all = p.All
				markIPChanges(app.all)
				app.blockedIPs = p.BlockedIP
				beginListPopulate(app.all)
			}
			saveSavedResults()
		}
	}
}

func beginListPopulate(rows []ServerInfo) {
	app.renderRows = rows
	app.renderIndex = 0
	app.rendering = false
	enable(app.btnScan, false)
	refreshResults()
	enable(app.btnScan, true)
	enable(app.btnCheck, true)
	setStatus(fmt.Sprintf("完成：共 %d 条结果，命中 %d 个 IP。", len(app.all), len(app.blockedIPs)))
}

func upsertServer(s ServerInfo) {
	if s.Address == "" {
		return
	}
	for i := range app.all {
		if app.all[i].Address == s.Address {
			app.all[i] = mergeServerInfo(app.all[i], s)
			return
		}
	}
	app.all = append(app.all, s)
}

func mergeServerInfo(old, next ServerInfo) ServerInfo {
	if next.Host == "" && old.Host != "" {
		next.Host = old.Host
	}
	if next.Map == "" && old.Map != "" {
		next.Map = old.Map
	}
	if next.Folder == "" && old.Folder != "" {
		next.Folder = old.Folder
	}
	if next.Game == "" && old.Game != "" {
		next.Game = old.Game
	}
	if next.Keywords == "" && old.Keywords != "" {
		next.Keywords = old.Keywords
	}
	if next.PreviousAddress == "" {
		next.PreviousAddress = old.PreviousAddress
	}
	next.IPChanged = next.IPChanged || old.IPChanged
	next.Blocked = next.Blocked || old.Blocked
	for _, reason := range old.BlockReasons {
		next.BlockReasons = appendUniqueReason(next.BlockReasons, reason)
	}
	if next.LastSeen.IsZero() {
		next.LastSeen = old.LastSeen
	}
	return next
}

func populateListBatch() {
	if app.renderIndex >= len(app.renderRows) {
		app.rendering = false
		enable(app.btnScan, true)
		enable(app.btnCheck, true)
		setStatus(fmt.Sprintf("完成：共 %d 条结果，命中 %d 个 IP。", len(app.all), len(app.blockedIPs)))
		return
	}
	send(app.list, WM_SETREDRAW, 0, 0)
	end := app.renderIndex + 50
	if end > len(app.renderRows) {
		end = len(app.renderRows)
	}
	for app.renderIndex < end {
		s := app.renderRows[app.renderIndex]
		if s.Error == "" && shouldShowServer(s) {
			insertServer(s)
		}
		app.renderIndex++
	}
	send(app.list, WM_SETREDRAW, 1, 0)
	procInvalidateRect.Call(app.list, 0, 1)
	if app.renderIndex < len(app.renderRows) {
		setStatus(fmt.Sprintf("正在渲染列表：%d/%d", app.renderIndex, len(app.renderRows)))
		procPostMessage.Call(app.hwnd, WM_POPULATE, 0, 0)
	} else {
		app.rendering = false
		enable(app.btnScan, true)
		enable(app.btnCheck, true)
		setStatus(fmt.Sprintf("完成：共 %d 条结果，命中 %d 个 IP。", len(app.all), len(app.blockedIPs)))
	}
}
func insertServer(s ServerInfo) {
	count := int(send(app.list, LVM_GETITEMCOUNT, 0, 0))
	insertResultRow(count, s)
}

func resultCells(s ServerInfo) []string {
	return []string{
		displayStatus(s),
		displayPing(s),
		displayPlayers(s),
		s.Map,
		s.Address,
		s.Host,
		displayReasons(s),
	}
}

func displayStatus(s ServerInfo) string {
	status := "正常"
	if s.Blocked {
		status = "命中"
	}
	if s.IPChanged {
		status = "变更"
	}
	if !s.Blocked && hasReason(s, "A2S未响应") {
		status = "未响应"
	}
	return status
}

func displayReasons(s ServerInfo) string {
	reasons := append([]string(nil), s.BlockReasons...)
	if s.IPChanged && s.PreviousAddress != "" {
		reasons = append(reasons, "IP变更:"+s.PreviousAddress)
	}
	return strings.Join(reasons, " ")
}

func hasReason(s ServerInfo, reason string) bool {
	for _, item := range s.BlockReasons {
		if item == reason {
			return true
		}
	}
	return false
}

func displayPing(s ServerInfo) string {
	if s.PingMS <= 0 {
		return "--"
	}
	return fmt.Sprintf("%dms", s.PingMS)
}

func displayPlayers(s ServerInfo) string {
	if s.Players < 0 || s.MaxPlayers < 0 {
		return "--"
	}
	if s.MaxPlayers == 0 {
		return fmt.Sprintf("%d", s.Players)
	}
	return fmt.Sprintf("%d/%d", s.Players, s.MaxPlayers)
}

func startAutoRefresh() {
	stopAutoRefresh()
	ctx, cancel := context.WithCancel(context.Background())
	app.refreshStop = cancel
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				procPostMessage.Call(app.hwnd, WM_REFRESH, 0, 0)
			}
		}
	}()
}

func stopAutoRefresh() {
	if app.refreshStop != nil {
		app.refreshStop()
		app.refreshStop = nil
	}
}

func shouldShowServer(s ServerInfo) bool {
	if s.Error != "" {
		return false
	}
	if !isIPv4Server(s.Address) {
		return false
	}
	if onlyShowMatched() && !s.Blocked {
		return false
	}
	return serverMatchesSearch(s, searchQuery())
}

func onlyShowMatched() bool {
	if app.chkMatched == 0 {
		return false
	}
	return send(app.chkMatched, BM_GETCHECK, 0, 0) == BST_CHECKED
}

func handleSearchCommand(notification int) {
	switch notification {
	case EN_SETFOCUS:
		if app.searchPlaceholder {
			setSearchPlaceholder(false)
		}
	case EN_KILLFOCUS:
		if strings.TrimSpace(getText(app.searchEdit)) == "" {
			setSearchPlaceholder(true)
		}
	case EN_CHANGE:
		if !app.updatingSearch {
			refreshResults()
		}
	}
}

func setSearchPlaceholder(on bool) {
	if app.searchEdit == 0 {
		return
	}
	app.updatingSearch = true
	defer func() { app.updatingSearch = false }()
	app.searchPlaceholder = on
	text := ""
	if on {
		text = "搜索结果"
	}
	procSetWindowText.Call(app.searchEdit, uintptr(unsafe.Pointer(utf16(text))))
	procInvalidateRect.Call(app.searchEdit, 0, 1)
}

func searchQuery() string {
	if app.searchPlaceholder {
		return ""
	}
	return getText(app.searchEdit)
}

func refreshResults() {
	if app.list == 0 {
		return
	}
	recalculateMatches()
	topIndex := int(send(app.list, LVM_GETTOPINDEX, 0, 0))
	rows := make([]ServerInfo, 0, len(app.all))
	shown := 0
	for _, s := range app.all {
		if shouldShowServer(s) {
			rows = append(rows, s)
			shown++
		}
	}
	applyResultRows(rows)
	if topIndex > 0 && topIndex < shown {
		send(app.list, LVM_ENSUREVISIBLE, uintptr(topIndex), 0)
	}
	procInvalidateRect.Call(app.list, 0, 1)
	updateResultSummary(shown, len(app.all), len(app.blockedIPs))
}

func recalculateMatches() {
	saveKeywordsFromEdit()
	app.blockedIPs = nil
	for i := range app.all {
		resetDynamicRules(&app.all[i])
		if app.all[i].Error != "" {
			continue
		}
		applyRules(&app.all[i], app.cfg)
		addBlockedIPFromServer(app.all[i])
	}
}

func markIPChanges(rows []ServerInfo) {
	for i := range rows {
		markIPChange(&rows[i])
	}
}

func markIPChange(s *ServerInfo) {
	if s == nil || s.Host == "" || s.Address == "" {
		return
	}
	nameKey := serverNameKey(s.Host)
	if nameKey == "" {
		return
	}
	if app.savedNameIPs == nil {
		app.savedNameIPs = map[string]string{}
	}
	currentIP := serverIP(s.Address)
	if currentIP == "" {
		return
	}
	if previous, ok := app.savedNameIPs[nameKey]; ok && previous != "" && previous != currentIP {
		s.IPChanged = true
		s.PreviousAddress = previous
	} else if s.PreviousAddress == "" {
		s.IPChanged = false
	}
}

func rebuildSavedNameIPs() {
	app.savedNameIPs = map[string]string{}
	for _, s := range app.all {
		nameKey := serverNameKey(s.Host)
		ip := serverIP(s.Address)
		if nameKey != "" && ip != "" {
			app.savedNameIPs[nameKey] = ip
		}
	}
}

func serverNameKey(name string) string {
	key := normalizeText(name)
	if key == "" ||
		strings.Contains(key, "候选服务器") ||
		strings.Contains(key, "暂未获取名称") ||
		key == "left 4 dead 2" ||
		key == "l4d2" ||
		key == "srcds" {
		return ""
	}
	return key
}

func serverIP(address string) string {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return ""
	}
	if !isUsableServerAddress(address) {
		return ""
	}
	ip := net.ParseIP(host)
	if ip == nil || ip.To4() == nil {
		return ""
	}
	return ip.String()
}

func resetDynamicRules(s *ServerInfo) {
	s.Blocked = false
	s.BlockReasons = nil
}

func isCandidateWithoutA2S(s ServerInfo) bool {
	return s.Players < 0 || s.MaxPlayers < 0
}

func isIPv4Server(address string) bool {
	return isUsableServerAddress(address)
}

func serverMatchesSearch(s ServerInfo, query string) bool {
	query = normalizeText(query)
	if query == "" {
		return true
	}
	haystack := normalizeText(strings.Join([]string{
		s.Address,
		s.Host,
		s.Map,
		s.Folder,
		s.Game,
		s.Keywords,
		s.PreviousAddress,
		strings.Join(s.BlockReasons, " "),
		displayPing(s),
		displayPlayers(s),
	}, " "))
	return strings.Contains(haystack, query)
}

func saveKeywordsFromEdit() bool {
	if app.edit == 0 {
		return false
	}
	text := getText(app.edit)
	if text == app.lastKeywordsText {
		return false
	}
	app.lastKeywordsText = text
	app.cfg.NameKeywords = splitLines(text)
	saveConfig()
	return true
}

func addBlockedIPFromServer(s ServerInfo) {
	if !s.Blocked || s.Error != "" {
		return
	}
	ip, _, err := net.SplitHostPort(s.Address)
	if err == nil && ip != "" && isUsableServerAddress(s.Address) && !stringInSlice(ip, app.blockedIPs) {
		app.blockedIPs = append(app.blockedIPs, ip)
	}
}

func applyFirewall() {
	prepareCurrentBlockedForExport()
	if len(app.blockedIPs) == 0 {
		message("没有可屏蔽 IP", "当前没有命中的 IP。请先扫描服务器或观察游戏。")
		return
	}
	if err := appendBlockRecords(recordsFromCurrentBlocked(app.mode)); err != nil {
		message("记录失败", err.Error())
		return
	}
	script, err := makeFirewallScript(appDir(), app.blockedIPs)
	if err != nil {
		message("脚本生成失败", err.Error())
		return
	}
	if err := runPowerShellElevated(script); err != nil {
		message("启动失败", err.Error())
		return
	}
	setStatus("已请求管理员权限应用屏蔽规则。")
}

func prepareCurrentBlockedForExport() {
	recalculateMatches()
	saveSavedResults()
	refreshResults()
}

func stringInSlice(s string, xs []string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func recordsFromCurrentBlocked(source string) []BlockRecord {
	now := time.Now()
	var records []BlockRecord
	seen := map[string]bool{}
	for _, s := range app.all {
		if !s.Blocked || s.Error != "" {
			continue
		}
		ip, _, err := net.SplitHostPort(s.Address)
		if err != nil || ip == "" || !isUsableServerAddress(s.Address) || seen[ip] {
			continue
		}
		seen[ip] = true
		records = append(records, BlockRecord{
			Time:         now,
			IP:           ip,
			Address:      s.Address,
			ServerName:   s.Host,
			Map:          s.Map,
			Players:      fmt.Sprintf("%d/%d", s.Players, s.MaxPlayers),
			Reasons:      append([]string(nil), s.BlockReasons...),
			Source:       source,
			FirewallRule: firewallRuleBase,
		})
	}
	return records
}

func removeFirewall() {
	script, err := makeFirewallRemoveScript(appDir())
	if err != nil {
		message("脚本生成失败", err.Error())
		return
	}
	if err := runPowerShellElevated(script); err != nil {
		message("启动失败", err.Error())
		return
	}
	setStatus("已请求管理员权限恢复屏蔽规则。")
}

func exportIPs() {
	prepareCurrentBlockedForExport()
	path := filepath.Join(appDir(), "blocked_ips.txt")
	if err := exportBlockedIPs(path, app.blockedIPs); err != nil {
		message("导出失败", err.Error())
		return
	}
	setStatus("已导出 IP 列表：" + path)
}

func exportHuorong() {
	prepareCurrentBlockedForExport()
	ipPath, err := exportHuorongIPBlacklist(appDir(), app.all, app.blockedIPs)
	if err != nil {
		message("火绒导出失败", err.Error())
		return
	}
	setStatus("已导出火绒 IP 黑名单：" + ipPath)
}

func clearResults() {
	if app.list == 0 {
		return
	}
	send(app.list, LVM_DELETEALLITEMS, 0, 0)
	app.renderedLines = nil
}

func initResultListView() {
	if app.list == 0 {
		return
	}
	send(app.list, LVM_SETEXTENDEDLISTVIEWSTYLE, 0, LVS_EX_FULLROWSELECT|LVS_EX_GRIDLINES|LVS_EX_DOUBLEBUFFER)
	send(app.list, LVM_SETBKCOLOR, 0, uintptr(rgb(24, 26, 31)))
	send(app.list, LVM_SETTEXTBKCOLOR, 0, uintptr(rgb(24, 26, 31)))
	send(app.list, LVM_SETTEXTCOLOR, 0, uintptr(rgb(229, 233, 240)))
	columns := []struct {
		title string
		width int32
	}{
		{"状态", 76},
		{"延迟", 78},
		{"人数", 78},
		{"地图", 150},
		{"地址", 164},
		{"服务器", 330},
		{"命中原因", 260},
	}
	for i, col := range columns {
		text := utf16(col.title)
		c := lvcolumn{
			Mask:    LVCF_TEXT | LVCF_WIDTH,
			Cx:      col.width,
			Text:    text,
			SubItem: int32(i),
		}
		send(app.list, LVM_INSERTCOLUMN, uintptr(i), uintptr(unsafe.Pointer(&c)))
	}
}

func applyResultRows(rows []ServerInfo) {
	if app.list == 0 {
		return
	}
	send(app.list, WM_SETREDRAW, 0, 0)
	send(app.list, LVM_DELETEALLITEMS, 0, 0)
	for i, row := range rows {
		insertResultRow(i, row)
	}
	send(app.list, WM_SETREDRAW, 1, 0)
}

func insertResultRow(index int, s ServerInfo) {
	cells := resultCells(s)
	if len(cells) == 0 {
		return
	}
	text := utf16(cells[0])
	item := lvitem{
		Mask:    LVIF_TEXT,
		Item:    int32(index),
		SubItem: 0,
		Text:    text,
	}
	send(app.list, LVM_INSERTITEM, 0, uintptr(unsafe.Pointer(&item)))
	for col := 1; col < len(cells); col++ {
		setResultCell(index, col, cells[col])
	}
}

func setResultCell(row, col int, value string) {
	text := utf16(value)
	item := lvitem{
		Mask:    LVIF_TEXT,
		Item:    int32(row),
		SubItem: int32(col),
		Text:    text,
	}
	send(app.list, LVM_SETITEMTEXT, uintptr(row), uintptr(unsafe.Pointer(&item)))
}

func updateResultSummary(shown, total, blocked int) {
	if app.resultMeta == 0 {
		return
	}
	mode := "全部结果"
	if onlyShowMatched() {
		mode = "仅命中"
	}
	text := fmt.Sprintf("%s    显示 %d / 总计 %d    命中 IP %d", mode, shown, total, blocked)
	procSetWindowText.Call(app.resultMeta, uintptr(unsafe.Pointer(utf16(text))))
}

func loadConfig() error {
	app.cfg = defaultConfig()
	path := filepath.Join(appDir(), "config.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, &app.cfg); err != nil {
		backupCorruptFile(path, err)
		saveConfig()
		return err
	}
	return nil
}

func saveConfig() {
	b, _ := json.MarshalIndent(app.cfg, "", "  ")
	_ = os.WriteFile(filepath.Join(appDir(), "config.json"), b, 0644)
}

func loadSavedResults() error {
	path := filepath.Join(appDir(), "scan_results.json")
	b, err := os.ReadFile(path)
	if err != nil {
		app.savedNameIPs = map[string]string{}
		return err
	}
	var rows []ServerInfo
	if err := json.Unmarshal(b, &rows); err != nil {
		app.savedNameIPs = map[string]string{}
		backupCorruptFile(path, err)
		_ = os.WriteFile(path, []byte("[]\n"), 0644)
		return err
	}
	for i := range rows {
		rows[i].IPChanged = false
		rows[i].PreviousAddress = ""
	}
	app.all = rows
	rebuildSavedNameIPs()
	return nil
}

func saveSavedResults() {
	if len(app.all) == 0 {
		return
	}
	rows := append([]ServerInfo(nil), app.all...)
	b, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		debugLog("save results marshal failed: " + err.Error())
		return
	}
	path := filepath.Join(appDir(), "scan_results.json")
	if err := os.WriteFile(path, b, 0644); err != nil {
		debugLog("save results write failed: " + err.Error())
		return
	}
	rebuildSavedNameIPs()
}

func backupCorruptFile(path string, cause error) {
	if path == "" {
		return
	}
	backup := fmt.Sprintf("%s.corrupt-%s.bak", path, time.Now().Format("20060102-150405"))
	if err := os.Rename(path, backup); err != nil {
		debugLog(fmt.Sprintf("backup corrupt file failed: %s: %v", path, err))
		return
	}
	debugLog(fmt.Sprintf("backed up corrupt file: %s -> %s: %v", path, backup, cause))
}

func appDir() string {
	exe, err := os.Executable()
	if err == nil {
		return filepath.Dir(exe)
	}
	wd, _ := os.Getwd()
	return wd
}

func debugLog(line string) {
	path := filepath.Join(appDir(), "startup.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = fmt.Fprintf(f, "%s %s\n", time.Now().Format("15:04:05.000"), line)
}

func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	var out []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func button(parent uintptr, text string, id int) uintptr {
	return child("BUTTON", text, WS_CHILD|WS_VISIBLE|WS_TABSTOP|BS_PUSHBUTTON, uintptr(id))
}

func child(class, title string, style uint32, id uintptr) uintptr {
	return childEx(0, class, title, style, id)
}

func childEx(exstyle uint32, class, title string, style uint32, id uintptr) uintptr {
	return createWindowEx(exstyle, class, title, style, 0, 0, 10, 10, app.hwnd, id, 0, 0)
}

func createWindowEx(exstyle uint32, class, title string, style uint32, x, y, w, h int32, parent, menu, inst, param uintptr) uintptr {
	r, _, _ := procCreateWindowEx.Call(uintptr(exstyle), uintptr(unsafe.Pointer(utf16(class))), uintptr(unsafe.Pointer(utf16(title))), uintptr(style), uintptr(x), uintptr(y), uintptr(w), uintptr(h), parent, menu, inst, param)
	return r
}

func insertColumn(hwnd uintptr, idx int32, text string, width int32) {
	t := utf16(text)
	col := lvcolumn{Mask: LVCF_TEXT | LVCF_WIDTH | LVCF_FMT, Cx: width, Text: t}
	send(hwnd, LVM_INSERTCOLUMN, uintptr(idx), uintptr(unsafe.Pointer(&col)))
}

func insertItem(hwnd uintptr, row, col int32, text string) {
	t := utf16(text)
	item := lvitem{Mask: LVIF_TEXT, Item: row, SubItem: col, Text: t}
	send(hwnd, LVM_INSERTITEM, 0, uintptr(unsafe.Pointer(&item)))
}

func setItem(hwnd uintptr, row, col int32, text string) {
	t := utf16(text)
	item := lvitem{Mask: LVIF_TEXT, Item: row, SubItem: col, Text: t}
	send(hwnd, LVM_SETITEM, 0, uintptr(unsafe.Pointer(&item)))
}

func move(hwnd uintptr, x, y, w, h int32) {
	procMoveWindow.Call(hwnd, uintptr(x), uintptr(y), uintptr(w), uintptr(h), 1)
}

func enable(hwnd uintptr, ok bool) {
	v := uintptr(0)
	if ok {
		v = 1
	}
	procEnableWindow.Call(hwnd, v)
}

func setStatus(s string) {
	procSetWindowText.Call(app.status, uintptr(unsafe.Pointer(utf16(s))))
}

func getText(hwnd uintptr) string {
	n, _, _ := procGetWindowTextLength.Call(hwnd)
	buf := make([]uint16, n+1)
	procGetWindowText.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), n+1)
	return syscall.UTF16ToString(buf)
}

func message(title, body string) {
	procMessageBox.Call(app.hwnd, uintptr(unsafe.Pointer(utf16(body))), uintptr(unsafe.Pointer(utf16(title))), 0)
}

func send(hwnd uintptr, msg uint32, w, l uintptr) uintptr {
	r, _, _ := procSendMessage.Call(hwnd, uintptr(msg), w, l)
	return r
}

func utf16(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

func loadCursor(id uintptr) uintptr {
	r, _, _ := procLoadCursor.Call(0, id)
	return r
}

func loadIcon(inst uintptr, id uintptr) uintptr {
	r, _, _ := procLoadIcon.Call(inst, id)
	return r
}

func createFont(name string, height int32, weight int32) uintptr {
	r, _, _ := procCreateFont.Call(uintptr(-height), 0, 0, 0, uintptr(weight), 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(utf16(name))))
	if r == 0 {
		r, _, _ = procGetStockObject.Call(17)
	}
	return r
}

func createSolidBrush(color uint32) uintptr {
	r, _, _ := procCreateSolidBrush.Call(uintptr(color))
	return r
}

func rgb(r, g, b byte) uint32 {
	return uint32(r) | uint32(g)<<8 | uint32(b)<<16
}

func releaseUIResources() {
	for _, h := range []uintptr{app.font, app.listFont, app.bgBrush, app.panelBrush, app.inputBrush, app.buttonBrush} {
		if h != 0 {
			procDeleteObject.Call(h)
		}
	}
	app.font = 0
	app.listFont = 0
	app.bgBrush = 0
	app.panelBrush = 0
	app.inputBrush = 0
	app.buttonBrush = 0
}
