"use client"

import { useCallback, useEffect, useMemo, useState } from "react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Field, FieldContent, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger } from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"

type SystemSettingsResponse = {
  success?: boolean
  config?: SystemSettingsConfig
  secrets?: SystemSettingsSecrets
  configPath?: string
  notes?: string[]
  error?: string
}

type SystemSettingsSecrets = {
  authInitialAdminKeySet?: boolean
  authOidcClientSecretSet?: boolean
  authSessionSecretSet?: boolean
  llmOpenaiApiKeySet?: boolean
  llmAnthropicApiKeySet?: boolean
  llmGoogleApiKeySet?: boolean
  searchSearxngConfigured?: boolean
  searchProviderConfigured?: boolean
}

type SystemSettingsConfig = {
  scraper: {
    userAgent: string
    timeoutMs: number
    linksSameDomainOnly: boolean
    linksMaxPerDocument: number
  }
  crawler: {
    maxDepthDefault: number
    maxPagesDefault: number
  }
  robots: {
    respect: boolean
  }
  rod: {
    enabled: boolean
  }
  worker: {
    maxConcurrentJobs: number
    pollIntervalMs: number
    maxConcurrentUrlsPerJob: number
    syncJobWaitTimeoutMs: number
  }
  ratelimit: {
    defaultPerMinute: number
  }
  auth: {
    enabled: boolean
    initialAdminKey: string
    local: {
      enabled: boolean
    }
    oidc: {
      enabled: boolean
      issuerURL: string
      clientID: string
      clientSecret: string
      redirectURL: string
      allowedDomains: string[]
    }
    session: {
      secret: string
      cookieName: string
      ttlMinutes: number
    }
  }
  search: {
    enabled: boolean
    provider: string
    maxResults: number
    timeoutMs: number
    maxConcurrentScrapes: number
    searxng: {
      baseURL: string
      defaultLimit: number
      timeoutMs: number
    }
  }
  llm: {
    defaultProvider: string
    openai: {
      apiKey: string
      baseURL: string
      model: string
    }
    anthropic: {
      apiKey: string
      model: string
    }
    google: {
      apiKey: string
      model: string
    }
  }
}

function isNumberInput(v: string) {
  return v.trim() === "" || /^-?\d+$/.test(v.trim())
}

function setBadge(isSet?: boolean) {
  return isSet ? (
    <Badge variant="secondary" className="text-xs">Set</Badge>
  ) : (
    <Badge variant="outline" className="text-xs">Not set</Badge>
  )
}

