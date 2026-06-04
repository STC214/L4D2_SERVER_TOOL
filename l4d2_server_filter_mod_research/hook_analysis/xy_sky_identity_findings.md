# XY/Starry Sky group-server identity findings

## Current conclusion

The Steam group-server row and the queryable server identity are split across two
address families.

- The queryable public endpoint observed from `A2S_INFO` is `110.42.9.24:29051`
  and adjacent ports.
- Those public endpoints expose the real server name, for example
  `星缘天空RPG战役 #-1`, plus ordinary A2S fields such as map and max players.
- The Steam group-server row that survived keyword filtering exposes a different
  direct endpoint family, such as `61.147.247.31:27423`.
- The Steam item row reports the visible name as `Left 4 Dead 2`, so keyword
  matching against `星缘`/`RPG` cannot hit that path.

This explains why keyword filtering killed most RPG rows but did not kill the
top XY/Starry Sky rows until address-field filtering was added.

## Evidence

From the game console after joining a remaining XY row:

```text
Adding direct connect IP to reservation 61.147.247.31:27423
Adding direct connect IP to connection 61.147.247.31:27423
Connecting to public(110.42.9.24:29051) private(0.0.0.0:27015) direct(61.147.247.31:27423)
Connected to 110.42.9.24:29051

Left 4 Dead 2
Map: c4m5_milltown_escape
Players: 2 (0 bots) / 14 humans
```

From `matched_entries.json`, public `110.42.9.24:29051..29065` entries expose
names like:

```text
星缘天空RPG战役 #-1
星缘天空RPG战役 #-2
...
```

From `matchmaking_row_filter.log` after adding Steam item address parsing:

```text
steam item address hit #111 address=31.247.147.61:27423/27423 61.147.247.31:27423/27423 name=Left 4 Dead 2 ...
```

The reversed address in the same log proves the Steam server item stores the IP
in byte order that must be rendered both ways until the exact field convention is
fully nailed down.

## Working rule

The stable blocker should not be a single `IP:port`. It should use this layered
identity chain:

1. Decode `A2S_INFO` / `GameDetailsServer` packets.
2. If decoded text fields match blocked identity keywords, such as `星缘` or
   `RPG`, record the endpoint and all exposed linked addresses.
3. For Steam group-server rows, parse the Steam item address fields.
4. Drop any row whose parsed address matches a recorded linked identity.
5. As a fallback, drop rows with suspicious default identity:
   `Name == "Left 4 Dead 2"`, official map, and `MaxPlayers > 8`.

## Tool changes made

`game_details_filter_gui` now records more evidence for each matched entry:

- `source`: the packet endpoint that produced the decoded identity
- `last_online`: decoded `GameDetailsServer.Server.adronline`
- `last_local`: decoded `GameDetailsServer.Server.adrlocal`
- `last_tags`: decoded A2S keywords/tags when present
- `raw_fields`: parsed raw A2S/GameDetails fields

When a keyword match occurs and endpoint blocking is enabled, it also adds
`last_online` and `last_local` to the runtime block table. If XY hides the direct
IP in GameDetails fields, the tool can now learn that link automatically.

## Remaining unknown

The current local evidence does not yet show whether `61.147.247.31` is embedded
inside the `GameDetailsServer` payload, returned by Steam matchmaking metadata,
or only revealed during the final connection reservation flow. The new raw field
logging will clarify this on the next GUI run.

## Follow-up run

The next GUI report did expose the real XY identity in A2S fields:

- `matched_entries.json` contains many `110.42.9.24:29051..` public endpoints.
- Their `raw_fields.A2S_INFO.Name` values are `星缘天空RPG战役 #-1`,
  `星缘天空RPG战役 #-2`, and so on.
- The same report did not contain `last_online` or `last_local` fields for these
  matched entries.
- It also did not contain `61.147.247.31` in the GUI-captured matched entries.

So the direct address family `61.147.247.31:27xxx` is not currently learned from
the decoded A2S/GameDetails payload. It is visible in the Steam group-server
item path and in the final connection log, while the real display identity is
visible from the public A2S endpoint family `110.42.9.24:29xxx`.

## Row-filter success run

After exporting learned address rules from `matched_entries.json`, the
row-filter DLL loaded both manual and learned connect-string lists:

```text
loaded 5 filter entries from blocked_connectstrings.txt
loaded 23 learned filter entries from learned_connectstrings.txt total=28
mode=steam_serverlist_drop
```

In the same run the Steam response hook dropped 160 rows, with no logged
`error`, `failed`, or `exception` lines. The observed game result was that
XY/Starry Sky no longer appeared in the Steam group-server list.

The current learned list contains the public identity family `110.42.9.24` and
the derived direct family `61.147.247.31`. The DLL now keeps normal address-hit
logs capped but always logs these high-value XY address families, so the next run
can prove whether the row disappeared through direct address matching or through
the default-name disguise heuristic.

## Auto-derive experiment

With `61.147.247.31` removed from manual and GUI-learned configs, the DLL still
observed the direct family in Steam group-server items and appended it to
`auto_derived_connectstrings.txt`. However, XY/Starry Sky still appeared in the
visible list because the runtime promotion happened after several default-name
rows had already been allowed into the list.

The relevant pattern was:

```text
steam auto-derived candidate host=61.147.247.31 count=1 ...
steam auto-derived candidate host=61.147.247.31 count=2 ...
...
steam item address hit ... 61.147.247.31 ... name=Left 4 Dead 2 ...
```

The fix is to block default-name large-pool rows immediately:

```text
Name == "Left 4 Dead 2"
MaxPlayers >= 128
```

This handles XY rows where the map field is not available early enough for the
older official-map heuristic. The auto-derive promotion threshold was also
lowered so learned direct hosts are persisted earlier for the next launch.
