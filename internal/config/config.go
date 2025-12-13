package config

import (
	"log"
	"os"

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

type AuthConfig struct {
	Enabled         bool   `yaml:"enabled"`
	InitialAdminKey string `yaml:"initialAdminKey"`
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
