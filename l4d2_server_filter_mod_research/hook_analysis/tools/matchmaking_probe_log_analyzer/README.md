# Matchmaking Probe Log Analyzer

Offline parser for `matchmaking_probe.log`.

It summarizes probe hit counts, decoded text samples, and row-relevant keywords
such as `Server/name`, `Server/adronline`, `Members/numSlots`, `RPG`, `星缘`, and
`破晓`.

Run:

```powershell
go run . -log ..\matchmaking_probe_dll\matchmaking_probe.log
```

Output:

```text
hook_analysis/reports/matchmaking_probe_log_report.md
```
