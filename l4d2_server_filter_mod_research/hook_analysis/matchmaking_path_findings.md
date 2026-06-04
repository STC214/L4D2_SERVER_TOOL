# Matchmaking.dll Path Findings

Generated from:

- `tools/signature_scout`
- `tools/gamedata_verify`
- `tools/matchmaking_function_scout`

Latest reports:

- `reports/gamedata_verify_combined_report.md`
- `reports/matchmaking_function_scout_report.md`
- `reports/matchmaking_table_scout_report.md`

## Current Best Target

Best current target:

`matchmaking.dll + 0x22750` (`details_fields_candidate`)

Why:

- It directly references `GameDetailsServer`.
- It directly extracts visible fields:
  - `Server/name`
  - `Server/server`
  - `Server/adronline`
  - `Server/adrlocal`
  - `Members/numSlots`
  - `Members/numPlayers`
- It appears to build or normalize a public server details tree:
  - `System { network LIVE access public }`
  - `Server { name = server = adronline = adrlocal = }`
  - `Members { numSlots ... numPlayers ... }`

This is the most promising point for deciding whether a Steam group-server row
should be allowed to become visible.

Additional table evidence:

- `0x22750` has no direct `call rel32` caller in the current static scan.
- It is referenced as a 32-bit VA pointer in `.rdata`:
  - pointer slot RVA: `0x54484`
  - file offset: `0x53684`
- Neighboring table entries also look like server detail pack/unpack callbacks:
  - `0x226B0`
  - `0x22CA0`
  - `0x22B20`
  - `0x22DC0`
  - `0x226F0`
- `0x22B20`, `0x22DC0`, and `0x226F0` reference `Settings`, `Members`,
  `numSlots`, `numPlayers`, and/or `reserve`.
- The compact callback span appears to be:
  - descriptor/data pointer slot: `0x5447C`
  - first function slot: `0x54480`
  - `details_fields_candidate`: `0x54484`
  - last observed function slot in this compact group: `0x54494`
- The first function slot `0x54480` is referenced by:
  - code around `0x4EB26`, which stores `0x10054480` into global/data slot
    `0x10066104`
  - data slot `0x66104`, which currently contains `0x10054480`
- The `0x22750` slot itself (`0x54484`) has no direct address xref, which means
  the group is likely consumed from the first slot or a nearby descriptor rather
  than by addressing `0x22750` individually.

Interpretation:

`0x22750` is probably invoked through an interface or dispatch table rather than
through a normal direct call. That matches a callback-style server-detail path.
For the goal of hiding Steam group-server rows, this strengthens `0x22750` as
the first runtime observation point.

The new initialization clue makes `0x4EB26` and data slot `0x66104` useful
secondary observation points for understanding how the callback table is
registered. They are not better filtering points yet; they identify the table
owner path.

## Important Inner RVAs

Within `details_fields_candidate`:

| RVA | Evidence |
|---:|---|
| `0x22750` | Function entry. |
| `0x22760` | Reads `[ebp+0x08]`, likely first explicit argument. |
| `0x227C8` | Pushes `GameDetailsServer`. |
| `0x22812` | Pushes `Server/name`. |
| `0x22846` | Pushes `Server/adronline`. |
| `0x2285D` | Pushes `Server/adrlocal`. |
| `0x228C5` | Pushes `Members/numSlots`. |
| `0x228FA` | Pushes `Members/numPlayers`. |
| `0x229B3` | Reads `[ebp+0x10]`. |
| `0x229B6` | Reads `[ebp+0x14]`. |
| `0x22C49` | Reads `[ebp+0x0C]`. |

These are the best observation points for manual debugger inspection.

## Secondary Target

Secondary target:

`matchmaking.dll + 0x21B80` (`details_consume_candidate`)

Why:

- It consumes/parses raw detail replies.
- It references:
  - `GameDetailsServer`
  - `InetSearchServerDetails/pingxuid`
  - `InetSearchServerDetails/timestamp`
  - `Server/adronline`
  - `Server/connectstring`
  - `Server/ping`
  - `OnMatchServerMgrUpdate`

This likely sits earlier in the pipeline than `0x22750`. It may be useful if
`0x22750` only sees already accepted/normalized data, but it is probably riskier
to modify because it touches manager update flow.

Table evidence:

