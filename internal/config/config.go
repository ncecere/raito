package config

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type ScraperConfig struct {
	UserAgent           string `yaml:"userAgent"`
	TimeoutMs           int    `yaml:"timeoutMs"`
	LinksSameDomainOnly bool   `yaml:"linksSameDomainOnly"`
	LinksMaxPerDocument int    `yaml:"linksMaxPerDocument"`
}

type CrawlerConfig struct {
	MaxDepthDefault int `yaml:"maxDepthDefault"`
	MaxPagesDefault int `yaml:"maxPagesDefault"`
}

type RobotsConfig struct {
	Respect bool `yaml:"respect"`
}

type RodConfig struct {
	Enabled bool `yaml:"enabled"`
}

type DatabaseConfig struct {
	DSN string `yaml:"dsn"`
}

type RedisConfig struct {
	URL string `yaml:"url"`
}

type LocalAuthConfig struct {
	Enabled bool `yaml:"enabled"`
}

type OIDCAuthConfig struct {
	Enabled        bool     `yaml:"enabled"`
	IssuerURL      string   `yaml:"issuerURL"`
	ClientID       string   `yaml:"clientID"`
	ClientSecret   string   `yaml:"clientSecret"`
	RedirectURL    string   `yaml:"redirectURL"`
	AllowedDomains []string `yaml:"allowedDomains"`
}

type SessionAuthConfig struct {
	Secret     string `yaml:"secret"`     // HS256 secret for JWT
	CookieName string `yaml:"cookieName"` // defaults to "raito_session"
	TTLMinutes int    `yaml:"ttlMinutes"` // session lifetime; default 1440 (24h)
}

type AuthConfig struct {
	Enabled         bool              `yaml:"enabled"`
	InitialAdminKey string            `yaml:"initialAdminKey"`
	Local           LocalAuthConfig   `yaml:"local"`
	OIDC            OIDCAuthConfig    `yaml:"oidc"`
	Session         SessionAuthConfig `yaml:"session"`
}

type RateLimitConfig struct {
	DefaultPerMinute int `yaml:"defaultPerMinute"`
}

type WorkerConfig struct {
	MaxConcurrentJobs       int `yaml:"maxConcurrentJobs"`
	PollIntervalMs          int `yaml:"pollIntervalMs"`
	MaxConcurrentURLsPerJob int `yaml:"maxConcurrentURLsPerJob"`
	SyncJobWaitTimeoutMs    int `yaml:"syncJobWaitTimeoutMs"`
}

type OpenAIConfig struct {
	APIKey  string `yaml:"apiKey"`
	BaseURL string `yaml:"baseURL"`
	Model   string `yaml:"model"`
}

type AnthropicConfig struct {
	APIKey string `yaml:"apiKey"`
	Model  string `yaml:"model"`
}

type GoogleLLMConfig struct {
	APIKey string `yaml:"apiKey"`
	Model  string `yaml:"model"`
}

type LLMConfig struct {
	DefaultProvider string          `yaml:"defaultProvider"`
	OpenAI          OpenAIConfig    `yaml:"openai"`
	Anthropic       AnthropicConfig `yaml:"anthropic"`
	Google          GoogleLLMConfig `yaml:"google"`
}

// SearxngConfig holds provider-specific configuration for SearxNG-based search.
type SearxngConfig struct {
	BaseURL      string `yaml:"baseURL"`
	DefaultLimit int    `yaml:"defaultLimit"`
	TimeoutMs    int    `yaml:"timeoutMs"`
}

// SearchConfig controls the optional /v1/search endpoint and its provider.
type SearchConfig struct {
	Enabled              bool          `yaml:"enabled"`
	Provider             string        `yaml:"provider"`
	MaxResults           int           `yaml:"maxResults"`
	TimeoutMs            int           `yaml:"timeoutMs"`
	MaxConcurrentScrapes int           `yaml:"maxConcurrentScrapes"`
	Searxng              SearxngConfig `yaml:"searxng"`
}

// JobTTLConfig controls per-job-type retention in days.
type JobTTLConfig struct {
	DefaultDays int `yaml:"defaultDays"`
	ScrapeDays  int `yaml:"scrapeDays"`
	MapDays     int `yaml:"mapDays"`
	ExtractDays int `yaml:"extractDays"`
	CrawlDays   int `yaml:"crawlDays"`
}

// DocumentTTLConfig controls retention for stored documents (currently
// used for crawl documents) in days.
type DocumentTTLConfig struct {
	DefaultDays int `yaml:"defaultDays"`
}

