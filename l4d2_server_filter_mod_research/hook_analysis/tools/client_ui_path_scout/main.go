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
)

type sectionInfo struct {
	Name       string
	FileStart  uint32
	FileSize   uint32
	RVAStart   uint32
	RVASize    uint32
	Executable bool
	Data       []byte
}

type targetString struct {
	Text   string
	Offset uint32
	RVA    uint32
	VA     uint32
}

type xref struct {
	Target  targetString
	Kind    string
	FileOff uint32
	RVA     uint32
	FuncRVA uint32
	Bytes   string
}

var needles = []string{
	"There %s %d players who you're in a Steam group with.",
	"GroupServer",
	"m_iServerSteamGroupID",
	"m_iServerPlayerCount",
	"m_iServerRank",
	"JoinSteamGroup",
	"joinsteamgroup",
	"HostnameLabel",
	"?CThirdPartyServerPanel",
	"Resource/UI/ThirdPartyServerPanel.res",
	".?AVCThirdPartyServerPanel@@",
	"Server/name",
	"ServerName",
	"ServerIP",
	"No Server Address",
}

func main() {
	gameDir := flag.String("game", `E:\SteamLibrary\steamapps\common\Left 4 Dead 2`, "Left 4 Dead 2 directory")
	out := flag.String("out", filepath.Join("..", "..", "reports", "client_ui_path_scout_report.md"), "report path")
	flag.Parse()

	clientPath := filepath.Join(*gameDir, "left4dead2", "bin", "client.dll")
	data, err := os.ReadFile(clientPath)
	must(err)
	sections, imageBase, err := loadSections(clientPath)
	must(err)

	targets := findTargets(data, sections, imageBase)
	var xrefs []xref
	seen := map[string]bool{}
	for _, t := range targets {
		xrefs = append(xrefs, findXrefs(data, sections, t, seen)...)
	}
	sort.Slice(xrefs, func(i, j int) bool {
		if xrefs[i].Target.Text == xrefs[j].Target.Text {
			return xrefs[i].RVA < xrefs[j].RVA
		}
		return xrefs[i].Target.Text < xrefs[j].Target.Text
	})

	var report strings.Builder
	report.WriteString("# Client UI Path Scout Report\n\n")
	report.WriteString(fmt.Sprintf("- Started: `%s`\n", time.Now().Format(time.RFC3339)))
	report.WriteString(fmt.Sprintf("- Module: `%s`\n", clientPath))
	report.WriteString(fmt.Sprintf("- ImageBase: `0x%X`\n\n", imageBase))

	report.WriteString("## Target Strings\n\n")
	report.WriteString("| String | File Offset | RVA | VA |\n")
	report.WriteString("|---|---:|---:|---:|\n")
	for _, t := range targets {
		report.WriteString(fmt.Sprintf("| `%s` | `0x%X` | `0x%X` | `0x%X` |\n", escape(t.Text), t.Offset, t.RVA, t.VA))
	}
	report.WriteString("\n## Direct Code Xrefs\n\n")
	if len(xrefs) == 0 {
		report.WriteString("No direct VA/RVA references found in executable sections.\n\n")
	} else {
		report.WriteString("| Target | Function RVA | Ref | Kind | Bytes |\n")
		report.WriteString("|---|---:|---:|---|---|\n")
		for _, xr := range xrefs {
			report.WriteString(fmt.Sprintf("| `%s` | `0x%X` | `0x%X` | `%s` | `%s` |\n",
				escape(xr.Target.Text), xr.FuncRVA, xr.RVA, xr.Kind, xr.Bytes))
		}
		report.WriteString("\n")
	}

	report.WriteString("## Candidate Runtime Probes\n\n")
	report.WriteString("- Prefer xrefs near `CThirdPartyServerPanel`, `Resource/UI/ThirdPartyServerPanel.res`, and `HostnameLabel` for UI object setup.\n")
	report.WriteString("- Prefer xrefs near `GroupServer`, `m_iServerSteamGroupID`, `m_iServerPlayerCount`, and `ServerName` for row data/cache population.\n")
	report.WriteString("- If no direct xrefs appear for a string, use neighboring class RTTI/vtable references rather than broad memory scanning.\n")

	must(os.MkdirAll(filepath.Dir(*out), 0755))
	must(os.WriteFile(*out, []byte(report.String()), 0644))
	fmt.Println("report:", *out)
	fmt.Println("targets:", len(targets))
	fmt.Println("xrefs:", len(xrefs))
}

