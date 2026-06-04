# Matchmaking Probe Loader

Local loader for `matchmaking_probe_v3.dll`.

Use this instead of x32dbg when you want the probe DLL to write
`matchmaking_probe.log`. x32dbg catches breakpoint exceptions first, which can
pause the game before the probe handles them.

Build:

```powershell
$env:GOARCH="386"
go build -o L4D2MatchmakingProbeLoader.exe .
Remove-Item Env:\GOARCH
```

Run:

```powershell
.\run_loader_admin.ps1
```

Then wait at the L4D2 main menu for group-server refresh and check:

```text
..\matchmaking_probe_dll\matchmaking_probe.log
```
