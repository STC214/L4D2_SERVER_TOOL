package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeKeywords(t *testing.T) {
	got := normalizeKeywords(" rpg \n# comment\nVIP,vip\n\nshop\r\n")
	want := []string{"rpg", "VIP", "shop"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("keyword[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestInstallAddonWritesKeywordFile(t *testing.T) {
	root := t.TempDir()
	gameDir := filepath.Join(root, "Left 4 Dead 2")

	report, err := installAddon(context.Background(), gameDir, "rpg\nvip\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(report, "Generated loose addon") {
		t.Fatalf("unexpected report: %s", report)
	}

	keywordsPath := filepath.Join(gameDir, "left4dead2", "addons", addonName, configRelPath)
	data, err := os.ReadFile(keywordsPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != "rpg\r\nvip\r\n" {
		t.Fatalf("keywords = %q", got)
	}

	addonInfo := filepath.Join(gameDir, "left4dead2", "addons", addonName, "addoninfo.txt")
	if _, err := os.Stat(addonInfo); err != nil {
		t.Fatal(err)
	}
}

func TestSafeAddonDirRejectsWrongBase(t *testing.T) {
	_, err := safeAddonDir(filepath.Join(t.TempDir(), "Not L4D2"))
	if err == nil {
		t.Fatal("expected error")
	}
}
