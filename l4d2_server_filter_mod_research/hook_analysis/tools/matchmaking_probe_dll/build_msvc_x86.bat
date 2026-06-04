@echo off
setlocal

rem Run this from a "x86 Native Tools Command Prompt for VS".
rem Output: matchmaking_probe.dll

cl /nologo /EHsc /W4 /O2 /MT /LD matchmaking_probe.cpp /Fe:matchmaking_probe.dll /link /NOLOGO
if errorlevel 1 exit /b %errorlevel%

echo Built matchmaking_probe.dll

