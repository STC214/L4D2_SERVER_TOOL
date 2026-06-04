package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	defaultGameDir = `E:\SteamLibrary\steamapps\common\Left 4 Dead 2`
	addonName      = "l4d2_server_filter"
	configRelPath  = `cfg\l4d2_server_filter\keywords.txt`
	exampleRelPath = `cfg\l4d2_server_filter\keywords.example.txt`

	WS_OVERLAPPEDWINDOW = 0x00CF0000
	WS_VISIBLE          = 0x10000000
	WS_CHILD            = 0x40000000
	WS_TABSTOP          = 0x00010000
	WS_BORDER           = 0x00800000
	WS_VSCROLL          = 0x00200000
	WS_EX_CLIENTEDGE    = 0x00000200
	ES_MULTILINE        = 0x0004
	ES_AUTOVSCROLL      = 0x0040
	ES_WANTRETURN       = 0x1000
	ES_AUTOHSCROLL      = 0x0080
	BS_PUSHBUTTON       = 0x00000000
	SS_LEFT             = 0x00000000

	SW_SHOW = 5

	WM_DESTROY     = 0x0002
	WM_SIZE        = 0x0005
	WM_COMMAND     = 0x0111
	WM_SETFONT     = 0x0030
	WM_SETTEXT     = 0x000C
	WM_GETTEXT     = 0x000D
	WM_GETTEXTLENG = 0x000E
	WM_APP         = 0x8000

	ID_GAME_DIR = 1001
	ID_KEYWORDS = 1002
	ID_LOAD     = 1003
	ID_SAVE     = 1004
	ID_INSTALL  = 1005

	WM_ASYNC_DONE = WM_APP + 1
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")

	procDefWindowProc       = user32.NewProc("DefWindowProcW")
	procDispatchMessage     = user32.NewProc("DispatchMessageW")
	procGetMessage          = user32.NewProc("GetMessageW")
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
	procGetWindowText       = user32.NewProc("GetWindowTextW")
	procGetWindowTextLength = user32.NewProc("GetWindowTextLengthW")
	procMoveWindow          = user32.NewProc("MoveWindow")
	procEnableWindow        = user32.NewProc("EnableWindow")
	procMessageBox          = user32.NewProc("MessageBoxW")
	procGetModuleHandle     = kernel32.NewProc("GetModuleHandleW")
	procGetStockObject      = gdi32.NewProc("GetStockObject")
)

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

type uiState struct {
	hwnd         uintptr
	gameEdit     uintptr
	keywordsEdit uintptr
	status       uintptr
	loadBtn      uintptr
	saveBtn      uintptr
	installBtn   uintptr
	font         uintptr
}

type asyncResult struct {
	title string
	text  string
	ok    bool
}

var app = &uiState{}

func main() {
	install := flag.Bool("install", false, "generate loose addon and write keywords")
	gameDir := flag.String("game", defaultGameDir, "Left 4 Dead 2 install directory")
	keywords := flag.String("keywords", "", "comma or newline separated keywords")
	keywordsFile := flag.String("keywords-file", "", "file containing keywords")
	flag.Parse()

	if *install {
		text, err := cliKeywordText(*keywords, *keywordsFile)
		if err != nil {
			fatal(err)
		}
		report, err := installAddon(context.Background(), *gameDir, text)
		if err != nil {
			fatal(err)
		}
		fmt.Println(report)
		return
	}

	runGUI()
}

func cliKeywordText(inline, file string) (string, error) {
	if file != "" {
		b, err := os.ReadFile(file)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	if inline == "" {
		return "", errors.New("use -keywords or -keywords-file with -install")
	}
	return strings.ReplaceAll(inline, ",", "\n"), nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

func runGUI() {
	hInstance := getModuleHandle()
	className := utf16Ptr("L4D2ServerFilterConfiguratorWindow")
	wc := wndclassex{
		Size:       uint32(unsafe.Sizeof(wndclassex{})),
		WndProc:    syscall.NewCallback(wndProc),
		Instance:   hInstance,
		Cursor:     loadCursor(32512),
		Background: 6,
		ClassName:  className,
	}
	procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))

	app.hwnd = createWindowEx(
		0,
		"L4D2ServerFilterConfiguratorWindow",
		"L4D2 Server Filter Configurator",
		WS_OVERLAPPEDWINDOW|WS_VISIBLE,
		100,
		100,
		820,
		600,
		0,
		0,
		hInstance,
	)
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
	case WM_DESTROY:
		procPostQuitMessage.Call(0)
		return 0
	case WM_SIZE:
		layout(hwnd)
		return 0
	case WM_COMMAND:
		switch loword(wParam) {
		case ID_LOAD:
			loadKeywords()
		case ID_SAVE:
			saveKeywords(false)
		case ID_INSTALL:
			saveKeywords(true)
		}
		return 0
	case WM_ASYNC_DONE:
		res := (*asyncResult)(unsafe.Pointer(lParam))
		setStatus(res.text)
		enableActions(true)
		if !res.ok {
			messageBox(res.title, res.text)
		}
		return 0
	}

	if app.gameEdit == 0 {
		createControls(hwnd)
	}
	ret, _, _ := procDefWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
	return ret
}

