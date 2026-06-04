package main

import (
	"debug/pe"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type target struct {
	Name string
	RVA  uint32
}

type sectionInfo struct {
	Name      string
	FileOff   uint32
	Size      uint32
	RVA       uint32
	VirtSize  uint32
	Bytes     []byte
	Exec      bool
	ImageBase uint32
}

var defaultTargets = []target{
	{"group_refresh_entry", 0x211E0},
	{"manager_wait_details", 0x21570},
	{"details_consume_candidate", 0x21B80},
	{"details_fields_candidate", 0x22750},
	{"dedicated_accept_reject", 0x2D9D0},
}

func main() {
	gameDir := flag.String("game", `E:\SteamLibrary\steamapps\common\Left 4 Dead 2`, "Left 4 Dead 2 install directory")
	module := flag.String("module", filepath.Join("left4dead2", "bin", "matchmaking.dll"), "module path relative to game dir")
	out := flag.String("out", filepath.Join("..", "..", "reports", "signature_scout_report.md"), "report path")
	length := flag.Int("len", 48, "signature byte length")
	flag.Parse()

	modulePath := filepath.Join(*gameDir, *module)
	data, err := os.ReadFile(modulePath)
	if err != nil {
		fatalf("read module: %v", err)
	}
	sections, imageBase, err := loadSections(modulePath)
	if err != nil {
		fatalf("read PE: %v", err)
	}

	var report strings.Builder
	report.WriteString("# Signature Scout Report\n\n")
	report.WriteString(fmt.Sprintf("- Module: `%s`\n", modulePath))
	report.WriteString(fmt.Sprintf("- Image base: `0x%X`\n", imageBase))
	report.WriteString(fmt.Sprintf("- Started: `%s`\n\n", time.Now().Format(time.RFC3339)))

	report.WriteString("## Signatures\n\n")
	report.WriteString("| Name | RVA | File Off | IDA Pattern | Mask | Unique Hits |\n")
	report.WriteString("|---|---:|---:|---|---|---:|\n")

	for _, t := range defaultTargets {
		sig, mask, fileOff, err := buildSignature(sections, t.RVA, *length)
		if err != nil {
			report.WriteString(fmt.Sprintf("| `%s` | `0x%X` |  | error: `%s` |  |  |\n", t.Name, t.RVA, escapeMD(err.Error())))
			continue
		}
		hits := countPatternHits(data, sig, mask)
		report.WriteString(fmt.Sprintf("| `%s` | `0x%X` | `0x%X` | `%s` | `%s` | `%d` |\n",
			t.Name,
			t.RVA,
			fileOff,
			toIDAPattern(sig, mask),
			maskString(mask),
			hits,
		))
	}

	report.WriteString("\n## Gamedata Draft\n\n")
	report.WriteString("```json\n")
	report.WriteString("{\n")
	report.WriteString("  \"module\": \"matchmaking.dll\",\n")
	report.WriteString("  \"targets\": {\n")
	for i, t := range defaultTargets {
		sig, mask, _, err := buildSignature(sections, t.RVA, *length)
		if err != nil {
			continue
		}
		comma := ","
		if i == len(defaultTargets)-1 {
			comma = ""
		}
		report.WriteString(fmt.Sprintf("    \"%s\": { \"rva\": \"0x%X\", \"pattern\": \"%s\", \"mask\": \"%s\" }%s\n",
			t.Name, t.RVA, toBytePattern(sig), maskString(mask), comma))
	}
	report.WriteString("  }\n")
	report.WriteString("}\n")
	report.WriteString("```\n\n")

	report.WriteString("## Notes\n\n")
	report.WriteString("- Wildcards mask common absolute addresses, relative calls/jumps, and immediate pointers.\n")
	report.WriteString("- Prefer signatures with exactly `1` unique hit.\n")
	report.WriteString("- Re-check this report after game updates before enabling a hook.\n")

	if err := os.MkdirAll(filepath.Dir(*out), 0755); err != nil {
		fatalf("create report dir: %v", err)
	}
	if err := os.WriteFile(*out, []byte(report.String()), 0644); err != nil {
		fatalf("write report: %v", err)
	}
	fmt.Println("report:", *out)
}

func loadSections(path string) ([]sectionInfo, uint32, error) {
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
			return nil, 0, fmt.Errorf("64-bit image base is unsupported")
		}
		imageBase = uint32(oh.ImageBase)
	default:
		return nil, 0, fmt.Errorf("unsupported optional header")
	}

	var sections []sectionInfo
	for _, s := range f.Sections {
		b, _ := s.Data()
		virtSize := s.VirtualSize
		if virtSize == 0 {
			virtSize = s.Size
		}
		sections = append(sections, sectionInfo{
			Name:      strings.TrimRight(s.Name, "\x00"),
			FileOff:   s.Offset,
			Size:      s.Size,
			RVA:       s.VirtualAddress,
			VirtSize:  virtSize,
			Bytes:     b,
			Exec:      s.Characteristics&0x20000000 != 0 || strings.HasPrefix(strings.ToLower(s.Name), ".text"),
			ImageBase: imageBase,
		})
	}
	sort.Slice(sections, func(i, j int) bool {
		return sections[i].RVA < sections[j].RVA
	})
	return sections, imageBase, nil
}