export function AdminSystemSettingsPanel() {
  const [activeTab, setActiveTab] = useState("general")
  const [config, setConfig] = useState<SystemSettingsConfig | null>(null)
  const [secrets, setSecrets] = useState<SystemSettingsSecrets>({})
  const [configPath, setConfigPath] = useState<string | null>(null)
  const [notes, setNotes] = useState<string[]>([])

  const [saving, setSaving] = useState(false)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [successMessage, setSuccessMessage] = useState<string | null>(null)

  // Local editable fields. We keep them as strings to avoid UX issues.
  const [scraperUserAgent, setScraperUserAgent] = useState("")
  const [scraperTimeoutMs, setScraperTimeoutMs] = useState("")
  const [scraperLinksSameDomainOnly, setScraperLinksSameDomainOnly] = useState(false)
  const [scraperLinksMaxPerDocument, setScraperLinksMaxPerDocument] = useState("")

  const [crawlerMaxDepthDefault, setCrawlerMaxDepthDefault] = useState("")
  const [crawlerMaxPagesDefault, setCrawlerMaxPagesDefault] = useState("")

  const [robotsRespect, setRobotsRespect] = useState(false)
  const [rodEnabled, setRodEnabled] = useState(false)

  const [workerMaxConcurrentJobs, setWorkerMaxConcurrentJobs] = useState("")
  const [workerPollIntervalMs, setWorkerPollIntervalMs] = useState("")
  const [workerMaxConcurrentURLsPerJob, setWorkerMaxConcurrentURLsPerJob] = useState("")
  const [workerSyncJobWaitTimeoutMs, setWorkerSyncJobWaitTimeoutMs] = useState("")

  const [rateLimitDefaultPerMinute, setRateLimitDefaultPerMinute] = useState("")

  const [authEnabled, setAuthEnabled] = useState(false)
  const [authInitialAdminKey, setAuthInitialAdminKey] = useState("")
  const [authLocalEnabled, setAuthLocalEnabled] = useState(false)
  const [authSessionCookieName, setAuthSessionCookieName] = useState("")
  const [authSessionTTLMinutes, setAuthSessionTTLMinutes] = useState("")
  const [authSessionSecret, setAuthSessionSecret] = useState("")

  const [authOIDCEnabled, setAuthOIDCEnabled] = useState(false)
  const [authOIDCIssuerURL, setAuthOIDCIssuerURL] = useState("")
  const [authOIDCClientID, setAuthOIDCClientID] = useState("")
  const [authOIDCClientSecret, setAuthOIDCClientSecret] = useState("")
  const [authOIDCRedirectURL, setAuthOIDCRedirectURL] = useState("")
  const [authOIDCAllowedDomains, setAuthOIDCAllowedDomains] = useState("")

  const [searchEnabled, setSearchEnabled] = useState(false)
  const [searchProvider, setSearchProvider] = useState("")
  const [searchMaxResults, setSearchMaxResults] = useState("")
  const [searchTimeoutMs, setSearchTimeoutMs] = useState("")
  const [searchMaxConcurrentScrapes, setSearchMaxConcurrentScrapes] = useState("")
  const [searchSearxngBaseURL, setSearchSearxngBaseURL] = useState("")
  const [searchSearxngDefaultLimit, setSearchSearxngDefaultLimit] = useState("")
  const [searchSearxngTimeoutMs, setSearchSearxngTimeoutMs] = useState("")

  const [llmDefaultProvider, setLlmDefaultProvider] = useState("")
  const [llmOpenAIModel, setLlmOpenAIModel] = useState("")
  const [llmOpenAIBaseURL, setLlmOpenAIBaseURL] = useState("")
  const [llmOpenAIAPIKey, setLlmOpenAIAPIKey] = useState("")
  const [llmAnthropicModel, setLlmAnthropicModel] = useState("")
  const [llmAnthropicAPIKey, setLlmAnthropicAPIKey] = useState("")
  const [llmGoogleModel, setLlmGoogleModel] = useState("")
  const [llmGoogleAPIKey, setLlmGoogleAPIKey] = useState("")

  const load = useCallback(async () => {
    setLoading(true)
    setError(null)
    setSuccessMessage(null)
    try {
      const res = await fetch("/admin/system-settings")
      const data = (await res.json()) as SystemSettingsResponse
      if (!res.ok || !data.success || !data.config) {
        setError(data.error || "Unable to load system settings")
        return
      }
      setConfig(data.config)
      setSecrets(data.secrets ?? {})
      setConfigPath(data.configPath ?? null)
      setNotes(data.notes ?? [])

      // Initialize editable state from loaded config.
      setScraperUserAgent(data.config.scraper?.userAgent ?? "")
      setScraperTimeoutMs(String(data.config.scraper?.timeoutMs ?? ""))
      setScraperLinksSameDomainOnly(!!data.config.scraper?.linksSameDomainOnly)
      setScraperLinksMaxPerDocument(String(data.config.scraper?.linksMaxPerDocument ?? ""))

      setCrawlerMaxDepthDefault(String(data.config.crawler?.maxDepthDefault ?? ""))
      setCrawlerMaxPagesDefault(String(data.config.crawler?.maxPagesDefault ?? ""))

      setRobotsRespect(!!data.config.robots?.respect)
      setRodEnabled(!!data.config.rod?.enabled)

      setWorkerMaxConcurrentJobs(String(data.config.worker?.maxConcurrentJobs ?? ""))
      setWorkerPollIntervalMs(String(data.config.worker?.pollIntervalMs ?? ""))
      setWorkerMaxConcurrentURLsPerJob(String(data.config.worker?.maxConcurrentUrlsPerJob ?? ""))
      setWorkerSyncJobWaitTimeoutMs(String(data.config.worker?.syncJobWaitTimeoutMs ?? ""))

      setRateLimitDefaultPerMinute(String(data.config.ratelimit?.defaultPerMinute ?? ""))

      setAuthEnabled(!!data.config.auth?.enabled)
      setAuthLocalEnabled(!!data.config.auth?.local?.enabled)
      setAuthSessionCookieName(data.config.auth?.session?.cookieName ?? "")
      setAuthSessionTTLMinutes(String(data.config.auth?.session?.ttlMinutes ?? ""))

      // Secrets are redacted; keep the inputs blank so we don't overwrite.
      setAuthInitialAdminKey("")
      setAuthSessionSecret("")
      setAuthOIDCClientSecret("")
      setLlmOpenAIAPIKey("")
      setLlmAnthropicAPIKey("")
      setLlmGoogleAPIKey("")

      setAuthOIDCEnabled(!!data.config.auth?.oidc?.enabled)
      setAuthOIDCIssuerURL(data.config.auth?.oidc?.issuerURL ?? "")
      setAuthOIDCClientID(data.config.auth?.oidc?.clientID ?? "")
      setAuthOIDCRedirectURL(data.config.auth?.oidc?.redirectURL ?? "")
      setAuthOIDCAllowedDomains((data.config.auth?.oidc?.allowedDomains ?? []).join(", "))

      setSearchEnabled(!!data.config.search?.enabled)
      setSearchProvider(data.config.search?.provider ?? "")
      setSearchMaxResults(String(data.config.search?.maxResults ?? ""))
      setSearchTimeoutMs(String(data.config.search?.timeoutMs ?? ""))
      setSearchMaxConcurrentScrapes(String(data.config.search?.maxConcurrentScrapes ?? ""))
      setSearchSearxngBaseURL(data.config.search?.searxng?.baseURL ?? "")
      setSearchSearxngDefaultLimit(String(data.config.search?.searxng?.defaultLimit ?? ""))
      setSearchSearxngTimeoutMs(String(data.config.search?.searxng?.timeoutMs ?? ""))

      setLlmDefaultProvider(data.config.llm?.defaultProvider ?? "")
      setLlmOpenAIModel(data.config.llm?.openai?.model ?? "")
      setLlmOpenAIBaseURL(data.config.llm?.openai?.baseURL ?? "")
      setLlmAnthropicModel(data.config.llm?.anthropic?.model ?? "")
      setLlmGoogleModel(data.config.llm?.google?.model ?? "")
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to load system settings")
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    load()
  }, [load])

  const validationError = useMemo(() => {
    const numericFields: Array<[string, string]> = [
      ["Scraper timeout (ms)", scraperTimeoutMs],
      ["Scraper links max per document", scraperLinksMaxPerDocument],
      ["Crawler max depth default", crawlerMaxDepthDefault],
      ["Crawler max pages default", crawlerMaxPagesDefault],
      ["Worker max concurrent jobs", workerMaxConcurrentJobs],
      ["Worker poll interval (ms)", workerPollIntervalMs],
      ["Worker max concurrent URLs per job", workerMaxConcurrentURLsPerJob],
      ["Worker sync job wait timeout (ms)", workerSyncJobWaitTimeoutMs],
      ["Rate limit default per minute", rateLimitDefaultPerMinute],
      ["Auth session TTL minutes", authSessionTTLMinutes],
      ["Search max results", searchMaxResults],
      ["Search timeout (ms)", searchTimeoutMs],
      ["Search max concurrent scrapes", searchMaxConcurrentScrapes],
      ["SearxNG default limit", searchSearxngDefaultLimit],
      ["SearxNG timeout (ms)", searchSearxngTimeoutMs],
    ]
    for (const [label, value] of numericFields) {
      if (!isNumberInput(value)) return `${label} must be an integer`
    }
    return null
  }, [
    authSessionTTLMinutes,
    crawlerMaxDepthDefault,
    crawlerMaxPagesDefault,
    rateLimitDefaultPerMinute,
    scraperLinksMaxPerDocument,
    scraperTimeoutMs,
    searchMaxConcurrentScrapes,
    searchMaxResults,
    searchSearxngDefaultLimit,
    searchSearxngTimeoutMs,
    searchTimeoutMs,
    workerMaxConcurrentJobs,
    workerMaxConcurrentURLsPerJob,
    workerPollIntervalMs,
    workerSyncJobWaitTimeoutMs,
  ])

  const onSave = useCallback(async () => {
    if (!config) return
    if (validationError) {
      setError(validationError)
      return
    }

    const patch: any = {
      scraper: {
        userAgent: scraperUserAgent,
        timeoutMs: Number(scraperTimeoutMs || 0),
        linksSameDomainOnly: scraperLinksSameDomainOnly,
        linksMaxPerDocument: Number(scraperLinksMaxPerDocument || 0),
      },
      crawler: {
        maxDepthDefault: Number(crawlerMaxDepthDefault || 0),
        maxPagesDefault: Number(crawlerMaxPagesDefault || 0),
      },
      robots: { respect: robotsRespect },
      rod: { enabled: rodEnabled },
      worker: {
        maxConcurrentJobs: Number(workerMaxConcurrentJobs || 0),
        pollIntervalMs: Number(workerPollIntervalMs || 0),
        maxConcurrentUrlsPerJob: Number(workerMaxConcurrentURLsPerJob || 0),
        syncJobWaitTimeoutMs: Number(workerSyncJobWaitTimeoutMs || 0),
      },
      ratelimit: { defaultPerMinute: Number(rateLimitDefaultPerMinute || 0) },
      auth: {
        enabled: authEnabled,
        local: { enabled: authLocalEnabled },
        oidc: {
          enabled: authOIDCEnabled,
          issuerURL: authOIDCIssuerURL,
          clientID: authOIDCClientID,
          redirectURL: authOIDCRedirectURL,
          allowedDomains: authOIDCAllowedDomains
            .split(",")
            .map((s) => s.trim())
            .filter(Boolean),
        },
        session: {
          cookieName: authSessionCookieName,
          ttlMinutes: Number(authSessionTTLMinutes || 0),
        },
      },
      search: {
        enabled: searchEnabled,
        provider: searchProvider,
        maxResults: Number(searchMaxResults || 0),
        timeoutMs: Number(searchTimeoutMs || 0),
        maxConcurrentScrapes: Number(searchMaxConcurrentScrapes || 0),
        searxng: {
          baseURL: searchSearxngBaseURL,
          defaultLimit: Number(searchSearxngDefaultLimit || 0),
          timeoutMs: Number(searchSearxngTimeoutMs || 0),
        },
      },
      llm: {
        defaultProvider: llmDefaultProvider,
        openai: {
          baseURL: llmOpenAIBaseURL,
          model: llmOpenAIModel,
        },
        anthropic: {
          model: llmAnthropicModel,
        },
        google: {
          model: llmGoogleModel,
        },
      },
    }

    // Only include secrets if the user entered a value (avoid wiping).
    if (authInitialAdminKey.trim()) patch.auth.initialAdminKey = authInitialAdminKey.trim()
    if (authSessionSecret.trim()) patch.auth.session.secret = authSessionSecret.trim()
    if (authOIDCClientSecret.trim()) patch.auth.oidc.clientSecret = authOIDCClientSecret.trim()
    if (llmOpenAIAPIKey.trim()) patch.llm.openai.apiKey = llmOpenAIAPIKey.trim()
    if (llmAnthropicAPIKey.trim()) patch.llm.anthropic.apiKey = llmAnthropicAPIKey.trim()
    if (llmGoogleAPIKey.trim()) patch.llm.google.apiKey = llmGoogleAPIKey.trim()

    setSaving(true)
    setError(null)
    setSuccessMessage(null)
    try {
      const res = await fetch("/admin/system-settings", {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(patch),
      })
      const data = (await res.json()) as SystemSettingsResponse
      if (!res.ok || !data.success || !data.config) {
        setError(data.error || "Unable to save system settings")
        return
      }
      setConfig(data.config)
      setSecrets(data.secrets ?? {})
      setNotes(data.notes ?? [])
      setSuccessMessage("Saved.")

      // Clear secret inputs after successful save.
      setAuthInitialAdminKey("")
      setAuthSessionSecret("")
      setAuthOIDCClientSecret("")
      setLlmOpenAIAPIKey("")
      setLlmAnthropicAPIKey("")
      setLlmGoogleAPIKey("")
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to save system settings")
    } finally {
      setSaving(false)
    }
  }, [
    authEnabled,
    authInitialAdminKey,
    authLocalEnabled,
    authOIDCAllowedDomains,
    authOIDCClientID,
    authOIDCClientSecret,
    authOIDCEnabled,
    authOIDCIssuerURL,
    authOIDCRedirectURL,
    authSessionCookieName,
    authSessionSecret,
    authSessionTTLMinutes,
    config,
    crawlerMaxDepthDefault,
    crawlerMaxPagesDefault,
    llmAnthropicAPIKey,
    llmAnthropicModel,
    llmDefaultProvider,
    llmGoogleAPIKey,
    llmGoogleModel,
    llmOpenAIAPIKey,
    llmOpenAIBaseURL,
    llmOpenAIModel,
    rateLimitDefaultPerMinute,
    robotsRespect,
    rodEnabled,
    scraperLinksMaxPerDocument,
    scraperLinksSameDomainOnly,
    scraperTimeoutMs,
    scraperUserAgent,
    searchEnabled,
    searchMaxConcurrentScrapes,
    searchMaxResults,
    searchProvider,
    searchSearxngBaseURL,
    searchSearxngDefaultLimit,
    searchSearxngTimeoutMs,
    searchTimeoutMs,
    validationError,
    workerMaxConcurrentJobs,
    workerMaxConcurrentURLsPerJob,
    workerPollIntervalMs,
    workerSyncJobWaitTimeoutMs,
  ])

  return (
    <Card>
      <CardHeader>
        <div className="flex items-start justify-between gap-3">
          <div className="space-y-1">
            <CardTitle className="text-sm">System settings</CardTitle>
            <CardDescription className="text-xs">
              Configure system-wide settings for this instance.
            </CardDescription>
          </div>

          <div className="flex items-center gap-2">
            <Button variant="secondary" size="sm" onClick={load} disabled={loading || saving}>
              Refresh
            </Button>
            <Button size="sm" onClick={onSave} disabled={loading || saving || !config}>
              {saving ? "Saving…" : "Save"}
            </Button>
          </div>
        </div>

        {configPath ? (
          <div className="text-muted-foreground text-xs">Config file: {configPath}</div>
        ) : null}
        {notes?.length ? (
          <div className="text-muted-foreground text-xs">{notes[0]}</div>
        ) : null}
      </CardHeader>
      <CardContent className="space-y-3">
        {error ? <div className="text-xs text-destructive">{error}</div> : null}
        {successMessage ? <div className="text-xs text-emerald-400">{successMessage}</div> : null}

        <Tabs value={activeTab} onValueChange={setActiveTab} className="w-full">
          <TabsList variant="line">
            <TabsTrigger value="general">General</TabsTrigger>
            <TabsTrigger value="auth">Auth</TabsTrigger>
            <TabsTrigger value="search">Search</TabsTrigger>
            <TabsTrigger value="llm">LLM</TabsTrigger>
          </TabsList>

          <TabsContent value="general" className="mt-4 space-y-3">
            <Card>
              <CardHeader>
                <CardTitle className="text-sm">Scraper</CardTitle>
                <CardDescription className="text-xs">Controls scraping behavior.</CardDescription>
              </CardHeader>
              <CardContent>
                <FieldGroup>
                  <Field>
                    <FieldLabel>Default user agent</FieldLabel>
                    <FieldContent>
                      <Input value={scraperUserAgent} onChange={(e) => setScraperUserAgent(e.target.value)} />
                      <FieldDescription>Sent as the User-Agent header for scraping.</FieldDescription>
                    </FieldContent>
                  </Field>
                  <Field orientation="responsive">
                    <FieldLabel>Timeout (ms)</FieldLabel>
                    <FieldContent>
                      <Input value={scraperTimeoutMs} onChange={(e) => setScraperTimeoutMs(e.target.value)} />
                      <FieldDescription>Base timeout used for scrape requests.</FieldDescription>
                    </FieldContent>
                  </Field>
                  <Field orientation="responsive">
                    <FieldLabel>Links max per document</FieldLabel>
                    <FieldContent>
                      <Input
                        value={scraperLinksMaxPerDocument}
                        onChange={(e) => setScraperLinksMaxPerDocument(e.target.value)}
                      />
                      <FieldDescription>Limits extracted links per document.</FieldDescription>
                    </FieldContent>
                  </Field>
                  <Field orientation="responsive">
                    <FieldLabel>Same-domain links only</FieldLabel>
                    <FieldContent className="flex-row items-center justify-between gap-3">
                      <div className="text-muted-foreground text-xs">
                        Only keep links in the same domain during scrape.
                      </div>
                      <Switch checked={scraperLinksSameDomainOnly} onCheckedChange={setScraperLinksSameDomainOnly} />
                    </FieldContent>
                  </Field>
                </FieldGroup>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className="text-sm">Crawler</CardTitle>
                <CardDescription className="text-xs">Default crawling limits.</CardDescription>
              </CardHeader>
              <CardContent>
                <FieldGroup>
                  <Field orientation="responsive">
                    <FieldLabel>Max depth default</FieldLabel>
                    <FieldContent>
                      <Input value={crawlerMaxDepthDefault} onChange={(e) => setCrawlerMaxDepthDefault(e.target.value)} />
                      <FieldDescription>Default crawl depth when not specified.</FieldDescription>
                    </FieldContent>
                  </Field>
                  <Field orientation="responsive">
                    <FieldLabel>Max pages default</FieldLabel>
                    <FieldContent>
                      <Input value={crawlerMaxPagesDefault} onChange={(e) => setCrawlerMaxPagesDefault(e.target.value)} />
                      <FieldDescription>Default page limit when not specified.</FieldDescription>
                    </FieldContent>
                  </Field>
                </FieldGroup>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className="text-sm">Robots</CardTitle>
                <CardDescription className="text-xs">Robots.txt behavior.</CardDescription>
              </CardHeader>
              <CardContent>
                <FieldGroup>
                  <Field orientation="responsive">
                    <FieldLabel>Respect robots.txt</FieldLabel>
                    <FieldContent className="flex-row items-center justify-between gap-3">
                      <div className="text-muted-foreground text-xs">
                        When enabled, the crawler respects robots.txt directives.
                      </div>
                      <Switch checked={robotsRespect} onCheckedChange={setRobotsRespect} />
                    </FieldContent>
                  </Field>
                </FieldGroup>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className="text-sm">Rod</CardTitle>
                <CardDescription className="text-xs">Browser automation support.</CardDescription>
              </CardHeader>
              <CardContent>
                <FieldGroup>
                  <Field orientation="responsive">
                    <FieldLabel>Enable Rod</FieldLabel>
                    <FieldContent className="flex-row items-center justify-between gap-3">
                      <div className="text-muted-foreground text-xs">
                        Enables Rod-based rendering when available.
                      </div>
                      <Switch checked={rodEnabled} onCheckedChange={setRodEnabled} />
                    </FieldContent>
                  </Field>
                </FieldGroup>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className="text-sm">Worker</CardTitle>
                <CardDescription className="text-xs">Job processing and concurrency.</CardDescription>
              </CardHeader>
              <CardContent>
                <FieldGroup>
                  <Field orientation="responsive">
                    <FieldLabel>Max concurrent jobs</FieldLabel>
                    <FieldContent>
                      <Input value={workerMaxConcurrentJobs} onChange={(e) => setWorkerMaxConcurrentJobs(e.target.value)} />
                      <FieldDescription>Maximum number of jobs processed in parallel.</FieldDescription>
                    </FieldContent>
                  </Field>
                  <Field orientation="responsive">
                    <FieldLabel>Poll interval (ms)</FieldLabel>
                    <FieldContent>
                      <Input value={workerPollIntervalMs} onChange={(e) => setWorkerPollIntervalMs(e.target.value)} />
                      <FieldDescription>How frequently the worker checks for pending jobs.</FieldDescription>
                    </FieldContent>
                  </Field>
                  <Field orientation="responsive">
                    <FieldLabel>Max concurrent URLs / job</FieldLabel>
                    <FieldContent>
                      <Input
                        value={workerMaxConcurrentURLsPerJob}
                        onChange={(e) => setWorkerMaxConcurrentURLsPerJob(e.target.value)}
                      />
                      <FieldDescription>Limits URL-level concurrency within a single job.</FieldDescription>
                    </FieldContent>
                  </Field>
                  <Field orientation="responsive">
                    <FieldLabel>Sync wait timeout (ms)</FieldLabel>
                    <FieldContent>
                      <Input
                        value={workerSyncJobWaitTimeoutMs}
                        onChange={(e) => setWorkerSyncJobWaitTimeoutMs(e.target.value)}
                      />
                      <FieldDescription>How long API waits for sync jobs to finish.</FieldDescription>
                    </FieldContent>
                  </Field>
                </FieldGroup>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className="text-sm">Rate limit</CardTitle>
                <CardDescription className="text-xs">Default API key limits.</CardDescription>
              </CardHeader>
              <CardContent>
                <FieldGroup>
                  <Field orientation="responsive">
                    <FieldLabel>Default per minute</FieldLabel>
                    <FieldContent>
                      <Input
                        value={rateLimitDefaultPerMinute}
                        onChange={(e) => setRateLimitDefaultPerMinute(e.target.value)}
                      />
                      <FieldDescription>Used when a key has no explicit per-minute rate limit.</FieldDescription>
                    </FieldContent>
                  </Field>
                </FieldGroup>
              </CardContent>
            </Card>
          </TabsContent>

          <TabsContent value="auth" className="mt-4 space-y-3">
            <Card>
              <CardHeader>
                <CardTitle className="text-sm">Auth</CardTitle>
                <CardDescription className="text-xs">Login providers and session configuration.</CardDescription>
              </CardHeader>
              <CardContent>
                <FieldGroup>
                  <Field orientation="responsive">
                    <FieldLabel>Enable auth</FieldLabel>
                    <FieldContent className="flex-row items-center justify-between gap-3">
                      <div className="text-muted-foreground text-xs">Enables session auth and API key validation.</div>
                      <Switch checked={authEnabled} onCheckedChange={setAuthEnabled} />
                    </FieldContent>
                  </Field>

                  <Field orientation="responsive">
                    <FieldLabel>Initial admin key</FieldLabel>
                    <FieldContent>
                      <div className="flex items-center gap-2">
                        <Input
                          value={authInitialAdminKey}
                          onChange={(e) => setAuthInitialAdminKey(e.target.value)}
                          placeholder={secrets.authInitialAdminKeySet ? "•••••••• (set)" : "Not set"}
                        />
                        {setBadge(secrets.authInitialAdminKeySet)}
                      </div>
                      <FieldDescription>Sets (or replaces) the bootstrap admin API key.</FieldDescription>
                    </FieldContent>
                  </Field>

                  <Field orientation="responsive">
                    <FieldLabel>Local auth</FieldLabel>
                    <FieldContent className="flex-row items-center justify-between gap-3">
                      <div className="text-muted-foreground text-xs">Allows local username/password auth.</div>
                      <Switch checked={authLocalEnabled} onCheckedChange={setAuthLocalEnabled} />
                    </FieldContent>
                  </Field>
                </FieldGroup>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className="text-sm">Session</CardTitle>
                <CardDescription className="text-xs">Cookie-based session settings.</CardDescription>
              </CardHeader>
              <CardContent>
                <FieldGroup>
                  <Field orientation="responsive">
                    <FieldLabel>Session secret</FieldLabel>
                    <FieldContent>
                      <div className="flex items-center gap-2">
                        <Input
                          value={authSessionSecret}
                          onChange={(e) => setAuthSessionSecret(e.target.value)}
                          placeholder={secrets.authSessionSecretSet ? "•••••••• (set)" : "Not set"}
                        />
                        {setBadge(secrets.authSessionSecretSet)}
                      </div>
                      <FieldDescription>HS256 secret used to sign session JWTs.</FieldDescription>
                    </FieldContent>
                  </Field>
                  <Field orientation="responsive">
                    <FieldLabel>Cookie name</FieldLabel>
                    <FieldContent>
                      <Input value={authSessionCookieName} onChange={(e) => setAuthSessionCookieName(e.target.value)} />
                      <FieldDescription>Name of the session cookie.</FieldDescription>
                    </FieldContent>
                  </Field>
                  <Field orientation="responsive">
                    <FieldLabel>TTL minutes</FieldLabel>
                    <FieldContent>
                      <Input value={authSessionTTLMinutes} onChange={(e) => setAuthSessionTTLMinutes(e.target.value)} />
                      <FieldDescription>Session lifetime in minutes.</FieldDescription>
                    </FieldContent>
                  </Field>
                </FieldGroup>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className="text-sm">OIDC</CardTitle>
                <CardDescription className="text-xs">OpenID Connect provider settings.</CardDescription>
              </CardHeader>
              <CardContent>
                <FieldGroup>
                  <Field orientation="responsive">
                    <FieldLabel>Enable OIDC</FieldLabel>
                    <FieldContent className="flex-row items-center justify-between gap-3">
                      <div className="text-muted-foreground text-xs">Enables OIDC login provider.</div>
                      <Switch checked={authOIDCEnabled} onCheckedChange={setAuthOIDCEnabled} />
                    </FieldContent>
                  </Field>
                  <Field>
                    <FieldLabel>Issuer URL</FieldLabel>
                    <FieldContent>
                      <Input value={authOIDCIssuerURL} onChange={(e) => setAuthOIDCIssuerURL(e.target.value)} />
                      <FieldDescription>OIDC issuer URL.</FieldDescription>
                    </FieldContent>
                  </Field>
                  <Field>
                    <FieldLabel>Client ID</FieldLabel>
                    <FieldContent>
                      <Input value={authOIDCClientID} onChange={(e) => setAuthOIDCClientID(e.target.value)} />
                      <FieldDescription>OIDC client identifier.</FieldDescription>
                    </FieldContent>
                  </Field>
                  <Field>
                    <FieldLabel>Client secret</FieldLabel>
                    <FieldContent>
                      <div className="flex items-center gap-2">
                        <Input
                          value={authOIDCClientSecret}
                          onChange={(e) => setAuthOIDCClientSecret(e.target.value)}
                          placeholder={secrets.authOidcClientSecretSet ? "•••••••• (set)" : "Not set"}
                        />
                        {setBadge(secrets.authOidcClientSecretSet)}
                      </div>
                      <FieldDescription>OIDC client secret.</FieldDescription>
                    </FieldContent>
                  </Field>
                  <Field>
                    <FieldLabel>Redirect URL</FieldLabel>
                    <FieldContent>
                      <Input value={authOIDCRedirectURL} onChange={(e) => setAuthOIDCRedirectURL(e.target.value)} />
                      <FieldDescription>Callback URL registered with the provider.</FieldDescription>
                    </FieldContent>
                  </Field>
                  <Field>
                    <FieldLabel>Allowed domains</FieldLabel>
                    <FieldContent>
                      <Input
                        value={authOIDCAllowedDomains}
                        onChange={(e) => setAuthOIDCAllowedDomains(e.target.value)}
                        placeholder="example.com, example.org"
                      />
                      <FieldDescription>Comma-separated email domains allowed to login.</FieldDescription>
                    </FieldContent>
                  </Field>
                </FieldGroup>
              </CardContent>
            </Card>
          </TabsContent>

          <TabsContent value="search" className="mt-4 space-y-3">
            <Card>
              <CardHeader>
                <CardTitle className="text-sm">Search</CardTitle>
                <CardDescription className="text-xs">System search provider configuration.</CardDescription>
              </CardHeader>
              <CardContent>
                <FieldGroup>
                  <Field orientation="responsive">
                    <FieldLabel>Enable search</FieldLabel>
                    <FieldContent className="flex-row items-center justify-between gap-3">
                      <div className="text-muted-foreground text-xs">Enables the `/v1/search` endpoint.</div>
                      <Switch checked={searchEnabled} onCheckedChange={setSearchEnabled} />
                    </FieldContent>
                  </Field>
                  <Field orientation="responsive">
                    <FieldLabel>Provider</FieldLabel>
                    <FieldContent>
                      <div className="flex items-center gap-2">
                        <Input
                          value={searchProvider}
                          onChange={(e) => setSearchProvider(e.target.value)}
                          placeholder="searxng"
                        />
                        {setBadge(secrets.searchProviderConfigured)}
                      </div>
                      <FieldDescription>Provider name (for now, `searxng`).</FieldDescription>
                    </FieldContent>
                  </Field>
                  <Field orientation="responsive">
                    <FieldLabel>Max results</FieldLabel>
                    <FieldContent>
                      <Input value={searchMaxResults} onChange={(e) => setSearchMaxResults(e.target.value)} />
                      <FieldDescription>Upper bound on results returned.</FieldDescription>
                    </FieldContent>
                  </Field>
                  <Field orientation="responsive">
                    <FieldLabel>Timeout (ms)</FieldLabel>
                    <FieldContent>
                      <Input value={searchTimeoutMs} onChange={(e) => setSearchTimeoutMs(e.target.value)} />
                      <FieldDescription>Search provider request timeout.</FieldDescription>
                    </FieldContent>
                  </Field>
                  <Field orientation="responsive">
                    <FieldLabel>Max concurrent scrapes</FieldLabel>
                    <FieldContent>
                      <Input
                        value={searchMaxConcurrentScrapes}
                        onChange={(e) => setSearchMaxConcurrentScrapes(e.target.value)}
                      />
                      <FieldDescription>Concurrency used when expanding search results.</FieldDescription>
                    </FieldContent>
                  </Field>
                </FieldGroup>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className="text-sm">SearxNG</CardTitle>
                <CardDescription className="text-xs">SearxNG connection details.</CardDescription>
              </CardHeader>
              <CardContent>
                <FieldGroup>
                  <Field>
                    <FieldLabel>Base URL</FieldLabel>
                    <FieldContent>
                      <div className="flex items-center gap-2">
                        <Input
                          value={searchSearxngBaseURL}
                          onChange={(e) => setSearchSearxngBaseURL(e.target.value)}
                          placeholder="http://localhost:8080"
                        />
                        {setBadge(secrets.searchSearxngConfigured)}
                      </div>
                      <FieldDescription>Base URL for the SearxNG instance.</FieldDescription>
                    </FieldContent>
                  </Field>
                  <Field orientation="responsive">
                    <FieldLabel>Default limit</FieldLabel>
                    <FieldContent>
                      <Input
                        value={searchSearxngDefaultLimit}
                        onChange={(e) => setSearchSearxngDefaultLimit(e.target.value)}
                      />
                      <FieldDescription>Default number of results requested from SearxNG.</FieldDescription>
                    </FieldContent>
                  </Field>
                  <Field orientation="responsive">
                    <FieldLabel>Timeout (ms)</FieldLabel>
                    <FieldContent>
                      <Input value={searchSearxngTimeoutMs} onChange={(e) => setSearchSearxngTimeoutMs(e.target.value)} />
                      <FieldDescription>Timeout for SearxNG requests.</FieldDescription>
                    </FieldContent>
                  </Field>
                </FieldGroup>
              </CardContent>
            </Card>
          </TabsContent>

          <TabsContent value="llm" className="mt-4 space-y-3">
            <Card>
              <CardHeader>
                <CardTitle className="text-sm">LLM</CardTitle>
                <CardDescription className="text-xs">Model provider configuration.</CardDescription>
              </CardHeader>
              <CardContent>
                <FieldGroup>
                  <Field orientation="responsive">
                    <FieldLabel>Default provider</FieldLabel>
                    <FieldContent>
                      <Select value={llmDefaultProvider} onValueChange={(v) => setLlmDefaultProvider(v ?? "")}>
                        <SelectTrigger className="w-[240px]">
                          <span className="min-w-0 truncate" title={llmDefaultProvider || ""}>
                            {llmDefaultProvider || "Select provider"}
                          </span>
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="openai">openai</SelectItem>
                          <SelectItem value="anthropic">anthropic</SelectItem>
                          <SelectItem value="google">google</SelectItem>
                        </SelectContent>
                      </Select>
                      <FieldDescription>Used when the request doesn’t specify a provider.</FieldDescription>
                    </FieldContent>
                  </Field>
                </FieldGroup>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className="text-sm">OpenAI</CardTitle>
                <CardDescription className="text-xs">OpenAI-compatible provider settings.</CardDescription>
              </CardHeader>
              <CardContent>
                <FieldGroup>
                  <Field>
                    <FieldLabel>API key</FieldLabel>
                    <FieldContent>
                      <div className="flex items-center gap-2">
                        <Input
                          value={llmOpenAIAPIKey}
                          onChange={(e) => setLlmOpenAIAPIKey(e.target.value)}
                          placeholder={secrets.llmOpenaiApiKeySet ? "•••••••• (set)" : "Not set"}
                        />
                        {setBadge(secrets.llmOpenaiApiKeySet)}
                      </div>
                      <FieldDescription>Stored in config; not returned by the API.</FieldDescription>
                    </FieldContent>
                  </Field>
                  <Field>
                    <FieldLabel>Base URL</FieldLabel>
                    <FieldContent>
                      <Input value={llmOpenAIBaseURL} onChange={(e) => setLlmOpenAIBaseURL(e.target.value)} />
                      <FieldDescription>Optional override for OpenAI base URL.</FieldDescription>
                    </FieldContent>
                  </Field>
                  <Field>
                    <FieldLabel>Model</FieldLabel>
                    <FieldContent>
                      <Input value={llmOpenAIModel} onChange={(e) => setLlmOpenAIModel(e.target.value)} />
                      <FieldDescription>Default model name for OpenAI provider.</FieldDescription>
                    </FieldContent>
                  </Field>
                </FieldGroup>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className="text-sm">Anthropic</CardTitle>
                <CardDescription className="text-xs">Anthropic model settings.</CardDescription>
              </CardHeader>
              <CardContent>
                <FieldGroup>
                  <Field>
                    <FieldLabel>API key</FieldLabel>
                    <FieldContent>
                      <div className="flex items-center gap-2">
                        <Input
                          value={llmAnthropicAPIKey}
                          onChange={(e) => setLlmAnthropicAPIKey(e.target.value)}
                          placeholder={secrets.llmAnthropicApiKeySet ? "•••••••• (set)" : "Not set"}
                        />
                        {setBadge(secrets.llmAnthropicApiKeySet)}
                      </div>
                      <FieldDescription>Stored in config; not returned by the API.</FieldDescription>
                    </FieldContent>
                  </Field>
                  <Field>
                    <FieldLabel>Model</FieldLabel>
                    <FieldContent>
                      <Input value={llmAnthropicModel} onChange={(e) => setLlmAnthropicModel(e.target.value)} />
                      <FieldDescription>Default model name for Anthropic provider.</FieldDescription>
                    </FieldContent>
                  </Field>
                </FieldGroup>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className="text-sm">Google</CardTitle>
                <CardDescription className="text-xs">Google Gemini settings.</CardDescription>
              </CardHeader>
              <CardContent>
                <FieldGroup>
                  <Field>
                    <FieldLabel>API key</FieldLabel>
                    <FieldContent>
                      <div className="flex items-center gap-2">
                        <Input
                          value={llmGoogleAPIKey}
                          onChange={(e) => setLlmGoogleAPIKey(e.target.value)}
                          placeholder={secrets.llmGoogleApiKeySet ? "•••••••• (set)" : "Not set"}
                        />
                        {setBadge(secrets.llmGoogleApiKeySet)}
                      </div>
                      <FieldDescription>Stored in config; not returned by the API.</FieldDescription>
                    </FieldContent>
                  </Field>
                  <Field>
                    <FieldLabel>Model</FieldLabel>
                    <FieldContent>
                      <Input value={llmGoogleModel} onChange={(e) => setLlmGoogleModel(e.target.value)} />
                      <FieldDescription>Default model name for Google provider.</FieldDescription>
                    </FieldContent>
                  </Field>
                </FieldGroup>
              </CardContent>
            </Card>
          </TabsContent>
        </Tabs>
      </CardContent>
    </Card>
  )
}
