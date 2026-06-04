package main

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf16"
)

type targetFile struct {
	Label string
	Path  string
}

type matchRecord struct {
	Module  string
	Keyword string
	String  string
	Offset  int
	Kind    string
}

type peSectionInfo struct {
	Name       string
	FileStart  uint32
	FileSize   uint32
	RVAStart   uint32
	RVASize    uint32
	Executable bool
	Data       []byte
}

type xrefRecord struct {
	TargetString string
	TargetOffset int
	TargetRVA    uint32
	TargetVA     uint32
	RefKind      string
	RefFileOff   uint32
	RefRVA       uint32
	FuncFileOff  uint32
	FuncRVA      uint32
	Section      string
	Context      string
}

var keywordGroups = map[string][]string{
	"group_list": {
		"Group", "SteamGroup", "group server", "GroupServer", "serverlist",
		"ServerList", "GameDetailsServer", "InetSearchServerDetails",
	},
	"server_browser": {
		"ServerBrowser", "CInternetGames", "GetNewServerList", "RefreshServer",
		"ConnectToServer", "FilterString", "TagFilter", "ServerBrowser003",
	},
	"matchmaking": {
		"ConnectServerDetailsRequest", "Establishing connection", "search results",
		"dedicated server", "ip filter", "rejected dedicated", "registered dedicated",
	},
	"steam_api": {
		"SteamMatchMakingServers", "ISteamMatchmakingServers", "gameserveritem_t",
		"RequestInternetServerList", "RequestFavoritesServerList", "RequestSpectatorServerList",
	},
	"vgui": {
		"ListPanel", "SectionedListPanel", "AddItem", "SetItemText", "OnItemSelected",
		"Panel", "PropertySheet", "Page_Filters",
	},
	"source_query": {
		"A2S_INFO", "challenge", "connect", "connectstring", "server", "HostName",
	},
}