func buildSignature(sections []sectionInfo, rva uint32, length int) ([]byte, []bool, uint32, error) {
	if length < 8 {
		length = 8
	}
	for _, s := range sections {
		if rva < s.RVA || rva >= s.RVA+uint32(len(s.Bytes)) {
			continue
		}
		pos := int(rva - s.RVA)
		end := pos + length
		if end > len(s.Bytes) {
			end = len(s.Bytes)
		}
		if end-pos < 8 {
			return nil, nil, 0, fmt.Errorf("not enough bytes in section %s", s.Name)
		}
		sig := append([]byte(nil), s.Bytes[pos:end]...)
		mask := make([]bool, len(sig))
		for i := range mask {
			mask[i] = true
		}
		maskVolatileX86(sig, mask)
		return sig, mask, s.FileOff + uint32(pos), nil
	}
	return nil, nil, 0, fmt.Errorf("rva 0x%X not found", rva)
}

func maskVolatileX86(sig []byte, mask []bool) {
	for i := 0; i < len(sig); i++ {
		switch sig[i] {
		case 0xE8, 0xE9:
			maskRange(mask, i+1, 4)
			i += 4
		case 0x68, 0xA1, 0xA3:
			maskRange(mask, i+1, 4)
			i += 4
		case 0xC7:
			if i+5 < len(sig) {
				maskRange(mask, i+2, 4)
				i += 5
			}
		case 0xFF:
			if i+5 < len(sig) && (sig[i+1] == 0x15 || sig[i+1] == 0x25 || sig[i+1] == 0x35) {
				maskRange(mask, i+2, 4)
				i += 5
			}
		}
	}
}

func maskRange(mask []bool, start, n int) {
	for i := 0; i < n; i++ {
		pos := start + i
		if pos >= 0 && pos < len(mask) {
			mask[pos] = false
		}
	}
}

func countPatternHits(data, sig []byte, mask []bool) int {
	if len(sig) == 0 || len(sig) != len(mask) || len(data) < len(sig) {
		return 0
	}
	hits := 0
	for i := 0; i <= len(data)-len(sig); i++ {
		ok := true
		for j := range sig {
			if mask[j] && data[i+j] != sig[j] {
				ok = false
				break
			}
		}
		if ok {
			hits++
		}
	}
	return hits
}

func toIDAPattern(sig []byte, mask []bool) string {
	var parts []string
	for i, b := range sig {
		if mask[i] {
			parts = append(parts, fmt.Sprintf("%02X", b))
		} else {
			parts = append(parts, "?")
		}
	}
	return strings.Join(parts, " ")
}

func toBytePattern(sig []byte) string {
	return strings.ToUpper(hex.EncodeToString(sig))
}

func maskString(mask []bool) string {
	var b strings.Builder
	for _, keep := range mask {
		if keep {
			b.WriteByte('x')
		} else {
			b.WriteByte('?')
		}
	}
	return b.String()
}

func escapeMD(s string) string {
	s = strings.ReplaceAll(s, "`", "'")
	s = strings.ReplaceAll(s, "|", "\\|")
	return s
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