// RetentionConfig controls TTL-like deletion of old jobs and documents
// so that the database does not grow without bound over time.
type RetentionConfig struct {
	Enabled                bool              `yaml:"enabled"`
	CleanupIntervalMinutes int               `yaml:"cleanupIntervalMinutes"`
	Jobs                   JobTTLConfig      `yaml:"jobs"`
	Documents              DocumentTTLConfig `yaml:"documents"`
}

type BootstrapUserConfig struct {
	Email         string `yaml:"email"`
	Name          string `yaml:"name"`
	IsSystemAdmin bool   `yaml:"isSystemAdmin"`
	Provider      string `yaml:"provider"` // local or oidc
	Password      string `yaml:"password,omitempty"`
}

type BootstrapTenantConfig struct {
	Slug    string   `yaml:"slug"`
	Name    string   `yaml:"name"`
	Type    string   `yaml:"type"` // personal or org
	Admins  []string `yaml:"admins"`
	Members []string `yaml:"members"`
}

type BootstrapConfig struct {
	AllowPlaintextPasswords bool                    `yaml:"allowPlaintextPasswords"`
	Users                   []BootstrapUserConfig   `yaml:"users"`
	Tenants                 []BootstrapTenantConfig `yaml:"tenants"`
}

type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Scraper   ScraperConfig   `yaml:"scraper"`
	Crawler   CrawlerConfig   `yaml:"crawler"`
	Robots    RobotsConfig    `yaml:"robots"`
	Rod       RodConfig       `yaml:"rod"`
	Database  DatabaseConfig  `yaml:"database"`
	Redis     RedisConfig     `yaml:"redis"`
	Auth      AuthConfig      `yaml:"auth"`
	RateLimit RateLimitConfig `yaml:"ratelimit"`
	Worker    WorkerConfig    `yaml:"worker"`
	LLM       LLMConfig       `yaml:"llm"`
	Search    SearchConfig    `yaml:"search"`
	Retention RetentionConfig `yaml:"retention"`
	Bootstrap BootstrapConfig `yaml:"bootstrap"`
}

func Load(path string) *Config {
	f, err := os.Open(path)
	if err != nil {
		log.Fatalf("failed to open config file: %v", err)
	}
	defer f.Close()

	var cfg Config
	if err := yaml.NewDecoder(f).Decode(&cfg); err != nil {
		log.Fatalf("failed to decode config: %v", err)
	}

	return &cfg
}

// Validate performs basic sanity checks on the loaded configuration.
// It focuses on LLM defaults so that obviously misconfigured providers
// fail fast at startup rather than during the first request.
func (cfg *Config) Validate() error {
	if cfg == nil {
		return errors.New("config is nil")
	}

	provider := strings.TrimSpace(cfg.LLM.DefaultProvider)
	if provider == "" {
		return errors.New("llm.defaultProvider must be set to 'openai', 'anthropic', or 'google'")
	}

	switch provider {
	case "openai":
		if cfg.LLM.OpenAI.APIKey == "" || cfg.LLM.OpenAI.Model == "" {
			return errors.New("openai llm provider is not fully configured")
		}
	case "anthropic":
		if cfg.LLM.Anthropic.APIKey == "" || cfg.LLM.Anthropic.Model == "" {
			return errors.New("anthropic llm provider is not fully configured")
		}
	case "google":
		if cfg.LLM.Google.APIKey == "" || cfg.LLM.Google.Model == "" {
			return errors.New("google llm provider is not fully configured")
		}
	default:
		return fmt.Errorf("unsupported llm.defaultProvider: %s", provider)
	}

	// Basic auth validation: ensure OIDC config is complete when enabled.
	if cfg.Auth.OIDC.Enabled {
		if strings.TrimSpace(cfg.Auth.OIDC.IssuerURL) == "" ||
			strings.TrimSpace(cfg.Auth.OIDC.ClientID) == "" ||
			strings.TrimSpace(cfg.Auth.OIDC.ClientSecret) == "" ||
			strings.TrimSpace(cfg.Auth.OIDC.RedirectURL) == "" {
			return errors.New("auth.oidc is enabled but issuerURL, clientID, clientSecret, or redirectURL is missing")
		}
	}

	// Prevent accidental plaintext passwords in bootstrap users unless
	// explicitly allowed in configuration.
	if !cfg.Bootstrap.AllowPlaintextPasswords {
		for _, u := range cfg.Bootstrap.Users {
			if strings.EqualFold(strings.TrimSpace(u.Provider), "local") && strings.TrimSpace(u.Password) != "" {
				return errors.New("bootstrap users contain plaintext passwords but bootstrap.allowPlaintextPasswords is false")
			}
		}
	}

	return nil
}
