# Probe Runbook

This is the next hands-on run for the aggressive path.

## Build

Open `x86 Native Tools Command Prompt for VS`, then run:

```bat
cd /d F:\Project\03_Game_Tools\L4D2_SERVER_TOOL\l4d2_server_filter_mod_research\hook_analysis\tools\matchmaking_probe_dll
build_msvc_x86.bat
```

Expected output:

`matchmaking_probe.dll`

Do not build this as x64. `left4dead2.exe` is 32-bit, so x64 DLL injection will fail.

## Run With x32dbg

1. Start L4D2 and wait at the main menu.
2. Attach x32dbg to `left4dead2.exe`.
3. Inject `matchmaking_probe.dll`.
4. Return focus to the game and wait for Steam group-server refresh.
5. Open:

`tools\matchmaking_probe_dll\matchmaking_probe.log`

## What To Capture

Useful lines look like:

- `hit details_consume_candidate`
- `hit details_fields_candidate`
- `ECX -> "..."`
- `[esp+0xNN] -> "..."`

The most useful capture is a hit where nearby strings include one or more of:

- `GameDetailsServer`
- `Server/name`
- `Server/adrlocal`
- `Server/adronline`
- `Members`
- a visible server name such as `星缘`, `破晓`, `RPG`, or `Left 4 Dead 2`
- a server address or `ip:port`

## Expected Decision

If `details_fields_candidate` (`matchmaking.dll + 0x22750`) sees complete server
name/address fields before the row appears, the next prototype should hook there
and skip/expire matched entries.

If `details_consume_candidate` (`matchmaking.dll + 0x21B80`) sees the raw
`GameDetailsServer` tree but `0x22750` does not, hook `0x21B80`.

If neither sees useful fields, inspect `serverbrowser.dll` as a secondary UI/list
insertion path.

