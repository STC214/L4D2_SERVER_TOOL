# Hook Findings

## 2026-05-31 Initial Correction

The first implementation step accidentally started from the fallback configurator path.

Corrected direction:

1. The hook-driven investigation is the main path now.
2. New hook/research files stay in `l4d2_server_filter_mod_research`.
3. `L4D2_Server_Filter_Tool` remains a reference/external helper only.
4. The configurator stays as a fallback artifact, not the primary solution.

## First File-Name Scan

High-priority binary targets:

- `bin/serverbrowser.dll`
- `platform/servers/serverbrowser.dll`
- `left4dead2/bin/matchmaking.dll`

High-priority UI/resource targets:

- `platform/servers/dialogserverbrowser.res`
- `platform/servers/addservergamespage.res`
- `platform/servers/customserverinfodlg.res`
- `platform/servers/serverbrowser_english.txt`
- `left4dead2/resource/matchsystem.360.res`

Working hypothesis:

- `platform/servers/serverbrowser.dll` is likely tied to the classic Source server browser UI.
- `left4dead2/bin/matchmaking.dll` is likely tied to lobby, group server discovery, and random matchmaking.
- If the unwanted list is the in-game group server list rather than the classic server browser, `matchmaking.dll` is likely the more valuable target.

## Next Required Evidence

Run the static probe and inspect:

- exported functions from target DLLs if available through local tools;
- ASCII/UTF-16 strings around server browser, gameserver, matchmaking, lobby, filters, and list row labels;
- `.res` layout keys that reveal class/control names;
- localization token names in `serverbrowser_english.txt`.

## Static Probe Result

Report:

```text
l4d2_server_filter_mod_research/hook_analysis/reports/static_probe_report.md
```

Important `serverbrowser.dll` clues:

- `SteamMatchMakingServers002`
- `ServerBrowser003`
- `CInternetGames`
- `GetNewServerList`
- `RefreshServer`
- `ConnectToServer`
- `FilterString`
- `GameFilter`
- `MapFilter`
- `TagFilter`
- `servers/%sPage_Filters.res`
- `servers/CustomGamesPage_Filters.res`

Interpretation:

- The classic Source server browser almost certainly has a normal Steam server-list flow and existing filter UI concepts.
- This is a good target for adding/observing keyword filtering in the built-in server browser.
- It may not be the same surface as the L4D2 group server list or random matchmaking.

Important `matchmaking.dll` clues:

- `registered dedicated server '%s' (%s) with ping %d`
- `rejected dedicated server '%s' (%s) due to ping '%d' greater than '%d'`
- `rejected dedicated server '%s' due to ip filter '%s'`
- `Establishing connection with %d search results.`
- `ConnectServerDetailsRequest`
- `ConnectServerDetailsRequest/server`
- `OnlineSearch - client fully connected to session, search finished.`
- `Server/connectstring`
- `Filter=/Options:server`
- `mm_server_search_inet_ping_refresh`

Interpretation:

- `matchmaking.dll` is now the highest-value target for random-match and group/lobby server candidate blocking.
- The string `rejected dedicated server ... due to ip filter` suggests there may already be an internal filter path or at least a diagnostic branch we can locate.
- If we can identify how that IP filter is populated or evaluated, we may not need to remove rows from UI first; we can block candidates before connection.

## Revised Hook Priority

1. Confirm which modules are loaded when opening:
   - main menu group server list;
   - classic server browser;
   - quick/random match.
2. For random-match blocking, prioritize `left4dead2\bin\matchmaking.dll`.
3. For visible server-browser row filtering, prioritize `platform\servers\serverbrowser.dll` and its `servers/*Page_Filters.res` resources.
4. Only after module/load confirmation choose a minimal hook boundary.

## Module Watch GUI Report 2026-05-31 13:06

Report:

```text
l4d2_server_filter_mod_research/hook_analysis/reports/module_watch_gui_report.md
```

Result:

- The watcher found `left4dead2.exe` PID `13464`.
- It reported no relevant modules.

