package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type BoardType string

const (
	BoardGreenhouse BoardType = "greenhouse"
	BoardLever      BoardType = "lever"
)

type CompanyConfig struct {
	Name  string    `yaml:"name"`
	Slug  string    `yaml:"slug"`
	Board BoardType `yaml:"board"`
}

type ResumeConfig struct {
	URL                   string `yaml:"url"`
	CachePath             string `yaml:"cache_path"`
	CacheTTLDays          int    `yaml:"cache_ttl_days"`
	FallbackYearsExperience int  `yaml:"fallback_years_experience"`
}

type EmailConfig struct {
	To       string `yaml:"to"`
	From     string `yaml:"from"`
	SMTPHost string `yaml:"smtp_host"`
	SMTPPort int    `yaml:"smtp_port"`
}

type ScoringConfig struct {
	PrioritySkills  []string `yaml:"priority_skills"`
	TargetTitles    []string `yaml:"target_titles"`
	SeniorityLevels []string `yaml:"seniority_levels"`
}

type Config struct {
	Schedule        string          `yaml:"schedule"`
	MetricsPort     int             `yaml:"metrics_port"`
	FeedbackPort    int             `yaml:"feedback_port"`
	FeedbackBaseURL string          `yaml:"feedback_base_url"`
	DBPath          string          `yaml:"db_path"`
	TopResults      int             `yaml:"top_results"`
	Resume          ResumeConfig    `yaml:"resume"`
	Email           EmailConfig     `yaml:"email"`
	Companies       []CompanyConfig `yaml:"companies"`
	Scoring         ScoringConfig   `yaml:"scoring"`

	// Loaded from environment, not YAML
	GmailAppPassword string `yaml:"-"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	cfg.GmailAppPassword = os.Getenv("GMAIL_APP_PASSWORD")

	if cfg.MetricsPort == 0 {
		cfg.MetricsPort = 2112
	}
	if cfg.TopResults == 0 {
		cfg.TopResults = 20
	}
	if cfg.Resume.CacheTTLDays == 0 {
		cfg.Resume.CacheTTLDays = 7
	}
	if cfg.FeedbackPort == 0 {
		cfg.FeedbackPort = 8080
	}
	if cfg.DBPath == "" {
		cfg.DBPath = "/opt/jobscout/jobscout.db"
	}

	return &cfg, nil
}
