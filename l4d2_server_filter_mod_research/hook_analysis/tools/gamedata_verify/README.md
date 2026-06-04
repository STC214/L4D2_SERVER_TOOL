# Gamedata Verify

Offline verifier for `hook_analysis/gamedata/matchmaking_targets.json`.

It checks whether each configured signature still has the expected number of hits
in the current `matchmaking.dll`. It does not attach to or modify the game process.

## Usage

```powershell
go run . -game "E:\SteamLibrary\steamapps\common\Left 4 Dead 2"
```

Optional:

```powershell
go run . `
  -game "E:\SteamLibrary\steamapps\common\Left 4 Dead 2" `
  -gamedata "..\..\gamedata\matchmaking_targets.json" `
  -out "..\..\reports\gamedata_verify_report.md"
```

Exit code:

- `0`: every target matched its expected hit count.
- `2`: at least one target failed or had a hit-count mismatch.
- `1`: input/output error.

## GUI Use Later

The later UI should call this verifier in the background and show:

- target name
- expected hits
- actual hits
- status
- report path

The UI should not parse `signature_scout_report.md` directly. It should use
`gamedata/matchmaking_targets.json` as the source of truth.