Interpretation:

- Treat this report as invalid/incomplete.
- A running L4D2 process must have at least core modules such as engine/client/Steam-related DLLs.
- The likely cause was using `TH32CS_SNAPMODULE` without `TH32CS_SNAPMODULE32` from a 64-bit Go watcher against a 32-bit game process.

Fix:

- `tools/module_watch_gui` now uses `TH32CS_SNAPMODULE | TH32CS_SNAPMODULE32`.
- `tools/module_watch` has the same fix.
- The GUI now logs module enumeration errors and shows total/relevant module counts during capture.

Action:

- Re-run `tools/module_watch_gui/L4D2ModuleWatchGUI.exe`.
- The next report should show non-zero relevant modules before we make hook-target decisions from it.

## Module Watch GUI Report 2026-05-31 13:16

Report:

```text
l4d2_server_filter_mod_research/hook_analysis/reports/module_watch_gui_report.md
```

Result:

- Module enumeration is fixed.
- `left4dead2.exe` PID `20500` was found.
- These relevant modules were already loaded at the start of capture:
  - `bin\steam_api.dll`
  - `bin\filesystem_stdio.dll`
  - `bin\engine.dll`
  - `bin\vgui2.dll`
  - `bin\vguimatsurface.dll`
  - `bin\serverbrowser.dll`
  - `left4dead2\bin\matchmaking.dll`
  - `left4dead2\bin\client.dll`
  - `left4dead2\bin\server.dll`
  - Steam `steamclient.dll`

Interpretation:

- Module-load timing alone is not enough to identify the active code path.
- Both `matchmaking.dll` and `bin\serverbrowser.dll` are present from the beginning of the main menu session.
- The next probe must capture behavior, not just module presence.

Revised next step:

1. Add phase-aware network/endpoint observation for `left4dead2.exe`.
2. During group-server background refresh, record new UDP/TCP remote endpoints and ports.
3. During classic server browser, compare endpoint patterns.
4. During quick/random match, record connect string / destination changes.
5. Use those observations to decide whether the first real hook should target:
   - Steam matchmaking/server-list calls;
   - matchmaking candidate filter/evaluation;
   - UI row creation in server browser;
   - connect command / connect string dispatch.

## Module Watch GUI Report 2026-06-01 08:03

Report:

```text
l4d2_server_filter_mod_research/hook_analysis/reports/module_watch_gui_report.md
```

Result:

- `matchmaking.dll` and `bin\serverbrowser.dll` are already loaded from the beginning.
- No useful TCP remote endpoint appeared.
- The game owns many UDP local sockets immediately on the main menu.
- Additional UDP sockets appeared during group-server phases:
  - `Group refresh background`: `0.0.0.0:51393`
  - `Group list inspect`: `0.0.0.0:61269`
  - `Group refresh recheck`: `0.0.0.0:62111`

Interpretation:

- The group-server flow is UDP-heavy.
- Windows owner tables only expose local UDP sockets, so they cannot tell us the remote server/master IPs.
- Module-level and endpoint-table-level observations are now exhausted.
- The next useful probe must be packet-level capture scoped to `left4dead2.exe` UDP sockets.

Next probe requirements:

1. Use WinDivert or equivalent packet capture to record remote UDP endpoints.
2. Correlate packets to the local UDP ports observed in each phase.
3. Decode Steam master / A2S-like payloads when possible.
4. Mark first-seen phase for each remote IP:port.
5. Keep output under `l4d2_server_filter_mod_research/hook_analysis/reports`.

Hook implication:

- For random/connection prevention, keep `matchmaking.dll` as the primary candidate because it contains server candidate rejection and connect-string strings.
- For visible group-list removal, packet data must first tell us whether the list rows are built from Steam master/A2S UDP data, matchmaking lobby metadata, or another response stream.

## Module Watch GUI Report 2026-06-01 09:19

Report:

```text
l4d2_server_filter_mod_research/hook_analysis/reports/module_watch_gui_report.md
```

