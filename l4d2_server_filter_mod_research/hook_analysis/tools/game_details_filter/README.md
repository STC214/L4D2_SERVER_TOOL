# L4D2 GameDetailsServer Filter

This is the first working filter prototype for the group-server list path discovered by `module_watch_gui`.

It does not inject into the game. It uses WinDivert to intercept UDP packets on port `27005`, decodes `GameDetailsServer` binary KeyValues responses, and drops matched responses before the game can use them.

## Build

```powershell
go build -o L4D2GameDetailsFilter.exe .
```

`WinDivert.dll` and `WinDivert64.sys` must be beside the exe.

## Run

The exe embeds a `requireAdministrator` manifest because WinDivert needs administrator privileges. Windows should show a UAC prompt automatically when the exe is launched.

Run from an elevated terminal, or double-click and accept UAC:

```powershell
.\L4D2GameDetailsFilter.exe -keywords "多特,绕过Steam验证,魔法战役"
```

Dry-run mode logs matches but forwards every packet:

```powershell
.\L4D2GameDetailsFilter.exe -keywords "多特,绕过Steam验证" -dry-run
```

Direct IP or IP:port blocks are also supported:

```powershell
.\L4D2GameDetailsFilter.exe -ips "1.12.235.136,114.132.242.47:10001"
```

## Current Scope

Filtered fields include:

- `GameDetailsServer.Server.Name`
- `GameDetailsServer.Server.adronline`
- `GameDetailsServer.Server.adrlocal`
- selected name/title fields from the decoded tree

When a response matches, the tool drops that response and stores both the IP:port and IP in a runtime block set.

This is a validation prototype, not the final VPK/workshop solution.
