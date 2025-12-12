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
	UserAgent string `yaml:"userAgent"`
	TimeoutMs int    `yaml:"timeoutMs"`
}

type CrawlerConfig struct {
	MaxDepthDefault int `yaml:"maxDepthDefault"`
	MaxPagesDefault int `yaml:"maxPagesDefault"`
}

type RobotsConfig struct {
	Respect bool `yaml:"respect"`
}

type RodConfig struct {
	Enabled    bool   `yaml:"enabled"`
	BrowserURL string `yaml:"browserURL"`
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
	LLM       LLMConfig       `yaml:"llm"`
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
