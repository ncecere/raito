package http

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"
	"gopkg.in/yaml.v3"

	"raito/internal/config"
	"raito/internal/store"
)

type adminSystemSettingsSecrets struct {
	AuthInitialAdminKeySet   bool `json:"authInitialAdminKeySet"`
	AuthOIDCClientSecretSet  bool `json:"authOidcClientSecretSet"`
	AuthSessionSecretSet     bool `json:"authSessionSecretSet"`
	LLMOpenAIAPIKeySet       bool `json:"llmOpenaiApiKeySet"`
	LLMAnthropicAPIKeySet    bool `json:"llmAnthropicApiKeySet"`
	LLMGoogleAPIKeySet       bool `json:"llmGoogleApiKeySet"`
	SearchSearxngConfigured  bool `json:"searchSearxngConfigured"`
	SearchProviderConfigured bool `json:"searchProviderConfigured"`
}

type adminSystemSettingsConfig struct {
	Scraper   adminScraperConfig   `json:"scraper"`
	Crawler   adminCrawlerConfig   `json:"crawler"`
	Robots    adminRobotsConfig    `json:"robots"`
	Rod       adminRodConfig       `json:"rod"`
	Worker    adminWorkerConfig    `json:"worker"`
	RateLimit adminRateLimitConfig `json:"ratelimit"`

	Auth   adminAuthConfig   `json:"auth"`
	Search adminSearchConfig `json:"search"`
	LLM    adminLLMConfig    `json:"llm"`
}

type adminSystemSettingsResponse struct {
	Success    bool                       `json:"success"`
	Config     adminSystemSettingsConfig  `json:"config"`
	Secrets    adminSystemSettingsSecrets `json:"secrets"`
	ConfigPath string                     `json:"configPath,omitempty"`
	Notes      []string                   `json:"notes,omitempty"`
}

type adminScraperConfig struct {
	UserAgent           string `json:"userAgent"`
	TimeoutMs           int    `json:"timeoutMs"`
	LinksSameDomainOnly bool   `json:"linksSameDomainOnly"`
	LinksMaxPerDocument int    `json:"linksMaxPerDocument"`
}

type adminCrawlerConfig struct {
	MaxDepthDefault int `json:"maxDepthDefault"`
	MaxPagesDefault int `json:"maxPagesDefault"`
}

type adminRobotsConfig struct {
	Respect bool `json:"respect"`
}

type adminRodConfig struct {
	Enabled bool `json:"enabled"`
}

type adminWorkerConfig struct {
	MaxConcurrentJobs       int `json:"maxConcurrentJobs"`
	PollIntervalMs          int `json:"pollIntervalMs"`
	MaxConcurrentUrlsPerJob int `json:"maxConcurrentUrlsPerJob"`
	SyncJobWaitTimeoutMs    int `json:"syncJobWaitTimeoutMs"`
}

type adminRateLimitConfig struct {
	DefaultPerMinute int `json:"defaultPerMinute"`
}

type adminAuthConfig struct {
	Enabled         bool             `json:"enabled"`
	InitialAdminKey string           `json:"initialAdminKey"`
	Local           adminLocalAuth   `json:"local"`
	OIDC            adminOIDCAuth    `json:"oidc"`
	Session         adminSessionAuth `json:"session"`
}

type adminLocalAuth struct {
	Enabled bool `json:"enabled"`
}

type adminOIDCAuth struct {
	Enabled        bool     `json:"enabled"`
	IssuerURL      string   `json:"issuerURL"`
	ClientID       string   `json:"clientID"`
	ClientSecret   string   `json:"clientSecret"`
	RedirectURL    string   `json:"redirectURL"`
	AllowedDomains []string `json:"allowedDomains"`
}

type adminSessionAuth struct {
	Secret     string `json:"secret"`
	CookieName string `json:"cookieName"`
	TTLMinutes int    `json:"ttlMinutes"`
}