- `0x21B80` has no direct `call rel32` caller in the current static scan.
- It is referenced as a 32-bit VA pointer in `.rdata`:
  - pointer slot RVA: `0x53FA4`
  - file offset: `0x531A4`
- Neighboring entries include functions around the server manager refresh/update
  path, including references to:
  - `Server manager refreshing...`
  - `searchstarted`
  - `OnMatchServerMgrUpdate`

Interpretation:

This is probably an earlier manager/detail-response callback table. It is useful
for confirming where detail replies enter the manager, but it is still secondary
for row hiding because it may affect more of the refresh state machine.

## Lower Priority

`matchmaking.dll + 0x2D9D0` remains useful for random/dedicated search behavior:

- `registered dedicated server`
- `rejected dedicated server ... ping`
- `rejected dedicated server ... ip filter`

It is not the first choice for hiding Steam group-server rows.

## Next Manual Observation

Use debugger observation only after `Verify Targets` reports all targets as OK.

Priority breakpoints:

```text
matchmaking.dll + 0x4EB26
matchmaking.dll + 0x66104
matchmaking.dll + 0x54480
matchmaking.dll + 0x22750
matchmaking.dll + 0x227C8
matchmaking.dll + 0x22812
matchmaking.dll + 0x22846
matchmaking.dll + 0x2285D
matchmaking.dll + 0x228C5
matchmaking.dll + 0x228FA
```

Inspect:

- `ECX`
- `[ESP]` to `[ESP+40]`
- `[EBP+08]`
- `[EBP+0C]`
- `[EBP+10]`
- `[EBP+14]`
- Any pointer that contains `GameDetailsServer`, `Server/name`, a server name, or
  an `ip:port`.

The next practical question is:

Can `0x22750` see enough name/address/player data before the group-server row is
inserted or accepted?

If yes, future filtering should be attached around this function or just after it
extracts visible fields. If no, move one step earlier to `0x21B80`.

## Current Direction

Do not treat the packet filter as the final answer for this branch. It can stop
joins, but it does not remove rows from the Steam group-server list.

The current row-suppression path is:

1. Verify signatures with `Verify Targets`.
2. Observe `0x22750` first.
3. Confirm whether the function sees:
   - server display name
   - online/local address
   - visible player count
   - max player count
4. If these fields are available before insertion, filter at or immediately after
   this details-field callback.
5. If not, observe `0x21B80` as the earlier detail-response callback.

Additional table-owner path:

- If `0x22750` sees the right fields but the return point is unclear, inspect the
  callback span owner:
  - first function slot: `0x54480`
  - initialization/write site: `0x4EB26`
  - global/data slot: `0x66104`
- The goal of this inspection is only to find where the callback result is
  accepted into the group-server list, not to change the random-match packet
  blocking path.

## Updated Observation Assets

- `debugger_scripts/matchmaking_row_suppression_breakpoints.x32dbg.txt`
  - Adds breakpoints for the details callback table owner path.
  - Keeps `0x22750` as the main visible-field observation point.
- `tools/matchmaking_probe_dll/matchmaking_probe.cpp`
  - Adds read-only logging for:
    - `0x4EB20`
    - `0x226B0`
    - `0x22750`
    - `0x22CA0`
    - `0x22B20`
    - `0x22DC0`
    - `0x226F0`
  - Logs EBP arguments and details-table pointer slots.
  - Uses per-point hit caps and auto-disable behavior so background refresh does
    not create unbounded logs.
  - v2 adds inner observation points:
    - `0x21BEE`
    - `0x21D94`
    - `0x21ECB`
    - `0x21FFF`
    - `0x22812`
    - `0x22846`
    - `0x2285D`
    - `0x228C5`
    - `0x228FA`
  - v2 scans object-adjacent memory and nested pointers for readable strings.
- `tools/matchmaking_probe_loader`
  - 32-bit local loader for `matchmaking_probe.dll`.
  - Use this for automatic log generation.
  - Do not load the probe DLL through x32dbg for automatic logs, because x32dbg
    receives breakpoint exceptions before the probe's VEH handler.
  - Current loader target is `matchmaking_probe_v2.dll`; restart L4D2 before
    loading v2 if an older probe DLL has already been loaded.
- `tools/matchmaking_probe_log_analyzer`
  - Parses `matchmaking_probe.log`.
  - Generates `reports/matchmaking_probe_log_report.md`.
  - Summarizes hit counts, decoded text samples, and row-relevant keywords.

