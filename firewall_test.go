package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestBuildHuorongRules(t *testing.T) {
	infos := []ServerInfo{
		{
			Address:      "1.2.3.4:27015",
			Host:         "bad rpg server",
			Blocked:      true,
			BlockReasons: []string{"名称:rpg"},
		},
	}
	rules := buildHuorongRules(infos, []string{"1.2.3.4"})
	payload := huorongIPBlacklistFile{Ver: "6.0", Tag: "ipblacklist", Data: rules}
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Ver  string `json:"ver"`
		Tag  string `json:"tag"`
		Data []struct {
			ID          int    `json:"id"`
			TmpFieldSel bool   `json:"tmp_field_sel"`
			RAddr       string `json:"raddr"`
			Memo        string `json:"memo"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Ver != "6.0" || decoded.Tag != "ipblacklist" {
		t.Fatalf("unexpected header: %+v", decoded)
	}
	if len(decoded.Data) != 1 || decoded.Data[0].RAddr != "1.2.3.4" || !decoded.Data[0].TmpFieldSel {
		t.Fatalf("unexpected data: %+v", decoded.Data)
	}
}

func TestMakeFirewallScriptCreatesInboundAndOutboundRules(t *testing.T) {
	dir := t.TempDir()
	path, err := makeFirewallScript(dir, []string{"1.2.3.4", "5.6.7.8"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body := string(b)
	for _, want := range []string{
		`name="L4D2 Server Tool Block OUT 01" dir=out`,
		`name="L4D2 Server Tool Block IN 01" dir=in`,
		`remoteip="1.2.3.4,5.6.7.8"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("firewall script missing %q:\n%s", want, body)
		}
	}
}

func TestMakeFirewallRemoveScriptDeletesInboundAndOutboundRules(t *testing.T) {
	dir := t.TempDir()
	path, err := makeFirewallRemoveScript(dir)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body := string(b)
	for _, want := range []string{
		`delete rule name="L4D2 Server Tool Block"`,
		`delete rule name="L4D2 Server Tool Block OUT"`,
		`delete rule name="L4D2 Server Tool Block IN"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("remove script missing %q:\n%s", want, body)
		}
	}
}