type adminSearchConfig struct {
	Enabled              bool               `json:"enabled"`
	Provider             string             `json:"provider"`
	MaxResults           int                `json:"maxResults"`
	TimeoutMs            int                `json:"timeoutMs"`
	MaxConcurrentScrapes int                `json:"maxConcurrentScrapes"`
	Searxng              adminSearxngConfig `json:"searxng"`
}

type adminSearxngConfig struct {
	BaseURL      string `json:"baseURL"`
	DefaultLimit int    `json:"defaultLimit"`
	TimeoutMs    int    `json:"timeoutMs"`
}

type adminLLMConfig struct {
	DefaultProvider string               `json:"defaultProvider"`
	OpenAI          adminOpenAIConfig    `json:"openai"`
	Anthropic       adminAnthropicConfig `json:"anthropic"`
	Google          adminGoogleLLMConfig `json:"google"`
}

type adminOpenAIConfig struct {
	APIKey  string `json:"apiKey"`
	BaseURL string `json:"baseURL"`
	Model   string `json:"model"`
}

type adminAnthropicConfig struct {
	APIKey string `json:"apiKey"`
	Model  string `json:"model"`
}

type adminGoogleLLMConfig struct {
	APIKey string `json:"apiKey"`
	Model  string `json:"model"`
}

type systemSettingsPatchRequest struct {
	Scraper   *scraperConfigPatch   `json:"scraper,omitempty"`
	Crawler   *crawlerConfigPatch   `json:"crawler,omitempty"`
	Robots    *robotsConfigPatch    `json:"robots,omitempty"`
	Rod       *rodConfigPatch       `json:"rod,omitempty"`
	Worker    *workerConfigPatch    `json:"worker,omitempty"`
	RateLimit *rateLimitConfigPatch `json:"ratelimit,omitempty"`

	Auth   *authConfigPatch   `json:"auth,omitempty"`
	Search *searchConfigPatch `json:"search,omitempty"`
	LLM    *llmConfigPatch    `json:"llm,omitempty"`
}

type scraperConfigPatch struct {
	UserAgent           *string `json:"userAgent,omitempty"`
	TimeoutMs           *int    `json:"timeoutMs,omitempty"`
	LinksSameDomainOnly *bool   `json:"linksSameDomainOnly,omitempty"`
	LinksMaxPerDocument *int    `json:"linksMaxPerDocument,omitempty"`
}

type crawlerConfigPatch struct {
	MaxDepthDefault *int `json:"maxDepthDefault,omitempty"`
	MaxPagesDefault *int `json:"maxPagesDefault,omitempty"`
}

type robotsConfigPatch struct {
	Respect *bool `json:"respect,omitempty"`
}

type rodConfigPatch struct {
	Enabled *bool `json:"enabled,omitempty"`
}

type workerConfigPatch struct {
	MaxConcurrentJobs       *int `json:"maxConcurrentJobs,omitempty"`
	PollIntervalMs          *int `json:"pollIntervalMs,omitempty"`
	MaxConcurrentURLsPerJob *int `json:"maxConcurrentUrlsPerJob,omitempty"`
	SyncJobWaitTimeoutMs    *int `json:"syncJobWaitTimeoutMs,omitempty"`
}

type rateLimitConfigPatch struct {
	DefaultPerMinute *int `json:"defaultPerMinute,omitempty"`
}

type authConfigPatch struct {
	Enabled         *bool             `json:"enabled,omitempty"`
	InitialAdminKey *string           `json:"initialAdminKey,omitempty"`
	Local           *localAuthPatch   `json:"local,omitempty"`
	OIDC            *oidcAuthPatch    `json:"oidc,omitempty"`
	Session         *sessionAuthPatch `json:"session,omitempty"`
}

type localAuthPatch struct {
	Enabled *bool `json:"enabled,omitempty"`
}

