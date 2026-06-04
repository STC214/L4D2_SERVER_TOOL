@echo off
setlocal

set GAME_DIR=E:\SteamLibrary\steamapps\common\Left 4 Dead 2
set OUT=..\..\reports\gamedata_verify_report.md
set GAMEDATA=..\..\gamedata\matchmaking_targets.json

go run . -game "%GAME_DIR%" -gamedata "%GAMEDATA%" -out "%OUT%"
exit /b %errorlevel%