func loadSections(path string) ([]sectionInfo, uint32, error) {
	f, err := pe.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	oh, ok := f.OptionalHeader.(*pe.OptionalHeader32)
	if !ok {
		return nil, 0, fmt.Errorf("not PE32")
	}
	var sections []sectionInfo
	for _, s := range f.Sections {
		d, _ := s.Data()
		rvaSize := s.VirtualSize
		if rvaSize == 0 {
			rvaSize = s.Size
		}
		name := strings.TrimRight(s.Name, "\x00")
		sections = append(sections, sectionInfo{
			Name:       name,
			FileStart:  s.Offset,
			FileSize:   s.Size,
			RVAStart:   s.VirtualAddress,
			RVASize:    rvaSize,
			Executable: s.Characteristics&0x20000000 != 0 || strings.HasPrefix(strings.ToLower(name), ".text"),
			Data:       d,
		})
	}
	return sections, oh.ImageBase, nil
}

func findTargets(data []byte, sections []sectionInfo, imageBase uint32) []targetString {
	var out []targetString
	for _, n := range needles {
		start := 0
		for {
			idx := bytes.Index(data[start:], []byte(n))
			if idx < 0 {
				break
			}
			off := uint32(start + idx)
			if rva, ok := fileOffsetToRVA(sections, off); ok {
				out = append(out, targetString{Text: n, Offset: off, RVA: rva, VA: imageBase + rva})
			}
			start += idx + 1
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Offset < out[j].Offset })
	return out
}

func findXrefs(data []byte, sections []sectionInfo, target targetString, seen map[string]bool) []xref {
	var out []xref
	var vaNeedle, rvaNeedle [4]byte
	binary.LittleEndian.PutUint32(vaNeedle[:], target.VA)
	binary.LittleEndian.PutUint32(rvaNeedle[:], target.RVA)
	for _, s := range sections {
		if !s.Executable || len(s.Data) < 4 {
			continue
		}
		for _, item := range []struct {
			kind   string
			needle []byte
		}{{"VA", vaNeedle[:]}, {"RVA", rvaNeedle[:]}} {
			pos := 0
			for {
				idx := bytes.Index(s.Data[pos:], item.needle)
				if idx < 0 {
					break
				}
				idx += pos
				fileOff := s.FileStart + uint32(idx)
				refRVA := s.RVAStart + uint32(idx)
				key := fmt.Sprintf("%s|%s|%x", target.Text, item.kind, refRVA)
				if !seen[key] {
					seen[key] = true
					out = append(out, xref{
						Target:  target,
						Kind:    item.kind,
						FileOff: fileOff,
						RVA:     refRVA,
						FuncRVA: guessFunctionRVA(s, idx),
						Bytes:   hexContext(data, int(fileOff), 14),
					})
				}
				pos = idx + 1
			}
		}
	}
	return out
}

func fileOffsetToRVA(sections []sectionInfo, off uint32) (uint32, bool) {
	for _, s := range sections {
		if off >= s.FileStart && off < s.FileStart+s.FileSize {
			return s.RVAStart + off - s.FileStart, true
		}
	}
	return 0, false
}

func guessFunctionRVA(s sectionInfo, refPos int) uint32 {
	start := refPos - 1536
	if start < 0 {
		start = 0
	}
	for i := refPos; i >= start; i-- {
		if i > 0 && i+2 < len(s.Data) && s.Data[i] == 0x55 && s.Data[i+1] == 0x8B && s.Data[i+2] == 0xEC {
			return s.RVAStart + uint32(i)
		}
		if i > 4 && !isPad(s.Data[i]) {
			pads := 0
			for j := i - 1; j >= 0 && pads < 8 && isPad(s.Data[j]); j-- {
				pads++
			}
			if pads >= 4 {
				return s.RVAStart + uint32(i)
			}
		}
	}
	return s.RVAStart + uint32(refPos)
}

func isPad(b byte) bool {
	return b == 0xCC || b == 0x90
}

func hexContext(data []byte, center int, radius int) string {
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

func escape(s string) string {
	return strings.ReplaceAll(s, "`", "'")
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
