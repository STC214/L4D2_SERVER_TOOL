# Matchmaking Probe DLL

This is an aggressive, read-only in-process probe for L4D2 group-server research.

It sets temporary software breakpoints on selected `matchmaking.dll` RVAs and logs
register/stack context when they are hit. It does not filter, skip, or modify
server rows yet.

## Targets

The probe watches:

- `matchmaking.dll + 0x211E0` group-server refresh entry
- `matchmaking.dll + 0x21570` server manager waiting for details
- `matchmaking.dll + 0x21B80` details consume/parse candidate
- `matchmaking.dll + 0x4EB20` details callback table pointer write
- `matchmaking.dll + 0x226B0` first function in the details callback table
- `matchmaking.dll + 0x22750` visible field extraction candidate
- `matchmaking.dll + 0x22CA0` QOS/details table neighbor
- `matchmaking.dll + 0x22B20` settings/member field table neighbor
- `matchmaking.dll + 0x22DC0` settings table neighbor
- `matchmaking.dll + 0x226F0` reserve/settings table neighbor
- `matchmaking.dll + 0x2D9D0` dedicated accept/reject path

These are from:

- `hook_analysis/aggressive_hook_targets.md`
- `hook_analysis/matchmaking_path_findings.md`
- `hook_analysis/reports/matchmaking_table_scout_report.md`

## Build

Use a 32-bit Visual Studio toolchain:

```bat
cd /d F:\Project\03_Game_Tools\L4D2_SERVER_TOOL\l4d2_server_filter_mod_research\hook_analysis\tools\matchmaking_probe_dll
build_msvc_x86.bat
```

From a normal PowerShell window after Visual Studio Build Tools is installed:

```powershell
cmd /s /c "call ""C:\Program Files (x86)\Microsoft Visual Studio\2022\BuildTools\VC\Auxiliary\Build\vcvarsall.bat"" x86 && build_msvc_x86.bat"
```

Important: L4D2 is a 32-bit process, so this DLL must be built as Win32/x86.

## Run

Do not load this DLL through x32dbg if you want automatic logging. x32dbg catches
the probe's breakpoint exceptions first, which can pause the game.

Preferred automatic-log flow:

```powershell
cd F:\Project\03_Game_Tools\L4D2_SERVER_TOOL\l4d2_server_filter_mod_research\hook_analysis\tools\matchmaking_probe_loader
powershell -ExecutionPolicy Bypass -File .\run_loader_admin.ps1
```

Current loader target:

```text
..\matchmaking_probe_dll\matchmaking_probe_v3.dll
```

If an older probe DLL was already loaded, restart L4D2 before loading v3. This
clears old software breakpoints from process memory.

Manual debugger-only flow:

1. Start L4D2 and wait for the main menu.
2. Use x32dbg only with `debugger_scripts\matchmaking_row_suppression_breakpoints.x32dbg.txt`.
3. Wait for the main menu Steam group-server refresh.
4. Open `matchmaking_probe.log` next to the DLL.

Useful signs in the log:

- Frequent hits on `group_refresh_entry` confirm the refresh entrypoint.
- Hits on `details_consume_candidate` or `details_fields_candidate` while group
  rows appear are the important ones.
- Each point has a hit cap and auto-disables after that cap, so the log should
  stay bounded during background group-server refresh.
- v2/v3 lower the high-frequency `details_consume_candidate` cap and add inner
  field-reference points around both `0x21B80` and `0x22750`.
- v3 is lighter than v2: low hit caps and nested pointer string checks only, to
  avoid freezing the game during group-server background refresh.
- `details_table_init_write` should show the table pointer path:
  - data slot `matchmaking.dll + 0x66104`
  - expected first slot pointer `matchmaking.dll + 0x54480`
- For `details_fields_candidate`, the log now includes:
  - `ECX`, `EAX`, `EDX`
  - stack values
  - EBP arguments
  - the current details-table pointer slots
- Stack/register values that decode as strings or KeyValues field names identify
  the hook signature for the next prototype.

After a run, summarize the log:

```powershell
cd F:\Project\03_Game_Tools\L4D2_SERVER_TOOL\l4d2_server_filter_mod_research\hook_analysis\tools\matchmaking_probe_log_analyzer
.\L4D2MatchmakingProbeLogAnalyzer.exe -log ..\matchmaking_probe_dll\matchmaking_probe.log
```

Then open:

```text
hook_analysis/reports/matchmaking_probe_log_report.md
```

## Safety Notes

- This is intentionally not Workshop-safe.
- This is not a VPK mod.
- This is for local analysis only and can trigger anti-cheat risk.
- First prototype is read-only. Do not add filtering behavior until the log proves
  which function receives the full server name/address data.
