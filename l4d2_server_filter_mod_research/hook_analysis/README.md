# Hook Analysis Workspace

This directory is the main workspace for the new hook-driven investigation.

Rules for this phase:

- All new research artifacts land under `l4d2_server_filter_mod_research/hook_analysis`.
- `L4D2_Server_Filter_Tool` can be referenced, but results and new tools stay here.
- First find the real server-list and matchmaking cut-in points.
- Do not put workshop/VPK fallback work ahead of the hook analysis.

Local game directory:

```text
E:\SteamLibrary\steamapps\common\Left 4 Dead 2
```

## Current Suspects

Initial file-name scan found these high-priority targets:

```text
E:\SteamLibrary\steamapps\common\Left 4 Dead 2\bin\serverbrowser.dll
E:\SteamLibrary\steamapps\common\Left 4 Dead 2\platform\servers\serverbrowser.dll
E:\SteamLibrary\steamapps\common\Left 4 Dead 2\left4dead2\bin\matchmaking.dll
E:\SteamLibrary\steamapps\common\Left 4 Dead 2\platform\servers\dialogserverbrowser.res
E:\SteamLibrary\steamapps\common\Left 4 Dead 2\platform\servers\addservergamespage.res
E:\SteamLibrary\steamapps\common\Left 4 Dead 2\platform\servers\serverbrowser_english.txt
E:\SteamLibrary\steamapps\common\Left 4 Dead 2\left4dead2\resource\matchsystem.360.res
```

## Workflow

1. Run the static probe to inventory candidate DLLs, resources, and strings.
2. Use the report to decide the smallest dynamic observation target.
3. Hook only the minimum candidate boundary needed to answer:
   - Where are server rows created?
   - Where is server name text assigned?
   - Where is connect/random-match candidate IP selected?
   - Is any filterable list exposed before UI rendering?
4. Record every result in `reports/`.

## Visual Module Capture

Use this first for dynamic evidence. It is a Win32 GUI wrapper around the module watcher.

```text
tools\module_watch_gui\L4D2ModuleWatchGUI.exe
```

Recommended flow:

1. Start `L4D2ModuleWatchGUI.exe`.
2. Click `Launch Game`, or start L4D2 manually.
3. Wait for the game main menu.
4. Click `Start Capture`.
5. Follow the current step shown in the window.
   - For group servers, stay on the main menu to let the background refresh run.
   - Open the group server list only to inspect visible results; return to the main menu and re-enter the list to see newer refresh results.
6. The tool writes:

```text
reports\module_watch_gui_report.md
```

The GUI keeps long-running work off the UI thread:

- module polling runs in a worker goroutine;
- UI controls are only updated through posted window messages;
- `Stop And Save` cancels the worker cooperatively;
- report writing happens after capture stops and does not block button handlers.

## Static Probe

```powershell
cd F:\Project\03_Game_Tools\L4D2_SERVER_TOOL\l4d2_server_filter_mod_research\hook_analysis\tools\static_probe
go run . -game "E:\SteamLibrary\steamapps\common\Left 4 Dead 2"
```