type oidcAuthPatch struct {
	Enabled        *bool     `json:"enabled,omitempty"`
	IssuerURL      *string   `json:"issuerURL,omitempty"`
	ClientID       *string   `json:"clientID,omitempty"`
	ClientSecret   *string   `json:"clientSecret,omitempty"`
	RedirectURL    *string   `json:"redirectURL,omitempty"`
	AllowedDomains *[]string `json:"allowedDomains,omitempty"`
}

type sessionAuthPatch struct {
	Secret     *string `json:"secret,omitempty"`
	CookieName *string `json:"cookieName,omitempty"`
	TTLMinutes *int    `json:"ttlMinutes,omitempty"`
}

type searchConfigPatch struct {
	Enabled              *bool         `json:"enabled,omitempty"`
	Provider             *string       `json:"provider,omitempty"`
	MaxResults           *int          `json:"maxResults,omitempty"`
	TimeoutMs            *int          `json:"timeoutMs,omitempty"`
	MaxConcurrentScrapes *int          `json:"maxConcurrentScrapes,omitempty"`
	Searxng              *searxngPatch `json:"searxng,omitempty"`
}

type searxngPatch struct {
	BaseURL      *string `json:"baseURL,omitempty"`
	DefaultLimit *int    `json:"defaultLimit,omitempty"`
	TimeoutMs    *int    `json:"timeoutMs,omitempty"`
}

type llmConfigPatch struct {
	DefaultProvider *string         `json:"defaultProvider,omitempty"`
	OpenAI          *openAIPatch    `json:"openai,omitempty"`
	Anthropic       *anthropicPatch `json:"anthropic,omitempty"`
	Google          *googleLLMPatch `json:"google,omitempty"`
}

type openAIPatch struct {
	APIKey  *string `json:"apiKey,omitempty"`
	BaseURL *string `json:"baseURL,omitempty"`
	Model   *string `json:"model,omitempty"`
}

type anthropicPatch struct {
	APIKey *string `json:"apiKey,omitempty"`
	Model  *string `json:"model,omitempty"`
}

type googleLLMPatch struct {
	APIKey *string `json:"apiKey,omitempty"`
	Model  *string `json:"model,omitempty"`
}

func adminGetSystemSettingsHandler(c *fiber.Ctx) error {
	cfg := c.Locals("config").(*config.Config)

	resp := adminSystemSettingsResponse{
		Success:    true,
		Config:     redactedSystemSettingsConfig(cfg),
		Secrets:    systemSettingsSecrets(cfg),
		ConfigPath: cfg.Path,
		Notes: []string{
			"Settings are loaded from the server config file. Saving updates the file; a restart may be required for some changes to take effect.",
		},
	}

	return c.Status(fiber.StatusOK).JSON(resp)
}

func adminUpdateSystemSettingsHandler(c *fiber.Ctx) error {
	cfg := c.Locals("config").(*config.Config)
	st := c.Locals("store").(*store.Store)

	if strings.TrimSpace(cfg.Path) == "" {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Success: false,
			Code:    "SYSTEM_SETTINGS_UNAVAILABLE",
			Error:   "config path is not set on the server",
		})
	}

	var req systemSettingsPatchRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Success: false,
			Code:    "BAD_REQUEST_INVALID_JSON",
			Error:   "Bad request, malformed JSON",
		})
	}

	next := *cfg
	applySystemSettingsPatch(&next, &req)

	if err := validateSystemSettings(&next); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   err.Error(),
		})
	}

	if err := writeConfigYAMLAtomic(cfg.Path, &next); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Success: false,
			Code:    "SYSTEM_SETTINGS_SAVE_FAILED",
			Error:   err.Error(),
		})
	}

	recordAuditEvent(c, st, "admin.system_settings.update", auditEventOptions{
		ResourceType: "system_settings",
		ResourceID:   "config",
	})

	return c.Status(fiber.StatusOK).JSON(adminSystemSettingsResponse{
		Success:    true,
		Config:     redactedSystemSettingsConfig(&next),
		Secrets:    systemSettingsSecrets(&next),
		ConfigPath: cfg.Path,
		Notes: []string{
			"Saved to config file. Restart the server/worker if changes do not apply immediately.",
		},
	})
}