Build note:

The probe DLL needs an x86 MSVC environment. Run `build_msvc_x86.bat` from a
Visual Studio x86 native tools command prompt.

## Probe Run Notes

### Run 1

- DLL: `matchmaking_probe.dll`
- Result:
  - Only `details_consume_candidate` fired.
  - `0x22750` and neighbor callbacks did not fire.
  - Direct strings were mostly not useful.

### Run 2

- DLL: `matchmaking_probe_v2.dll`
- Result:
  - Internal `0x21B80` field-reference points fired:
    - `consume_GameDetailsServer_ref`
    - `consume_Server_adronline_ref`
    - `consume_Server_connectstring_ref`
    - `consume_mgr_update_ref`
  - The log showed real server endpoint strings, including examples such as:
    - `183.216.53.181:27060`
    - `183.216.53.181:27065`
  - `consume_mgr_update_ref` showed `GameDetailsServer` in a nearby object.
  - `details_fields_candidate` / `0x22750` still did not fire.
  - v2 object scanning was too heavy and could freeze the game.

Interpretation:

The earlier `0x21B80` path is confirmed to see server-detail payloads and
endpoints. The current group-server row suppression path should now bias toward
the `0x21B80` consume/update region unless a lighter follow-up run catches
`0x22750`.

### Next Run

- DLL: `matchmaking_probe_v3.dll`
- Loader target: v3.
- Changes:
  - Lower hit caps.
  - Fewer probe points.
  - No broad inline memory scan.
  - Keeps nested pointer string checks for endpoint evidence.

Restart L4D2 before loading v3, because older probe DLLs leave software
breakpoints in the current process memory.

### Run 3

- DLL: `matchmaking_probe_v3.dll`
- Result:
  - The run completed with bounded logs.
  - `consume_Server_connectstring_ref` (`0x21ECB`) directly exposed server
    connect strings in `EAX` and `[esp+0]`.
  - Examples observed:
    - `118.25.230.120:28888`
    - `183.216.53.181:27039`
    - `183.216.53.181:27056`
    - `183.216.53.181:27060`
    - `183.216.53.181:27065`
  - `consume_mgr_update_ref` (`0x21FFF`) followed the same processing batch and
    showed `server` / `update`.
  - `details_fields_candidate` (`0x22750`) did fire, but only showed weapon
    fields such as `weapon_smg`, `weapon_hunting_rifle`, and
    `weapon_sniper_scout`. It is not the Steam group-server row path.

Updated interpretation:

`0x22750` is no longer the primary row-suppression target for Steam group
servers. It is a generic details/settings callback that can fire for unrelated
game data. The best current target is the `0x21B80` internal path:

- read/filter candidate: `0x21ECB` (`Server/connectstring`)
- update emission point: `0x21FFF` (`OnMatchServerMgrUpdate`, `server`, `update`)

First practical row-hiding prototype should try to suppress or skip the
`server/update` emission for blocked connect strings observed at `0x21ECB`.

## Row Filter Prototype

Prototype added:

- `tools/matchmaking_row_filter_dll/matchmaking_row_filter.cpp`
- `tools/matchmaking_row_filter_dll/matchmaking_row_filter.dll`
- `tools/matchmaking_row_filter_dll/blocked_connectstrings.txt`

Current behavior:

- Breaks at `0x21ECB` and reads the current `Server/connectstring` from `EAX`.
- If the connect string contains a configured blocked substring, marks the
  current thread for skip.
- Breaks at `0x21FE7`; if the current thread is marked, jumps to `0x22039` to
  skip the `server/update` creation/dispatch block.
- Writes `matchmaking_row_filter.log` next to the DLL.

Current default blocked substrings:

- `183.216.53.181`
- `118.25.230.120:28888`
- `118.25.230.120:33333`

This is a research-only local runtime prototype, not a VPK/Workshop-safe mod.

### Row Filter Run 1

- DLL: `matchmaking_row_filter.dll`
- Config loaded:
  - `183.216.53.181`
  - `118.25.230.120:28888`
  - `118.25.230.120:33333`
- Result:
  - The DLL loaded successfully.
  - Breakpoints armed at `0x21ECB` and `0x21FE7`.
  - The prototype matched blocked connect strings and skipped the corresponding
    `server/update` emission.
  - Observed summary from `matchmaking_row_filter.log`:
    - `match #182` through `match #240`
    - `skipped server/update #1` through `#54`
  - Matched examples:
    - `118.25.230.120:33333`
    - `118.25.230.120:28888`
    - `183.216.53.181:27064`
    - `183.216.53.181:27039`
    - `183.216.53.181:27066`