func createControls(hwnd uintptr) {
	app.font, _, _ = procGetStockObject.Call(17)

	createChild("STATIC", "Game directory:", SS_LEFT, 16, 18, 120, 22, hwnd, 0)
	app.gameEdit = createChild("EDIT", defaultGameDir, WS_BORDER|WS_TABSTOP|ES_AUTOHSCROLL, 136, 14, 520, 26, hwnd, ID_GAME_DIR)
	app.loadBtn = createChild("BUTTON", "Load", BS_PUSHBUTTON|WS_TABSTOP, 668, 14, 118, 28, hwnd, ID_LOAD)

	createChild("STATIC", "Keywords:", SS_LEFT, 16, 58, 120, 22, hwnd, 0)
	app.keywordsEdit = createChild("EDIT", defaultKeywords(), WS_BORDER|WS_TABSTOP|WS_VSCROLL|ES_MULTILINE|ES_AUTOVSCROLL|ES_WANTRETURN, 16, 84, 770, 380, hwnd, ID_KEYWORDS)

	app.saveBtn = createChild("BUTTON", "Save Keywords", BS_PUSHBUTTON|WS_TABSTOP, 16, 480, 150, 32, hwnd, ID_SAVE)
	app.installBtn = createChild("BUTTON", "Generate Addon", BS_PUSHBUTTON|WS_TABSTOP, 178, 480, 150, 32, hwnd, ID_INSTALL)
	app.status = createChild("STATIC", "Ready", SS_LEFT, 16, 528, 770, 24, hwnd, 0)
	layout(hwnd)
}

func createChild(className, text string, style uint32, x, y, w, height int32, parent uintptr, id uintptr) uintptr {
	ctrl := createWindowEx(WS_EX_CLIENTEDGE, className, text, WS_CHILD|WS_VISIBLE|style, x, y, w, height, parent, id, 0)
	if app.font != 0 {
		procSendMessage.Call(ctrl, WM_SETFONT, app.font, 1)
	}
	return ctrl
}

func layout(hwnd uintptr) {
	if app.gameEdit == 0 {
		return
	}
	width := int32(820)
	height := int32(600)
	_ = hwnd
	procMoveWindow.Call(app.gameEdit, 136, 14, uintptr(width-300), 26, 1)
	procMoveWindow.Call(app.loadBtn, uintptr(width-152), 14, 118, 28, 1)
	procMoveWindow.Call(app.keywordsEdit, 16, 84, uintptr(width-50), uintptr(height-220), 1)
	procMoveWindow.Call(app.saveBtn, 16, uintptr(height-110), 150, 32, 1)
	procMoveWindow.Call(app.installBtn, 178, uintptr(height-110), 150, 32, 1)
	procMoveWindow.Call(app.status, 16, uintptr(height-62), uintptr(width-50), 24, 1)
}

func loadKeywords() {
	gameDir := getText(app.gameEdit)
	path := keywordPath(gameDir)
	b, err := os.ReadFile(path)
	if err != nil {
		setStatus("No installed keywords found, using example keywords.")
		setText(app.keywordsEdit, defaultKeywords())
		return
	}
	setText(app.keywordsEdit, string(b))
	setStatus("Loaded " + path)
}

func saveKeywords(install bool) {
	gameDir := getText(app.gameEdit)
	text := getText(app.keywordsEdit)
	enableActions(false)
	setStatus("Working...")
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		var report string
		var err error
		if install {
			report, err = installAddon(ctx, gameDir, text)
		} else {
			report, err = saveKeywordFile(ctx, gameDir, text)
		}
		res := &asyncResult{title: "L4D2 Server Filter Configurator", ok: err == nil}
		if err != nil {
			res.text = err.Error()
		} else {
			res.text = report
		}
		procPostMessage.Call(app.hwnd, WM_ASYNC_DONE, 0, uintptr(unsafe.Pointer(res)))
	}()
}

