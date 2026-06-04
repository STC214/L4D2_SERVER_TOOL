package main

import (
	"bytes"
	"debug/pe"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf16"
)

const defaultGameDir = `E:\SteamLibrary\steamapps\common\Left 4 Dead 2`

var needles = []string{
	"serverbrowser",
	"server browser",
	"gameserver",
	"game server",
	"matchmaking",
	"matchmakingservers",
	"lobby",
	"steam",
	"favorites",
	"internet",
	"filter",
	"blacklist",
	"secure",
	"connect",
	"serverlist",
	"server list",
	"hostname",
	"map",
	"ping",
	"tags",
	"refresh",
	"addserver",
}

type target struct {
	Path string
	Kind string
}

type hit struct {
	File  string
	Kind  string
	Value string
}

func main() {
	gameDir := flag.String("game", defaultGameDir, "Left 4 Dead 2 install directory")
	out := flag.String("out", "", "output markdown report path")
	flag.Parse()

	report, err := run(*gameDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	outPath := *out
	if outPath == "" {
		outPath = filepath.Clean(filepath.Join("..", "..", "reports", "static_probe_report.md"))
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(outPath, []byte(report), 0644); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Println(outPath)
}

func run(gameDir string) (string, error) {
	gameDir = filepath.Clean(gameDir)
	targets, err := collectTargets(gameDir)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Static Probe Report\n\n")
	fmt.Fprintf(&b, "Game directory:\n\n```text\n%s\n```\n\n", gameDir)
	fmt.Fprintf(&b, "## Targets\n\n")
	for _, t := range targets {
		fmt.Fprintf(&b, "- `%s` (%s)\n", t.Path, t.Kind)
	}
	fmt.Fprintf(&b, "\n")

	var allHits []hit
	for _, t := range targets {
		data, err := os.ReadFile(t.Path)
		if err != nil {
			continue
		}
		switch t.Kind {
		case "pe":
			writePEInfo(&b, t.Path)
			allHits = append(allHits, scanStrings(t, data)...)
		case "resource", "localization", "script":
			allHits = append(allHits, scanText(t, data)...)
		}
	}

	sort.Slice(allHits, func(i, j int) bool {
		if allHits[i].File == allHits[j].File {
			return allHits[i].Value < allHits[j].Value
		}
		return allHits[i].File < allHits[j].File
	})

	fmt.Fprintf(&b, "## String Hits\n\n")
	current := ""
	for _, h := range allHits {
		if h.File != current {
			current = h.File
			fmt.Fprintf(&b, "\n### `%s`\n\n", current)
		}
		fmt.Fprintf(&b, "- `%s`: %s\n", h.Kind, sanitizeMarkdownLine(h.Value))
	}
	if len(allHits) == 0 {
		fmt.Fprintf(&b, "No hits.\n")
	}

	fmt.Fprintf(&b, "\n## Next Hook Candidates\n\n")
	fmt.Fprintf(&b, "1. Prefer `left4dead2\\bin\\matchmaking.dll` for group-server and random-match candidate selection.\n")
	fmt.Fprintf(&b, "2. Prefer `platform\\servers\\serverbrowser.dll` for classic server browser row creation and filtering UI.\n")
	fmt.Fprintf(&b, "3. Use `.res` files to identify control/class names before choosing an in-process hook boundary.\n")

	return b.String(), nil
}

func collectTargets(gameDir string) ([]target, error) {
	var targets []target
	explicit := []string{
		`bin\serverbrowser.dll`,
		`platform\servers\serverbrowser.dll`,
		`left4dead2\bin\matchmaking.dll`,
		`left4dead2\bin\matchmaking_ds.dll`,
		`platform\servers\dialogserverbrowser.res`,
		`platform\servers\addservergamespage.res`,
		`platform\servers\customserverinfodlg.res`,
		`left4dead2\resource\matchsystem.360.res`,
		`left4dead2\scripts\gameserverconfig.vdf`,
		`platform\friends\servers.vdf`,
		`platform\servers\serverbrowser_english.txt`,
		`platform\servers\serverbrowser_schinese.txt`,
	}
	for _, rel := range explicit {
		path := filepath.Join(gameDir, rel)
		if _, err := os.Stat(path); err == nil {
			targets = append(targets, target{Path: path, Kind: classify(path)})
		}
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("no probe targets found under %s", gameDir)
	}
	return targets, nil
}

func classify(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".dll", ".exe":
		return "pe"
	case ".res":
		return "resource"
	case ".txt":
		return "localization"
	case ".vdf":
		return "script"
	default:
		return "file"
	}
}

func writePEInfo(b *strings.Builder, path string) {
	f, err := pe.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(b, "## PE: `%s`\n\n", path)
	fmt.Fprintf(b, "- Machine: `0x%X`\n", f.FileHeader.Machine)
	fmt.Fprintf(b, "- Sections: `%d`\n", f.FileHeader.NumberOfSections)
	if syms, err := f.ImportedSymbols(); err == nil {
		fmt.Fprintf(b, "- Imported symbols containing probe terms:\n")
		count := 0
		for _, sym := range syms {
			if matchesNeedle(sym) {
				fmt.Fprintf(b, "  - `%s`\n", sym)
				count++
				if count >= 80 {
					fmt.Fprintf(b, "  - ... truncated\n")
					break
				}
			}
		}
		if count == 0 {
			fmt.Fprintf(b, "  - none\n")
		}
	}
	fmt.Fprintf(b, "\n")
}

func scanText(t target, data []byte) []hit {
	lines := strings.Split(string(bytes.ReplaceAll(data, []byte{0}, nil)), "\n")
	var hits []hit
	seen := map[string]bool{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || len(line) > 240 || !matchesNeedle(line) {
			continue
		}
		key := strings.ToLower(line)
		if seen[key] {
			continue
		}
		seen[key] = true
		hits = append(hits, hit{File: t.Path, Kind: t.Kind, Value: line})
	}
	return hits
}

func scanStrings(t target, data []byte) []hit {
	values := append(extractASCII(data, 6), extractUTF16LE(data, 6)...)
	seen := map[string]bool{}
	var hits []hit
	for _, s := range values {
		if len(s) > 220 || !matchesNeedle(s) {
			continue
		}
		key := strings.ToLower(s)
		if seen[key] {
			continue
		}
		seen[key] = true
		hits = append(hits, hit{File: t.Path, Kind: t.Kind, Value: s})
		if len(hits) >= 160 {
			break
		}
	}
	return hits
}

func extractASCII(data []byte, minLen int) []string {
	var out []string
	start := -1
	for i, c := range data {
		if c >= 32 && c <= 126 {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 && i-start >= minLen {
			out = append(out, string(data[start:i]))
		}
		start = -1
	}
	if start >= 0 && len(data)-start >= minLen {
		out = append(out, string(data[start:]))
	}
	return out
}

func extractUTF16LE(data []byte, minLen int) []string {
	var out []string
	var buf []uint16
	flush := func() {
		if len(buf) >= minLen {
			out = append(out, string(utf16.Decode(buf)))
		}
		buf = buf[:0]
	}
	for i := 0; i+1 < len(data); i += 2 {
		v := uint16(data[i]) | uint16(data[i+1])<<8
		if v >= 32 && v <= 126 {
			buf = append(buf, v)
			continue
		}
		flush()
	}
	flush()
	return out
}

func matchesNeedle(s string) bool {
	lower := strings.ToLower(s)
	for _, needle := range needles {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

func sanitizeMarkdownLine(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "`", "'")
	return strings.TrimSpace(s)
}