Interpretation:

The current runtime prototype is no longer only blocking joins. It is reaching
the group-server row update path and suppressing the `server/update` messages
for configured connect strings. If blocked rows still appear visually, the next
thing to distinguish is stale UI/cache rows versus a second display path that
does not pass through this exact `server/update` block.

### Row Filter Prototype Update

The prototype now also observes `matchmaking.dll + 0x211E0`, the group-server
refresh entry. New log markers:

- `group refresh #... begin`: confirms a new background refresh batch started.
- `new blocked connectstring`: records each unique blocked endpoint seen during
  the run.

Next validation should restart L4D2, load the updated DLL before waiting for the
main-menu refresh, then inspect only these key lines:

```powershell
Select-String matchmaking_row_filter.log -Pattern "loaded|armed|group refresh|new blocked|match|skipped|error|failed"
```

If blocked rows still remain visible after a fresh restart and a refresh batch
with matching `skipped server/update` lines, that points to either already
materialized UI/cache rows or another row insertion path after
`OnMatchServerMgrUpdate`.

### Row Filter Run 2

- DLL: `matchmaking_row_filter.dll`
- Result:
  - The DLL loaded and armed `0x211E0`, `0x21ECB`, and `0x21FE7`.
  - It observed 54 unique/skipped blocked detail responses.
  - The visible Steam group-server rows for Xingyuan/Poxiao still remained.
  - No `group refresh #... begin` marker appeared, so `0x211E0` was not a
    reliable runtime batch boundary for this specific run.

Interpretation:

Skipping only the late `server/update` block is not enough to remove these rows.
The rows are probably accepted into an internal candidate/UI list before
`0x21FE7`, or a separate display path is inserting them.

Prototype update after this run:

- When a blocked `Server/connectstring` is observed at `0x21ECB`, the DLL now
  jumps directly to the `details_consume_candidate` cleanup path at `0x2203B`.
- The new log marker is `early skipped detail consume`.
- This tests whether suppressing the rest of `0x21B80` after connectstring
  extraction prevents the visible row from being materialized.

### Row Filter Run 3 Crash

- Result:
  - The DLL loaded and armed successfully.
  - First blocked endpoint:
    - `118.25.230.120:28888`
  - The game crashed immediately after `early skipped detail consume #1`.

Cause:

`0x22039` is safe for the older late skip because the object used by the nearby
call has already been prepared. It is not safe for the early skip from
`0x21ECB`; jumping there too early can execute a call with state that has not
been initialized by the skipped code.

Fix:

- Early skip target changed from `0x22039` to `0x2203B`.
- `0x2203B` enters the plain cleanup sequence (`pop edi`, `pop esi`, `pop ebx`,
  stack-cookie check, `ret 4`) and avoids the unsafe call.
- Late `server/update` skip still uses `0x22039`.

### Row Filter Run 4 Crash

- Result:
  - The DLL loaded and armed successfully.
  - First blocked endpoint:
    - `183.216.53.181:27064`
  - The game crashed immediately after
    `early skipped detail consume #1 target=0x2203B`.

Interpretation:

Jumping from `0x21ECB` directly to the function cleanup path is not viable. The
problem is not only the exact cleanup address; `0x21ECB` is too deep inside a
function whose state does not tolerate skipping the remaining body this way.

Stabilization:

- `early_consume_skip` is now disabled by default.
- The DLL defaults back to `late_update_skip`, which was stable but did not hide
  already materialized rows.
- Optional `row_filter_mode.txt` can enable early mode only for controlled
  research, but it is currently marked crash-prone.

Next direction:

Stop trying to hard-jump from inside `0x21B80`. The next useful path is either:

- hook a safer callback/table boundary before `0x21B80` starts consuming
  `GameDetailsServer`; or
- identify the internal candidate/list insertion path that runs before
  `server/update`, then suppress/remove rows there.

### Row Filter Prototype Update 2

The DLL now observes `matchmaking.dll + 0x21B80`
(`details_consume_candidate`) entry without changing execution. For each thread,
it records:

- entry sequence number;
- entry `ECX`;
- entry return address from `[ESP]`;
- first stack argument from `[ESP+4]`.

When a blocked `Server/connectstring` is later seen at `0x21ECB`, the match log
now includes:

- `entry_seq`
- `entry_ecx`
- `entry_ret`
- `entry_arg0`

Purpose:

This should show whether a blocked response can be identified at the safer
`0x21B80` boundary before the function body reaches `Server/connectstring`.
Default behavior remains `late_update_skip`; crash-prone early skip is disabled
unless explicitly enabled through `row_filter_mode.txt`.

### Row Filter Run 5

- Result:
  - The DLL loaded and did not crash.
  - Mode was `late_update_skip`.
  - `0x21B80`, `0x21ECB`, and `0x21FE7` were armed.
  - `g_seen` was already `511` before group-refresh markers appeared.
  - No configured blocked connect strings were matched.

Interpretation:

This run was stable, but the current blocked list did not cover the visible
Xingyuan/Poxiao rows in this session. The next build logs up to 128 unique
observed connect strings as `seen connectstring`, so changing IP/port families
can be copied into `blocked_connectstrings.txt` without a broad log dump.

### Keyword-Based Filtering Update

IP/port matching is only a temporary diagnostic tool because the unwanted RPG
entry servers can move across addresses. The row-filter DLL now also loads:

- `tools/matchmaking_row_filter_dll/blocked_keywords.txt`

Default keywords:

- `rpg`

Keyword matching is ASCII case-insensitive, so `rpg` matches `RPG`, `Rpg`, and
other ASCII case combinations.

Behavior:

- At `0x21B80`, the DLL performs a bounded scan around the entry `ECX` object and
  first stack argument.
- If a keyword is found, the entry context is marked as keyword-blocked.
- When the same thread reaches `0x21ECB`, the later connectstring event is
  blocked with `reason=keyword`.

This keeps the matching rule stable while avoiding a broad memory scan. The next
run should verify whether the visible group-server names are present in this
entry object path. If no `entry keyword hit` appears, the server name is likely
stored in a different object/path and the keyword scan target must move.

### Keyword Run 1

- Result:
  - Keyword matching worked.
  - Many entries produced `entry keyword hit ... keyword=RPG`.
  - Many connectstrings were matched with `reason=keyword`.
  - The DLL skipped the corresponding late `server/update` events.
  - Visible RPG rows still remained in the Steam group-server list.

Interpretation:

The stable keyword signal is available in the `0x21B80` detail path, but the
current suppression point is still too late for hiding rows already accepted by
the group-server UI/list. The next target should move before row materialization
rather than widening IP or keyword rules.

### Keyword Run 2

- Result:
  - `rpg` keyword matching worked case-insensitively.
  - Many rows matched with `reason=keyword keyword=rpg`.
  - The DLL skipped late `server/update` events up through at least `#192`.
  - Visible RPG rows still remained in the Steam group-server list.

Interpretation:

This confirms the matching side is solved. The remaining problem is locating the
row materialization/insert path. Late `server/update` suppression only blocks a
follow-up update event, not the row that is already in the group-server list.

Prototype update:

- Added diagnostic-only breakpoints for nearby callback-table functions:
  - `0x219D0` (`table_update_callback`)
  - `0x22120` (`table_slot2_callback`)
- New log marker:
  - `table probe`
- Each probe logs a bounded sample of `ECX`, return address, first two stack
  arguments, and whether nearby memory contains the current keyword.

Purpose:

Determine whether one of the neighboring table callbacks is a safer earlier
boundary for row insertion or refresh state changes.

### Table Probe Run 1

- Result:
  - `table_slot2_callback` fired in batches with index-like `arg1` values.
  - The table probe samples did not contain `rpg`.
  - Later `0x21B80` details still contained `rpg` and matched normally.
  - Late `server/update` skips reached at least `#236`, but visible RPG rows
    remained.

Interpretation:

`0x22120` looks like an earlier detail-request/index callback rather than a
point that already has server-name keywords. It is useful for understanding the
refresh pipeline, but not enough by itself for keyword filtering.

Prototype update:

- Default mode changed to `neutralize_keyword_then_late_skip`.
- When `rpg` is found near the `0x21B80` detail object, the DLL now replaces the
  matched ASCII bytes in mutable, non-image memory with underscores before the
  normal parser continues.
