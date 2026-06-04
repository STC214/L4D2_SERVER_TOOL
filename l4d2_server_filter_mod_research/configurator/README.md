# L4D2 Server Filter Configurator

This is the first runnable implementation for the VPK/addon configuration path.

It edits the keyword file used by the addon skeleton:

```text
left4dead2\addons\l4d2_server_filter\cfg\l4d2_server_filter\keywords.txt
```

Default game directory:

```text
E:\SteamLibrary\steamapps\common\Left 4 Dead 2
```

## Run

GUI:

```powershell
go run .
```

CLI install/generate addon:

```powershell
go run . -install -keywords "rpg,vip,shop"
```

Custom game directory:

```powershell
go run . -install -game "E:\SteamLibrary\steamapps\common\Left 4 Dead 2" -keywords-file keywords.txt
```

## Notes

- The UI thread only owns Win32 controls.
- Save/install work runs in a background goroutine.
- Keyword writes use a temporary file and backup before replacement.
- The configurator does not inject into the game and does not modify official game files.

