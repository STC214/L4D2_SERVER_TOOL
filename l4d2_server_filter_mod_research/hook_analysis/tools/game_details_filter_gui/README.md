# L4D2 GameDetails Filter GUI

Go + Win32 GUI for filtering unwanted L4D2 server entries at the packet/details
layer, with an added offline target verifier for the deeper `matchmaking.dll`
research path.

The packet filter still requires administrator privileges because it uses
WinDivert. The target verifier is read-only and only scans local DLL bytes.

## Run

Double-click:

```text
L4D2GameDetailsFilterGUI.exe
```

Allow the UAC prompt.

## Normal Filtering Flow

1. Click `Verify Targets` first. All five targets should be `OK`.
2. Add or edit keywords and manual IP/IP:port entries.
3. Use `RPG Preset`, `XY IPs`, or `Known IPs` if needed.
4. Keep `Dry run only` enabled for the first pass.
5. Click `Start`.
6. Enter the L4D2 main menu and wait for the Steam group-server list to refresh in
   the background.
7. Leave and re-enter the group-server list to observe updated rows.
8. If matches look correct, disable `Dry run only` and run again.

## Buttons

- `Start`: start the WinDivert packet/details filter.
- `Stop`: stop the filter. The UI should remain responsive while stopping.
- `Clear Log`: clear the visible log.
- `RPG Preset`: append common RPG/entry-server keywords.
- `XY IPs`: import current Starry Sky endpoints from the known web page.
- `Known IPs`: import manually confirmed entry endpoints.
- `Cache`: move common Steam/Source server-browser cache files into a timestamped
  backup folder.
- `Save IPs`: save manual IP/IP:port entries to `matched_entries.json`.
- `Verify Targets`: offline-scan local `matchmaking.dll` against
  `gamedata/matchmaking_targets.json`.
- `Target Report`: open the latest combined target verification report.

## Default Keywords

```text
RPG,星缘,破晓,杀戮,神域
```

## Entry Server Heuristics

Known spoofed/default-server style:

```text
Name == "Left 4 Dead 2"
Map is an official L4D2 map code
MaxPlayers > 8
```

Known Hong Kong/Starry Sky style:

```text
Name contains "Valve Left4Dead 2 Hong Kong Server"
Map is an official L4D2 map code
```

Player count is not a required condition because entry servers may display values
such as `3/12` or `15/27`.

Official map codes are recorded in:

```text
l4d2_server_filter_mod_research/hook_analysis/official_l4d2_maps.md
```

## Saved Files

Next to the executable:

```text
matched_entries.json
cache_backups\
```

`matched_entries.json` records the packet source endpoint, decoded online/local
addresses, tags, and raw parsed A2S/GameDetails fields for each match. When a
keyword match exposes `adronline` or `adrlocal`, those linked addresses are added
to the runtime block table too.

When `matched_entries.json` is saved, the GUI also regenerates:

```text
..\matchmaking_row_filter_dll\learned_connectstrings.txt
```

The in-process row-filter DLL reads that file at injection time. It contains
hosts learned from matched A2S/GameDetails identities, plus currently observed
derived links such as Starry Sky public `110.42.9.24` to direct
`61.147.247.31`.

Research reports:

```text
l4d2_server_filter_mod_research/hook_analysis/reports/
```

XY/Starry Sky split-address findings are tracked in:

```text
l4d2_server_filter_mod_research/hook_analysis/xy_sky_identity_findings.md
```

The combined target verifier writes:

```text
gamedata_verify_combined_report.md
```

## Why Some Group Rows May Still Show

This tool can block detail responses, A2S information, and later joins. Some Steam
group-server rows can still be displayed because the game may create or cache rows
from Steam group metadata before packet-level filtering can remove them.

For full row suppression, the likely research target remains an in-process path in
`matchmaking.dll` or a UI/list insertion path. The built-in `Verify Targets` button
checks whether the currently researched `matchmaking.dll` target signatures still
match this game build.

## Required Runtime Files

Keep these next to the executable:

```text
WinDivert.dll
WinDivert64.sys
```