func redactedSystemSettingsConfig(cfg *config.Config) adminSystemSettingsConfig {
	c := adminSystemSettingsConfig{
		Scraper: adminScraperConfig{
			UserAgent:           cfg.Scraper.UserAgent,
			TimeoutMs:           cfg.Scraper.TimeoutMs,
			LinksSameDomainOnly: cfg.Scraper.LinksSameDomainOnly,
			LinksMaxPerDocument: cfg.Scraper.LinksMaxPerDocument,
		},
		Crawler: adminCrawlerConfig{
			MaxDepthDefault: cfg.Crawler.MaxDepthDefault,
			MaxPagesDefault: cfg.Crawler.MaxPagesDefault,
		},
		Robots: adminRobotsConfig{
			Respect: cfg.Robots.Respect,
		},
		Rod: adminRodConfig{
			Enabled: cfg.Rod.Enabled,
		},
		Worker: adminWorkerConfig{
			MaxConcurrentJobs:       cfg.Worker.MaxConcurrentJobs,
			PollIntervalMs:          cfg.Worker.PollIntervalMs,
			MaxConcurrentUrlsPerJob: cfg.Worker.MaxConcurrentURLsPerJob,
			SyncJobWaitTimeoutMs:    cfg.Worker.SyncJobWaitTimeoutMs,
		},
		RateLimit: adminRateLimitConfig{
			DefaultPerMinute: cfg.RateLimit.DefaultPerMinute,
		},
		Auth: adminAuthConfig{
			Enabled:         cfg.Auth.Enabled,
			InitialAdminKey: cfg.Auth.InitialAdminKey,
			Local: adminLocalAuth{
				Enabled: cfg.Auth.Local.Enabled,
			},
			OIDC: adminOIDCAuth{
				Enabled:        cfg.Auth.OIDC.Enabled,
				IssuerURL:      cfg.Auth.OIDC.IssuerURL,
				ClientID:       cfg.Auth.OIDC.ClientID,
				ClientSecret:   cfg.Auth.OIDC.ClientSecret,
				RedirectURL:    cfg.Auth.OIDC.RedirectURL,
				AllowedDomains: cfg.Auth.OIDC.AllowedDomains,
			},
			Session: adminSessionAuth{
				Secret:     cfg.Auth.Session.Secret,
				CookieName: cfg.Auth.Session.CookieName,
				TTLMinutes: cfg.Auth.Session.TTLMinutes,
			},
		},
		Search: adminSearchConfig{
			Enabled:              cfg.Search.Enabled,
			Provider:             cfg.Search.Provider,
			MaxResults:           cfg.Search.MaxResults,
			TimeoutMs:            cfg.Search.TimeoutMs,
			MaxConcurrentScrapes: cfg.Search.MaxConcurrentScrapes,
			Searxng: adminSearxngConfig{
				BaseURL:      cfg.Search.Searxng.BaseURL,
				DefaultLimit: cfg.Search.Searxng.DefaultLimit,
				TimeoutMs:    cfg.Search.Searxng.TimeoutMs,
			},
		},
		LLM: adminLLMConfig{
			DefaultProvider: cfg.LLM.DefaultProvider,
			OpenAI: adminOpenAIConfig{
				APIKey:  cfg.LLM.OpenAI.APIKey,
				BaseURL: cfg.LLM.OpenAI.BaseURL,
				Model:   cfg.LLM.OpenAI.Model,
			},
			Anthropic: adminAnthropicConfig{
				APIKey: cfg.LLM.Anthropic.APIKey,
				Model:  cfg.LLM.Anthropic.Model,
			},
			Google: adminGoogleLLMConfig{
				APIKey: cfg.LLM.Google.APIKey,
				Model:  cfg.LLM.Google.Model,
			},
		},
	}

	// Redact secrets in the response; the UI can set new values but does not need the existing ones.
	c.Auth.InitialAdminKey = ""
	c.Auth.OIDC.ClientSecret = ""
	c.Auth.Session.Secret = ""
	c.LLM.OpenAI.APIKey = ""
	c.LLM.Anthropic.APIKey = ""
	c.LLM.Google.APIKey = ""

	return c
}

