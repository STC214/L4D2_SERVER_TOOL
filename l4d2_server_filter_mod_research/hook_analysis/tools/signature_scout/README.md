# Signature Scout

Offline signature generator for the L4D2 group-server research path.

It reads `matchmaking.dll`, extracts byte patterns for the current target RVAs,
masks common volatile x86 operands, counts pattern hits in the module, and writes a
markdown report plus a gamedata-style JSON draft.

It does not attach to or modify a running game process.

## Usage

```powershell
go run . -game "E:\SteamLibrary\steamapps\common\Left 4 Dead 2" -out "..\..\reports\signature_scout_report.md"
```

Current target RVAs:

- `0x211E0` group refresh entry
- `0x21570` manager waiting for details
- `0x21B80` details consume candidate
- `0x22750` details field extraction candidate
- `0x2D9D0` dedicated accept/reject candidate

The most useful result is `Unique Hits`. A target should remain at `1` before it is
trusted by later tooling.

