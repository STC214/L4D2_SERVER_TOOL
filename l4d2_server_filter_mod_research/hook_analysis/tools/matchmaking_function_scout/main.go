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

type Ref struct {
	Off     int
	RVA     uint32
	Kind    string
	Target  uint32
	Comment string
}

type Caller struct {
	CallRVA     uint32
	CallFileOff uint32
	CallerRVA   uint32
	Section     string
	Context     string
	StringRefs  []string
}

type PtrXref struct {
	FileOff uint32
	RVA     uint32
	Section string
	Kind    string
	Context string
}

func main() {
	gameDir := flag.String("game", `E:\SteamLibrary\steamapps\common\Left 4 Dead 2`, "Left 4 Dead 2 install directory")
	gamedataPath := flag.String("gamedata", filepath.Join("..", "..", "gamedata", "matchmaking_targets.json"), "gamedata JSON path")
	out := flag.String("out", filepath.Join("..", "..", "reports", "matchmaking_function_scout_report.md"), "report path")
	flag.Parse()

	cfg, err := loadGamedata(*gamedataPath)
	if err != nil {
		fatalf("load gamedata: %v", err)
	}
	modulePath := filepath.Join(*gameDir, filepath.FromSlash(cfg.Module))
	data, sections, imageBase, err := loadPE(modulePath)
	if err != nil {
		fatalf("load PE: %v", err)
	}
	stringIndex := buildStringIndex(data, sections, imageBase)

	var b strings.Builder
	b.WriteString("# Matchmaking Function Scout Report\n\n")
	b.WriteString(fmt.Sprintf("- Started: `%s`\n", time.Now().Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("- Module: `%s`\n", modulePath))
	b.WriteString(fmt.Sprintf("- Image base: `0x%X`\n", imageBase))
	b.WriteString(fmt.Sprintf("- Gamedata: `%s`\n\n", *gamedataPath))

	for _, target := range cfg.Targets {
		rva64, err := parseHex(target.RVA)
		if err != nil {
			continue
		}
		writeFunctionReport(&b, target, uint32(rva64), sections, imageBase, stringIndex)
	}

	writeCallerSummary(&b, cfg.Targets, sections, imageBase, stringIndex)

	b.WriteString("## Next Static Conclusions\n\n")
	b.WriteString("- Prefer the target that references `GameDetailsServer`, `Server/name`, `Server/adrlocal`, and `Server/adronline` close together.\n")
	b.WriteString("- A target with repeated `[ebp+08]` and saved `ECX` usually has a useful object/argument shape for debugger inspection.\n")
	b.WriteString("- Use this report to decide which function to inspect manually before any runtime component is changed.\n")

	if err := os.MkdirAll(filepath.Dir(*out), 0755); err != nil {
		fatalf("create report dir: %v", err)
	}
	if err := os.WriteFile(*out, []byte(b.String()), 0644); err != nil {
		fatalf("write report: %v", err)
	}
	fmt.Println("report:", *out)
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

func loadPE(path string) ([]byte, []Section, uint32, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, 0, err
	}
	f, err := pe.Open(path)
	if err != nil {
		return nil, nil, 0, err
	}
	defer f.Close()
	var imageBase uint32
	switch oh := f.OptionalHeader.(type) {
	case *pe.OptionalHeader32:
		imageBase = oh.ImageBase
	default:
		return nil, nil, 0, fmt.Errorf("expected 32-bit PE")
	}
	var sections []Section
	for _, s := range f.Sections {
		data, _ := s.Data()
		sections = append(sections, Section{
			Name:    strings.TrimRight(s.Name, "\x00"),
			RVA:     s.VirtualAddress,
			FileOff: s.Offset,
			Data:    data,
			Exec:    s.Characteristics&0x20000000 != 0 || strings.HasPrefix(strings.ToLower(s.Name), ".text"),
		})
	}
	sort.Slice(sections, func(i, j int) bool { return sections[i].RVA < sections[j].RVA })
	return raw, sections, imageBase, nil
}

func writeFunctionReport(b *strings.Builder, target Target, rva uint32, sections []Section, imageBase uint32, stringsByVA map[uint32]string) {
	sec, pos, ok := findSectionPos(sections, rva)
	if !ok {
		return
	}
	fn := sec.Data[pos:guessFunctionEnd(sec.Data, pos)]
	b.WriteString(fmt.Sprintf("## %s\n\n", target.Name))
	b.WriteString(fmt.Sprintf("- Role: `%s`\n", target.Role))
	b.WriteString(fmt.Sprintf("- Function RVA: `0x%X`\n", rva))
	b.WriteString(fmt.Sprintf("- File offset: `0x%X`\n", sec.FileOff+uint32(pos)))
	b.WriteString(fmt.Sprintf("- Guessed size: `%d` bytes\n", len(fn)))
	b.WriteString(fmt.Sprintf("- Prologue bytes: `%s`\n\n", hexBytes(fn[:min(len(fn), 32)])))

	refs := scanFunctionRefs(fn, rva, imageBase, stringsByVA)
	writeRefs(b, refs)
	writeArgHints(b, fn, rva)
	callers := findDirectCallers(sections, rva, imageBase, stringsByVA)
	writeCallers(b, callers)
	ptrs := findPointerXrefs(sections, rva, imageBase)
	writePointerXrefs(b, ptrs)
}

func scanFunctionRefs(fn []byte, baseRVA, imageBase uint32, stringsByVA map[uint32]string) []Ref {
	var refs []Ref
	for i := 0; i < len(fn); i++ {
		op := fn[i]
		if (op == 0xE8 || op == 0xE9) && i+4 < len(fn) {
			rel := int32(binary.LittleEndian.Uint32(fn[i+1 : i+5]))
			dst := uint32(int64(baseRVA) + int64(i) + 5 + int64(rel))
			kind := "call"
			if op == 0xE9 {
				kind = "jmp"
			}
			refs = append(refs, Ref{Off: i, RVA: baseRVA + uint32(i), Kind: kind, Target: dst})
			i += 4
			continue
		}
		if op == 0x68 && i+4 < len(fn) {
			imm := binary.LittleEndian.Uint32(fn[i+1 : i+5])
			comment := ""
			if s, ok := stringsByVA[imm]; ok {
				comment = s
			}
			refs = append(refs, Ref{Off: i, RVA: baseRVA + uint32(i), Kind: "push_imm", Target: imm - imageBase, Comment: comment})
			i += 4
			continue
		}
		if (op == 0xA1 || op == 0xA3) && i+4 < len(fn) {
			imm := binary.LittleEndian.Uint32(fn[i+1 : i+5])
			refs = append(refs, Ref{Off: i, RVA: baseRVA + uint32(i), Kind: fmt.Sprintf("moffs_%02X", op), Target: imm - imageBase})
			i += 4
			continue
		}
		if op == 0xC7 && i+5 < len(fn) {
			imm := binary.LittleEndian.Uint32(fn[i+2 : i+6])
			comment := ""
			if s, ok := stringsByVA[imm]; ok {
				comment = s
			}
			if comment != "" {
				refs = append(refs, Ref{Off: i, RVA: baseRVA + uint32(i), Kind: "mov_imm_string", Target: imm - imageBase, Comment: comment})
			}
		}
	}
	return refs
}

func writeRefs(b *strings.Builder, refs []Ref) {
	b.WriteString("### Direct References\n\n")
	if len(refs) == 0 {
		b.WriteString("No simple direct references found.\n\n")
		return
	}
	b.WriteString("| RVA | Kind | Target RVA | Comment |\n")
	b.WriteString("|---:|---|---:|---|\n")
	for _, r := range refs[:min(len(refs), 80)] {
		comment := r.Comment
		if len(comment) > 120 {
			comment = comment[:117] + "..."
		}
		b.WriteString(fmt.Sprintf("| `0x%X` | `%s` | `0x%X` | `%s` |\n", r.RVA, r.Kind, r.Target, escapeMD(comment)))
	}
	b.WriteString("\n")
}

func writeArgHints(b *strings.Builder, fn []byte, baseRVA uint32) {
	type hint struct {
		RVA  uint32
		Text string
	}
	var hints []hint
	for i := 0; i+2 < len(fn); i++ {
		if fn[i] == 0x8B && fn[i+1] == 0x45 {
			hints = append(hints, hint{baseRVA + uint32(i), fmt.Sprintf("mov eax, [ebp+0x%02X]", fn[i+2])})
		}
		if fn[i] == 0x8B && fn[i+1] == 0x4D {
			hints = append(hints, hint{baseRVA + uint32(i), fmt.Sprintf("mov ecx, [ebp+0x%02X]", fn[i+2])})
		}
		if fn[i] == 0x8B && fn[i+1] == 0x55 {
			hints = append(hints, hint{baseRVA + uint32(i), fmt.Sprintf("mov edx, [ebp+0x%02X]", fn[i+2])})
		}
		if fn[i] == 0x89 && fn[i+1] == 0x4D {
			hints = append(hints, hint{baseRVA + uint32(i), fmt.Sprintf("mov [ebp-0x%02X], ecx", byte(0-fn[i+2]))})
		}
		if fn[i] == 0x89 && fn[i+1] == 0x45 {
			hints = append(hints, hint{baseRVA + uint32(i), fmt.Sprintf("mov [ebp-0x%02X], eax", byte(0-fn[i+2]))})
		}
	}
	b.WriteString("### Argument/Object Hints\n\n")
	if len(hints) == 0 {
		b.WriteString("No simple EBP argument/object hints found.\n\n")
		return
	}
	for _, h := range hints[:min(len(hints), 60)] {
		b.WriteString(fmt.Sprintf("- `0x%X`: `%s`\n", h.RVA, h.Text))
	}
	b.WriteString("\n")
}

func writeCallers(b *strings.Builder, callers []Caller) {
	b.WriteString("### Direct Callers\n\n")
	if len(callers) == 0 {
		b.WriteString("No direct `call rel32` callers found.\n\n")
		return
	}
	b.WriteString("| Call RVA | Caller Function | Section | Nearby Bytes | Caller String Refs |\n")
	b.WriteString("|---:|---:|---|---|---|\n")
	for _, caller := range callers[:min(len(callers), 40)] {
		b.WriteString(fmt.Sprintf("| `0x%X` | `0x%X` | `%s` | `%s` | `%s` |\n",
			caller.CallRVA,
			caller.CallerRVA,
			caller.Section,
			caller.Context,
			escapeMD(strings.Join(firstStrings(caller.StringRefs, 8), "; ")),
		))
	}
	b.WriteString("\n")
}

func writePointerXrefs(b *strings.Builder, refs []PtrXref) {
	b.WriteString("### Function Pointer Xrefs\n\n")
	if len(refs) == 0 {
		b.WriteString("No 32-bit VA/RVA pointer references found.\n\n")
		return
	}
	b.WriteString("| RVA | File Off | Section | Kind | Nearby Bytes |\n")
	b.WriteString("|---:|---:|---|---|---|\n")
	for _, ref := range refs[:min(len(refs), 80)] {
		b.WriteString(fmt.Sprintf("| `0x%X` | `0x%X` | `%s` | `%s` | `%s` |\n", ref.RVA, ref.FileOff, ref.Section, ref.Kind, ref.Context))
	}
	b.WriteString("\n")
}

func writeCallerSummary(b *strings.Builder, targets []Target, sections []Section, imageBase uint32, stringsByVA map[uint32]string) {
	b.WriteString("## Caller Summary\n\n")
	b.WriteString("| Target | RVA | Direct Callers | Caller Function RVAs |\n")
	b.WriteString("|---|---:|---:|---|\n")
	for _, target := range targets {
		rva64, err := parseHex(target.RVA)
		if err != nil {
			continue
		}
		callers := findDirectCallers(sections, uint32(rva64), imageBase, stringsByVA)
		var funcs []string
		seen := map[uint32]bool{}
		for _, caller := range callers {
			if !seen[caller.CallerRVA] {
				seen[caller.CallerRVA] = true
				funcs = append(funcs, fmt.Sprintf("0x%X", caller.CallerRVA))
			}
		}
		b.WriteString(fmt.Sprintf("| `%s` | `0x%X` | `%d` | `%s` |\n", target.Name, uint32(rva64), len(callers), strings.Join(funcs, ", ")))
	}
	b.WriteString("\n")
}

func findDirectCallers(sections []Section, targetRVA, imageBase uint32, stringsByVA map[uint32]string) []Caller {
	var callers []Caller
	for _, s := range sections {
		if !s.Exec {
			continue
		}
		for i := 0; i+4 < len(s.Data); i++ {
			if s.Data[i] != 0xE8 {
				continue
			}
			rel := int32(binary.LittleEndian.Uint32(s.Data[i+1 : i+5]))
			callRVA := s.RVA + uint32(i)
			dst := uint32(int64(callRVA) + 5 + int64(rel))
			if dst != targetRVA {
				continue
			}
			callerRVA := guessFunctionStartRVA(s, i)
			callerStart := int(callerRVA - s.RVA)
			callerEnd := guessFunctionEnd(s.Data, callerStart)
			refs := scanFunctionRefs(s.Data[callerStart:callerEnd], callerRVA, imageBase, stringsByVA)
			callers = append(callers, Caller{
				CallRVA:     callRVA,
				CallFileOff: s.FileOff + uint32(i),
				CallerRVA:   callerRVA,
				Section:     s.Name,
				Context:     hexContext(s.Data, i, 12),
				StringRefs:  stringRefs(refs),
			})
		}
	}
	sort.Slice(callers, func(i, j int) bool {
		return callers[i].CallRVA < callers[j].CallRVA
	})
	return callers
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
					FileOff: s.FileOff + uint32(i),
					RVA:     s.RVA + uint32(i),
					Section: s.Name,
					Kind:    needle.kind,
					Context: hexContext(s.Data, i, 16),
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

func guessFunctionStartRVA(section Section, pos int) uint32 {
	searchStart := pos - 1024
	if searchStart < 0 {
		searchStart = 0
	}
	best := -1
	for i := pos; i >= searchStart; i-- {
		if looksLikePaddedFunctionStart(section.Data, i) {
			best = i
			break
		}
	}
	if best < 0 {
		for i := pos; i >= searchStart; i-- {
			if i+2 < len(section.Data) && section.Data[i] == 0x55 && section.Data[i+1] == 0x8B && section.Data[i+2] == 0xEC {
				best = i
				break
			}
		}
	}
	if best < 0 {
		best = pos
	}
	return section.RVA + uint32(best)
}

func looksLikePaddedFunctionStart(data []byte, pos int) bool {
	if pos <= 0 || pos >= len(data) || data[pos] == 0xCC || data[pos] == 0x90 {
		return false
	}
	run := 0
	for i := pos - 1; i >= 0 && run < 16; i-- {
		if data[i] != 0xCC && data[i] != 0x90 {
			break
		}
		run++
	}
	return run >= 4
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

func buildStringIndex(raw []byte, sections []Section, imageBase uint32) map[uint32]string {
	out := map[uint32]string{}
	for _, s := range sections {
		if strings.Contains(strings.ToLower(s.Name), "text") {
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
				va := imageBase + s.RVA + uint32(start)
				out[va] = string(s.Data[start:i])
			}
			start = -1
		}
		if start >= 0 && len(s.Data)-start >= 5 {
			va := imageBase + s.RVA + uint32(start)
			out[va] = string(s.Data[start:])
		}
	}
	_ = raw
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

func hexBytes(data []byte) string {
	var parts []string
	for _, b := range data {
		parts = append(parts, fmt.Sprintf("%02X", b))
	}
	return strings.Join(parts, " ")
}

func parseHex(s string) (uint64, error) {
	s = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(s)), "0x")
	return strconv.ParseUint(s, 16, 64)
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