func main() {
	gameDir := flag.String("game", `E:\SteamLibrary\steamapps\common\Left 4 Dead 2`, "Left 4 Dead 2 install directory")
	out := flag.String("out", filepath.Join("..", "..", "reports", "hook_target_scout_report.md"), "report path")
	flag.Parse()

	targets := buildTargets(*gameDir)
	var report strings.Builder
	report.WriteString("# Hook Target Scout Report\n\n")
	report.WriteString(fmt.Sprintf("- Game directory: `%s`\n", *gameDir))
	report.WriteString(fmt.Sprintf("- Started: `%s`\n\n", time.Now().Format(time.RFC3339)))

	var allMatches []matchRecord
	for _, target := range targets {
		matches, err := scanTarget(target, &report)
		if err != nil {
			report.WriteString(fmt.Sprintf("## %s\n\n`%s`\n\nError: `%v`\n\n", target.Label, target.Path, err))
			continue
		}
		allMatches = append(allMatches, matches...)
	}

	writeSummary(&report, allMatches)
	if err := os.MkdirAll(filepath.Dir(*out), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "create report dir: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, []byte(report.String()), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "write report: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("report:", *out)
	fmt.Println("matches:", len(allMatches))
}

func buildTargets(gameDir string) []targetFile {
	return []targetFile{
		{"matchmaking.dll", filepath.Join(gameDir, "left4dead2", "bin", "matchmaking.dll")},
		{"client.dll", filepath.Join(gameDir, "left4dead2", "bin", "client.dll")},
		{"engine.dll", filepath.Join(gameDir, "bin", "engine.dll")},
		{"serverbrowser.dll", filepath.Join(gameDir, "bin", "serverbrowser.dll")},
		{"platform serverbrowser.dll", filepath.Join(gameDir, "platform", "servers", "serverbrowser.dll")},
		{"vgui2.dll", filepath.Join(gameDir, "bin", "vgui2.dll")},
		{"steam_api.dll", filepath.Join(gameDir, "bin", "steam_api.dll")},
		{"dialogserverbrowser.res", filepath.Join(gameDir, "platform", "servers", "dialogserverbrowser.res")},
		{"serverbrowser_english.txt", filepath.Join(gameDir, "platform", "servers", "serverbrowser_english.txt")},
		{"Page_Filters.res", filepath.Join(gameDir, "platform", "servers", "Page_Filters.res")},
	}
}

func scanTarget(target targetFile, report *strings.Builder) ([]matchRecord, error) {
	data, err := os.ReadFile(target.Path)
	if err != nil {
		return nil, err
	}
	report.WriteString(fmt.Sprintf("## %s\n\n`%s`\n\n", target.Label, target.Path))
	report.WriteString(fmt.Sprintf("- Size: `%d` bytes\n", len(data)))
	writePEInfo(target.Path, report)

	strs := append(extractASCIIStrings(data, 5), extractUTF16Strings(data, 5)...)
	sort.Slice(strs, func(i, j int) bool {
		return strs[i].Offset < strs[j].Offset
	})
	matches := matchStrings(target.Label, strs)
	writeMatches(report, matches)
	writeXrefs(target.Path, data, matches, report)
	return matches, nil
}

type foundString struct {
	Text   string
	Offset int
	Kind   string
}

func extractASCIIStrings(data []byte, minLen int) []foundString {
	var out []foundString
	start := -1
	for i, b := range data {
		if b >= 0x20 && b <= 0x7e {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 && i-start >= minLen {
			out = append(out, foundString{Text: string(data[start:i]), Offset: start, Kind: "ascii"})
		}
		start = -1
	}
	if start >= 0 && len(data)-start >= minLen {
		out = append(out, foundString{Text: string(data[start:]), Offset: start, Kind: "ascii"})
	}
	return out
}

func extractUTF16Strings(data []byte, minLen int) []foundString {
	var out []foundString
	start := -1
	var chars []uint16
	flush := func(end int) {
		if start >= 0 && len(chars) >= minLen {
			out = append(out, foundString{Text: string(utf16.Decode(chars)), Offset: start, Kind: "utf16le"})
		}
		start = -1
		chars = nil
		_ = end
	}
	for i := 0; i+1 < len(data); i += 2 {
		ch := uint16(data[i]) | uint16(data[i+1])<<8
		if ch >= 0x20 && ch <= 0x7e {
			if start < 0 {
				start = i
			}
			chars = append(chars, ch)
			continue
		}
		flush(i)
	}
	flush(len(data))
	return out
}

func matchStrings(module string, stringsFound []foundString) []matchRecord {
	var out []matchRecord
	seen := map[string]bool{}
	for _, fs := range stringsFound {
		lower := strings.ToLower(fs.Text)
		for group, keywords := range keywordGroups {
			for _, keyword := range keywords {
				if strings.Contains(lower, strings.ToLower(keyword)) {
					key := fmt.Sprintf("%s|%s|%d|%s", module, group, fs.Offset, fs.Text)
					if seen[key] {
						continue
					}
					seen[key] = true
					out = append(out, matchRecord{
						Module:  module,
						Keyword: group + ":" + keyword,
						String:  cleanString(fs.Text),
						Offset:  fs.Offset,
						Kind:    fs.Kind,
					})
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Keyword == out[j].Keyword {
			return out[i].Offset < out[j].Offset
		}
		return out[i].Keyword < out[j].Keyword
	})
	return out
}

func writePEInfo(path string, report *strings.Builder) {
	f, err := pe.Open(path)
	if err != nil {
		report.WriteString("- PE: no\n\n")
		return
	}
	defer f.Close()
	report.WriteString("- PE: yes\n")
	if f.FileHeader.Machine == pe.IMAGE_FILE_MACHINE_I386 {
		report.WriteString("- Machine: `i386`\n")
	} else {
		report.WriteString(fmt.Sprintf("- Machine: `0x%x`\n", f.FileHeader.Machine))
	}
	if imports, err := f.ImportedLibraries(); err == nil && len(imports) > 0 {
		sort.Strings(imports)
		report.WriteString("- Imported libraries:\n")
		for _, lib := range imports {
			report.WriteString(fmt.Sprintf("  - `%s`\n", lib))
		}
	}
	if symbols, err := f.ImportedSymbols(); err == nil {
		var interesting []string
		for _, sym := range symbols {
			lower := strings.ToLower(sym)
			if strings.Contains(lower, "steam") || strings.Contains(lower, "vgui") || strings.Contains(lower, "server") || strings.Contains(lower, "match") {
				interesting = append(interesting, sym)
			}
		}
		sort.Strings(interesting)
		if len(interesting) > 0 {
			report.WriteString("- Interesting imports:\n")
			for _, sym := range firstN(interesting, 80) {
				report.WriteString(fmt.Sprintf("  - `%s`\n", sym))
			}
		}
	}
	report.WriteString("\n")
}

func writeMatches(report *strings.Builder, matches []matchRecord) {
	if len(matches) == 0 {
		report.WriteString("No keyword matches.\n\n")
		return
	}
	report.WriteString("| Group/Keyword | Offset | Kind | String |\n")
	report.WriteString("|---|---:|---|---|\n")
	for _, m := range firstMatchN(matches, 180) {
		report.WriteString(fmt.Sprintf("| `%s` | `0x%X` | `%s` | `%s` |\n", m.Keyword, m.Offset, m.Kind, escapeMD(m.String)))
	}
	if len(matches) > 180 {
		report.WriteString(fmt.Sprintf("\nTruncated `%d` additional matches.\n", len(matches)-180))
	}
	report.WriteString("\n")
}

func writeXrefs(path string, data []byte, matches []matchRecord, report *strings.Builder) {
	peInfo, imageBase, err := loadPESections(path)
	if err != nil {
		return
	}
	targets := selectXrefTargets(matches)
	if len(targets) == 0 {
		return
	}

	var xrefs []xrefRecord
	seen := map[string]bool{}
	for _, target := range targets {
		targetRVA, ok := fileOffsetToRVA(peInfo, uint32(target.Offset))
		if !ok {
			continue
		}
		targetVA := imageBase + targetRVA
		xrefs = append(xrefs, findXrefsForTarget(peInfo, data, target, targetRVA, targetVA, seen)...)
	}

	report.WriteString("### Static String Xrefs\n\n")
	if len(xrefs) == 0 {
		report.WriteString("No direct 32-bit VA/RVA references found in executable sections for high-value strings.\n\n")
		return
	}

	sort.Slice(xrefs, func(i, j int) bool {
		if xrefs[i].TargetOffset == xrefs[j].TargetOffset {
			return xrefs[i].RefRVA < xrefs[j].RefRVA
		}
		return xrefs[i].TargetOffset < xrefs[j].TargetOffset
	})

	report.WriteString("| Target | Target Off | Target RVA | Function | Ref | Section | Bytes |\n")
	report.WriteString("|---|---:|---:|---:|---:|---|---|\n")
	for _, xr := range firstXrefN(xrefs, 160) {
		report.WriteString(fmt.Sprintf("| `%s` | `0x%X` | `0x%X` | `0x%X` | `%s 0x%X / RVA 0x%X` | `%s` | `%s` |\n",
			escapeMD(cleanString(xr.TargetString)),
			xr.TargetOffset,
			xr.TargetRVA,
			xr.FuncRVA,
			xr.RefKind,
			xr.RefFileOff,
			xr.RefRVA,
			xr.Section,
			xr.Context,
		))
	}
	if len(xrefs) > 160 {
		report.WriteString(fmt.Sprintf("\nTruncated `%d` additional xrefs.\n", len(xrefs)-160))
	}
	report.WriteString("\n")
}

func loadPESections(path string) ([]peSectionInfo, uint32, error) {
	f, err := pe.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()

	var imageBase uint32
	switch oh := f.OptionalHeader.(type) {
	case *pe.OptionalHeader32:
		imageBase = oh.ImageBase
	case *pe.OptionalHeader64:
		if oh.ImageBase > 0xffffffff {
			return nil, 0, fmt.Errorf("64-bit image base does not fit 32-bit xref scan")
		}
		imageBase = uint32(oh.ImageBase)
	default:
		return nil, 0, fmt.Errorf("unsupported optional header")
	}

	var sections []peSectionInfo
	for _, s := range f.Sections {
		sectionData, err := s.Data()
		if err != nil {
			sectionData = nil
		}
		rvaSize := s.VirtualSize
		if rvaSize == 0 {
			rvaSize = s.Size
		}
		sections = append(sections, peSectionInfo{
			Name:       strings.TrimRight(s.Name, "\x00"),
			FileStart:  s.Offset,
			FileSize:   s.Size,
			RVAStart:   s.VirtualAddress,
			RVASize:    rvaSize,
			Executable: s.Characteristics&0x20000000 != 0 || strings.HasPrefix(strings.ToLower(s.Name), ".text"),
			Data:       sectionData,
		})
	}
	return sections, imageBase, nil
}

func selectXrefTargets(matches []matchRecord) []matchRecord {
	var out []matchRecord
	seen := map[int]bool{}
	for _, m := range matches {
		if seen[m.Offset] || !isHighValueXrefString(m.String) {
			continue
		}
		seen[m.Offset] = true
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Offset < out[j].Offset
	})
	return firstMatchN(out, 80)
}

func isHighValueXrefString(s string) bool {
	lower := strings.ToLower(s)
	needles := []string{
		"requesting group server list",
		"server manager waiting",
		"server manager refreshing",
		"server manager refresh completed",
		"inetsearchserverdetails",
		"gamedetailsserver",
		"connectserverdetailsrequest",
		"server/adrlocal",
		"server/adronline",
		"server/name",
		"received %d search results",
		"establishing connection with %d search results",
		"registered dedicated server",
		"rejected dedicated server",
		"options/serverlist",
		"steam matchmaking",
		"steammatchmakingservers",
		"serverbrowser003",
		"vguimoduleserverbrowser",
		"cinternetgames",
		"getnewserverlist",
		"connecttoserver",
		"filterstring",
		"additem",
		"setitemtext",
	}
	for _, needle := range needles {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

func fileOffsetToRVA(sections []peSectionInfo, off uint32) (uint32, bool) {
	for _, s := range sections {
		if off >= s.FileStart && off < s.FileStart+s.FileSize {
			return s.RVAStart + (off - s.FileStart), true
		}
	}
	return 0, false
}

func findXrefsForTarget(sections []peSectionInfo, fileData []byte, target matchRecord, targetRVA, targetVA uint32, seen map[string]bool) []xrefRecord {
	var out []xrefRecord
	var vaNeedle [4]byte
	var rvaNeedle [4]byte
	binary.LittleEndian.PutUint32(vaNeedle[:], targetVA)
	binary.LittleEndian.PutUint32(rvaNeedle[:], targetRVA)

	for _, section := range sections {
		if !section.Executable || len(section.Data) < 4 {
			continue
		}
		for _, refKind := range []struct {
			name   string
			needle []byte
		}{
			{"VA", vaNeedle[:]},
			{"RVA", rvaNeedle[:]},
		} {
			idx := 0
			for {
				pos := bytes.Index(section.Data[idx:], refKind.needle)
				if pos < 0 {
					break
				}
				pos += idx
				refFileOff := section.FileStart + uint32(pos)
				refRVA := section.RVAStart + uint32(pos)
				key := fmt.Sprintf("%d|%s|%d", target.Offset, refKind.name, refFileOff)
				if !seen[key] {
					seen[key] = true
					funcFileOff, funcRVA := guessFunctionStart(section, pos)
					out = append(out, xrefRecord{
						TargetString: target.String,
						TargetOffset: target.Offset,
						TargetRVA:    targetRVA,
						TargetVA:     targetVA,
						RefKind:      refKind.name,
						RefFileOff:   refFileOff,
						RefRVA:       refRVA,
						FuncFileOff:  funcFileOff,
						FuncRVA:      funcRVA,
						Section:      section.Name,
						Context:      hexContext(fileData, int(refFileOff), 14),
					})
				}
				idx = pos + 1
			}
		}
	}
	return out
}

func guessFunctionStart(section peSectionInfo, refPos int) (uint32, uint32) {
	searchStart := refPos - 1024
	if searchStart < 0 {
		searchStart = 0
	}

	best := -1
	for i := refPos; i >= searchStart; i-- {
		if looksLikePaddedFunctionStart(section.Data, i) {
			best = i
			break
		}
	}
	if best < 0 {
		for i := refPos; i >= searchStart; i-- {
			if looksLikeFramePrologue(section.Data, i) {
				best = i
				break
			}
		}
	}
	if best < 0 {
		best = refPos
	}
	return section.FileStart + uint32(best), section.RVAStart + uint32(best)
}

func looksLikePaddedFunctionStart(data []byte, pos int) bool {
	if pos <= 0 || pos >= len(data) {
		return false
	}
	if isPaddingByte(data[pos]) {
		return false
	}
	padding := 0
	for i := pos - 1; i >= 0 && padding < 12; i-- {
		if !isPaddingByte(data[i]) {
			break
		}
		padding++
	}
	if padding < 4 {
		return false
	}
	return true
}

func looksLikeFramePrologue(data []byte, pos int) bool {
	if pos+2 >= len(data) {
		return false
	}
	return data[pos] == 0x55 && data[pos+1] == 0x8B && data[pos+2] == 0xEC
}

func isPaddingByte(b byte) bool {
	return b == 0xCC || b == 0x90
}

func hexContext(data []byte, center, radius int) string {
	start := center - radius
	if start < 0 {
		start = 0
	}
	end := center + 4 + radius
	if end > len(data) {
		end = len(data)
	}
	var b strings.Builder
	for i := start; i < end; i++ {
		if i > start {
			b.WriteByte(' ')
		}
		if i == center {
			b.WriteByte('[')
		}
		b.WriteString(fmt.Sprintf("%02X", data[i]))
		if i == center+3 {
			b.WriteByte(']')
		}
	}
	return b.String()
}

func writeSummary(report *strings.Builder, matches []matchRecord) {
	report.WriteString("## Summary\n\n")
	counts := map[string]int{}
	for _, m := range matches {
		counts[m.Module]++
	}
	var modules []string
	for module := range counts {
		modules = append(modules, module)
	}
	sort.Strings(modules)
	for _, module := range modules {
		report.WriteString(fmt.Sprintf("- `%s`: `%d` matches\n", module, counts[module]))
	}
	report.WriteString("\n## Next Hook Hypotheses\n\n")
	report.WriteString("- Prefer `matchmaking.dll` if group-server rows share the same server candidate path as random matchmaking.\n")
	report.WriteString("- Prefer `serverbrowser.dll` if group-server UI uses the classic Steam server browser panels.\n")
	report.WriteString("- Prefer VGUI/list-panel hooks only after finding `AddItem`/`SetItemText` style calls near server strings.\n")
	report.WriteString("- Pure VPK remains unlikely unless resource overrides expose an existing filter control with bound behavior.\n")
}

func cleanString(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	if len(s) > 180 {
		s = s[:177] + "..."
	}
	return s
}

func escapeMD(s string) string {
	s = strings.ReplaceAll(s, "`", "'")
	s = strings.ReplaceAll(s, "|", "\\|")
	return s
}

func firstN(items []string, n int) []string {
	if len(items) <= n {
		return items
	}
	return items[:n]
}

func firstMatchN(items []matchRecord, n int) []matchRecord {
	if len(items) <= n {
		return items
	}
	return items[:n]
}

func firstXrefN(items []xrefRecord, n int) []xrefRecord {
	if len(items) <= n {
		return items
	}
	return items[:n]
}

func bytesContainsFold(haystack []byte, needle string) bool {
	return bytes.Contains(bytes.ToLower(haystack), []byte(strings.ToLower(needle)))
}