func systemSettingsSecrets(cfg *config.Config) adminSystemSettingsSecrets {
	searxngConfigured := strings.TrimSpace(cfg.Search.Searxng.BaseURL) != ""
	providerConfigured := strings.TrimSpace(cfg.Search.Provider) != ""
	return adminSystemSettingsSecrets{
		AuthInitialAdminKeySet:   strings.TrimSpace(cfg.Auth.InitialAdminKey) != "",
		AuthOIDCClientSecretSet:  strings.TrimSpace(cfg.Auth.OIDC.ClientSecret) != "",
		AuthSessionSecretSet:     strings.TrimSpace(cfg.Auth.Session.Secret) != "",
		LLMOpenAIAPIKeySet:       strings.TrimSpace(cfg.LLM.OpenAI.APIKey) != "",
		LLMAnthropicAPIKeySet:    strings.TrimSpace(cfg.LLM.Anthropic.APIKey) != "",
		LLMGoogleAPIKeySet:       strings.TrimSpace(cfg.LLM.Google.APIKey) != "",
		SearchSearxngConfigured:  searxngConfigured,
		SearchProviderConfigured: providerConfigured,
	}
}

func applySystemSettingsPatch(cfg *config.Config, req *systemSettingsPatchRequest) {
	if cfg == nil || req == nil {
		return
	}

	if req.Scraper != nil {
		if req.Scraper.UserAgent != nil {
			cfg.Scraper.UserAgent = *req.Scraper.UserAgent
		}
		if req.Scraper.TimeoutMs != nil {
			cfg.Scraper.TimeoutMs = *req.Scraper.TimeoutMs
		}
		if req.Scraper.LinksSameDomainOnly != nil {
			cfg.Scraper.LinksSameDomainOnly = *req.Scraper.LinksSameDomainOnly
		}
		if req.Scraper.LinksMaxPerDocument != nil {
			cfg.Scraper.LinksMaxPerDocument = *req.Scraper.LinksMaxPerDocument
		}
	}

	if req.Crawler != nil {
		if req.Crawler.MaxDepthDefault != nil {
			cfg.Crawler.MaxDepthDefault = *req.Crawler.MaxDepthDefault
		}
		if req.Crawler.MaxPagesDefault != nil {
			cfg.Crawler.MaxPagesDefault = *req.Crawler.MaxPagesDefault
		}
	}

	if req.Robots != nil {
		if req.Robots.Respect != nil {
			cfg.Robots.Respect = *req.Robots.Respect
		}
	}

	if req.Rod != nil {
		if req.Rod.Enabled != nil {
			cfg.Rod.Enabled = *req.Rod.Enabled
		}
	}

	if req.Worker != nil {
		if req.Worker.MaxConcurrentJobs != nil {
			cfg.Worker.MaxConcurrentJobs = *req.Worker.MaxConcurrentJobs
		}
		if req.Worker.PollIntervalMs != nil {
			cfg.Worker.PollIntervalMs = *req.Worker.PollIntervalMs
		}
		if req.Worker.MaxConcurrentURLsPerJob != nil {
			cfg.Worker.MaxConcurrentURLsPerJob = *req.Worker.MaxConcurrentURLsPerJob
		}
		if req.Worker.SyncJobWaitTimeoutMs != nil {
			cfg.Worker.SyncJobWaitTimeoutMs = *req.Worker.SyncJobWaitTimeoutMs
		}
	}

	if req.RateLimit != nil {
		if req.RateLimit.DefaultPerMinute != nil {
			cfg.RateLimit.DefaultPerMinute = *req.RateLimit.DefaultPerMinute
		}
	}

	if req.Auth != nil {
		if req.Auth.Enabled != nil {
			cfg.Auth.Enabled = *req.Auth.Enabled
		}
		if req.Auth.InitialAdminKey != nil {
			cfg.Auth.InitialAdminKey = *req.Auth.InitialAdminKey
		}
		if req.Auth.Local != nil && req.Auth.Local.Enabled != nil {
			cfg.Auth.Local.Enabled = *req.Auth.Local.Enabled
		}
		if req.Auth.OIDC != nil {
			if req.Auth.OIDC.Enabled != nil {
				cfg.Auth.OIDC.Enabled = *req.Auth.OIDC.Enabled
			}
			if req.Auth.OIDC.IssuerURL != nil {
				cfg.Auth.OIDC.IssuerURL = *req.Auth.OIDC.IssuerURL
			}
			if req.Auth.OIDC.ClientID != nil {
				cfg.Auth.OIDC.ClientID = *req.Auth.OIDC.ClientID
			}
			if req.Auth.OIDC.ClientSecret != nil {
				cfg.Auth.OIDC.ClientSecret = *req.Auth.OIDC.ClientSecret
			}
			if req.Auth.OIDC.RedirectURL != nil {
				cfg.Auth.OIDC.RedirectURL = *req.Auth.OIDC.RedirectURL
			}
			if req.Auth.OIDC.AllowedDomains != nil {
				cfg.Auth.OIDC.AllowedDomains = *req.Auth.OIDC.AllowedDomains
			}
		}
		if req.Auth.Session != nil {
			if req.Auth.Session.Secret != nil {
				cfg.Auth.Session.Secret = *req.Auth.Session.Secret
			}
			if req.Auth.Session.CookieName != nil {
				cfg.Auth.Session.CookieName = *req.Auth.Session.CookieName
			}
			if req.Auth.Session.TTLMinutes != nil {
				cfg.Auth.Session.TTLMinutes = *req.Auth.Session.TTLMinutes
			}
		}
	}

	if req.Search != nil {
		if req.Search.Enabled != nil {
			cfg.Search.Enabled = *req.Search.Enabled
		}
		if req.Search.Provider != nil {
			cfg.Search.Provider = *req.Search.Provider
		}
		if req.Search.MaxResults != nil {
			cfg.Search.MaxResults = *req.Search.MaxResults
		}
		if req.Search.TimeoutMs != nil {
			cfg.Search.TimeoutMs = *req.Search.TimeoutMs
		}
		if req.Search.MaxConcurrentScrapes != nil {
			cfg.Search.MaxConcurrentScrapes = *req.Search.MaxConcurrentScrapes
		}
		if req.Search.Searxng != nil {
			if req.Search.Searxng.BaseURL != nil {
				cfg.Search.Searxng.BaseURL = *req.Search.Searxng.BaseURL
			}
			if req.Search.Searxng.DefaultLimit != nil {
				cfg.Search.Searxng.DefaultLimit = *req.Search.Searxng.DefaultLimit
			}
			if req.Search.Searxng.TimeoutMs != nil {
				cfg.Search.Searxng.TimeoutMs = *req.Search.Searxng.TimeoutMs
			}
		}
	}

	if req.LLM != nil {
		if req.LLM.DefaultProvider != nil {
			cfg.LLM.DefaultProvider = *req.LLM.DefaultProvider
		}
		if req.LLM.OpenAI != nil {
			if req.LLM.OpenAI.APIKey != nil {
				cfg.LLM.OpenAI.APIKey = *req.LLM.OpenAI.APIKey
			}
			if req.LLM.OpenAI.BaseURL != nil {
				cfg.LLM.OpenAI.BaseURL = *req.LLM.OpenAI.BaseURL
			}
			if req.LLM.OpenAI.Model != nil {
				cfg.LLM.OpenAI.Model = *req.LLM.OpenAI.Model
			}
		}
		if req.LLM.Anthropic != nil {
			if req.LLM.Anthropic.APIKey != nil {
				cfg.LLM.Anthropic.APIKey = *req.LLM.Anthropic.APIKey
			}
			if req.LLM.Anthropic.Model != nil {
				cfg.LLM.Anthropic.Model = *req.LLM.Anthropic.Model
			}
		}
		if req.LLM.Google != nil {
			if req.LLM.Google.APIKey != nil {
				cfg.LLM.Google.APIKey = *req.LLM.Google.APIKey
			}
			if req.LLM.Google.Model != nil {
				cfg.LLM.Google.Model = *req.LLM.Google.Model
			}
		}
	}
}

