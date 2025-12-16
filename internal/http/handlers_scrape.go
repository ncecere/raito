package http

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"

	"raito/internal/config"
	"raito/internal/llm"
	"raito/internal/metrics"
	"raito/internal/scraper"
	"raito/internal/scrapeutil"
	"raito/internal/services"
)

// scrapeHandler implements a minimal Firecrawl v2-compatible scrape endpoint.
// It currently supports basic HTML pages via the HTTP scraper.
func scrapeHandler(c *fiber.Ctx) error {
	var reqBody ScrapeRequest
	if err := c.BodyParser(&reqBody); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Success: false,
			Code:    "BAD_REQUEST_INVALID_JSON",
			Error:   "Bad request, malformed JSON",
		})
	}

	if reqBody.URL == "" {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "Missing required field 'url'",
		})
	}

	cfg := c.Locals("config").(*config.Config)

	timeoutMs := cfg.Scraper.TimeoutMs
	if reqBody.Timeout != nil && *reqBody.Timeout > 0 {
		timeoutMs = *reqBody.Timeout
	}

	// When a job queue-backed executor is available, delegate the heavy
	// scrape work to it so that API-only nodes can remain lightweight and
	// workers perform the browser/LLM work.
	if execVal := c.Locals("executor"); execVal != nil {
		if exec, ok := execVal.(WorkExecutor); ok && exec != nil {
			baseCtx := context.Background()
			if val := c.Locals("principal"); val != nil {
				if p, ok := val.(Principal); ok {
					if p.TenantID != nil {
						baseCtx = context.WithValue(baseCtx, "tenant_id", *p.TenantID)
					}
					if p.APIKeyID != nil {
						baseCtx = context.WithValue(baseCtx, "api_key_id", *p.APIKeyID)
					}
				}
			}

			ctx, cancel := context.WithTimeout(baseCtx, time.Duration(timeoutMs)*time.Millisecond)
			defer cancel()

			res, err := exec.Scrape(ctx, &reqBody)
			if err != nil {
				status := fiber.StatusBadGateway
				if errors.Is(err, context.DeadlineExceeded) {
					status = http.StatusGatewayTimeout
				}
				return c.Status(status).JSON(ErrorResponse{
					Success: false,
					Code:    "SCRAPE_FAILED",
					Error:   err.Error(),
				})
			}
			if res == nil {
				return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
					Success: false,
					Code:    "SCRAPE_FAILED",
					Error:   "empty scrape response",
				})
			}

			status := http.StatusOK
			if !res.Success {
				// Job-level failures are treated as upstream errors.
				status = http.StatusBadGateway
				if res.Code == "SCRAPE_TIMEOUT" || res.Code == "JOB_NOT_STARTED" {
					status = http.StatusGatewayTimeout
				}
			}

			return c.Status(status).JSON(res)
		}
	}

	// Determine whether screenshot format was requested and its options.
	hasScreenshot, screenshotFullPage := getScreenshotFormatConfig(reqBody.Formats)

	// Choose scraper engine: HTTP by default, rod when requested and enabled.
	useBrowser := false
	if reqBody.UseBrowser != nil {
		useBrowser = *reqBody.UseBrowser
	}
	if hasScreenshot {
		// Screenshot always uses the browser engine.
		useBrowser = true
	}

	var engine scraper.Scraper
	if useBrowser {
		if !cfg.Rod.Enabled {
			if hasScreenshot {
				return c.Status(http.StatusInternalServerError).JSON(ErrorResponse{
					Success: false,
					Code:    "SCREENSHOT_NOT_AVAILABLE",
					Error:   "screenshot format requires browser scraping, but rod is disabled in server configuration",
				})
			}
			engine = scraper.NewHTTPScraper(time.Duration(timeoutMs) * time.Millisecond)
		} else {
			// When rod is enabled, always use a locally managed headless browser
			// via RodScraper. The browser pool / BrowserURL support has been
			// removed for now to simplify deployment.
			engine = scraper.NewRodScraper(time.Duration(timeoutMs) * time.Millisecond)
		}
	} else {
		engine = scraper.NewHTTPScraper(time.Duration(timeoutMs) * time.Millisecond)
	}

	var locOpts *scraper.LocationOptions
	if reqBody.Location != nil {
		locOpts = &scraper.LocationOptions{
			Country:   reqBody.Location.Country,
			Languages: reqBody.Location.Languages,
		}
	}

	scrapeReq := scraper.BuildRequestFromOptions(scraper.RequestOptions{
		URL:       reqBody.URL,
		Headers:   reqBody.Headers,
		TimeoutMs: timeoutMs,
		UserAgent: cfg.Scraper.UserAgent,
		Location:  locOpts,
	})

	ctx, cancel := context.WithTimeout(c.Context(), time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	res, err := engine.Scrape(ctx, scrapeReq)
	if err != nil {
		status := fiber.StatusBadGateway
		if errors.Is(err, context.DeadlineExceeded) {
			status = http.StatusGatewayTimeout
		}
		return c.Status(status).JSON(ErrorResponse{
			Success: false,
			Code:    "SCRAPE_FAILED",
			Error:   err.Error(),
		})
	}

	svc := services.NewScrapeService(cfg)
	svcRes, err := svc.Scrape(ctx, &services.ScrapeRequest{
		Result:  res,
		Formats: reqBody.Formats,
	})
	if err != nil {
		status := fiber.StatusBadGateway
		if errors.Is(err, context.DeadlineExceeded) {
			status = http.StatusGatewayTimeout
		}
		return c.Status(status).JSON(ErrorResponse{
			Success: false,
			Code:    "SCRAPE_FAILED",
			Error:   err.Error(),
		})
	}
	if svcRes == nil || svcRes.Document == nil {
		return c.Status(http.StatusInternalServerError).JSON(ErrorResponse{
			Success: false,
			Code:    "SCRAPE_FAILED",
			Error:   "empty scrape document",
		})
	}

	doc := svcRes.Document

	// Optional screenshot format using the browser engine when requested.
	if hasScreenshot {
		screenshotCtx, screenshotCancel := context.WithTimeout(c.Context(), time.Duration(timeoutMs)*time.Millisecond)
		defer screenshotCancel()

		shot, err := scraper.CaptureScreenshot(screenshotCtx, res.URL, time.Duration(timeoutMs)*time.Millisecond, screenshotFullPage)
		if err != nil {
			status := fiber.StatusBadGateway
			if errors.Is(err, context.DeadlineExceeded) {
				status = http.StatusGatewayTimeout
			}
			return c.Status(status).JSON(ErrorResponse{
				Success: false,
				Code:    "SCREENSHOT_FAILED",
				Error:   err.Error(),
			})
		}

		doc.Screenshot = base64.StdEncoding.EncodeToString(shot)
	}

	// Optional summary format using the configured LLM provider when requested.
	if scrapeutil.WantsFormat(reqBody.Formats, "summary") {
		client, provider, modelName, err := llm.NewClientFromConfig(cfg, "", "")
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(ErrorResponse{
				Success: false,
				Code:    "LLM_NOT_CONFIGURED",
				Error:   err.Error(),
			})
		}

		// Expose LLM info to logging middleware via locals.
		c.Locals("llm_provider", string(provider))
		c.Locals("llm_model", modelName)

		fieldSpecs := []llm.FieldSpec{
			{
				Name:        "summary",
				Description: "Short natural-language summary of the page content.",
				Type:        "string",
			},
		}

		llmTimeout := time.Duration(timeoutMs) * time.Millisecond
		llmCtx, llmCancel := context.WithTimeout(c.Context(), llmTimeout)
		defer llmCancel()

		llmRes, err := client.ExtractFields(llmCtx, llm.ExtractRequest{
			URL:      reqBody.URL,
			Markdown: res.Markdown,
			Fields:   fieldSpecs,
			Prompt:   "",
			Timeout:  llmTimeout,
			Strict:   false,
		})
		if err != nil {
			metrics.RecordLLMExtract(string(provider), modelName, false)
			status := fiber.StatusBadGateway
			if errors.Is(err, context.DeadlineExceeded) {
				status = http.StatusGatewayTimeout
			}
			return c.Status(status).JSON(ErrorResponse{
				Success: false,
				Code:    "SUMMARY_FAILED",
				Error:   err.Error(),
			})
		}

		metrics.RecordLLMExtract(string(provider), modelName, true)

		if v, ok := llmRes.Fields["summary"]; ok {
			if s, ok := v.(string); ok {
				doc.Summary = s
			} else {
				doc.Summary = fmt.Sprint(v)
			}
		}
	}

	// Optional json format using the configured LLM provider when requested.
	if hasJSON, jsonPrompt, jsonSchema := scrapeutil.GetJSONFormatConfig(reqBody.Formats); hasJSON {
		client, provider, modelName, err := llm.NewClientFromConfig(cfg, "", "")
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(ErrorResponse{
				Success: false,
				Code:    "LLM_NOT_CONFIGURED",
				Error:   err.Error(),
			})
		}

		// Expose LLM info to logging middleware via locals (may override previous LLM info).
		c.Locals("llm_provider", string(provider))
		c.Locals("llm_model", modelName)

		desc := "Arbitrary JSON object extracted from the page content."
		if len(jsonSchema) > 0 {
			if schemaBytes, err := json.Marshal(jsonSchema); err == nil {
				desc = desc + " Schema: " + string(schemaBytes)
			}
		}

		fieldSpecs := []llm.FieldSpec{
			{
				Name:        "json",
				Description: desc,
				Type:        "object",
			},
		}

		llmTimeout := time.Duration(timeoutMs) * time.Millisecond
		llmCtx, llmCancel := context.WithTimeout(c.Context(), llmTimeout)
		defer llmCancel()

		llmRes, err := client.ExtractFields(llmCtx, llm.ExtractRequest{
			URL:      reqBody.URL,
			Markdown: res.Markdown,
			Fields:   fieldSpecs,
			Prompt:   jsonPrompt,
			Timeout:  llmTimeout,
			Strict:   false,
		})
		if err != nil {
			metrics.RecordLLMExtract(string(provider), modelName, false)
			status := fiber.StatusBadGateway
			if errors.Is(err, context.DeadlineExceeded) {
				status = http.StatusGatewayTimeout
			}
			return c.Status(status).JSON(ErrorResponse{
				Success: false,
				Code:    "JSON_EXTRACT_FAILED",
				Error:   err.Error(),
			})
		}

		metrics.RecordLLMExtract(string(provider), modelName, true)

		if v, ok := llmRes.Fields["json"]; ok {
			// v is expected to be a nested map[string]any representing structured JSON.
			if m, ok := v.(map[string]any); ok {
				doc.JSON = m
			} else {
				// If the LLM returns a non-object, still expose it as best-effort.
				// The client can decide how to interpret this.
				// We wrap it into a single-field object for consistency.
				doc.JSON = map[string]any{"_value": v}
			}
		}
	}

	// Optional branding format using the configured LLM provider when requested.
	if hasBranding, brandingPrompt := scrapeutil.GetBrandingFormatConfig(reqBody.Formats); hasBranding {
		client, provider, modelName, err := llm.NewClientFromConfig(cfg, "", "")
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(ErrorResponse{
				Success: false,
				Code:    "LLM_NOT_CONFIGURED",
				Error:   err.Error(),
			})
		}

		// Expose LLM info to logging middleware via locals (may override previous LLM info).
		c.Locals("llm_provider", string(provider))
		c.Locals("llm_model", modelName)

		// Default branding prompt modeled after Firecrawl's BrandingProfile,
		// asking for a structured object with keys like colorScheme, colors,
		// typography, spacing, components, images, fonts, tone, and personality.
		if brandingPrompt == "" {
			brandingPrompt = "You are a brand design expert analyzing a website. Analyze the page and return a single JSON object describing the brand, matching this structure as closely as possible: " +
				"{colorScheme?: 'light'|'dark', colors?: {primary?: string, secondary?: string, accent?: string, background?: string, textPrimary?: string, textSecondary?: string, link?: string, success?: string, warning?: string, error?: string}, " +
				"typography?: {fontFamilies?: {primary?: string, heading?: string, code?: string}, fontStacks?: {primary?: string[], heading?: string[], body?: string[], paragraph?: string[]}, fontSizes?: {h1?: string, h2?: string, h3?: string, body?: string, small?: string}}, " +
				"spacing?: {baseUnit?: number, borderRadius?: string}, components?: {buttonPrimary?: {background?: string, textColor?: string, borderColor?: string, borderRadius?: string}, buttonSecondary?: {...}}, " +
				"images?: {logo?: string|null, favicon?: string|null, ogImage?: string|null}, personality?: {tone?: string, energy?: string, targetAudience?: string}}. " +
				"Only include fields you can infer with reasonable confidence."
		}

		descBranding := "Brand identity and design system information (colors, typography, logo, components, personality, etc.) extracted from the page, following Firecrawl's BrandingProfile conventions."

		fieldSpecs := []llm.FieldSpec{
			{
				Name:        "branding",
				Description: descBranding,
				Type:        "object",
			},
		}

		llmTimeout := time.Duration(timeoutMs) * time.Millisecond
		llmCtx, llmCancel := context.WithTimeout(c.Context(), llmTimeout)
		defer llmCancel()

		llmRes, err := client.ExtractFields(llmCtx, llm.ExtractRequest{
			URL:      reqBody.URL,
			Markdown: res.Markdown,
			Fields:   fieldSpecs,
			Prompt:   brandingPrompt,
			Timeout:  llmTimeout,
			Strict:   false,
		})

		if err != nil {
			metrics.RecordLLMExtract(string(provider), modelName, false)
			status := fiber.StatusBadGateway
			if errors.Is(err, context.DeadlineExceeded) {
				status = http.StatusGatewayTimeout
			}
			return c.Status(status).JSON(ErrorResponse{
				Success: false,
				Code:    "BRANDING_FAILED",
				Error:   err.Error(),
			})
		}

		metrics.RecordLLMExtract(string(provider), modelName, true)

		if v, ok := llmRes.Fields["branding"]; ok {
			if m, ok := v.(map[string]any); ok {
				scrapeutil.NormalizeBrandingImages(m)
				doc.Branding = m
			} else {
				doc.Branding = map[string]any{"_value": v}
			}
		}
	}

	response := ScrapeResponse{

		Success: true,
		Data:    doc,
	}

	return c.Status(http.StatusOK).JSON(response)
}
