@echo off
setlocal

rem Run this from a Visual Studio x86 native tools environment.
rem Output: matchmaking_row_filter.dll

cl /nologo /EHsc /W4 /O2 /MT /LD matchmaking_row_filter.cpp /Fe:matchmaking_row_filter.dll /link /NOLOGO
if errorlevel 1 exit /b %errorlevel%

echo Built matchmaking_row_filter.dll