func validateSystemSettings(cfg *config.Config) error {
	if cfg.Scraper.TimeoutMs < 0 {
		return fiber.NewError(fiber.StatusBadRequest, "scraper.timeoutMs must be >= 0")
	}
	if cfg.Scraper.LinksMaxPerDocument < 0 {
		return fiber.NewError(fiber.StatusBadRequest, "scraper.linksMaxPerDocument must be >= 0")
	}
	if cfg.Crawler.MaxDepthDefault < 0 {
		return fiber.NewError(fiber.StatusBadRequest, "crawler.maxDepthDefault must be >= 0")
	}
	if cfg.Crawler.MaxPagesDefault < 0 {
		return fiber.NewError(fiber.StatusBadRequest, "crawler.maxPagesDefault must be >= 0")
	}
	if cfg.RateLimit.DefaultPerMinute < 0 {
		return fiber.NewError(fiber.StatusBadRequest, "ratelimit.defaultPerMinute must be >= 0")
	}
	if cfg.Worker.MaxConcurrentJobs < 0 {
		return fiber.NewError(fiber.StatusBadRequest, "worker.maxConcurrentJobs must be >= 0")
	}
	if cfg.Worker.PollIntervalMs < 0 {
		return fiber.NewError(fiber.StatusBadRequest, "worker.pollIntervalMs must be >= 0")
	}
	if cfg.Worker.MaxConcurrentURLsPerJob < 0 {
		return fiber.NewError(fiber.StatusBadRequest, "worker.maxConcurrentUrlsPerJob must be >= 0")
	}
	if cfg.Worker.SyncJobWaitTimeoutMs < 0 {
		return fiber.NewError(fiber.StatusBadRequest, "worker.syncJobWaitTimeoutMs must be >= 0")
	}
	if cfg.Auth.Session.TTLMinutes < 0 {
		return fiber.NewError(fiber.StatusBadRequest, "auth.session.ttlMinutes must be >= 0")
	}
	if cfg.Search.MaxResults < 0 {
		return fiber.NewError(fiber.StatusBadRequest, "search.maxResults must be >= 0")
	}
	if cfg.Search.TimeoutMs < 0 {
		return fiber.NewError(fiber.StatusBadRequest, "search.timeoutMs must be >= 0")
	}
	if cfg.Search.MaxConcurrentScrapes < 0 {
		return fiber.NewError(fiber.StatusBadRequest, "search.maxConcurrentScrapes must be >= 0")
	}
	if cfg.Search.Searxng.DefaultLimit < 0 {
		return fiber.NewError(fiber.StatusBadRequest, "search.searxng.defaultLimit must be >= 0")
	}
	if cfg.Search.Searxng.TimeoutMs < 0 {
		return fiber.NewError(fiber.StatusBadRequest, "search.searxng.timeoutMs must be >= 0")
	}
	return nil
}

func writeConfigYAMLAtomic(path string, cfg *config.Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	f, err := os.CreateTemp(dir, "raito-config-*.yaml")
	if err != nil {
		return err
	}
	tmpPath := f.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	enc := yaml.NewEncoder(f)
	enc.SetIndent(2)
	if err := enc.Encode(cfg); err != nil {
		_ = f.Close()
		return err
	}
	if err := enc.Close(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	return os.Rename(tmpPath, path)
}
