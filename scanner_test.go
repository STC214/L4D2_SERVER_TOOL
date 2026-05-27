package main

import "testing"

func TestApplyRulesOnlyMatchesServerNameKeywords(t *testing.T) {
	cfg := Config{
		NameKeywords: []string{"rpg"},
		TagKeywords:  []string{"modded"},
		MapKeywords:  []string{"c8m1"},
		IPExact:      []string{"1.2.3.4"},
		HideEmpty:    true,
		HidePassword: true,
		MaxPingMS:    1,
		MaxPlayers:   4,
	}
	info := ServerInfo{
		Address:    "1.2.3.4:27015",
		Host:       "normal coop server",
		Map:        "c8m1_apartment",
		Keywords:   "modded",
		Players:    0,
		MaxPlayers: 8,
		Password:   true,
		PingMS:     50,
	}

	applyRules(&info, cfg)

	if info.Blocked {
		t.Fatalf("server should not be marked as matched without a server-name keyword, got reasons: %v", info.BlockReasons)
	}
	if len(info.BlockReasons) != 0 {
		t.Fatalf("unexpected match reasons: %v", info.BlockReasons)
	}
}

func TestApplyRulesMatchesServerNameCaseInsensitive(t *testing.T) {
	cfg := Config{NameKeywords: []string{"RpG"}}
	info := ServerInfo{
		Address:    "1.2.3.4:27015",
		Host:       "BEST rpg server",
		Players:    1,
		MaxPlayers: 8,
	}

	applyRules(&info, cfg)

	if !info.Blocked {
		t.Fatal("server should be marked as matched by server-name keyword")
	}
	if len(info.BlockReasons) != 1 || info.BlockReasons[0] != "名称:RpG" {
		t.Fatalf("unexpected reasons: %v", info.BlockReasons)
	}
}
