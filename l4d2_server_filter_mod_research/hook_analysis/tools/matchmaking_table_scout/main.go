package main

import (
	"debug/pe"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type GameData struct {
	Module  string   `json:"module"`
	Targets []Target `json:"targets"`
}

type Target struct {
	Name string `json:"name"`
	RVA  string `json:"rva"`
	Role string `json:"role"`
}

type Section struct {
	Name    string
	RVA     uint32
	FileOff uint32
	Data    []byte
	Exec    bool
}

type PtrXref struct {
	RVA     uint32
	FileOff uint32
	Section string
	Kind    string
}

type TableEntry struct {
	Index       int
	SlotRVA     uint32
	SlotFileOff uint32
	Value       uint32
	FunctionRVA uint32
	ValidText   bool
	Pointed     string
	StringRefs  []string
	Prologue    string
}

type AddressXref struct {
	RVA     uint32
	FileOff uint32
	Section string
	Kind    string
	Context string
}

func main() {
	gameDir := flag.String("game", `E:\SteamLibrary\steamapps\common\Left 4 Dead 2`, "Left 4 Dead 2 install directory")
	gamedataPath := flag.String("gamedata", filepath.Join("..", "..", "gamedata", "matchmaking_targets.json"), "gamedata JSON path")
	out := flag.String("out", filepath.Join("..", "..", "reports", "matchmaking_table_scout_report.md"), "report path")
	radius := flag.Int("radius", 10, "number of table slots to dump on each side")
	flag.Parse()

	cfg, err := loadGamedata(*gamedataPath)
	if err != nil {
		fatalf("load gamedata: %v", err)
	}
	modulePath := filepath.Join(*gameDir, filepath.FromSlash(cfg.Module))
	sections, imageBase, err := loadPE(modulePath)
	if err != nil {
		fatalf("load PE: %v", err)
	}
	stringIndex := buildStringIndex(sections, imageBase)

	var b strings.Builder
	b.WriteString("# Matchmaking Table Scout Report\n\n")
	b.WriteString(fmt.Sprintf("- Started: `%s`\n", time.Now().Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("- Module: `%s`\n", modulePath))
	b.WriteString(fmt.Sprintf("- Image base: `0x%X`\n", imageBase))
	b.WriteString(fmt.Sprintf("- Gamedata: `%s`\n", *gamedataPath))
	b.WriteString(fmt.Sprintf("- Radius: `%d` slots each side\n\n", *radius))

	for _, target := range cfg.Targets {
		targetRVA64, err := parseHex(target.RVA)
		if err != nil {
			continue
		}
		targetRVA := uint32(targetRVA64)
		xrefs := findPointerXrefs(sections, targetRVA, imageBase)
		writeTargetTables(&b, target, targetRVA, xrefs, sections, imageBase, stringIndex, *radius)
	}

	b.WriteString("## Practical Read\n\n")
	b.WriteString("- `details_fields_candidate` remains the first row-suppression candidate if its table neighbors also look like server-detail transformation callbacks.\n")
	b.WriteString("- A `.rdata` table xref with no direct caller usually means the function is invoked through an interface/dispatch table, so manual observation should break on the function entry and on the table owner if identified later.\n")
	b.WriteString("- For the Steam group-server display goal, prefer the path that sees `Server/name`, `Server/adronline`, `Server/adrlocal`, `Members/numSlots`, and `Members/numPlayers` before UI insertion.\n")

	if err := os.MkdirAll(filepath.Dir(*out), 0755); err != nil {
		fatalf("create report dir: %v", err)
	}
	if err := os.WriteFile(*out, []byte(b.String()), 0644); err != nil {
		fatalf("write report: %v", err)
	}
	fmt.Println("report:", *out)
}

func writeTargetTables(b *strings.Builder, target Target, targetRVA uint32, xrefs []PtrXref, sections []Section, imageBase uint32, stringsByVA map[uint32]string, radius int) {
	b.WriteString(fmt.Sprintf("## %s\n\n", target.Name))
	b.WriteString(fmt.Sprintf("- Role: `%s`\n", target.Role))
	b.WriteString(fmt.Sprintf("- Target RVA: `0x%X`\n\n", targetRVA))
	if len(xrefs) == 0 {
		b.WriteString("No VA/RVA pointer references found.\n\n")
		return
	}
	for _, xref := range xrefs {
		b.WriteString(fmt.Sprintf("### Table Around `%s` Xref At `0x%X`\n\n", xref.Kind, xref.RVA))
		b.WriteString(fmt.Sprintf("- Section: `%s`\n", xref.Section))
		b.WriteString(fmt.Sprintf("- File offset: `0x%X`\n\n", xref.FileOff))
		entries := dumpTableAround(xref, sections, imageBase, stringsByVA, radius)
		writeProbableSpan(b, xref, entries, sections, imageBase)
		writeEntries(b, entries)
	}
}

func writeEntries(b *strings.Builder, entries []TableEntry) {
	if len(entries) == 0 {
		b.WriteString("Could not dump table entries.\n\n")
		return
	}
	b.WriteString("| Rel Slot | Slot RVA | Value | Function RVA | Text? | Points To | Prologue | String refs |\n")
	b.WriteString("|---:|---:|---:|---:|---|---|---|---|\n")
	for _, e := range entries {
		valid := ""
		if e.ValidText {
			valid = "yes"
		}
		fn := ""
		if e.ValidText {
			fn = fmt.Sprintf("`0x%X`", e.FunctionRVA)
		}
		refs := strings.Join(firstStrings(e.StringRefs, 5), "; ")
		b.WriteString(fmt.Sprintf("| `%+d` | `0x%X` | `0x%X` | %s | `%s` | `%s` | `%s` | `%s` |\n",
			e.Index,
			e.SlotRVA,
			e.Value,
			fn,
			valid,
			escapeMD(e.Pointed),
			e.Prologue,
			escapeMD(refs),
		))
	}
	b.WriteString("\n")
}

func writeProbableSpan(b *strings.Builder, xref PtrXref, entries []TableEntry, sections []Section, imageBase uint32) {
	if len(entries) == 0 {
		return
	}
	centerIdx := -1
	for i, e := range entries {
		if e.SlotRVA == xref.RVA {
			centerIdx = i
			break
		}
	}
	if centerIdx < 0 {
		return
	}
	start := centerIdx
	for start > 0 && looksLikeTablePointer(entries[start-1]) {
		start--
	}
	end := centerIdx
	for end+1 < len(entries) && looksLikeTablePointer(entries[end+1]) {
		end++
	}
	if end-start+1 < 2 {
		return
	}
	startRVA := entries[start].SlotRVA
	endRVA := entries[end].SlotRVA
	b.WriteString("#### Probable Compact Pointer Span\n\n")
	b.WriteString(fmt.Sprintf("- Span RVA: `0x%X` - `0x%X`\n", startRVA, endRVA))
	b.WriteString(fmt.Sprintf("- Slot count: `%d`\n", end-start+1))
	for _, probe := range []struct {
		label string
		rva   uint32
	}{
		{"span start", startRVA},
		{"first function slot", firstFunctionSlot(entries[start : end+1])},
		{"target slot", xref.RVA},
	} {
		if probe.rva == 0 {
			continue
		}
		refs := findAddressXrefs(sections, imageBase+probe.rva)
		b.WriteString(fmt.Sprintf("- Xrefs to %s VA `0x%X`: `%d`\n", probe.label, imageBase+probe.rva, len(refs)))
		for _, ref := range refs[:min(len(refs), 8)] {
			b.WriteString(fmt.Sprintf("  - `%s` `0x%X` file `0x%X`: `%s`\n", ref.Section, ref.RVA, ref.FileOff, ref.Context))
		}
	}
	b.WriteString("\n")
	_ = imageBase
}

func firstFunctionSlot(entries []TableEntry) uint32 {
	for _, e := range entries {
		if e.ValidText {
			return e.SlotRVA
		}
	}
	return 0
}

func looksLikeTablePointer(e TableEntry) bool {
	return e.ValidText || e.Pointed != ""
}

func dumpTableAround(xref PtrXref, sections []Section, imageBase uint32, stringsByVA map[uint32]string, radius int) []TableEntry {
	sec, pos, ok := findSectionPos(sections, xref.RVA)
	if !ok {
		return nil
	}
	var entries []TableEntry
	center := pos
	start := center - radius*4
	end := center + (radius+1)*4
	if start < 0 {
		start = 0
	}
	if end > len(sec.Data) {
		end = len(sec.Data)
	}
	start -= start % 4
	for p := start; p+4 <= end; p += 4 {
		value := binary.LittleEndian.Uint32(sec.Data[p : p+4])
		entry := TableEntry{
			Index:       (p - center) / 4,
			SlotRVA:     sec.RVA + uint32(p),
			SlotFileOff: sec.FileOff + uint32(p),
			Value:       value,
		}
		if value >= imageBase {
			fnRVA := value - imageBase
			fnSec, fnPos, fnOK := findSectionPos(sections, fnRVA)
			if fnOK && fnSec.Exec {
				fnEnd := guessFunctionEnd(fnSec.Data, fnPos)
				fn := fnSec.Data[fnPos:fnEnd]
				entry.FunctionRVA = fnRVA
				entry.ValidText = true
				entry.Prologue = hexBytes(fn[:min(len(fn), 8)])
				entry.StringRefs = stringRefs(scanFunctionRefs(fn, fnRVA, imageBase, stringsByVA))
			} else if fnOK {
				entry.Pointed = describeDataPointer(fnSec, fnPos)
			}
		}
		entries = append(entries, entry)
	}
	return entries
}

func describeDataPointer(sec Section, pos int) string {
	ascii := asciiAt(sec.Data, pos)
	if ascii != "" {
		return fmt.Sprintf("%s ascii '%s'", sec.Name, ascii)
	}
	return fmt.Sprintf("%s + 0x%X", sec.Name, pos)
}

func asciiAt(data []byte, pos int) string {
	if pos < 0 || pos >= len(data) {
		return ""
	}
	end := pos
	for end < len(data) && data[end] >= 0x20 && data[end] <= 0x7e {
		end++
	}
	if end-pos >= 4 {
		s := string(data[pos:end])
		if len(s) > 48 {
			s = s[:45] + "..."
		}
		return s
	}
	return ""
}

type Ref struct {
	Comment string
}

func scanFunctionRefs(fn []byte, baseRVA, imageBase uint32, stringsByVA map[uint32]string) []Ref {
	var refs []Ref
	for i := 0; i < len(fn); i++ {
		op := fn[i]
		if op == 0x68 && i+4 < len(fn) {
			imm := binary.LittleEndian.Uint32(fn[i+1 : i+5])
			if s, ok := stringsByVA[imm]; ok {
				refs = append(refs, Ref{Comment: s})
			}
			i += 4
			continue
		}
		if op == 0xC7 && i+5 < len(fn) {
			imm := binary.LittleEndian.Uint32(fn[i+2 : i+6])
			if s, ok := stringsByVA[imm]; ok {
				refs = append(refs, Ref{Comment: s})
			}
		}
	}
	_ = baseRVA
	_ = imageBase
	return refs
}

func findPointerXrefs(sections []Section, targetRVA, imageBase uint32) []PtrXref {
	var out []PtrXref
	targetVA := imageBase + targetRVA
	var vaNeedle [4]byte
	var rvaNeedle [4]byte
	binary.LittleEndian.PutUint32(vaNeedle[:], targetVA)
	binary.LittleEndian.PutUint32(rvaNeedle[:], targetRVA)
	for _, s := range sections {
		for _, needle := range []struct {
			kind string
			data []byte
		}{
			{"VA", vaNeedle[:]},
			{"RVA", rvaNeedle[:]},
		} {
			for i := 0; i+4 <= len(s.Data); i++ {
				if string(s.Data[i:i+4]) != string(needle.data) {
					continue
				}
				out = append(out, PtrXref{
					RVA:     s.RVA + uint32(i),
					FileOff: s.FileOff + uint32(i),
					Section: s.Name,
					Kind:    needle.kind,
				})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].RVA == out[j].RVA {
			return out[i].Kind < out[j].Kind
		}
		return out[i].RVA < out[j].RVA
	})
	return out
}

func findAddressXrefs(sections []Section, value uint32) []AddressXref {
	var out []AddressXref
	var needle [4]byte
	binary.LittleEndian.PutUint32(needle[:], value)
	for _, s := range sections {
		for i := 0; i+4 <= len(s.Data); i++ {
			if string(s.Data[i:i+4]) != string(needle[:]) {
				continue
			}
			out = append(out, AddressXref{
				RVA:     s.RVA + uint32(i),
				FileOff: s.FileOff + uint32(i),
				Section: s.Name,
				Kind:    "VA",
				Context: hexContext(s.Data, i, 12),
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RVA < out[j].RVA })
	return out
}

func loadGamedata(path string) (GameData, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return GameData{}, err
	}
	var cfg GameData
	if err := json.Unmarshal(data, &cfg); err != nil {
		return GameData{}, err
	}
	return cfg, nil
}

func loadPE(path string) ([]Section, uint32, error) {
	f, err := pe.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	var imageBase uint32
	switch oh := f.OptionalHeader.(type) {
	case *pe.OptionalHeader32:
		imageBase = oh.ImageBase
	default:
		return nil, 0, fmt.Errorf("expected 32-bit PE")
	}
	var sections []Section
	for _, s := range f.Sections {
		data, _ := s.Data()
		name := strings.TrimRight(s.Name, "\x00")
		sections = append(sections, Section{
			Name:    name,
			RVA:     s.VirtualAddress,
			FileOff: s.Offset,
			Data:    data,
			Exec:    s.Characteristics&0x20000000 != 0 || strings.HasPrefix(strings.ToLower(name), ".text"),
		})
	}
	sort.Slice(sections, func(i, j int) bool { return sections[i].RVA < sections[j].RVA })
	return sections, imageBase, nil
}

func buildStringIndex(sections []Section, imageBase uint32) map[uint32]string {
	out := map[uint32]string{}
	for _, s := range sections {
		if s.Exec {
			continue
		}
		start := -1
		for i, b := range s.Data {
			if b >= 0x20 && b <= 0x7e {
				if start < 0 {
					start = i
				}
				continue
			}
			if start >= 0 && i-start >= 5 {
				out[imageBase+s.RVA+uint32(start)] = string(s.Data[start:i])
			}
			start = -1
		}
		if start >= 0 && len(s.Data)-start >= 5 {
			out[imageBase+s.RVA+uint32(start)] = string(s.Data[start:])
		}
	}
	return out
}

func stringRefs(refs []Ref) []string {
	seen := map[string]bool{}
	var out []string
	for _, ref := range refs {
		if ref.Comment == "" || seen[ref.Comment] {
			continue
		}
		seen[ref.Comment] = true
		out = append(out, ref.Comment)
	}
	return out
}

func findSectionPos(sections []Section, rva uint32) (Section, int, bool) {
	for _, s := range sections {
		if rva >= s.RVA && rva < s.RVA+uint32(len(s.Data)) {
			return s, int(rva - s.RVA), true
		}
	}
	return Section{}, 0, false
}

func guessFunctionEnd(data []byte, start int) int {
	maxEnd := min(len(data), start+4096)
	for i := start + 16; i < maxEnd; i++ {
		if data[i] != 0xCC && data[i] != 0x90 {
			continue
		}
		run := 0
		for j := i; j < maxEnd && (data[j] == 0xCC || data[j] == 0x90); j++ {
			run++
		}
		if run >= 4 {
			return i
		}
	}
	return maxEnd
}

func parseHex(s string) (uint64, error) {
	s = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(s)), "0x")
	return strconv.ParseUint(s, 16, 64)
}

func firstStrings(items []string, n int) []string {
	if len(items) <= n {
		return items
	}
	return items[:n]
}

func hexContext(data []byte, center, radius int) string {
	start := center - radius
	if start < 0 {
		start = 0
	}
	end := center + 5 + radius
	if end > len(data) {
		end = len(data)
	}
	var parts []string
	for i := start; i < end; i++ {
		part := fmt.Sprintf("%02X", data[i])
		if i == center {
			part = "[" + part
		}
		if i == center+4 {
			part += "]"
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, " ")
}

func hexBytes(data []byte) string {
	var parts []string
	for _, b := range data {
		parts = append(parts, fmt.Sprintf("%02X", b))
	}
	return strings.Join(parts, " ")
}

func escapeMD(s string) string {
	s = strings.ReplaceAll(s, "`", "'")
	s = strings.ReplaceAll(s, "|", "\\|")
	return s
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
