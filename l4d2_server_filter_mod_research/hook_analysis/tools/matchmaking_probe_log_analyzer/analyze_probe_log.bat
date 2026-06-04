@echo off
setlocal

rem Run from this directory after matchmaking_probe.log has been generated.
rem Output: ..\..\reports\matchmaking_probe_log_report.md

L4D2MatchmakingProbeLogAnalyzer.exe -log ..\matchmaking_probe_dll\matchmaking_probe.log
if errorlevel 1 exit /b %errorlevel%

echo Wrote ..\..\reports\matchmaking_probe_log_report.md
