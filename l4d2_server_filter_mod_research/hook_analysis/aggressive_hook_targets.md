# Aggressive Hook Targets

This note narrows the next in-process hook/debugging step to concrete RVAs found by
`tools/hook_target_scout`.

Game build scanned:

- `E:\SteamLibrary\steamapps\common\Left 4 Dead 2`
- Primary module: `left4dead2\bin\matchmaking.dll`
- Report: `reports/hook_target_scout_report.md`

## Current Conclusion

`matchmaking.dll` is the primary target for Steam group-server suppression.

The packet filter can already block joining many unwanted RPG entries, but rows can
remain in the Steam group-server list because the game/UI has already accepted or
cached the candidate. The stronger fix should happen before or during the internal
server-manager update path, not only at UDP packet boundaries.

## Priority Targets

All addresses below are RVAs. At runtime use:

`runtime_address = module_base(matchmaking.dll) + RVA`

| Priority | Function RVA | Evidence | Why it matters |
|---|---:|---|---|
| A | `0x21B80` | References `ConnectServerDetailsRequest`, `InetSearchServerDetails`, `GameDetailsServer`, `InetSearchServerDetails/timestamp`, `InetSearchServerDetails/pingxuid` | Strongest candidate for parsing/consuming game-detail replies. If a server is rejected here, group-list rows may never be finalized. |
| A | `0x22750` | References `GameDetailsServer`, `Server/name`, `Server/adrlocal`, `Server/adronline` | Strong candidate for extracting visible server fields after decode. Best place to test keyword/IP rejection before UI insertion or manager registration. |
| B | `0x21570` | References `Server manager waiting for game details from %d servers...` and `InetSearchServerDetails` | Likely manages outstanding detail requests. Useful for tracing the transition from candidate address to detail response. |
| B | `0x211E0` | References `Requesting group server list for groups %s...` | Group-server refresh entry point. Useful for confirming the exact call path used by the main menu group list. |
| C | `0x219E0` | References `Server manager refreshing...` | Refresh lifecycle marker. Good breakpoint, less likely to hold per-server data. |
| C | `0x21A70` | References `Server manager refresh completed.` | Refresh completion marker. Useful to compare counts before/after filtering. |
| C | `0x22052` / `0x220AE` | References `ConnectServerDetailsRequest/server` / `ConnectServerDetailsRequest` | Likely builder/lookup helpers for KeyValues request objects. Useful if A-level hooks only observe partial state. |
| D | `0x2D9D0` | References `registered dedicated server`, `rejected dedicated server`, `rejected dedicated server ... ip filter` | Strong random-match/dedicated search path. Useful for blocking random matchmaking, but probably not sufficient for visible group-server rows. |

## Debugger Pass

First dynamic pass should use x32dbg/x64dbg on the 32-bit game process and break on:

- `matchmaking.dll + 0x211E0`
- `matchmaking.dll + 0x21570`
- `matchmaking.dll + 0x21B80`
- `matchmaking.dll + 0x22750`
- `matchmaking.dll + 0x2D9D0`

Recommended sequence:

1. Start L4D2 with VAC exposure accepted for local analysis.
2. Attach x32dbg after the main menu is loaded.
3. Set the breakpoints above.
4. Return to the main menu and wait for Steam group-server auto refresh.
5. When `0x21B80` or `0x22750` hits, inspect stack arguments and `ECX`.
6. Look for KeyValues-like pointers containing:
   - `GameDetailsServer`
   - `Server/name`
   - `Server/adrlocal`
   - `Server/adronline`
   - `Members/numSlots`
   - `Members/numPlayers`
7. If blocked keywords are visible at `0x22750`, this is the best hook point for row suppression.

## Hook Direction

The intended injected hook should be read-only during the first prototype:

- Log function hits and candidate pointers.
- Decode only stable KeyValues/string fields.
- Do not modify return values until the call arguments are understood.

After the call shape is confirmed:

- If the target returns a bool/status, return the same value used by invalid or timed-out servers.
- If the target inserts into a manager/list, skip the original call for blocked entries.
- If it updates an existing row, force the same path as missing/expired server details.

## Probe DLL Prototype

The first read-only prototype is now under:

`tools/matchmaking_probe_dll`

It uses a vectored exception handler plus temporary `INT3` software breakpoints on
the priority RVAs. On hit, it logs x86 registers, stack dwords, and any stack/register
values that look like printable ASCII pointers to `matchmaking_probe.log`.

This should answer the next important question: whether `0x21B80` or `0x22750`
receives enough server data to make a row-level reject decision before the Steam
group-server list displays the entry.

Build requirement: 32-bit MSVC toolchain, because the game process is 32-bit.

## Safer Static Signature Path

If the environment or platform policy flags live injection/debugging work, continue
with the static signature path first.

Tool:

`tools/signature_scout`

Generated report:

`reports/signature_scout_report.md`

Current result: all five priority targets produce unique signatures in the scanned
`matchmaking.dll` build:

| Target | RVA | Unique Hits |
|---|---:|---:|
| `group_refresh_entry` | `0x211E0` | `1` |
| `manager_wait_details` | `0x21570` | `1` |
| `details_consume_candidate` | `0x21B80` | `1` |
| `details_fields_candidate` | `0x22750` | `1` |
| `dedicated_accept_reject` | `0x2D9D0` | `1` |

This means the next implementation should not depend on fixed RVAs. It should use
the signature patterns from `signature_scout_report.md` as a gamedata-like target
table.

## Why Not ServerBrowser First

`serverbrowser.dll` has strong classic server-browser targets (`CInternetGames`,
`ConnectToServer`, `FilterString`), but the user's current pain is Steam group
servers on the main menu. The classic browser path already responds to packet
filtering, while group servers still show cached/accepted rows. Therefore
`serverbrowser.dll` is a secondary target unless the group UI proves to reuse it.