Packet-level capture succeeded.

Summary:

- Total parsed packet observations: `8798`
- By phase:
  - `Group refresh background`: `4546`
  - `Main menu`: `2650`
  - `Group list inspect`: `1258`
  - `Group refresh recheck`: `344`
- By phase/hint:
  - `Group refresh background / A2S_INFO query`: `1795`
  - `Group refresh background / A2S_INFO response`: `1628`
  - `Main menu / A2S_INFO query`: `1067`
  - `Main menu / A2S_INFO response`: `1003`
  - `Group refresh background / A2S challenge`: `813`
  - `Group list inspect / Source packet 0x00`: `420`
  - `Group list inspect / A2S_INFO query`: `345`
  - `Group refresh recheck / Source packet 0x00`: `332`
  - `Group list inspect / A2S_INFO response`: `278`

Interpretation:

- The group-server background refresh is definitely A2S-heavy.
- The main menu already sends large A2S_INFO query bursts to candidate servers.
- The group-list inspection phase adds a notable `Source packet 0x00` stream, which may be list/view metadata or a protocol path not yet decoded by the probe.
- Local addresses `198.18.0.1` and `192.168.13.225` indicate traffic is likely passing through a local accelerator/proxy/NAT path; reports must deduplicate mirrored directions.

Next implementation change:

- Do not write every packet to the markdown report.
- Aggregate packet observations by phase, hint, direction, remote endpoint, and server name.
- Parse A2S_INFO response server names and include them in the report.
- Keep only a small sample of unusual payloads such as `Source packet 0x00`.

Hook implication:

- The visible group-server list is very likely populated from the same UDP/A2S candidate stream, at least partially.
- Before actual in-process Hook, the next probe should identify server names from A2S responses and correlate them with the visible unwanted names.

## Module Watch GUI Report 2026-06-01 10:56

Report:

```text
l4d2_server_filter_mod_research/hook_analysis/reports/module_watch_gui_report.md
```

Packet capture was stopped during the `Group refresh background` phase, before the later guided phases.

Summary:

- Unique packet observations: `1069`
- Total packet events represented: `2007`
- By phase:
  - `Main menu`: `1855`
  - `Group refresh background`: `152`
- By phase/hint:
  - `Main menu / A2S_INFO query`: `782`
  - `Main menu / A2S_INFO response`: `519`
  - `Main menu / Source packet 0x00`: `282`
  - `Main menu / A2S challenge`: `272`
  - `Group refresh background / Source packet 0x00`: `143`
  - `Group refresh background / A2S_INFO query`: `6`
  - `Group refresh background / A2S challenge`: `3`

Interpretation:

- The main-menu background path is already doing most of the A2S query/response work.
- During the explicit group refresh background phase, the dominant remaining signal is `Source packet 0x00`, usually one 80-byte outbound payload followed by a roughly 700-byte inbound payload from the same remote endpoint.
- This strongly suggests the group-list path has a second protocol/data shape beyond plain A2S_INFO, and this stream is now the most important one to decode.
- Parsed A2S server names are present, but Chinese names were mojibake because the probe treated the raw bytes as UTF-8. The probe has been updated to decode non-UTF-8 server names through Windows CP936/GBK.

Tool update after this report:

- `tools/module_watch_gui` now records a short hex sample for unusual payloads such as `Source packet 0x00`.
- The next report should let us inspect the actual bytes for the group-list specific payloads instead of only seeing payload length.

Next evidence needed:

1. Re-run the GUI capture through at least:
   - `Main menu`
   - `Group refresh background`
   - `Group list inspect`
   - `Group refresh recheck`
2. Compare visible unwanted server names in the list with:
   - A2S server-name samples;
   - `Source packet 0x00` remote endpoints;
   - repeated IP:port families such as `123.113.157.17:*`, `117.72.73.87:*`, and similar grouped hosts.
