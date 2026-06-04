package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type Hit struct {
	Point string
	Line  int
	Texts []string
}

func main() {
	logPath := flag.String("log", filepath.Join("..", "matchmaking_probe_dll", "matchmaking_probe.log"), "probe log path")
	out := flag.String("out", filepath.Join("..", "..", "reports", "matchmaking_probe_log_report.md"), "report path")
	flag.Parse()

	hits, lines, err := parseLog(*logPath)
	if err != nil {
		fatalf("parse log: %v", err)
	}

	report := buildReport(*logPath, hits, lines)
	if err := os.MkdirAll(filepath.Dir(*out), 0755); err != nil {
		fatalf("create report dir: %v", err)
	}
	if err := os.WriteFile(*out, []byte(report), 0644); err != nil {
		fatalf("write report: %v", err)
	}
	fmt.Println("report:", *out)
}

func parseLog(path string) ([]Hit, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()

	hitRe := regexp.MustCompile(`hit ([A-Za-z0-9_]+) #`)
	textRe := regexp.MustCompile(`-> "([^"]+)"`)

	var hits []Hit
	var current *Hit
	lineNo := 0
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		if m := hitRe.FindStringSubmatch(line); len(m) == 2 {
			hits = append(hits, Hit{Point: m[1], Line: lineNo})
			current = &hits[len(hits)-1]
			continue
		}
		if current == nil {
			continue
		}
		for _, m := range textRe.FindAllStringSubmatch(line, -1) {
			if len(m) == 2 {
				current.Texts = append(current.Texts, m[1])
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, lineNo, err
	}
	return hits, lineNo, nil
}

func buildReport(logPath string, hits []Hit, lines int) string {
	var b strings.Builder
	b.WriteString("# Matchmaking Probe Log Report\n\n")
	b.WriteString(fmt.Sprintf("- Started: `%s`\n", time.Now().Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("- Log: `%s`\n", logPath))
	b.WriteString(fmt.Sprintf("- Lines: `%d`\n", lines))
	b.WriteString(fmt.Sprintf("- Hits: `%d`\n\n", len(hits)))

	counts := map[string]int{}
	textsByPoint := map[string]map[string]int{}
	keywords := []string{
		"GameDetailsServer",
		"Server/name",
		"Server/server",
		"Server/adronline",
		"Server/adrlocal",
		"Members/numSlots",
		"Members/numPlayers",
		"RPG",
		"星缘",
		"破晓",
		"杀戮",
		"神域",
	}
	keywordHits := map[string]int{}

	for _, hit := range hits {
		counts[hit.Point]++
		if textsByPoint[hit.Point] == nil {
			textsByPoint[hit.Point] = map[string]int{}
		}
		for _, text := range hit.Texts {
			textsByPoint[hit.Point][text]++
			for _, keyword := range keywords {
				if strings.Contains(strings.ToLower(text), strings.ToLower(keyword)) {
					keywordHits[keyword]++
				}
			}
		}
	}

	b.WriteString("## Hit Counts\n\n")
	if len(counts) == 0 {
		b.WriteString("No probe hits found.\n\n")
	} else {
		b.WriteString("| Point | Hits |\n")
		b.WriteString("|---|---:|\n")
		for _, point := range sortedKeys(counts) {
			b.WriteString(fmt.Sprintf("| `%s` | `%d` |\n", point, counts[point]))
		}
		b.WriteString("\n")
	}

	b.WriteString("## Keyword Evidence\n\n")
	b.WriteString("| Keyword | Text Hits |\n")
	b.WriteString("|---|---:|\n")
	for _, keyword := range keywords {
		b.WriteString(fmt.Sprintf("| `%s` | `%d` |\n", keyword, keywordHits[keyword]))
	}
	b.WriteString("\n")

	b.WriteString("## Distinct Text Samples By Point\n\n")
	for _, point := range sortedKeys(textsByPoint) {
		b.WriteString(fmt.Sprintf("### %s\n\n", point))
		samples := topTextSamples(textsByPoint[point], 20)
		if len(samples) == 0 {
			b.WriteString("No decoded text samples.\n\n")
			continue
		}
		b.WriteString("| Count | Text |\n")
		b.WriteString("|---:|---|\n")
		for _, sample := range samples {
			b.WriteString(fmt.Sprintf("| `%d` | `%s` |\n", sample.Count, escapeMD(sample.Text)))
		}
		b.WriteString("\n")
	}

	b.WriteString("## Practical Read\n\n")
	b.WriteString("- If `details_fields_candidate` contains server names, addresses, and member counts, it is the first practical row-suppression target.\n")
	b.WriteString("- If only `details_consume_candidate` sees useful fields, move one step earlier in the detail-response path.\n")
	b.WriteString("- If neither point exposes row data, inspect the callback-table owner path around `details_table_init_write` and the first table callback.\n")
	return b.String()
}

type TextSample struct {
	Text  string
	Count int
}

func topTextSamples(items map[string]int, n int) []TextSample {
	var out []TextSample
	for text, count := range items {
		out = append(out, TextSample{Text: text, Count: count})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count == out[j].Count {
			return out[i].Text < out[j].Text
		}
		return out[i].Count > out[j].Count
	})
	if len(out) > n {
		return out[:n]
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	var keys []string
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func escapeMD(s string) string {
	s = strings.ReplaceAll(s, "`", "'")
	s = strings.ReplaceAll(s, "|", "\\|")
	if len(s) > 220 {
		s = s[:217] + "..."
	}
	return s
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
