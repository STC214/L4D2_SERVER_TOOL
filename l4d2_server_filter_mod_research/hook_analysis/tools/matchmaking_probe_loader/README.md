# Matchmaking Probe Loader

Local 32-bit loader for probe/filter DLLs loaded into the 32-bit L4D2 process.

Use this instead of x32dbg when you want the probe DLL to write
`matchmaking_probe.log`. x32dbg catches breakpoint exceptions first, which can
pause the game before the probe handles them.

Build as 32-bit. This matters: a 64-bit loader can find `left4dead2.exe`, but
the remote `LoadLibraryW` call can return null when targeting the 32-bit game
process.

```powershell
$env:GOARCH="386"
rsrc.exe -arch 386 -manifest ..\row_filter_manager_gui\app.manifest -ico ..\row_filter_manager_gui\assets\app_icon.ico -o .\rsrc.syso
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
