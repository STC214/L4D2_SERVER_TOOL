# Implementation Plan

This plan keeps the current work useful while avoiding live-process work until the
offline pieces are stable.

## Stage 1: Offline Target Stability

Completed in this stage:

- `gamedata/matchmaking_targets.json`
  - Source-of-truth target table for `matchmaking.dll`.
  - Stores target names, roles, RVAs, byte patterns, masks, and expected hit count.
- `tools/gamedata_verify`
  - Reads the gamedata file.
  - Checks signatures against the local game DLL.
  - Writes `reports/gamedata_verify_report.md`.
- `tools/gamedata_verify_gui`
  - Dark Win32 GUI wrapper for the same offline verification logic.
  - Runs verification in a worker goroutine so button clicks do not block the UI.
  - Writes `reports/gamedata_verify_gui_report.md`.
- `tools/matchmaking_function_scout`
  - Builds an offline static function profile for each `matchmaking.dll` target.
  - Confirms `details_fields_candidate` (`matchmaking.dll + 0x22750`) is the best
    current path for visible group-server fields.
- `tools/matchmaking_table_scout`
  - Dumps `.rdata` function-pointer neighborhoods around gamedata targets.
  - Confirms `details_fields_candidate` is table-driven and sits beside other
    server detail pack/unpack callbacks.

Goal:

Before any runtime helper is used, verify that every target still has exactly one
match in the current game build.

## Stage 2: GUI Coordinator

Initial coordinator UI exists as `tools/gamedata_verify_gui`.

Next combined UI should still be a coordinator, not a heavy analyzer:

- Choose or confirm L4D2 directory.
- Run `gamedata_verify` in the background.
- Show target status without freezing the UI.
- Open the generated report.
- Show whether the current game build is compatible with the known target table.
- Keep existing packet filter actions separate from target verification.

Win32/Go constraints:

- UI thread owns controls.
- Worker goroutines run verification.
- Worker-to-UI communication uses posted messages or a bounded channel drained by
  the UI thread.
- No synchronous process execution on button handlers.
- No unbounded log append.
- Buttons reflect state: idle, running, done, failed.

## Stage 3: Runtime Research

Only after Stage 1 reports all targets as stable:

- Use manual debugger observation or a separate local probe to confirm function
  arguments.
- Prefer `details_fields_candidate` (`matchmaking.dll + 0x22750`) first, then
  `details_consume_candidate` (`matchmaking.dll + 0x21B80`).
- Treat `0x22750` as a dispatch-table callback:
  - `.rdata` pointer slot: `0x54484`
  - compact table span: `0x5447C` - `0x54494`
  - first function slot: `0x54480`
  - table pointer write site: `0x4EB26`
  - global/data slot storing the first function slot: `0x66104`
  - neighboring callbacks: `0x226B0`, `0x22CA0`, `0x22B20`, `0x22DC0`, `0x226F0`
  - likely row-relevant fields: `Server/name`, `Server/adronline`,
    `Server/adrlocal`, `Members/numSlots`, `Members/numPlayers`
- For `0x22750`, inspect the inner references around:
  - `0x227C8` `GameDetailsServer`
  - `0x22812` `Server/name`
  - `0x22846` `Server/adronline`
  - `0x2285D` `Server/adrlocal`
  - `0x228C5` `Members/numSlots`
  - `0x228FA` `Members/numPlayers`
- Keep packet filtering as the safety net for join prevention.
- Use `debugger_scripts/matchmaking_row_suppression_breakpoints.x32dbg.txt` for
  manual observation focused on row suppression.
- If using the probe DLL, build it from an x86 MSVC native tools prompt and use
  the updated log fields to decide whether `0x22750` already sees enough row
  data to filter before UI insertion.
- Use `tools/matchmaking_probe_loader` for the probe DLL automatic-log path.
  x32dbg should be reserved for manual breakpoint inspection only.
- After the first successful probe run, use v2 for the next run because the first
  run only hit `details_consume_candidate`. v2 reduces that high-frequency cap
  and adds inner field-reference breakpoints plus object memory string scanning.
- v2 confirmed the `0x21B80` internal path sees real server endpoints, but its
  deep scanning was too heavy. Use v3 for follow-up runs.
- v3 confirmed `0x21ECB` directly exposes `Server/connectstring` and `0x21FFF`
  follows with `server/update`. Treat this pair as the current first prototype
  path for hiding Steam group-server rows.
- De-prioritize `0x22750` for row suppression because v3 showed it firing on
  unrelated weapon fields.
- A first row-filter prototype now exists in `tools/matchmaking_row_filter_dll`.
  It skips the `server/update` block for configured connectstring substrings.
- Use `tools/matchmaking_probe_log_analyzer` after each probe run to summarize:
  - which observation points actually fired
  - whether `details_fields_candidate` exposed row-relevant fields
  - whether RPG/blocked-family keywords were visible in decoded strings

## Stage 4: Filtering Product Shape

Likely final shape:

- Config GUI for keywords, manual IPs, known server families, cache cleanup, and
  offline target verification.
- Packet filter for blocking joins and random matchmaking.
- Optional local research-only runtime component if group-server row suppression
  cannot be achieved through safer files/config/cache paths.