3. Use the new hex samples to decide whether `Source packet 0x00` is:
   - a Source split-packet response;
   - a compressed or challenge-wrapped query response;
   - local accelerator/proxy metadata;
   - or another game-specific response that should be targeted before UI row insertion.

## Module Watch GUI Report 2026-06-01 11:06

Report:

```text
l4d2_server_filter_mod_research/hook_analysis/reports/module_watch_gui_report.md
```

Summary:

- Unique packet observations: `8879`
- Total packet events represented: `12300`
- By phase:
  - `Group refresh background`: `5497`
  - `Main menu`: `3260`
  - `Group list inspect`: `3256`
  - `Group refresh recheck`: `287`
- By phase/hint:
  - `Group refresh background / A2S_INFO query`: `2697`
  - `Group refresh background / A2S_INFO response`: `1670`
  - `Group list inspect / A2S_INFO query`: `1433`
  - `Group list inspect / A2S_INFO response`: `842`
  - `Group list inspect / Source packet 0x00`: `402`
  - `Group refresh recheck / Source packet 0x00`: `280`
  - `Group refresh background / Source packet 0x00`: `186`

Important corrections:

- CP936/GBK decoding works. Server names now resolve to readable Chinese names such as `81.多人多特`, `【CN】33.无限火力`, `浩方黑党(纯净战役服)`, and other visible candidate names.
- Some names are still mojibake. This likely means those individual servers publish malformed or differently encoded names, not that the whole parser is broken.

Key discovery:

- The `Source packet 0x00` stream is not random or opaque.
- Outbound 80-byte samples contain the ASCII root `InetSearchServerDetails`.
- Inbound 690-760 byte samples contain the ASCII root `GameDetailsServer`.
- The inbound data begins like a Valve binary KeyValues tree, with fields such as `System`, `network=LIVE`, and `access`.

Interpretation:

- The group list inspection phase is explicitly querying per-server details through `InetSearchServerDetails`.
- Responses named `GameDetailsServer` are very likely the data source used to enrich or populate visible group-server rows.
- This gives us a stronger hook boundary than raw A2S alone:
  - A2S_INFO gives server name/IP candidates.
  - `InetSearchServerDetails` / `GameDetailsServer` appears to be the group-list-specific detail query/response path.

Next implementation change:

- Update the GUI report to decode binary KeyValues payloads into a compact `Payload Summary` column.
- Use that decoded summary to identify which field contains the displayed group-list title, connect string, lobby/server ID, or IP/port.

Hook implication:

- For visible group-list removal, the first practical target is likely the code that receives/parses `GameDetailsServer` data or inserts the decoded result into the group list.
- For random matchmaking prevention, still keep `matchmaking.dll` candidate filtering and the internal `ip filter` string as the separate blocking path.

## Module Watch GUI Report 2026-06-01 11:11

Report:

```text
l4d2_server_filter_mod_research/hook_analysis/reports/module_watch_gui_report.md
```

Summary:

- Unique packet observations: `8729`
- Total packet events represented: `11344`
- By phase:
  - `Group refresh background`: `6022`
  - `Main menu`: `2646`
  - `Group list inspect`: `2408`
  - `Group refresh recheck`: `268`
- By phase/hint:
  - `Group refresh background / A2S_INFO query`: `2967`
  - `Group refresh background / A2S_INFO response`: `1924`
  - `Group list inspect / A2S_INFO query`: `1004`
  - `Group list inspect / A2S_INFO response`: `567`
  - `Group list inspect / Source packet 0x00`: `404`
  - `Group refresh recheck / Source packet 0x00`: `242`

Result:

- The new `Payload Summary` column confirms the same group-list detail path:
  - outbound `root=InetSearchServerDetails`;
  - inbound `root=GameDetailsServer`.
- Outbound detail requests include at least:
  - `timestamp`;
  - `pingxuid`.
- Inbound detail responses currently decode only:
  - `System.network=LIVE`;
  - `System.access=public`.

Interpretation:

- The parser is positioned correctly, but the summary filter was too narrow.
- The response payload length is much larger than the shown fields, so useful row data is probably deeper in the same binary KeyValues tree or under keys that were filtered out.

