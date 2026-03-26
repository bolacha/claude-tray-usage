package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config holds user-configurable limits.
type Config struct {
	// DailyBudgetUSD is the user's self-imposed daily spending cap (0 = no limit).
	DailyBudgetUSD float64 `json:"daily_budget_usd"`

	// HourlyTokenLimit is the API rate limit (tokens/min * 60). 0 = no limit shown.
	// Sonnet Pro tier: ~80k tokens/min = ~4.8M/hour.
	// Leave 0 to auto-hide the hourly gauge.
	HourlyTokenLimit int64 `json:"hourly_token_limit"`

	// MonthlyBudgetUSD monthly cap (0 = no limit).
	MonthlyBudgetUSD float64 `json:"monthly_budget_usd"`
}

var defaultConfig = Config{
	DailyBudgetUSD:   10.0,
	HourlyTokenLimit: 0,
	MonthlyBudgetUSD: 0,
}

func path() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "tray-config.json")
}

// Load reads the config file, returning defaults if it doesn't exist.
func Load() Config {
	data, err := os.ReadFile(path())
	if err != nil {
		return defaultConfig
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return defaultConfig
	}
	return cfg
}

// Save writes the config to disk.
func Save(cfg Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path(), data, 0600)
}