- The old late `server/update` skip remains as a fallback.
- New log fields:
  - `neutralized=...`
  - `total_neutralized=...`

Purpose:

This tests whether the visible group-server row text is sourced from the same
`GameDetailsServer` detail object. If visible `RPG` text changes to `___` but
the row remains, the next step is a row-removal path. If nothing changes, the
visible row text comes from an even earlier/source object outside this detail
parser.

### Neutralization Run 1

- User-observed result:
  - Some visible `RPG` labels changed to `___`.
  - Some visible `RPG` labels remained unchanged.
- Log result:
  - `neutralized` counts were positive.
  - `total_neutralized` reached at least `150`.

Interpretation:

The visible list uses at least some strings from the `0x21B80`
`GameDetailsServer` detail object. Remaining visible `RPG` labels likely come
from duplicate string copies, cached row labels, or another nearby object layer.

Prototype update:

- A two-level recursive neutralization walk with global address de-duplication was
  tried after this run.
- Regression result:
  - User observed all RPG rows displayed normally again; even the prior
    `RPG` -> `___` partial effect disappeared.
  - The log still showed many `entry keyword hit ... keyword=rpg` rows.
  - `total_neutralized` stalled at `52`, with later hits reporting zero new
    replacements.

Interpretation:

The keyword detector still worked, but global address de-duplication suppressed
later replacement opportunities. The recursive walk is therefore too broad for
this unstable object graph.

Current prototype update:

- Keyword neutralization now uses a bounded first-level scan only:
  - scan the root detail object memory;
  - scan first-level pointer fields;
  - do not keep a global visited-address table.
- Log fields now split replacements into:
  - `neutralized_ecx=...`
  - `neutralized_arg0=...`
  - `total_neutralized=...`

Expected next test:

This should restore the prior partial visible `RPG` -> `___` effect while giving
enough log detail to see whether remaining visible RPG text is sourced from
`ECX`, first stack argument, cached UI rows, or another earlier object.

### Neutralization Diagnostic Update

The row-filter DLL now annotates each `entry keyword hit` with the first keyword
copy location before neutralization:

- `source=ecx|arg0`
- `field=...`
- `direct=0|1`
- `ptr=0x...`

Purpose:

The last successful run showed all positive replacements came from `arg0`, not
`ECX`. The next run should identify which `arg0` field indexes contain RPG text.
If unchanged visible rows use the same field indexes, the missing behavior is
likely UI/cache materialization. If they use different indexes or do not produce
field hits, the next filter target should move to another copy/source path.

### Hot-Field Second-Level Experiment

The row-filter DLL now adds a controlled second-level neutralization pass under
only the high-frequency `arg0` fields observed in the diagnostic run:

- `8`
- `13`
- `25`
- `37`
- `49`
- `61`
- `73`
- `97`
- `109`
- `121`

This is deliberately not a recursive/global walk. Each hot field scans up to 32
subfields with bounded 1024-byte mutable-range checks.

New log fields:

- `neutralized_arg0_deep=...`
- `deep=...`, for example `f8.s3=1;`

Expected next test:

If `neutralized_arg0_deep` becomes positive and more visible RPG labels change,
then the remaining text is still near the `0x21B80` detail object but one layer
deeper. If the row labels still remain despite positive deep replacements, the
remaining issue is likely row-cache/UI materialization rather than the detail
string source.

### Hot-Field Second-Level Run 1

- Result:
  - `keyword_hits=188`
  - `last_total=461`
  - `deep_positive=16`
  - `deep_sum=44`
- Field distribution remained dominated by `arg0`:
  - `field=8`: 54 hits
  - `field=-1` direct: 31 hits
  - `field=13`: 22 hits
  - `field=97`: 13 hits
  - `field=25`: 11 hits
- Positive second-level examples:
  - `f8.s1`
  - `f13.s8`
  - `f13.s12`
  - `f13.s18`
  - `f13.s24`
  - `f13.s30`
  - `f25.s30`
  - `f73.s8`
  - `f73.s24`
  - `f97.s6`
  - `f109.s24`

Interpretation:

The controlled second-level pass does find additional mutable RPG string copies,
but most replacements still come from the first-level `arg0` object/fields. If
visible rows remain after this run, the strongest next hypothesis is that the
remaining visible labels are already copied into row/UI cache objects or created
before the current detail-consume mutation point.

### Client UI Path Probe Update