Tool update after this report:

- `tools/module_watch_gui` now emits more binary KeyValues fields with full paths instead of filtering to a small key allow-list.
- The next report should show whether `GameDetailsServer` contains the visible title, map/mission, lobby ID, connect address, or other row fields.

Follow-up correction:

- The parser should not expand the field range incrementally.
- `tools/module_watch_gui` has been changed to dump the full decoded binary KeyValues tree for sampled `InetSearchServerDetails` / `GameDetailsServer` payloads.
- It now handles string, int, float, pointer, wide string, color, and uint64 value types.
- Unknown value types are written into the report with their byte offset instead of silently truncating the rest of the payload.
- Long field summaries are no longer intentionally shortened.

## Module Watch GUI Report 2026-06-01 11:19

Report:

```text
l4d2_server_filter_mod_research/hook_analysis/reports/module_watch_gui_report.md
```

Summary:

- Unique packet observations: `9030`
- Total packet events represented: `12453`
- By phase:
  - `Group refresh background`: `5938`
  - `Group list inspect`: `2883`
  - `Main menu`: `2773`
  - `Group refresh recheck`: `859`
- By phase/hint:
  - `Group refresh background / A2S_INFO query`: `2935`
  - `Group refresh background / A2S_INFO response`: `1917`
  - `Group list inspect / A2S_INFO query`: `1241`
  - `Group list inspect / A2S_INFO response`: `747`
  - `Group refresh recheck / Source packet 0x00`: `444`
  - `Group list inspect / Source packet 0x00`: `418`

Result:

- The full-field parser exposed the next protocol detail instead of hiding it:
  - `0x0b` appears immediately after `GameDetailsServer.System.access=public`.
  - Treating `0x0b` as an unknown value type caused the parser to desynchronize and report `unparsed_tail` around 600 bytes.
- This pattern strongly suggests `0x0b` is an alternate object terminator or separator in this specific binary KeyValues stream, not a normal field value.
- The outbound `timestamp` field is encoded with type `0x03`, but interpreting it only as float produces nonsense. Keep both `uint32` and `float32` views in the report for this type.

Tool update after this report:

- `tools/module_watch_gui` now treats both `0x08` and `0x0b` as object terminators while walking binary KeyValues.
- Type `0x03` now reports both raw uint32 and float32 interpretations.
- Binary KeyValues samples now keep up to 2048 bytes of hex, enough to inspect the full `GameDetailsServer` payload when parsing still fails.

## Module Watch GUI Report 2026-06-01 11:23

Report:

```text
l4d2_server_filter_mod_research/hook_analysis/reports/module_watch_gui_report.md
```

Summary:

- Unique packet observations: `9054`
- Total packet events represented: `12494`
- By phase:
  - `Group refresh background`: `5911`
  - `Group list inspect`: `3260`
  - `Main menu`: `2943`
  - `Group refresh recheck`: `380`
- By phase/hint:
  - `Group refresh background / A2S_INFO query`: `2892`
  - `Group refresh background / A2S_INFO response`: `1837`
  - `Group list inspect / A2S_INFO query`: `1427`
  - `Group list inspect / A2S_INFO response`: `839`
  - `Group list inspect / Source packet 0x00`: `413`
  - `Group refresh recheck / Source packet 0x00`: `373`

Result:

- Treating `0x0b` as an object terminator fixed the parser desync.
- `GameDetailsServer` now expands the important group-list detail fields:
  - `Server.Name`
  - `Server.Server`
  - `Server.adronline`
  - `Server.adrlocal`
  - `Members.numSlots`
  - `Members.numPlayers`
  - `game.state`
  - `game.Mode`
  - `game.campaign`
  - `game.chapter`
  - `game.MissionInfo.DisplayTitle`
  - `game.MissionInfo.MissionFile`
  - `game.ModeInfo.DisplayTitle`
  - `game.difficulty`
