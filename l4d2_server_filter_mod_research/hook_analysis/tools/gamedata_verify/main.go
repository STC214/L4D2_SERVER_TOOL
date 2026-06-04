package main

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type GameData struct {
	Schema       int      `json:"schema"`
	Game         string   `json:"game"`
	Module       string   `json:"module"`
	ModuleName   string   `json:"module_name"`
	ImageBase    string   `json:"image_base"`
	SourceReport string   `json:"source_report"`
	Targets      []Target `json:"targets"`
}

type Target struct {
	Name         string `json:"name"`
	RVA          string `json:"rva"`
	Role         string `json:"role"`
	Pattern      string `json:"pattern"`
	Mask         string `json:"mask"`
	ExpectedHits int    `json:"expected_hits"`
}

type Result struct {
	Name         string
	Role         string
	RVA          uint64
	ExpectedHits int
	Hits         []int
	Error        string
}

func main() {
	gameDir := flag.String("game", `E:\SteamLibrary\steamapps\common\Left 4 Dead 2`, "Left 4 Dead 2 install directory")
	gamedataPath := flag.String("gamedata", filepath.Join("..", "..", "gamedata", "matchmaking_targets.json"), "gamedata JSON path")
	out := flag.String("out", filepath.Join("..", "..", "reports", "gamedata_verify_report.md"), "verification report path")
	flag.Parse()

	cfg, err := loadGameData(*gamedataPath)
	if err != nil {
		fatalf("load gamedata: %v", err)
	}
	modulePath := filepath.Join(*gameDir, filepath.FromSlash(cfg.Module))
	moduleBytes, err := os.ReadFile(modulePath)
	if err != nil {
		fatalf("read module: %v", err)
	}

	var results []Result
	for _, target := range cfg.Targets {
		results = append(results, verifyTarget(moduleBytes, target))
	}

	if err := writeReport(*out, *gameDir, modulePath, *gamedataPath, cfg, results); err != nil {
		fatalf("write report: %v", err)
	}
	fmt.Println("report:", *out)
	if !allOK(results) {
		os.Exit(2)
	}
}

func loadGameData(path string) (GameData, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return GameData{}, err
	}
	var cfg GameData
	if err := json.Unmarshal(data, &cfg); err != nil {
		return GameData{}, err
	}
	if cfg.Module == "" || len(cfg.Targets) == 0 {
		return GameData{}, fmt.Errorf("missing module or targets")
	}
	return cfg, nil
}

func verifyTarget(moduleBytes []byte, target Target) Result {
	result := Result{
		Name:         target.Name,
		Role:         target.Role,
		ExpectedHits: target.ExpectedHits,
	}
	if target.ExpectedHits == 0 {
		result.ExpectedHits = 1
	}
	rva, err := parseHexUint(target.RVA)
	if err != nil {
		result.Error = "invalid rva: " + err.Error()
		return result
	}
	result.RVA = rva

	pattern, err := hex.DecodeString(strings.TrimSpace(target.Pattern))
	if err != nil {
		result.Error = "invalid pattern: " + err.Error()
		return result
	}
	if len(pattern) != len(target.Mask) {
		result.Error = fmt.Sprintf("pattern length %d != mask length %d", len(pattern), len(target.Mask))
		return result
	}
	for _, ch := range target.Mask {
		if ch != 'x' && ch != '?' {
			result.Error = "mask may only contain x and ?"
			return result
		}
	}
	result.Hits = findPattern(moduleBytes, pattern, target.Mask)
	return result
}

func parseHexUint(s string) (uint64, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(strings.ToLower(s), "0x")
	return strconv.ParseUint(s, 16, 64)
}

func findPattern(data, pattern []byte, mask string) []int {
	if len(pattern) == 0 || len(pattern) != len(mask) || len(data) < len(pattern) {
		return nil
	}
	var hits []int
	for i := 0; i <= len(data)-len(pattern); i++ {
		matched := true
		for j := range pattern {
			if mask[j] == 'x' && data[i+j] != pattern[j] {
				matched = false
				break
			}
		}
		if matched {
			hits = append(hits, i)
		}
	}
	return hits
}

func writeReport(path, gameDir, modulePath, gamedataPath string, cfg GameData, results []Result) error {
	var b strings.Builder
	b.WriteString("# Gamedata Verify Report\n\n")
	b.WriteString(fmt.Sprintf("- Started: `%s`\n", time.Now().Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("- Game directory: `%s`\n", gameDir))
	b.WriteString(fmt.Sprintf("- Module: `%s`\n", modulePath))
	b.WriteString(fmt.Sprintf("- Gamedata: `%s`\n", gamedataPath))
	b.WriteString(fmt.Sprintf("- Source report: `%s`\n\n", cfg.SourceReport))

	b.WriteString("## Results\n\n")
	b.WriteString("| Target | RVA | Role | Expected | Hits | Status |\n")
	b.WriteString("|---|---:|---|---:|---:|---|\n")
	for _, result := range results {
		status := "OK"
		if result.Error != "" {
			status = "ERROR: " + result.Error
		} else if len(result.Hits) != result.ExpectedHits {
			status = "MISMATCH"
		}
		b.WriteString(fmt.Sprintf("| `%s` | `0x%X` | `%s` | `%d` | `%d` | `%s` |\n",
			escapeMD(result.Name),
			result.RVA,
			escapeMD(result.Role),
			result.ExpectedHits,
			len(result.Hits),
			escapeMD(status),
		))
	}

	b.WriteString("\n## Hit Offsets\n\n")
	for _, result := range results {
		b.WriteString(fmt.Sprintf("### %s\n\n", result.Name))
		if result.Error != "" {
			b.WriteString(fmt.Sprintf("- Error: `%s`\n\n", escapeMD(result.Error)))
			continue
		}
		if len(result.Hits) == 0 {
			b.WriteString("- No hits.\n\n")
			continue
		}
		for _, hit := range result.Hits {
			b.WriteString(fmt.Sprintf("- File offset: `0x%X`\n", hit))
		}
		b.WriteString("\n")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(b.String()), 0644)
}

func allOK(results []Result) bool {
	for _, result := range results {
		if result.Error != "" || len(result.Hits) != result.ExpectedHits {
			return false
		}
	}
	return true
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
