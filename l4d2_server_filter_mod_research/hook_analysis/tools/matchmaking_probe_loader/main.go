package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	processCreateThread     = 0x0002
	processQueryInformation = 0x0400
	processVMOperation      = 0x0008
	processVMWrite          = 0x0020
	processVMRead           = 0x0010

	memCommit  = 0x1000
	memReserve = 0x2000

	pageReadWrite = 0x04

	th32csSnapProcess = 0x00000002
	maxPath           = 260
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
	ExeFile         [maxPath]uint16
}

var (
	kernel32                 = syscall.NewLazyDLL("kernel32.dll")
	procCreateToolhelp32Snap = kernel32.NewProc("CreateToolhelp32Snapshot")
	procProcess32FirstW      = kernel32.NewProc("Process32FirstW")
	procProcess32NextW       = kernel32.NewProc("Process32NextW")
	procOpenProcess          = kernel32.NewProc("OpenProcess")
	procVirtualAllocEx       = kernel32.NewProc("VirtualAllocEx")
	procWriteProcessMemory   = kernel32.NewProc("WriteProcessMemory")
	procCreateRemoteThread   = kernel32.NewProc("CreateRemoteThread")
	procWaitForSingleObject  = kernel32.NewProc("WaitForSingleObject")
	procGetExitCodeThread    = kernel32.NewProc("GetExitCodeThread")
	procCloseHandle          = kernel32.NewProc("CloseHandle")
	procGetModuleHandleW     = kernel32.NewProc("GetModuleHandleW")
	procGetProcAddress       = kernel32.NewProc("GetProcAddress")
)

func main() {
	processName := flag.String("process", "left4dead2.exe", "target process name")
	dllPath := flag.String("dll", filepath.Join("..", "matchmaking_probe_dll", "matchmaking_probe_v3.dll"), "DLL path to load")
	timeout := flag.Duration("timeout", 15*time.Second, "remote load wait timeout")
	flag.Parse()

	absDLL, err := filepath.Abs(*dllPath)
	if err != nil {
		fatalf("resolve dll path: %v", err)
	}
	if _, err := os.Stat(absDLL); err != nil {
		fatalf("dll not found: %s", absDLL)
	}

	pid, err := findProcessID(*processName)
	if err != nil {
		fatalf("%v", err)
	}
	fmt.Printf("target: %s pid=%d\n", *processName, pid)
	fmt.Printf("dll: %s\n", absDLL)

	if err := loadDLL(pid, absDLL, *timeout); err != nil {
		fatalf("load dll: %v", err)
	}
	fmt.Println("loaded. wait for matchmaking_probe.log next to the DLL.")
}

func findProcessID(name string) (uint32, error) {
	snap, _, err := procCreateToolhelp32Snap.Call(th32csSnapProcess, 0)
	if snap == uintptr(syscall.InvalidHandle) {
		return 0, fmt.Errorf("CreateToolhelp32Snapshot: %w", err)
	}
	defer closeHandle(snap)

	var entry processEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	ok, _, err := procProcess32FirstW.Call(snap, uintptr(unsafe.Pointer(&entry)))
	for ok != 0 {
		exe := syscall.UTF16ToString(entry.ExeFile[:])
		if strings.EqualFold(exe, name) {
			return entry.ProcessID, nil
		}
		ok, _, err = procProcess32NextW.Call(snap, uintptr(unsafe.Pointer(&entry)))
	}
	if err != syscall.ERROR_NO_MORE_FILES {
		return 0, fmt.Errorf("Process32NextW: %w", err)
	}
	return 0, fmt.Errorf("%s is not running", name)
}

func loadDLL(pid uint32, dllPath string, timeout time.Duration) error {
	access := uintptr(processCreateThread | processQueryInformation | processVMOperation | processVMWrite | processVMRead)
	process, _, err := procOpenProcess.Call(access, 0, uintptr(pid))
	if process == 0 {
		return fmt.Errorf("OpenProcess failed: %w", err)
	}
	defer closeHandle(process)

	wide, err := syscall.UTF16FromString(dllPath)
	if err != nil {
		return err
	}
	remoteBytes := uintptr(len(wide) * 2)
	remoteMem, _, err := procVirtualAllocEx.Call(process, 0, remoteBytes, memCommit|memReserve, pageReadWrite)
	if remoteMem == 0 {
		return fmt.Errorf("VirtualAllocEx failed: %w", err)
	}

	var written uintptr
	ok, _, err := procWriteProcessMemory.Call(
		process,
		remoteMem,
		uintptr(unsafe.Pointer(&wide[0])),
		remoteBytes,
		uintptr(unsafe.Pointer(&written)),
	)
	if ok == 0 || written != remoteBytes {
		return fmt.Errorf("WriteProcessMemory failed: wrote %d/%d: %w", written, remoteBytes, err)
	}

	loadLibrary, err := loadLibraryWAddress()
	if err != nil {
		return err
	}

	thread, _, err := procCreateRemoteThread.Call(process, 0, 0, loadLibrary, remoteMem, 0, 0)
	if thread == 0 {
		return fmt.Errorf("CreateRemoteThread failed: %w", err)
	}
	defer closeHandle(thread)

	waitMS := uint32(timeout / time.Millisecond)
	wait, _, err := procWaitForSingleObject.Call(thread, uintptr(waitMS))
	if wait != 0 {
		return fmt.Errorf("WaitForSingleObject returned 0x%X: %w", wait, err)
	}

	var exitCode uint32
	ok, _, err = procGetExitCodeThread.Call(thread, uintptr(unsafe.Pointer(&exitCode)))
	if ok == 0 {
		return fmt.Errorf("GetExitCodeThread failed: %w", err)
	}
	if exitCode == 0 {
		return errors.New("LoadLibraryW returned null")
	}
	return nil
}

func loadLibraryWAddress() (uintptr, error) {
	kernelName, _ := syscall.UTF16PtrFromString("kernel32.dll")
	module, _, err := procGetModuleHandleW.Call(uintptr(unsafe.Pointer(kernelName)))
	if module == 0 {
		return 0, fmt.Errorf("GetModuleHandleW(kernel32.dll): %w", err)
	}
	nameBytes := append([]byte("LoadLibraryW"), 0)
	addr, _, err := procGetProcAddress.Call(module, uintptr(unsafe.Pointer(&nameBytes[0])))
	if addr == 0 {
		return 0, fmt.Errorf("GetProcAddress(LoadLibraryW): %w", err)
	}
	return addr, nil
}

func closeHandle(handle uintptr) {
	if handle != 0 && handle != uintptr(syscall.InvalidHandle) {
		procCloseHandle.Call(handle)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
