# Mature Hooking References

This file records mature open-source references for the next aggressive L4D2 group
server filtering path.

## Short Conclusion

There is no obvious ready-made GitHub project that already filters L4D2 Steam
group-server rows on the client main menu.

However, there are mature pieces we can reuse as references:

1. Use L4D2VR as the closest L4D2 client-side injection/signature-scanning model.
2. Use MinHook as the smallest practical detour library once the function signature
   is known.
3. Use Source SDK 2013 and AlliedModders DHooks docs as the conceptual map for
   Source Engine interfaces, vtables, gamedata, signatures, and detours.
4. Treat Metamod:Source/SourceMod/DHooks as server-side references, not direct
   client-main-menu solutions.

## Candidate References

### L4D2VR

Repository:

`https://github.com/sd805/l4d2vr`

Why it matters:

- It is a real L4D2 client-side mod.
- It builds as x86.
- It uses a local DLL placed into the game directory.
- It has a known VAC-risk / `-insecure` workflow.
- Its offset/signature-scanning pattern is directly relevant to our `matchmaking.dll`
  RVA problem.

Useful idea to borrow:

- Replace fixed RVAs with pattern signatures after we confirm the correct function.
- Keep an offset/signature table instead of scattering raw addresses through code.
- Prefer a loader path that is obvious and debuggable during research.

Limit:

- L4D2VR focuses rendering/input/VR, not server list filtering.

### MinHook

Repository:

`https://github.com/TsudaKageyu/minhook`

Why it matters:

- Mature x86/x64 Windows function detour library.
- Much safer than our temporary `INT3 + VEH` probe for a real filtering hook.
- Good fit once we know the exact function signature at `matchmaking.dll + 0x21B80`
  or `matchmaking.dll + 0x22750`.

Useful idea to borrow:

- Use `MH_CreateHook`/trampoline style for stable pre/post inspection.
- Avoid manually keeping software breakpoints in production.

Limit:

- It does not solve injection, signature discovery, or Source Engine object decoding
  by itself.

### Source SDK 2013

Repository:

`https://github.com/ValveSoftware/source-sdk-2013`

Why it matters:

- Official Source 1 public codebase.
- Not L4D2's exact branch, but useful for interface names, KeyValues usage,
  server browser concepts, VGUI list panels, and Steam server browser patterns.

Useful idea to borrow:

- Search SDK for `ISteamMatchmakingServerListResponse`,
  `ISteamMatchmakingServers`, server browser panels, and KeyValues field access.
- Use it to infer class layouts and callback flow before writing detours.

Limit:

- L4D2's `matchmaking.dll` is not open-source here, so this is a map, not source
  for the exact target.

### AlliedModders / DHooks / Metamod:Source

References:

- `https://github.com/alliedmodders`
- `https://wiki.alliedmods.net/DHooks_(SourceMod_Scripting)`
- `https://wiki.alliedmods.net/User:Nosoop/Guide/Advanced`
- `https://metamodsource.net/about`

Why it matters:

- This is the mature Source Engine hooking ecosystem.
- DHooks documents gamedata, signatures, calling conventions, raw hooks, virtual
  hooks, and detours.
- Metamod:Source explains the SourceHook idea and why centralized hook management
  avoids conflicts.

Useful idea to borrow:

- Create a "gamedata-like" config for our target:
  - module name
  - signature bytes
  - calling convention
  - return type
  - argument guesses
  - offsets
- Mask absolute addresses and call displacements in signatures.

Limit:

- Mostly server-side. It will not directly hook the L4D2 client main menu group
  server list.

## Recommended Direction

Do not continue with raw fixed-RVA hooks as the final design.

Next implementation path:

1. Keep `matchmaking_probe_dll` as a short-lived read-only probe.
2. Once the target function and parameters are confirmed, replace the breakpoint
   probe with a MinHook-based x86 DLL.
3. Add L4D2VR-style signature scanning so the hook can survive minor game updates.
4. Keep filter rules in a plain config file shared with the existing Go GUI.
5. Use packet filtering only as a secondary safety layer for random matchmaking and
   late joins.

Most likely final aggressive stack:

- Go GUI: manages rules, cache cleanup, known IP discovery, and launches helper.
- Packet filter: prevents joins and catches network-level details.
- Client hook DLL: suppresses accepted/cached Steam group-server rows before they
  appear in the main menu list.

## Sources

- L4D2VR README/build/use notes: `https://github.com/sd805/l4d2vr`
- L4D2VR offset/signature overview: `https://deepwiki.com/sd805/l4d2vr/4.1-memory-management-and-offsets/`
- MinHook README/version/build notes: `https://github.com/TsudaKageyu/minhook`
- AlliedModders GitHub organization: `https://github.com/alliedmodders`
- DHooks wiki: `https://wiki.alliedmods.net/DHooks_(SourceMod_Scripting)`
- Nosoop advanced SourceMod hooking guide: `https://wiki.alliedmods.net/User:Nosoop/Guide/Advanced`
- Metamod:Source about page: `https://metamodsource.net/about`
- Source SDK 2013: `https://github.com/ValveSoftware/source-sdk-2013`

