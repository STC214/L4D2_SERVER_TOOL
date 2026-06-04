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

func TestApplyRulesMarksUnknownCandidateAsBlocked(t *testing.T) {
	info := ServerInfo{
		Address:    "2.3.4.5:27015",
		Host:       "候选服务器，暂未获取名称",
		Players:    -1,
		MaxPlayers: -1,
		PingMS:     0,
	}

	applyRules(&info, Config{})

	if !info.Blocked {
		t.Fatal("unknown candidate should be marked as matched")
	}
	if len(info.BlockReasons) != 1 || info.BlockReasons[0] != "未知服务器" {
		t.Fatalf("unexpected reasons: %v", info.BlockReasons)
	}
}

func TestMaskLikeAddressIsNotMatchedOrExported(t *testing.T) {
	for _, address := range []string{
		"0.0.255.255:27015",
		"0.0.0.0:27015",
		"255.255.255.255:27015",
		"255.255.0.0:27015",
		"224.0.0.1:27015",
		"12.34.0.0:27015",
		"12.0.0.0:27015",
		"12.34.56.0:27015",
		"12.34.56.78:2701",
		"12.34.56.78:270150",
	} {
		info := ServerInfo{
			Address:    address,
			Host:       "bad rpg server",
			Players:    -1,
			MaxPlayers: -1,
			PingMS:     0,
		}

		applyRules(&info, Config{NameKeywords: []string{"rpg"}})

		if info.Blocked {
			t.Fatalf("mask-like address should not be marked as matched: %s reasons=%v", address, info.BlockReasons)
		}
		if isUsableServerAddress(address) {
			t.Fatalf("mask-like address should not be usable: %s", address)
		}
	}
}

func TestUsableServerAddressRequiresFiveDigitPortAndHostAddress(t *testing.T) {
	valid := "12.34.56.78:27015"
	if !isUsableServerAddress(valid) {
		t.Fatalf("expected usable server address: %s", valid)
	}
	for _, address := range []string{
		"12.34.56.78:2701",
		"12.34.56.78:270150",
		"12.34.56.0:27015",
		"12.34.0.0:27015",
		"12.0.0.0:27015",
	} {
		if isUsableServerAddress(address) {
			t.Fatalf("expected unusable server address: %s", address)
		}
	}
}
