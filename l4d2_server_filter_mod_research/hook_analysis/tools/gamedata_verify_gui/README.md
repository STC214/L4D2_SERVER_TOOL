# L4D2 Gamedata Verify GUI

Small dark Win32 GUI for checking whether the current local L4D2
`matchmaking.dll` still matches our known group-server research targets.

It is read-only:

- no packet capture
- no process attach
- no injection
- no admin requirement

## Run

Double-click:

`L4D2GamedataVerifyGUI.exe`

Defaults:

- Game directory: `E:\SteamLibrary\steamapps\common\Left 4 Dead 2`
- Gamedata: `..\..\gamedata\matchmaking_targets.json`
- Report: `..\..\reports\gamedata_verify_gui_report.md`

Click `Verify`.

Expected result for the currently scanned build:

- `group_refresh_entry`: `1`
- `manager_wait_details`: `1`
- `details_consume_candidate`: `1`
- `details_fields_candidate`: `1`
- `dedicated_accept_reject`: `1`

All five should be `OK`.

## How It Fits

Use this before deeper runtime research:

1. Run this GUI.
2. Confirm all targets are stable.
3. Then use the packet filter GUI normally.
4. Only if group-server rows still need deeper suppression, continue with manual
   debugger/probe work using the stable target table.

The later combined UI can reuse the same logic and report format.

## Build

```powershell
go build -ldflags="-H=windowsgui" -o L4D2GamedataVerifyGUI.exe .
```