- Example decoded rows include:
  - `Anne云服#23` at `1.12.219.128:2331`
  - `81.多人多特` at `1.12.235.136:10081`
  - `[小花火]纯净多特#3` at `101.33.251.27:5354`
  - `技能◆魔法战役▶(绕过Steam验证)` at `114.132.242.47:10001`

Interpretation:

- The visible group server list can be filtered at the `GameDetailsServer` detail-response layer by matching `Server.Name`, `Server.adronline`, and possibly mission/mode fields.
- This is now a concrete hook boundary: intercept the parser/receiver or row insertion path after `GameDetailsServer` is decoded, and drop rows whose decoded fields match block keywords or blocked IPs.
- A separate random-match block path is still needed, likely through `matchmaking.dll` candidate filtering or its existing IP-filter branch.

Tool update after this report:

- 32-bit integer values in this stream are big-endian; the parser now reports type `0x02` as big-endian so fields such as `numSlots` and `chapter` become human-readable.
- Type `0x03` now reports both big-endian and little-endian uint/float interpretations until its exact meaning is pinned down.
- Trailing `0x0b/0x08/0x00` terminators/padding are skipped so they do not appear as `unparsed_tail`.

## First Filter Prototype

Artifact:

```text
l4d2_server_filter_mod_research/hook_analysis/tools/game_details_filter
```

Purpose:

- Validate filtering before writing an in-process hook.
- Use WinDivert externally instead of DLL injection.
- Intercept UDP packets on port `27005`.
- Decode inbound `GameDetailsServer` binary KeyValues responses.
- Drop matched responses before the game can add/enrich a visible group-server row.

Filter fields:

- `GameDetailsServer.Server.Name`
- `GameDetailsServer.Server.adronline`
- `GameDetailsServer.Server.adrlocal`
- selected name/title fields from the decoded KeyValues tree

Runtime behavior:

- A keyword match drops the current detail response.
- The matched `IP:port` and bare IP are added to an in-memory block set.
- Future packets to/from that endpoint are dropped while the tool is running.
- The exe embeds a `requireAdministrator` manifest because WinDivert requires administrator privileges.

Example:

```powershell
.\L4D2GameDetailsFilter.exe -keywords "多特,绕过Steam验证,魔法战役"
```

Validation plan:

1. Run the filter as administrator.
2. Start L4D2 and wait at the main menu.
3. Open/re-enter the group server list after background refresh.
4. Confirm matched server rows no longer appear or stop being enriched.
5. If rows still appear from A2S-only data, add a second blocking layer for A2S_INFO responses from matched endpoints.

## GUI Filter Prototype

Artifact:

```text
l4d2_server_filter_mod_research/hook_analysis/tools/game_details_filter_gui/L4D2GameDetailsFilterGUI.exe
```

Purpose:

- Provide a simple Go + Win32 UI for the `GameDetailsServer` filter.
- Avoid asking the tester to run command-line flags for common keyword/IP filtering.
- Keep the UI stable while WinDivert receives and forwards packets in the background.

UI behavior:

- `Keywords`: comma-separated server-name or title keywords.
- `IP / IP:port`: comma-separated IP or endpoint block entries.
- `Dry run`: logs matched rows without dropping packets.
- `Verbose allow log`: logs allowed decoded rows; useful for research, but should stay off during long captures.
- `Start Filter` starts the background WinDivert worker.
- `Stop` cancels the worker and calls WinDivert shutdown to unblock receive.

Anti-freeze notes:

- The UI thread is locked to one OS thread and only runs the Win32 message loop/control updates.
- The packet filter runs in a worker goroutine.
- The worker never touches controls directly; it queues log events and wakes the UI with `PostMessage`.
- No Go heap pointers are passed through `PostMessage`.
- The visible log is capped at 500 lines.
- Status counters refresh on a low-frequency timer instead of per-packet UI updates.
- The exe embeds `requireAdministrator` and is built with `-H=windowsgui`, so it requests UAC and does not open a console window.

Follow-up fix after first GUI run:

- The Steam group-server refresh report showed a large `A2S_INFO query/response` path during `Group refresh background` and `Group list inspect`, so the GUI filter now parses and matches `A2S_INFO` server names in addition to `GameDetailsServer`.
- Endpoint blacklisting is now opt-in through `Also block endpoint`. The first GUI version added matched IPs to the runtime block set automatically, which could also suppress unrelated server-browser traffic from the same host.
- Log refresh now preserves the current first visible line instead of resetting the log scrollbar on every append.

RPG/fake-entry refinement:

- Many RPG servers occupy the Steam group-server list as launch/entry placeholders instead of real ready-to-join sessions.
- Some fake entry servers still display player counts in the group-server list, so player count is not a reliable readiness signal.
- Clicking these rows can show the local "creating game" flow, which suggests the group-server row is acting as a launch/entry trigger rather than a normal already-running dedicated server. That UI text is local and may not exist in packet fields.
- The GUI filter now decodes more `A2S_INFO` fields, including player count, max players, version, EDF flags, game port, SteamID, spectator name, tags/keywords, and GameID when present.
- Keyword matching now scans all decoded `A2S_INFO` and `GameDetailsServer` field values, not only name/title fields. This makes rules such as `RPG`, `入口`, and `开服` useful even when the server hides the clue outside the display name.
- The GUI has an `Add RPG Preset` button that appends common entry-server keywords such as `RPG`, `入口`, `正在开启服务器`, `正在创建游戏`, `创建游戏`, `星缘天空`, and `破晓`.
- `http://l4d2.xygamers.com/?page=1` exposes XY server endpoints in a JavaScript `server_ot` array. The GUI now has an `Import XY IPs` button that downloads this page in a background goroutine, parses `IP:port` endpoints, and appends them to the explicit IP block field.
- `http://www.616111.xyz` currently looks like a PXRPG article/account site and did not expose a direct server endpoint array in the fetched homepage. Keep using keyword rules for PX until a better endpoint source is found.
- PX endpoint observed from the classic server browser history after entering a PX RPG entry: `114.66.17.54:27017`, server name shown as `Left 4 Dead 2`, map `c5m3_cemetery`, player count `3/12`, VAC unsafe. The GUI has a `Known Entry IPs` button that imports this endpoint into the explicit block list.
- New disguised-default-server heuristic: treat `Name == "Left 4 Dead 2"`, official map code, and `MaxPlayers > 8` as a suspicious entry/rental-server disguise. Player count is recorded but not required because fake entries can also report plausible player counts.
- Default GUI keywords changed to `RPG,星缘,破晓,杀戮,神域`.
- Official map code reference was added at `hook_analysis/official_l4d2_maps.md`.
- Hit persistence added: each run summarizes matched blocked entries into `matched_entries.json` next to the GUI exe, including endpoint, name, map, packet type, reason, players/max players, first/last seen, hit count, and recent run summaries. This is meant to compare whether entry servers rotate IP/port between runs.
- Current external WinDivert filtering can prevent successful join/creation flows, but may not remove rows already inserted by Steam group metadata or local UI cache. Fully hiding those rows likely requires an in-process hook at the matchmaking/serverbrowser/VGUI row insertion or data-source layer.
- Another XY/rental-server disguise pattern was observed: `Valve Left4Dead 2 Hong Kong Server (...)`, official map such as `c4m1_milltown_a`, and endpoint `103.28.54.214:27199`. The GUI now treats names containing `Valve Left4Dead 2 Hong Kong Server` plus an official map code as suspicious, without depending on the version/build suffix.
- Manual or imported IP/IP:port values from the GUI are now persisted into `matched_entries.json` under `manual_ips`, and are reloaded into the IP field at the next startup.
- Added a `Clear Cache` button. It safely moves known Steam/Source server-browser cache/history candidates into `cache_backups/<timestamp>` next to the GUI exe instead of permanently deleting them. This may reduce local cached rows, but cannot guarantee removal of Steam group metadata cached elsewhere.
