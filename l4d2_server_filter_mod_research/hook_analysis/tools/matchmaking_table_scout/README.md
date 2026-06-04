# Matchmaking Table Scout

Offline static helper for `matchmaking.dll`.

It reads the local PE file, finds VA/RVA pointer references to targets in
`hook_analysis/gamedata/matchmaking_targets.json`, and dumps nearby pointer-table
slots with short function summaries.

This is intended to identify dispatch/vtable neighborhoods for the Steam
group-server details path.

Run:

```powershell
go run . -game "E:\SteamLibrary\steamapps\common\Left 4 Dead 2"
```

Output:

```text
hook_analysis/reports/matchmaking_table_scout_report.md
```
