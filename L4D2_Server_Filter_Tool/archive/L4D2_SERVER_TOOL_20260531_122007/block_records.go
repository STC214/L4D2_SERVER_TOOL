package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type BlockRecord struct {
	Time         time.Time `json:"time"`
	IP           string    `json:"ip"`
	Address      string    `json:"address"`
	ServerName   string    `json:"server_name"`
	Map          string    `json:"map"`
	Players      string    `json:"players"`
	Reasons      []string  `json:"reasons"`
	Source       string    `json:"source"`
	FirewallRule string    `json:"firewall_rule"`
}

func blockRecordsPath() string {
	return filepath.Join(appDir(), "blocked_records.json")
}

func loadBlockRecords() ([]BlockRecord, error) {
	b, err := os.ReadFile(blockRecordsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var records []BlockRecord
	if len(b) == 0 {
		return nil, nil
	}
	return records, json.Unmarshal(b, &records)
}

func appendBlockRecords(records []BlockRecord) error {
	old, err := loadBlockRecords()
	if err != nil {
		return err
	}
	old = append(old, records...)
	b, err := json.MarshalIndent(old, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(blockRecordsPath(), b, 0644)
}
