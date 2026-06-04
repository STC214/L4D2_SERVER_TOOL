# Hook Targets

This file tracks the current cut-in hypotheses.

## Target 1: Matchmaking Search Candidate Filter

Module:

```text
E:\SteamLibrary\steamapps\common\Left 4 Dead 2\left4dead2\bin\matchmaking.dll
```

Evidence:

```text
registered dedicated server '%s' (%s) with ping %d
rejected dedicated server '%s' (%s) due to ping '%d' greater than '%d'
rejected dedicated server '%s' due to ip filter '%s'
Establishing connection with %d search results.
ConnectServerDetailsRequest/server
Server/connectstring
Filter=/Options:server
```

Goal:

- Find where dedicated server candidates are accepted/rejected.
- Determine the structure carrying server name, IP/connect string, ping, and filters.
- Confirm whether an existing IP filter can be configured or whether it is only an internal branch.

Why this comes first:

- It directly affects random matchmaking.
- It may block connection before UI display becomes relevant.

## Target 2: Classic Server Browser Row Creation / Refresh

Modules:

```text
E:\SteamLibrary\steamapps\common\Left 4 Dead 2\bin\serverbrowser.dll
E:\SteamLibrary\steamapps\common\Left 4 Dead 2\platform\servers\serverbrowser.dll
```

Evidence:

```text
SteamMatchMakingServers002
ServerBrowser003
CInternetGames
GetNewServerList
RefreshServer
ConnectToServer
FilterString
TagFilter
servers/%sPage_Filters.res
```

Goal:

- Find where server rows are inserted or updated.
- Check whether existing filter controls can be extended via resources.
- Determine whether a keyword filter can be injected at the Steam callback layer or UI row layer.

## Target 3: Resource/UI Filter Surface

Files:

```text
E:\SteamLibrary\steamapps\common\Left 4 Dead 2\platform\servers\dialogserverbrowser.res
E:\SteamLibrary\steamapps\common\Left 4 Dead 2\platform\servers\addservergamespage.res
E:\SteamLibrary\steamapps\common\Left 4 Dead 2\platform\servers\serverbrowser_english.txt
```

Goal:

- Identify control names and existing filter widgets.
- Test whether loose addon/VPK can override the relevant `.res` files.
- If resources can add a text box but cannot bind custom behavior, treat it as UI-only and keep filtering in hook/external logic.

## Immediate Dynamic Evidence Needed

Before writing any in-process hook, verify:

- Is `matchmaking.dll` loaded during group-server refresh?
- Is either `serverbrowser.dll` loaded during the group-server list?
- Does `platform\servers\serverbrowser.dll` load only when opening the classic server browser?
- Which module loads before random matchmaking begins?

