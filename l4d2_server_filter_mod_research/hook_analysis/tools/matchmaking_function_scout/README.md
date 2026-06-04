# Matchmaking Function Scout

Offline function profiler for the current `matchmaking.dll` target path.

It reads the target RVAs from `gamedata/matchmaking_targets.json`, extracts a
guessed function body, and reports:

- function RVA and file offset
- guessed function size
- prologue bytes
- direct calls/jumps/immediate references
- pushed string references
- simple EBP argument/object hints

It does not attach to or modify a running game process.

## Usage

```powershell
go run . `
  -game "E:\SteamLibrary\steamapps\common\Left 4 Dead 2" `
  -gamedata "..\..\gamedata\matchmaking_targets.json" `
  -out "..\..\reports\matchmaking_function_scout_report.md"
```

## Main Finding

The best current `matchmaking.dll` path target is:

```text
details_fields_candidate: matchmaking.dll + 0x22750
```

It references `GameDetailsServer`, `Server/name`, `Server/adronline`,
`Server/adrlocal`, `Members/numSlots`, and `Members/numPlayers` close together.