The next probe moves closer to row creation and UI text-cache paths instead of
only mutating the `matchmaking.dll + 0x21B80` detail object.

Static scout source:

- `hook_analysis/tools/client_ui_path_scout`
- Report:
  `hook_analysis/reports/client_ui_path_scout_report.md`

Current read-only `client.dll` probes:

- `client.dll + 0x1A0290`: `GroupServer` candidate.
- `client.dll + 0x1A0A70`: `Server/name` candidate.
- `client.dll + 0x1A16F0`: `ServerName` / `ServerIP` row-data candidate.
- `client.dll + 0x2EA760`: `CThirdPartyServerPanel` /
  `Resource/UI/ThirdPartyServerPanel.res` setup candidate.

The DLL now logs only positive keyword hits from these probes:

```text
client ui keyword probe #... name=... keyword=rpg source=ecx|arg0|arg1 ...
```

Implementation note:

- Client probes are diagnostic only. They do not mutate or skip execution.
- Single-step breakpoint recovery is now tracked per thread to reduce the chance
  of multi-thread breakpoint restoration races after adding the UI probes.

Expected next test:

If `client ui keyword probe` appears for visible RPG rows, use the reported
probe name/source as the next candidate for row hiding or text neutralization. If
no client UI keyword hits appear while `entry keyword hit` still appears, the UI
row may be populated from another client path or from cached data before these
candidate functions.

### Client UI Path Probe Run 1

Observed key-line counts:

- `entry keyword hit`: 618
- `client ui keyword probe`: 120, capped by the probe limit
- `skipped server/update`: 512
- `group refresh`: 114

Client UI hit distribution:

- 120 / 120 hits were:
  - `name=client_server_name_candidate`
  - `source=arg1`
  - `ret=0x53506F85`

Interpretation:

This confirms a more UI-adjacent path for RPG server-name materialization:

- `client.dll + 0x1A0A70` is the strongest current candidate.
- The keyword-bearing object is not `ECX` or `arg0`; it is consistently reachable
  from the second stack argument (`arg1`).
- The repeated return address in `matchmaking.dll` suggests the client UI
  function is being called from the group-server detail/materialization path,
  not from the classic server-browser-only path.

Prototype update:

- `client ui keyword probe` logging now includes:
  - `field=...`
  - `direct=0|1`
  - `ptr=0x...`

Expected next test:

Run the same flow again. The next report should identify whether `arg1` contains
the visible row name directly or through a stable field. That field can then be
used for a narrower UI-side neutralization or row-suppression experiment.

### Client UI Field Probe Run 1

Observed key-line counts:

- `client ui keyword probe`: 120, capped by the probe limit.
- `entry keyword hit`: 66.
- `skipped server/update`: 82.
- `new blocked connectstring`: 82.

Client UI distribution:

- `client_server_name_candidate source=arg1 field=11 ptr=0x0643F180`: 103 hits.
- Other `client_server_name_candidate` hits were scattered across fields such as
  `13`, `27`, `37`, `49`, `61`, `73`, and `97`.
- `client_thirdparty_panel_setup source=arg1 field=11 ptr=0x530F0884`: 2 hits.

Interpretation:

`client.dll + 0x1A0A70` remains the strongest UI-side candidate. Most visible
server-name materialization appears to pass through `arg1 field=11`; the stable
heap pointer `0x0643F180` is especially suspicious as a shared UI text/cache
source. The `client_thirdparty_panel_setup` hits are likely panel/resource setup
or a less direct path, not the main row-name write point.

Prototype update:

- At `client_server_name_candidate`, the DLL now performs a narrow UI-side
  keyword neutralization at the first keyword pointer reported by the probe.
- New log fields:
  - `ui_neutralized=...`
  - `total_ui_neutralized=...`

Expected next test:

If visible group-server `RPG` text changes to underscores more consistently than
the previous detail-object neutralization, `client.dll + 0x1A0A70` is likely the
right UI text-cache path. If visible rows remain unchanged despite positive
`ui_neutralized`, the next step should move one level earlier to the caller of
`client.dll + 0x1A0A70` or to the actual row insertion/list population point.

### Client UI Neutralization Run 1

Observed key-line counts:

- `client ui keyword probe`: 60.
- Positive `ui_neutralized`: 56 lines.
- `total_ui_neutralized`: 103.
- `entry keyword hit`: 144.
- `skipped server/update`: 91.