func enableActions(enable bool) {
	v := uintptr(0)
	if enable {
		v = 1
	}
	procEnableWindow.Call(app.loadBtn, v)
	procEnableWindow.Call(app.saveBtn, v)
	procEnableWindow.Call(app.installBtn, v)
}

func installAddon(ctx context.Context, gameDir, keywordText string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	addonDir, err := safeAddonDir(gameDir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(addonDir, 0755); err != nil {
		return "", err
	}
	skeleton, err := skeletonDir()
	if err != nil {
		return "", err
	}
	if err := copyTree(skeleton, addonDir); err != nil {
		return "", err
	}
	report, err := saveKeywordFile(ctx, gameDir, keywordText)
	if err != nil {
		return "", err
	}
	return "Generated loose addon at " + addonDir + "\r\n" + report, nil
}

func saveKeywordFile(ctx context.Context, gameDir, keywordText string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	addonDir, err := safeAddonDir(gameDir)
	if err != nil {
		return "", err
	}
	keywords := normalizeKeywords(keywordText)
	if len(keywords) == 0 {
		return "", errors.New("at least one keyword is required")
	}
	target := filepath.Join(addonDir, configRelPath)
	if err := ensureInside(addonDir, target); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return "", err
	}
	content := strings.Join(keywords, "\r\n") + "\r\n"
	if err := atomicWriteWithBackup(target, []byte(content)); err != nil {
		return "", err
	}
	return fmt.Sprintf("Saved %d keyword(s) to %s", len(keywords), target), nil
}

func safeAddonDir(gameDir string) (string, error) {
	gameDir = strings.TrimSpace(gameDir)
	if gameDir == "" {
		return "", errors.New("game directory is empty")
	}
	clean := filepath.Clean(gameDir)
	if !strings.EqualFold(filepath.Base(clean), "Left 4 Dead 2") {
		return "", errors.New("game directory should end with Left 4 Dead 2")
	}
	return filepath.Join(clean, "left4dead2", "addons", addonName), nil
}

func keywordPath(gameDir string) string {
	addonDir, err := safeAddonDir(gameDir)
	if err != nil {
		return filepath.Join(gameDir, "left4dead2", "addons", addonName, configRelPath)
	}
	return filepath.Join(addonDir, configRelPath)
}

func normalizeKeywords(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.ReplaceAll(text, ",", "\n")
	seen := map[string]bool{}
	var out []string
	for _, line := range strings.Split(text, "\n") {
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

func atomicWriteWithBackup(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".keywords-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if _, err := os.ReadFile(tmpName); err != nil {
		os.Remove(tmpName)
		return err
	}
	if _, err := os.Stat(path); err == nil {
		backup := path + "." + time.Now().Format("20060102-150405") + ".bak"
		if err := copyFile(path, backup); err != nil {
			os.Remove(tmpName)
			return err
		}
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

func skeletonDir() (string, error) {
	exe, err := os.Executable()
	if err == nil {
		candidate := filepath.Clean(filepath.Join(filepath.Dir(exe), "..", "mod_skeleton"))
		if dirExists(candidate) {
			return candidate, nil
		}
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	candidates := []string{
		filepath.Clean(filepath.Join(cwd, "..", "mod_skeleton")),
		filepath.Clean(filepath.Join(cwd, "mod_skeleton")),
	}
	for _, candidate := range candidates {
		if dirExists(candidate) {
			return candidate, nil
		}
	}
	return "", errors.New("mod_skeleton directory was not found")
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if err := ensureInside(dst, target); err != nil {
			return err
		}
		if info.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func ensureInside(root, target string) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil {
		return err
	}
	if rel == "." || (!strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && rel != ".." && !filepath.IsAbs(rel)) {
		return nil
	}
	return fmt.Errorf("target path escapes addon directory: %s", target)
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func defaultKeywords() string {
	example, err := os.ReadFile(filepath.Join("..", "mod_skeleton", exampleRelPath))
	if err == nil {
		return string(example)
	}
	return "rpg\r\nvip\r\nshop\r\n"
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

func setStatus(text string) {
	if app.status != 0 {
		setText(app.status, text)
	}
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

func utf16Ptr(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

func loword(v uintptr) uintptr {
	return v & 0xffff
}
