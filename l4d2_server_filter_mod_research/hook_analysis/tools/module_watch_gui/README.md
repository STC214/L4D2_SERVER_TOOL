# L4D2 Module Watch GUI

This Win32 helper visualizes the current hook-analysis collection step.

It does not inject into the game. It watches `left4dead2.exe` modules, TCP endpoints, UDP local sockets, WinDivert packet-level remote endpoints, and writes a markdown report.

## Run

For packet-level capture, run the built exe. It embeds a `requireAdministrator` manifest, so Windows should show a UAC prompt automatically:

```text
L4D2ModuleWatchGUI.exe
```

`WinDivert.dll` and `WinDivert64.sys` must be in the same directory as the exe.

Development run:

```powershell
cd F:\Project\03_Game_Tools\L4D2_SERVER_TOOL\l4d2_server_filter_mod_research\hook_analysis\tools\module_watch_gui
go run .
```

Release build:

```powershell
go build -ldflags "-H=windowsgui" -o L4D2ModuleWatchGUI.exe .
```

The `-H=windowsgui` linker flag is required so double-clicking the GUI does not open an extra console window.

## Workflow

1. Click `Launch Game` or launch L4D2 manually.
2. Click `Start Capture`.
3. Follow the highlighted phase shown in the window:
   - Main menu
   - Group background refresh on the main menu
   - Group list inspection after leaving and re-entering the list
   - Classic server browser
   - Quick/random match
4. Click `Stop And Save` if you finish early.

The report is written to:

```text
l4d2_server_filter_mod_research\hook_analysis\reports\module_watch_gui_report.md
```

The report includes:

- module observations;
- TCP owner-table endpoints;
- UDP owner-table local sockets;
- WinDivert packet observations with remote UDP/TCP endpoints and payload hints;
- full binary KeyValues field summaries for `InetSearchServerDetails` / `GameDetailsServer` samples when they are detected.

If the packet section is empty, confirm the UAC prompt was accepted and `WinDivert.dll` / `WinDivert64.sys` are beside the exe.

The UI intentionally does not print every packet in real time. Packet capture runs continuously in the backend, while a Win32 timer refreshes low-frequency counters and the latest summary once per second. Full packet observations are written to the final report.

## UI Freeze Avoidance

- The UI thread only handles Win32 messages and control updates.
- Process/module polling runs in one background goroutine.
- TCP/UDP table snapshots run in the same worker goroutine.
- WinDivert packet capture runs in worker goroutines and is cancelled by handle shutdown.
- High-frequency packet events are aggregated in memory and written to the final report instead of being appended to the UI log one by one.
- The status line is refreshed by a UI-thread `SetTimer`, so the window still shows progress even when no new log line is appended.
- The worker never touches Win32 controls directly.
- Worker events are delivered through a mutex-protected queue and `PostMessage` notification.
- Stop is cooperative through `context.CancelFunc`.

See `win32_stability_checklist.md` for the full crash/freeze checklist.