Interpretation:

The UI-side experiment is definitely modifying mutable keyword copies near
`client_server_name_candidate`. The visual result was not obvious enough for the
user to judge, which suggests this is still text/materialization rather than a
clean row-removal point. The function entry at `client.dll + 0x1A0A70` has a
clear `ret 4` convention, so it can be skipped safely enough for a controlled
experiment when a keyword-bearing object is present.

Prototype update:

- Added `row_filter_mode.txt = ui_skip_name`.
- In this mode, when `client_server_name_candidate` sees a keyword hit, the DLL
  emulates the function return:
  - `EIP = [ESP]`
  - `ESP += 8`
  - `EAX = 0`
- New log line:

```text
client skipped server-name function #...
```

Expected next test:

If the remaining RPG rows become blank or disappear, `client.dll + 0x1A0A70` is
on the visible row-name path. Blank rows mean the true row insertion point is
later/elsewhere; disappearance means this function participates in row
materialization enough to be used as the suppression point.

### UI Skip Name Run 1

Observed key-line counts:

- `mode=ui_skip_name (experimental)`.
- `client ui keyword probe`: 120.
- `client skipped server-name function`: 114.
- Positive `ui_neutralized`: 114.
- `entry keyword hit`: 165.
- `skipped server/update`: 95 in the summary check, with later tail lines
  reaching `#209`.

User-observed result:

- Visible RPG text appeared as underscores.

Interpretation:

The `client_server_name_candidate` skip path is executing, but this run still
performed UI/text neutralization. Therefore the visible underscore result cannot
prove row suppression. It only proves that the name/materialization path is
touched and mutable text is being changed before display.

Prototype update:

- Added `ui_skip_name_only` / `client_skip_name_only`.
- This mode keeps the `client_server_name_candidate` skip experiment, but turns
  off all keyword neutralization so the next run can distinguish:
  - row disappears or becomes blank: the skip affects visible row materialization;
  - row remains with original `RPG`: `client.dll + 0x1A0A70` is only a text/name
    path, and the real row insertion point is elsewhere.
- `row_filter_mode.txt` is now set to `ui_skip_name_only`.

### UI Skip Name Only Run 1

Observed key-line counts:

- `mode=ui_skip_name_only (experimental, no neutralization)`.
- `client ui keyword probe`: 120.
- `client skipped server-name function`: 120.
- Positive `ui_neutralized`: 0.
- `entry keyword hit`: 3651.
- `skipped server/update`: 3105 in the summary check, with later tail lines
  reaching `#3225`.
- Errors/failures: 0.

Interpretation:

This run cleanly separates function skipping from text neutralization. The
client-side name function was skipped every time it saw a keyword, and no
underscore mutation happened in this mode. Therefore any visible underscore from
previous runs came from neutralization, not from skipping `client.dll + 0x1A0A70`.

Next decision:

- If the user sees original RPG rows in this mode, `client.dll + 0x1A0A70` is not
  the row insertion/suppression point; continue toward the caller/list insertion
  path.
- If the user sees blank/odd rows, this function affects visible row
  materialization but still is not enough to remove rows cleanly.

### Row Creation Candidate Probe Update

The pure `ui_skip_name_only` run confirmed that skipping
`client.dll + 0x1A0A70` alone is not sufficient to prove row removal. The next
probe moves earlier in the client-side row population path.

New client probes:

- `client.dll + 0x1A0440` as `client_row_field_candidate`.
  - This function handles several server-row field updates and calls a UI object
    vtable method after copying field text such as address/name fallbacks.
- `client.dll + 0x1A0630` as `client_row_submit_candidate`.
  - This function builds a larger row payload and ends with a call through an UI
    object vtable at `[esi+0x80] + 0x18`, making it a stronger candidate for row
    submission/materialization.

Mode for the next diagnostic run:

```text
row_filter_mode.txt = late_update_skip
```

This disables UI/name skipping and keyword neutralization so the new probes can
be observed without underscore or function-skip side effects.

Expected next test:

Look for `client ui keyword probe` lines where `name` is either:

- `client_row_field_candidate`
- `client_row_submit_candidate`

If `client_row_submit_candidate` reliably sees RPG-bearing objects, the next
controlled experiment should skip that function on keyword hit. If only
`client_row_field_candidate` sees keywords, it may be another text field path and
we should continue toward the vtable call/list insertion.
