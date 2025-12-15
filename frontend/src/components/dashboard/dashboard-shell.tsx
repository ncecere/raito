"use client"

import { useEffect, useState } from "react"

import { ThemeToggle } from "@/components/theme/theme-toggle"
import {
  TenantSwitcher,
  type TenantSwitcherTenant,
} from "@/components/dashboard/tenant-switcher"
import { ProfilePanel } from "@/components/dashboard/profile-panel"
import { UsagePanel } from "@/components/dashboard/usage-panel"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Separator } from "@/components/ui/separator"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetFooter,
  SheetTitle,
} from "@/components/ui/sheet"
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarInset,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
  SidebarRail,
  SidebarTrigger,
} from "@/components/ui/sidebar"
import { HugeiconsIcon } from "@hugeicons/react"
import { Analytics01Icon, Key01Icon, Logout01Icon, ProfileIcon, Task01Icon, UserCircle02Icon } from "@hugeicons/core-free-icons"


export type DashboardSection = "apiKeys" | "jobs" | "usage" | "profile"

export interface SessionInfo {
  user: {
    id: string
    email: string
    name?: string
    isSystemAdmin: boolean
    defaultTenantId?: string
    themePreference?: "light" | "dark" | "system"
  }
  personalTenant?: {
    id: string
    slug: string
    name: string
    type?: string
  }
  activeTenant?: {
    id: string
    slug: string
    name: string
    type?: string
  }
}

interface DashboardShellProps {
  session: SessionInfo
  onLogout: () => void
  onTenantChanged?: () => Promise<void> | void
}

export function DashboardShell({ session, onLogout, onTenantChanged }: DashboardShellProps) {
  const [section, setSection] = useState<DashboardSection>("apiKeys")
  const [activeTenant, setActiveTenant] = useState<
    { id: string; name: string; type: "personal" | "org" | string } | undefined
  >(
    session.activeTenant
      ? { id: session.activeTenant.id, name: session.activeTenant.name, type: session.activeTenant.type || "org" }
      : session.personalTenant
        ? { id: session.personalTenant.id, name: session.personalTenant.name, type: session.personalTenant.type || "personal" }
        : undefined
  )

  async function handleTenantSelected(tenant: TenantSwitcherTenant) {
    setActiveTenant({ id: tenant.id, name: tenant.name, type: tenant.type })
    if (onTenantChanged) {
      await onTenantChanged()
    }
  }

  const activeTenantDisplayName =
    activeTenant?.type === "personal" ? "Personal" : activeTenant?.name

  const userDisplayName =
    session.user.isSystemAdmin ? "Admin" : session.user.name?.trim() || "Profile"

  return (
    <SidebarProvider>
      <Sidebar collapsible="icon" variant="inset">
        <SidebarHeader>
          <TenantSwitcher
            activeTenantName={activeTenantDisplayName}
            activeTenantType={activeTenant?.type}
            onTenantSelected={handleTenantSelected}
          />
        </SidebarHeader>
        <SidebarContent>
          <SidebarGroup>
            <SidebarGroupLabel>Platform</SidebarGroupLabel>
            <SidebarGroupContent>
              <SidebarMenu>
                <SidebarMenuItem>
                <SidebarMenuButton
                  isActive={section === "apiKeys"}
                  onClick={() => setSection("apiKeys")}
                  tooltip="API keys"
                >
                  <HugeiconsIcon icon={Key01Icon} strokeWidth={2} />
                  <span>API keys</span>
                </SidebarMenuButton>
              </SidebarMenuItem>
                <SidebarMenuItem>
                <SidebarMenuButton
                  isActive={section === "jobs"}
                  onClick={() => setSection("jobs")}
                  tooltip="Jobs"
                >
                  <HugeiconsIcon icon={Task01Icon} strokeWidth={2} />
                  <span>Jobs</span>
                </SidebarMenuButton>
              </SidebarMenuItem>
                <SidebarMenuItem>
                  <SidebarMenuButton
                    isActive={section === "usage"}
                    onClick={() => setSection("usage")}
                    tooltip="Usage"
                  >
                    <HugeiconsIcon icon={Analytics01Icon} strokeWidth={2} />
                    <span>Usage</span>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
          {session.user.isSystemAdmin ? (
            <SidebarGroup>
              <SidebarGroupLabel>Admin</SidebarGroupLabel>
              <SidebarGroupContent>
                <SidebarMenu>
                  <SidebarMenuItem>
                    <SidebarMenuButton
                      isActive={false}
                      onClick={() => {
                        // Placeholder: future admin section.
                      }}
                      tooltip="System settings"
                    >
                      <span>System settings</span>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                </SidebarMenu>
              </SidebarGroupContent>
            </SidebarGroup>
          ) : null}
        </SidebarContent>
        <SidebarFooter>
          <SidebarMenu>
            <SidebarMenuItem>
              <DropdownMenu>
                <DropdownMenuTrigger
                  render={
                    <SidebarMenuButton
                      size="lg"
                      className="data-[state=open]:bg-sidebar-accent data-[state=open]:text-sidebar-accent-foreground rounded-lg border border-sidebar-border bg-sidebar group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:gap-0"
                      title={session.user.email}
                    >
                      <div className="bg-sidebar-primary text-sidebar-primary-foreground flex aspect-square size-9 items-center justify-center rounded-md">
                        <HugeiconsIcon icon={UserCircle02Icon} strokeWidth={2} />
                      </div>
                      <div className="grid flex-1 text-left text-sm leading-tight group-data-[collapsible=icon]:hidden">
                        <span className="truncate font-medium">
                          {userDisplayName}
                        </span>
                        <span className="truncate text-xs text-muted-foreground">
                          {session.user.email}
                        </span>
                      </div>
                      <span className="text-muted-foreground ml-auto text-xs group-data-[collapsible=icon]:hidden">⌄</span>
                    </SidebarMenuButton>
                  }
                />
                <DropdownMenuContent
                  className="w-(--anchor-width) min-w-56 rounded-lg"
                  align="start"
                  side="top"
                  sideOffset={4}
                >
                  <DropdownMenuGroup>
                    <DropdownMenuLabel className="text-muted-foreground text-xs">
                      {activeTenant ? `Workspace: ${activeTenantDisplayName}` : "Workspace"}
                    </DropdownMenuLabel>
                    <DropdownMenuSeparator />
                    <DropdownMenuItem
                      className="gap-2 p-2"
                      onClick={() => setSection("profile")}
                    >
                      <HugeiconsIcon icon={ProfileIcon} strokeWidth={2} />
                      Profile
                    </DropdownMenuItem>
                  </DropdownMenuGroup>
                  <DropdownMenuSeparator />
                  <DropdownMenuGroup>
                    <DropdownMenuItem
                      className="gap-2 p-2"
                      onClick={() => {
                        onLogout()
                      }}
                    >
                      <HugeiconsIcon icon={Logout01Icon} strokeWidth={2} />
                      Sign out
                    </DropdownMenuItem>
                  </DropdownMenuGroup>
                </DropdownMenuContent>
              </DropdownMenu>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarFooter>
        <SidebarRail />
      </Sidebar>

      <SidebarInset className="bg-background text-foreground flex min-h-svh w-full">
        <header className="flex h-16 shrink-0 items-center gap-2 transition-[width,height] ease-linear group-has-data-[collapsible=icon]/sidebar-wrapper:h-12">
          <div className="flex flex-1 items-center gap-2 px-4">
            <SidebarTrigger className="-ml-1" />
            <Separator
              orientation="vertical"
              className="mr-2 data-[orientation=vertical]:h-4"
            />
            <div className="flex flex-col gap-1">
              <h1 className="text-sm font-semibold">
                {section === "apiKeys"
                  ? "API keys"
                  : section === "jobs"
                    ? "Jobs"
                    : section === "usage"
                      ? "Usage"
                      : "Profile"}
              </h1>
              <p className="text-xs text-muted-foreground">
                {section === "apiKeys"
                  ? "Manage API keys for programmatic access."
                  : section === "jobs"
                    ? "Inspect recent scrape and crawl jobs."
                    : section === "usage"
                      ? "Understand activity and storage over time."
                      : "Manage your account preferences."}
              </p>
            </div>
          </div>
          <div className="px-4">
            <ThemeToggle />
          </div>
        </header>

        <div className="flex flex-1 flex-col gap-4 p-4 pt-0">
          {section === "apiKeys" ? (
            <ApiKeysPanel tenantId={activeTenant?.id} isPersonal={activeTenant?.type === "personal"} />
          ) : section === "jobs" ? (
            <JobsPanel activeTenantId={activeTenant?.id} sessionEmail={session.user.email} />
          ) : section === "usage" ? (
            <UsagePanel tenantId={activeTenant?.id} />
          ) : (
            <ProfilePanel
              userEmail={session.user.email}
              userName={session.user.name}
              defaultTenantId={session.user.defaultTenantId}
              themePreference={session.user.themePreference}
              onUpdated={onTenantChanged}
            />
          )}
        </div>
      </SidebarInset>
    </SidebarProvider>
  )
}

interface ApiKeysPanelProps {
  tenantId?: string
  isPersonal?: boolean
}

interface TenantAPIKeyItem {
  id: string
  label: string
  isAdmin: boolean
  createdAt: string
}

function ApiKeysPanel({ tenantId, isPersonal }: ApiKeysPanelProps) {
  const [keys, setKeys] = useState<TenantAPIKeyItem[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const [createFormOpen, setCreateFormOpen] = useState(false)
  const [creating, setCreating] = useState(false)
  const [newLabel, setNewLabel] = useState("")
  const [newRateLimit, setNewRateLimit] = useState("")
  const [createdKey, setCreatedKey] = useState<string | null>(null)
  const [createError, setCreateError] = useState<string | null>(null)
  const [revokingId, setRevokingId] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    if (!tenantId) {
      setKeys([])
      setError(null)
      setCreatedKey(null)
      setCreateFormOpen(false)
      setCreating(false)
      setCopied(false)
      return
    }

    let cancelled = false

    async function load() {
      // Reset tenant-scoped UI state immediately so we don't show stale data when switching tenants.
      setKeys([])
      setError(null)
      setCreatedKey(null)
      setCreateError(null)
      setCreateFormOpen(false)
      setCreating(false)
      setCopied(false)

      setLoading(true)
      setError(null)
      try {
        const res = await fetch(`/v1/tenants/${tenantId}/api-keys`)
        const data = (await res.json()) as {
          success?: boolean
          keys?: TenantAPIKeyItem[]
          error?: string
        }
        if (!res.ok || !data.success) {
          if (!cancelled) {
            setError(data.error || "Unable to load API keys")
          }
          return
        }
        if (!cancelled) {
          setKeys(data.keys ?? [])
        }
      } catch (err) {
        if (!cancelled) {
          setError("Network error while loading API keys")
        }
      } finally {
        if (!cancelled) {
          setLoading(false)
        }
      }
    }

    load()

    return () => {
      cancelled = true
    }
  }, [tenantId])

  async function refreshKeys() {
    if (!tenantId) return
    setLoading(true)
    setError(null)
    try {
      const res = await fetch(`/v1/tenants/${tenantId}/api-keys`)
      const data = (await res.json()) as {
        success?: boolean
        keys?: TenantAPIKeyItem[]
        error?: string
      }
      if (!res.ok || !data.success) {
        setError(data.error || "Unable to load API keys")
        return
      }
      setKeys(data.keys ?? [])
    } catch (err) {
      setError("Network error while loading API keys")
    } finally {
      setLoading(false)
    }
  }

  async function handleCreateKey(event: React.FormEvent) {
    event.preventDefault()
    if (!tenantId) return

    setCreateError(null)

    const label = newLabel.trim()
    if (!label) {
      setCreateError("Label is required")
      return
    }

    const body: { label: string; rateLimitPerMinute?: number } = { label }
    const rate = parseInt(newRateLimit, 10)
    if (!Number.isNaN(rate) && rate > 0) {
      body.rateLimitPerMinute = rate
    }

    setCreating(true)

    try {
      const res = await fetch(`/v1/tenants/${tenantId}/api-keys`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify(body),
      })
      const data = (await res.json()) as {
        success?: boolean
        key?: string
        error?: string
      }
      if (!res.ok || !data.success) {
        setCreateError(data.error || "Unable to create API key")
        return
      }
      setCreatedKey(data.key ?? null)
      setNewLabel("")
      setNewRateLimit("")
      setCreateFormOpen(false)
      await refreshKeys()
    } catch (err) {
      setCreateError("Network error while creating API key")
    } finally {
      setCreating(false)
    }
  }

  async function handleRevoke(id: string) {
    if (!tenantId) return
    setRevokingId(id)
    try {
      const res = await fetch(`/v1/tenants/${tenantId}/api-keys/${id}`, {
        method: "DELETE",
      })
      const data = (await res.json()) as { success?: boolean; error?: string }
      if (!res.ok || !data.success) {
        console.error("Failed to revoke API key", data.error)
        return
      }
      await refreshKeys()
    } catch (err) {
      console.error("Network error revoking API key", err)
    } finally {
      setRevokingId(null)
    }
  }

  async function handleCopyKey() {
    if (!createdKey) return
    try {
      if (navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(createdKey)
      } else {
        const textarea = document.createElement("textarea")
        textarea.value = createdKey
        textarea.setAttribute("readonly", "true")
        textarea.style.position = "fixed"
        textarea.style.left = "-9999px"
        textarea.style.top = "0"
        document.body.appendChild(textarea)
        textarea.select()
        document.execCommand("copy")
        document.body.removeChild(textarea)
      }
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1500)
    } catch (err) {
      console.error("Failed to copy API key", err)
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>API keys</CardTitle>
        <CardDescription>
          Tenant-scoped API keys are tied to the currently selected tenant.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {!tenantId ? (
          <p className="text-xs text-muted-foreground">
            Select a tenant in the sidebar to manage API keys for that
            workspace.
          </p>
        ) : (
          <>
            {error ? (
              <p className="text-xs text-destructive">{error}</p>
            ) : null}

            {createdKey ? (
              <div className="bg-muted/50 border border-border rounded-none px-3 py-2 text-[11px]">
                <div className="mb-1 font-medium">
                  New key (copy it now – it will not be shown again):
                </div>
                <div className="flex items-center gap-2">
                  <code className="break-all text-[11px] flex-1">
                    {createdKey}
                  </code>
                  <Button
                    variant="outline"
                    size="xs"
                    type="button"
                    onClick={handleCopyKey}
                  >
                    {copied ? "Copied" : "Copy"}
                  </Button>
                </div>
              </div>
            ) : null}

            <div className="flex items-center justify-between gap-2">
              <Button
                variant="outline"
                size="xs"
                type="button"
                onClick={() => {
                  setCreateFormOpen(true)
                  setCreateError(null)
                  setCreatedKey(null)
                }}
                disabled={createFormOpen || creating}
              >
                New API key
              </Button>
              <Button
                variant="ghost"
                size="xs"
                type="button"
                onClick={refreshKeys}
                disabled={loading || creating}
              >
                {loading ? "Refreshing…" : "Refresh"}
              </Button>
            </div>

            {createFormOpen ? (
              <form
                onSubmit={handleCreateKey}
                className="border-border mt-2 rounded-none border px-3 py-2"
              >
                <FieldGroup>
                  <Field>
                    <FieldLabel htmlFor="api-key-label">Label</FieldLabel>
                    <Input
                      id="api-key-label"
                      value={newLabel}
                      onChange={(event) => setNewLabel(event.target.value)}
                      required
                      placeholder="e.g. cli-tool, integration"
                    />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="api-key-rate-limit">
                      Rate limit per minute (optional)
                    </FieldLabel>
                    <Input
                      id="api-key-rate-limit"
                      type="number"
                      min={1}
                      value={newRateLimit}
                      onChange={(event) => setNewRateLimit(event.target.value)}
                      placeholder="leave blank for default"
                    />
                    <FieldDescription>
                      Controls how many requests this key can make per minute.
                    </FieldDescription>
                  </Field>
                  {createError ? (
                    <FieldError>{createError}</FieldError>
                  ) : null}
                  <Field orientation="horizontal">
                    <Button type="submit" size="xs" disabled={creating}>
                      {creating ? "Creating…" : "Create key"}
                    </Button>
                    <Button
                      variant="ghost"
                      type="button"
                      size="xs"
                      onClick={() => {
                        setCreateFormOpen(false)
                        setNewLabel("")
                        setNewRateLimit("")
                        setCreateError(null)
                      }}
                      disabled={creating}
                    >
                      Cancel
                    </Button>
                  </Field>
                </FieldGroup>
              </form>
            ) : null}

            <div className="mt-2">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Label</TableHead>
                    <TableHead>Type</TableHead>
                    <TableHead>Created</TableHead>
                    <TableHead className="text-right">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {keys.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={4} className="text-xs text-muted-foreground">
                        No API keys for this tenant yet.
                      </TableCell>
                    </TableRow>
                  ) : (
                    keys.map((key) => (
                      <TableRow key={key.id}>
                        <TableCell>{key.label}</TableCell>
                        <TableCell>
                          {key.isAdmin ? "Admin" : isPersonal ? "Personal" : "Tenant"}
                        </TableCell>
                        <TableCell>
                          {new Date(key.createdAt).toLocaleString()}
                        </TableCell>
                        <TableCell className="text-right">
                          <Button
                            variant="ghost"
                            size="xs"
                            type="button"
                            onClick={() => handleRevoke(key.id)}
                            disabled={revokingId === key.id}
                          >
                            {revokingId === key.id ? "Revoking…" : "Revoke"}
                          </Button>
                        </TableCell>
                      </TableRow>
                    ))
                  )}
                </TableBody>
              </Table>
            </div>
          </>
        )}
      </CardContent>
    </Card>
  )
}

interface JobsPanelProps {
  activeTenantId?: string
  sessionEmail: string
}

interface JobListItem {
  id: string
  type: string
  status: string
  apiKeyId?: string
  apiKeyLabel?: string
  url: string
  sync: boolean
  priority: number
  createdAt: string
  expiresAt?: string | null
  updatedAt: string
  completedAt?: string | null
}

interface JobDetailItem extends JobListItem {
  error?: string
  formats?: string[]
}

function JobsPanel({ activeTenantId, sessionEmail }: JobsPanelProps) {
  const [jobs, setJobs] = useState<JobListItem[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [filtersOpen, setFiltersOpen] = useState(false)
  const [deletingJobId, setDeletingJobId] = useState<string | null>(null)
  const [downloadingJobId, setDownloadingJobId] = useState<string | null>(null)

  const [typeFilter, setTypeFilter] = useState("")
  const [statusFilter, setStatusFilter] = useState("")
  const [syncFilter, setSyncFilter] = useState<"" | "true" | "false">("")

  const [createOpen, setCreateOpen] = useState(false)
  const [createType, setCreateType] = useState<"scrape" | "crawl" | "extract" | "map" | "batch">(
    "scrape"
  )
  const [advancedOpen, setAdvancedOpen] = useState(false)
  const [createSubmitting, setCreateSubmitting] = useState(false)
  const [createError, setCreateError] = useState<string | null>(null)
  const [createAbortController, setCreateAbortController] = useState<AbortController | null>(null)

  const [scrapeUrl, setScrapeUrl] = useState("")
  const [scrapeFormats, setScrapeFormats] = useState("markdown")
  const [scrapeTimeoutMs, setScrapeTimeoutMs] = useState("")
  const [scrapeWaitForMs, setScrapeWaitForMs] = useState("")
  const [scrapeUseBrowser, setScrapeUseBrowser] = useState<"" | "true" | "false">("")
  const [scrapeOnlyMainContent, setScrapeOnlyMainContent] = useState<"" | "true" | "false">("")
  const [scrapeSkipTlsVerification, setScrapeSkipTlsVerification] = useState<"" | "true" | "false">("")

  const [mapUrl, setMapUrl] = useState("")
  const [mapSearch, setMapSearch] = useState("")
  const [mapIncludeSubdomains, setMapIncludeSubdomains] = useState<"" | "true" | "false">("")
  const [mapIgnoreQueryParameters, setMapIgnoreQueryParameters] = useState<"" | "true" | "false">("")
  const [mapAllowExternalLinks, setMapAllowExternalLinks] = useState<"" | "true" | "false">("")
  const [mapLimit, setMapLimit] = useState("")
  const [mapTimeoutMs, setMapTimeoutMs] = useState("")

  const [crawlUrl, setCrawlUrl] = useState("")
  const [crawlFormats, setCrawlFormats] = useState("markdown")
  const [crawlLimit, setCrawlLimit] = useState("")
  const [crawlIncludePaths, setCrawlIncludePaths] = useState("")
  const [crawlExcludePaths, setCrawlExcludePaths] = useState("")
  const [crawlAllowSubdomains, setCrawlAllowSubdomains] = useState<"" | "true" | "false">("")
  const [crawlAllowExternalLinks, setCrawlAllowExternalLinks] = useState<"" | "true" | "false">("")
  const [crawlIgnoreRobotsTxt, setCrawlIgnoreRobotsTxt] = useState<"" | "true" | "false">("")
  const [crawlMaxDiscoveryDepth, setCrawlMaxDiscoveryDepth] = useState("")

  const [extractUrls, setExtractUrls] = useState("")
  const [extractSchema, setExtractSchema] = useState("{\n  \n}")
  const [extractPrompt, setExtractPrompt] = useState("")
  const [extractStrict, setExtractStrict] = useState(false)
  const [extractProvider, setExtractProvider] = useState("")
  const [extractModel, setExtractModel] = useState("")
  const [extractEnableWebSearch, setExtractEnableWebSearch] = useState<"" | "true" | "false">("")

  const [batchUrls, setBatchUrls] = useState("")
  const [batchFormats, setBatchFormats] = useState("markdown")

  const [selectedJobId, setSelectedJobId] = useState<string | null>(null)
  const [detailOpen, setDetailOpen] = useState(false)
  const [detailLoading, setDetailLoading] = useState(false)
  const [detail, setDetail] = useState<JobDetailItem | null>(null)
  const [detailError, setDetailError] = useState<string | null>(null)

  const limit = 50

  useEffect(() => {
    // Reset on tenant switches so we never show stale jobs.
    setJobs([])
    setError(null)
    setSelectedJobId(null)
    setDetailOpen(false)
    setDetail(null)
    setDetailError(null)
    setCreateOpen(false)
    setCreateError(null)
    setCreateSubmitting(false)
    setCreateAbortController(null)

    refreshJobs({ reset: true })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeTenantId])

  async function refreshJobs({ reset }: { reset: boolean }) {
    setLoading(true)
    setError(null)

    try {
      const params = new URLSearchParams()
      params.set("limit", String(limit))
      params.set("offset", reset ? "0" : String(jobs.length))

      const trimmedType = typeFilter.trim()
      if (trimmedType) params.set("type", trimmedType)

      const trimmedStatus = statusFilter.trim()
      if (trimmedStatus) params.set("status", trimmedStatus)

      if (syncFilter) params.set("sync", syncFilter)

      const res = await fetch(`/v1/jobs?${params.toString()}`)
      const data = (await res.json()) as {
        success?: boolean
        jobs?: JobListItem[]
        error?: string
      }
      if (!res.ok || !data.success) {
        setError(data.error || "Unable to load jobs")
        return
      }

      const next = data.jobs ?? []
      setJobs((prev) => (reset ? next : [...prev, ...next]))
    } catch {
      setError("Network error while loading jobs")
    } finally {
      setLoading(false)
    }
  }

  async function openJobDetail(jobId: string) {
    setSelectedJobId(jobId)
    setDetailOpen(true)
    setDetail(null)
    setDetailError(null)
    setDetailLoading(true)

    try {
      const res = await fetch(`/v1/jobs/${jobId}`)
      const data = (await res.json()) as {
        success?: boolean
        job?: JobDetailItem
        error?: string
      }
      if (!res.ok || !data.success || !data.job) {
        setDetailError(data.error || "Unable to load job details")
        return
      }
      setDetail(data.job)
    } catch {
      setDetailError("Network error while loading job details")
    } finally {
      setDetailLoading(false)
    }
  }

  function statusBadgeVariant(status: string): "default" | "secondary" | "outline" | "destructive" {
    switch (status) {
      case "pending":
        return "secondary"
      case "running":
        return "default"
      case "completed":
        return "outline"
      case "failed":
        return "destructive"
      default:
        return "secondary"
    }
  }

  function formatActor(job: JobListItem): string {
    if (job.apiKeyId) {
      return job.apiKeyLabel || "Unknown key"
    }
    return `Web UI (${sessionEmail})`
  }

  function filenameFromContentDisposition(headerValue: string | null): string | null {
    if (!headerValue) return null
    const match = headerValue.match(/filename="([^"]+)"/i)
    return match?.[1] ?? null
  }

  async function downloadJob(job: JobListItem) {
    setDownloadingJobId(job.id)
    try {
      const res = await fetch(`/v1/jobs/${job.id}/download`)
      if (!res.ok) {
        const data = (await res.json().catch(() => null)) as { error?: string } | null
        setError(data?.error || "Unable to download job output")
        return
      }

      const filename =
        filenameFromContentDisposition(res.headers.get("content-disposition")) ||
        `job-${job.id}.bin`
      const blob = await res.blob()
      const url = URL.createObjectURL(blob)
      const a = document.createElement("a")
      a.href = url
      a.download = filename
      document.body.appendChild(a)
      a.click()
      document.body.removeChild(a)
      URL.revokeObjectURL(url)
    } catch {
      setError("Network error while downloading job output")
    } finally {
      setDownloadingJobId(null)
    }
  }

  async function deleteJob(job: JobListItem) {
    const confirmed = window.confirm("Delete this job? This cannot be undone.")
    if (!confirmed) return

    setDeletingJobId(job.id)
    setError(null)
    try {
      const res = await fetch(`/v1/jobs/${job.id}`, { method: "DELETE" })
      const data = (await res.json().catch(() => null)) as { success?: boolean; error?: string } | null
      if (!res.ok || !data?.success) {
        setError(data?.error || "Unable to delete job")
        return
      }

      if (selectedJobId === job.id) {
        setDetailOpen(false)
        setSelectedJobId(null)
        setDetail(null)
        setDetailError(null)
      }

      await refreshJobs({ reset: true })
    } catch {
      setError("Network error while deleting job")
    } finally {
      setDeletingJobId(null)
    }
  }

  function resetCreateForm() {
    setCreateType("scrape")
    setAdvancedOpen(false)
    setCreateSubmitting(false)
    setCreateError(null)
    setCreateAbortController(null)
    setScrapeUrl("")
    setScrapeFormats("markdown")
    setScrapeTimeoutMs("")
    setScrapeWaitForMs("")
    setScrapeUseBrowser("")
    setScrapeOnlyMainContent("")
    setScrapeSkipTlsVerification("")

    setMapUrl("")
    setMapSearch("")
    setMapIncludeSubdomains("")
    setMapIgnoreQueryParameters("")
    setMapAllowExternalLinks("")
    setMapLimit("")
    setMapTimeoutMs("")

    setCrawlUrl("")
    setCrawlFormats("markdown")
    setCrawlLimit("")
    setCrawlIncludePaths("")
    setCrawlExcludePaths("")
    setCrawlAllowSubdomains("")
    setCrawlAllowExternalLinks("")
    setCrawlIgnoreRobotsTxt("")
    setCrawlMaxDiscoveryDepth("")

    setExtractUrls("")
    setExtractSchema("{\n  \n}")
    setExtractPrompt("")
    setExtractStrict(false)
    setExtractProvider("")
    setExtractModel("")
    setExtractEnableWebSearch("")

    setBatchUrls("")
    setBatchFormats("markdown")
  }

  function parseOptionalInt(value: string): number | undefined {
    const trimmed = value.trim()
    if (!trimmed) return undefined
    const n = Number.parseInt(trimmed, 10)
    if (Number.isNaN(n)) return undefined
    return n
  }

  function parseOptionalBool(value: "" | "true" | "false"): boolean | undefined {
    if (!value) return undefined
    return value === "true"
  }

  function parseLines(value: string): string[] {
    return value
      .split(/\r?\n/)
      .map((v) => v.trim())
      .filter(Boolean)
  }

  function parseCsv(value: string): string[] {
    return value
      .split(",")
      .map((v) => v.trim())
      .filter(Boolean)
  }

  async function handleCreateJob(event: React.FormEvent) {
    event.preventDefault()
    setCreateError(null)

    let endpoint = ""
    const payload: Record<string, unknown> = {}

    if (createType === "scrape") {
      endpoint = "/v1/scrape"
      const url = scrapeUrl.trim()
      if (!url) {
        setCreateError("URL is required")
        return
      }

      payload.url = url

      const formats = parseCsv(scrapeFormats)
      if (formats.length > 0) payload.formats = formats

      const timeout = parseOptionalInt(scrapeTimeoutMs)
      if (timeout !== undefined) payload.timeout = timeout

      const waitFor = parseOptionalInt(scrapeWaitForMs)
      if (waitFor !== undefined) payload.waitFor = waitFor

      const useBrowser = parseOptionalBool(scrapeUseBrowser)
      if (useBrowser !== undefined) payload.useBrowser = useBrowser

      const onlyMainContent = parseOptionalBool(scrapeOnlyMainContent)
      if (onlyMainContent !== undefined) payload.onlyMainContent = onlyMainContent

      const skipTlsVerification = parseOptionalBool(scrapeSkipTlsVerification)
      if (skipTlsVerification !== undefined) payload.skipTlsVerification = skipTlsVerification
    } else if (createType === "map") {
      endpoint = "/v1/map"
      const url = mapUrl.trim()
      if (!url) {
        setCreateError("URL is required")
        return
      }
      payload.url = url

      const search = mapSearch.trim()
      if (search) payload.search = search

      const includeSubdomains = parseOptionalBool(mapIncludeSubdomains)
      if (includeSubdomains !== undefined) payload.includeSubdomains = includeSubdomains

      const ignoreQueryParameters = parseOptionalBool(mapIgnoreQueryParameters)
      if (ignoreQueryParameters !== undefined) payload.ignoreQueryParameters = ignoreQueryParameters

      const allowExternalLinks = parseOptionalBool(mapAllowExternalLinks)
      if (allowExternalLinks !== undefined) payload.allowExternalLinks = allowExternalLinks

      const limit = parseOptionalInt(mapLimit)
      if (limit !== undefined) payload.limit = limit

      const timeout = parseOptionalInt(mapTimeoutMs)
      if (timeout !== undefined) payload.timeout = timeout
    } else if (createType === "crawl") {
      endpoint = "/v1/crawl"
      const url = crawlUrl.trim()
      if (!url) {
        setCreateError("URL is required")
        return
      }
      payload.url = url

      const formats = parseCsv(crawlFormats)
      if (formats.length > 0) payload.formats = formats

      const limit = parseOptionalInt(crawlLimit)
      if (limit !== undefined) payload.limit = limit

      const includePaths = parseCsv(crawlIncludePaths)
      if (includePaths.length > 0) payload.includePaths = includePaths

      const excludePaths = parseCsv(crawlExcludePaths)
      if (excludePaths.length > 0) payload.excludePaths = excludePaths

      const allowSubdomains = parseOptionalBool(crawlAllowSubdomains)
      if (allowSubdomains !== undefined) payload.allowSubdomains = allowSubdomains

      const allowExternalLinks = parseOptionalBool(crawlAllowExternalLinks)
      if (allowExternalLinks !== undefined) payload.allowExternalLinks = allowExternalLinks

      const ignoreRobotsTxt = parseOptionalBool(crawlIgnoreRobotsTxt)
      if (ignoreRobotsTxt !== undefined) payload.ignoreRobotsTxt = ignoreRobotsTxt

      const maxDiscoveryDepth = parseOptionalInt(crawlMaxDiscoveryDepth)
      if (maxDiscoveryDepth !== undefined) payload.maxDiscoveryDepth = maxDiscoveryDepth
    } else if (createType === "extract") {
      endpoint = "/v1/extract"
      const urls = parseLines(extractUrls)
      if (urls.length === 0) {
        setCreateError("At least one URL is required")
        return
      }

      let schema: unknown
      try {
        schema = JSON.parse(extractSchema)
      } catch {
        setCreateError("Schema must be valid JSON")
        return
      }
      if (!schema || typeof schema !== "object" || Array.isArray(schema)) {
        setCreateError("Schema must be a JSON object")
        return
      }

      payload.urls = urls
      payload.schema = schema
      payload.strict = extractStrict

      const prompt = extractPrompt.trim()
      if (prompt) payload.prompt = prompt

      const provider = extractProvider.trim()
      if (provider) payload.provider = provider

      const model = extractModel.trim()
      if (model) payload.model = model

      const enableWebSearch = parseOptionalBool(extractEnableWebSearch)
      if (enableWebSearch !== undefined) payload.enableWebSearch = enableWebSearch
    } else if (createType === "batch") {
      endpoint = "/v1/batch/scrape"
      const urls = parseLines(batchUrls)
      if (urls.length === 0) {
        setCreateError("At least one URL is required")
        return
      }
      payload.urls = urls

      const formats = parseCsv(batchFormats)
      if (formats.length > 0) payload.formats = formats
    } else {
      setCreateError("Unknown job type")
      return
    }

    const controller = new AbortController()
    setCreateAbortController(controller)
    setCreateSubmitting(true)

    try {
      const res = await fetch(endpoint, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify(payload),
        signal: controller.signal,
      })
      const data = (await res.json()) as { success?: boolean; error?: string }
      if (!res.ok || !data.success) {
        setCreateError(data.error || "Unable to create job")
        return
      }

      setCreateOpen(false)
      resetCreateForm()
      await refreshJobs({ reset: true })
    } catch (err) {
      const e = err as { name?: string }
      if (e?.name === "AbortError") {
        return
      }
      setCreateError("Network error while creating job")
    } finally {
      setCreateSubmitting(false)
      setCreateAbortController(null)
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Jobs</CardTitle>
        <CardDescription>
          Recent map, crawl, scrape, and extract jobs for the active tenant.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {error ? <p className="text-xs text-destructive">{error}</p> : null}

        <div className="flex items-center justify-between gap-2">
          <Button
            variant="outline"
            size="xs"
            type="button"
            onClick={() => setFiltersOpen((v) => !v)}
          >
            {filtersOpen ? "Hide filters" : "Show filters"}
          </Button>
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="xs"
              type="button"
              onClick={() => {
                setDetailOpen(false)
                setCreateOpen(true)
              }}
            >
              Create job
            </Button>
            <Button
              variant="ghost"
              size="xs"
              type="button"
              onClick={() => refreshJobs({ reset: true })}
              disabled={loading}
            >
              {loading ? "Refreshing…" : "Refresh"}
            </Button>
          </div>
        </div>

        <Sheet
          open={createOpen}
          onOpenChange={(open) => {
            setCreateOpen(open)
            if (!open) {
              createAbortController?.abort()
              resetCreateForm()
            }
          }}
        >
          <SheetContent side="right" className="w-full sm:max-w-lg">
            <SheetHeader>
              <SheetTitle>Create job</SheetTitle>
              <SheetDescription>
                Create a new job for the active tenant.
              </SheetDescription>
            </SheetHeader>

            <form onSubmit={handleCreateJob} className="px-4 pb-4 text-xs space-y-4">
              <FieldGroup>
                <Field>
                  <FieldLabel htmlFor="create-job-type">Type</FieldLabel>
                  <Select
                    value={createType}
                    onValueChange={(value) => {
                      setCreateType(value as typeof createType)
                    }}
                  >
                    <SelectTrigger id="create-job-type" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                  <SelectContent align="start">
                      <SelectGroup>
                        <SelectLabel>Job type</SelectLabel>
                        <SelectItem value="scrape">Scrape</SelectItem>
                        <SelectItem value="crawl">Crawl</SelectItem>
                        <SelectItem value="map">Map</SelectItem>
                        <SelectItem value="extract">Extract</SelectItem>
                        <SelectItem value="batch">Batch scrape</SelectItem>
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                </Field>

                {createType === "scrape" ? (
                  <>
                    <Field>
                      <FieldLabel htmlFor="create-scrape-url">URL</FieldLabel>
                      <Input
                        id="create-scrape-url"
                        value={scrapeUrl}
                        onChange={(event) => setScrapeUrl(event.target.value)}
                        placeholder="https://example.com"
                        required
                        disabled={createSubmitting}
                      />
                    </Field>

                    <Field>
                      <FieldLabel htmlFor="create-scrape-formats">Formats</FieldLabel>
                      <Input
                        id="create-scrape-formats"
                        value={scrapeFormats}
                        onChange={(event) => setScrapeFormats(event.target.value)}
                        placeholder="markdown, html, rawHtml"
                        disabled={createSubmitting}
                      />
                      <FieldDescription>
                        Comma-separated; defaults to <code>markdown</code>.
                      </FieldDescription>
                    </Field>
                  </>
                ) : null}

                {createType === "map" ? (
                  <>
                    <Field>
                      <FieldLabel htmlFor="create-map-url">URL</FieldLabel>
                      <Input
                        id="create-map-url"
                        value={mapUrl}
                        onChange={(event) => setMapUrl(event.target.value)}
                        placeholder="https://example.com"
                        required
                        disabled={createSubmitting}
                      />
                    </Field>
                    <Field>
                      <FieldLabel htmlFor="create-map-search">Search (optional)</FieldLabel>
                      <Input
                        id="create-map-search"
                        value={mapSearch}
                        onChange={(event) => setMapSearch(event.target.value)}
                        placeholder="Filter links by text"
                        disabled={createSubmitting}
                      />
                    </Field>
                  </>
                ) : null}

                {createType === "crawl" ? (
                  <>
                    <Field>
                      <FieldLabel htmlFor="create-crawl-url">URL</FieldLabel>
                      <Input
                        id="create-crawl-url"
                        value={crawlUrl}
                        onChange={(event) => setCrawlUrl(event.target.value)}
                        placeholder="https://example.com"
                        required
                        disabled={createSubmitting}
                      />
                    </Field>
                    <Field>
                      <FieldLabel htmlFor="create-crawl-formats">Formats</FieldLabel>
                      <Input
                        id="create-crawl-formats"
                        value={crawlFormats}
                        onChange={(event) => setCrawlFormats(event.target.value)}
                        placeholder="markdown, html, rawHtml"
                        disabled={createSubmitting}
                      />
                      <FieldDescription>
                        Comma-separated; defaults to <code>markdown</code>.
                      </FieldDescription>
                    </Field>
                  </>
                ) : null}

                {createType === "extract" ? (
                  <>
                    <Field>
                      <FieldLabel htmlFor="create-extract-urls">
                        URLs (one per line)
                      </FieldLabel>
                      <Textarea
                        id="create-extract-urls"
                        value={extractUrls}
                        onChange={(event) => setExtractUrls(event.target.value)}
                        placeholder={"https://example.com/page-1\nhttps://example.com/page-2"}
                        required
                        disabled={createSubmitting}
                      />
                    </Field>
                    <Field>
                      <FieldLabel htmlFor="create-extract-schema">
                        Schema (JSON)
                      </FieldLabel>
                      <Textarea
                        id="create-extract-schema"
                        value={extractSchema}
                        onChange={(event) => setExtractSchema(event.target.value)}
                        className="min-h-32 font-mono text-[11px]"
                        placeholder={'{\n  "title": "string"\n}'}
                        required
                        disabled={createSubmitting}
                      />
                      <FieldDescription>
                        A JSON object describing the data you want extracted.
                      </FieldDescription>
                    </Field>
                    <Field orientation="horizontal" className="items-center justify-between">
                      <FieldLabel htmlFor="create-extract-strict">Strict</FieldLabel>
                      <Switch
                        id="create-extract-strict"
                        checked={extractStrict}
                        onCheckedChange={(checked) => setExtractStrict(Boolean(checked))}
                        disabled={createSubmitting}
                      />
                    </Field>
                  </>
                ) : null}

                {createType === "batch" ? (
                  <>
                    <Field>
                      <FieldLabel htmlFor="create-batch-urls">
                        URLs (one per line)
                      </FieldLabel>
                      <Textarea
                        id="create-batch-urls"
                        value={batchUrls}
                        onChange={(event) => setBatchUrls(event.target.value)}
                        placeholder={"https://example.com/page-1\nhttps://example.com/page-2"}
                        required
                        disabled={createSubmitting}
                      />
                    </Field>
                    <Field>
                      <FieldLabel htmlFor="create-batch-formats">Formats</FieldLabel>
                      <Input
                        id="create-batch-formats"
                        value={batchFormats}
                        onChange={(event) => setBatchFormats(event.target.value)}
                        placeholder="markdown, html, rawHtml"
                        disabled={createSubmitting}
                      />
                      <FieldDescription>
                        Comma-separated; defaults to <code>markdown</code>.
                      </FieldDescription>
                    </Field>
                  </>
                ) : null}

                <Field orientation="horizontal">
                  <Button
                    variant="outline"
                    size="xs"
                    type="button"
                    onClick={() => setAdvancedOpen((v) => !v)}
                    disabled={createSubmitting}
                  >
                    {advancedOpen ? "Hide advanced" : "Advanced options"}
                  </Button>
                </Field>

                {advancedOpen ? (
                  <div className="grid grid-cols-2 gap-4">
                    {createType === "scrape" ? (
                      <>
                        <Field>
                          <FieldLabel htmlFor="create-scrape-timeout">
                            Timeout (ms)
                          </FieldLabel>
                          <Input
                            id="create-scrape-timeout"
                            type="number"
                            min={1}
                            value={scrapeTimeoutMs}
                            onChange={(event) => setScrapeTimeoutMs(event.target.value)}
                            placeholder="e.g. 30000"
                            disabled={createSubmitting}
                          />
                        </Field>
                        <Field>
                          <FieldLabel htmlFor="create-scrape-waitfor">
                            Wait for (ms)
                          </FieldLabel>
                          <Input
                            id="create-scrape-waitfor"
                            type="number"
                            min={0}
                            value={scrapeWaitForMs}
                            onChange={(event) => setScrapeWaitForMs(event.target.value)}
                            placeholder="e.g. 1000"
                            disabled={createSubmitting}
                          />
                        </Field>
                        <Field>
                          <FieldLabel htmlFor="create-scrape-use-browser">
                            Use browser
                          </FieldLabel>
                          <Select
                            value={scrapeUseBrowser}
                            onValueChange={(value) => {
                              setScrapeUseBrowser(value as "" | "true" | "false")
                            }}
                          >
                            <SelectTrigger id="create-scrape-use-browser" className="w-full">
                              {scrapeUseBrowser ? (
                                <SelectValue />
                              ) : (
                                <span className="text-muted-foreground">Default</span>
                              )}
                            </SelectTrigger>
                            <SelectContent align="start">
                              <SelectGroup>
                                <SelectItem value="">Default</SelectItem>
                                <SelectItem value="true">Yes</SelectItem>
                                <SelectItem value="false">No</SelectItem>
                              </SelectGroup>
                            </SelectContent>
                          </Select>
                        </Field>
                        <Field>
                          <FieldLabel htmlFor="create-scrape-only-main">
                            Only main content
                          </FieldLabel>
                          <Select
                            value={scrapeOnlyMainContent}
                            onValueChange={(value) => {
                              setScrapeOnlyMainContent(value as "" | "true" | "false")
                            }}
                          >
                            <SelectTrigger id="create-scrape-only-main" className="w-full">
                              {scrapeOnlyMainContent ? (
                                <SelectValue />
                              ) : (
                                <span className="text-muted-foreground">Default</span>
                              )}
                            </SelectTrigger>
                            <SelectContent align="start">
                              <SelectGroup>
                                <SelectItem value="">Default</SelectItem>
                                <SelectItem value="true">Yes</SelectItem>
                                <SelectItem value="false">No</SelectItem>
                              </SelectGroup>
                            </SelectContent>
                          </Select>
                        </Field>
                        <Field>
                          <FieldLabel htmlFor="create-scrape-skip-tls">
                            Skip TLS verification
                          </FieldLabel>
                          <Select
                            value={scrapeSkipTlsVerification}
                            onValueChange={(value) => {
                              setScrapeSkipTlsVerification(value as "" | "true" | "false")
                            }}
                          >
                            <SelectTrigger id="create-scrape-skip-tls" className="w-full">
                              {scrapeSkipTlsVerification ? (
                                <SelectValue />
                              ) : (
                                <span className="text-muted-foreground">Default</span>
                              )}
                            </SelectTrigger>
                            <SelectContent align="start">
                              <SelectGroup>
                                <SelectItem value="">Default</SelectItem>
                                <SelectItem value="true">Yes</SelectItem>
                                <SelectItem value="false">No</SelectItem>
                              </SelectGroup>
                            </SelectContent>
                          </Select>
                        </Field>
                      </>
                    ) : null}

                    {createType === "map" ? (
                      <>
                        <Field>
                          <FieldLabel htmlFor="create-map-limit">Limit</FieldLabel>
                          <Input
                            id="create-map-limit"
                            type="number"
                            min={1}
                            value={mapLimit}
                            onChange={(event) => setMapLimit(event.target.value)}
                            placeholder="e.g. 100"
                            disabled={createSubmitting}
                          />
                        </Field>
                        <Field>
                          <FieldLabel htmlFor="create-map-timeout">
                            Timeout (ms)
                          </FieldLabel>
                          <Input
                            id="create-map-timeout"
                            type="number"
                            min={1}
                            value={mapTimeoutMs}
                            onChange={(event) => setMapTimeoutMs(event.target.value)}
                            placeholder="e.g. 30000"
                            disabled={createSubmitting}
                          />
                        </Field>
                        <Field>
                          <FieldLabel htmlFor="create-map-include-subdomains">
                            Include subdomains
                          </FieldLabel>
                          <Select
                            value={mapIncludeSubdomains}
                            onValueChange={(value) => {
                              setMapIncludeSubdomains(value as "" | "true" | "false")
                            }}
                          >
                            <SelectTrigger id="create-map-include-subdomains" className="w-full">
                              {mapIncludeSubdomains ? (
                                <SelectValue />
                              ) : (
                                <span className="text-muted-foreground">Default</span>
                              )}
                            </SelectTrigger>
                            <SelectContent align="start">
                              <SelectGroup>
                                <SelectItem value="">Default</SelectItem>
                                <SelectItem value="true">Yes</SelectItem>
                                <SelectItem value="false">No</SelectItem>
                              </SelectGroup>
                            </SelectContent>
                          </Select>
                        </Field>
                        <Field>
                          <FieldLabel htmlFor="create-map-ignore-query">
                            Ignore query params
                          </FieldLabel>
                          <Select
                            value={mapIgnoreQueryParameters}
                            onValueChange={(value) => {
                              setMapIgnoreQueryParameters(value as "" | "true" | "false")
                            }}
                          >
                            <SelectTrigger id="create-map-ignore-query" className="w-full">
                              {mapIgnoreQueryParameters ? (
                                <SelectValue />
                              ) : (
                                <span className="text-muted-foreground">Default</span>
                              )}
                            </SelectTrigger>
                            <SelectContent align="start">
                              <SelectGroup>
                                <SelectItem value="">Default</SelectItem>
                                <SelectItem value="true">Yes</SelectItem>
                                <SelectItem value="false">No</SelectItem>
                              </SelectGroup>
                            </SelectContent>
                          </Select>
                        </Field>
                        <Field>
                          <FieldLabel htmlFor="create-map-allow-external">
                            Allow external links
                          </FieldLabel>
                          <Select
                            value={mapAllowExternalLinks}
                            onValueChange={(value) => {
                              setMapAllowExternalLinks(value as "" | "true" | "false")
                            }}
                          >
                            <SelectTrigger id="create-map-allow-external" className="w-full">
                              {mapAllowExternalLinks ? (
                                <SelectValue />
                              ) : (
                                <span className="text-muted-foreground">Default</span>
                              )}
                            </SelectTrigger>
                            <SelectContent align="start">
                              <SelectGroup>
                                <SelectItem value="">Default</SelectItem>
                                <SelectItem value="true">Yes</SelectItem>
                                <SelectItem value="false">No</SelectItem>
                              </SelectGroup>
                            </SelectContent>
                          </Select>
                        </Field>
                      </>
                    ) : null}

                    {createType === "crawl" ? (
                      <>
                        <Field>
                          <FieldLabel htmlFor="create-crawl-limit">Limit</FieldLabel>
                          <Input
                            id="create-crawl-limit"
                            type="number"
                            min={1}
                            value={crawlLimit}
                            onChange={(event) => setCrawlLimit(event.target.value)}
                            placeholder="e.g. 100"
                            disabled={createSubmitting}
                          />
                        </Field>
                        <Field>
                          <FieldLabel htmlFor="create-crawl-max-depth">
                            Max discovery depth
                          </FieldLabel>
                          <Input
                            id="create-crawl-max-depth"
                            type="number"
                            min={0}
                            value={crawlMaxDiscoveryDepth}
                            onChange={(event) => setCrawlMaxDiscoveryDepth(event.target.value)}
                            placeholder="e.g. 3"
                            disabled={createSubmitting}
                          />
                        </Field>
                        <Field className="col-span-2">
                          <FieldLabel htmlFor="create-crawl-include-paths">
                            Include paths (comma-separated)
                          </FieldLabel>
                          <Input
                            id="create-crawl-include-paths"
                            value={crawlIncludePaths}
                            onChange={(event) => setCrawlIncludePaths(event.target.value)}
                            placeholder="/docs, /blog"
                            disabled={createSubmitting}
                          />
                        </Field>
                        <Field className="col-span-2">
                          <FieldLabel htmlFor="create-crawl-exclude-paths">
                            Exclude paths (comma-separated)
                          </FieldLabel>
                          <Input
                            id="create-crawl-exclude-paths"
                            value={crawlExcludePaths}
                            onChange={(event) => setCrawlExcludePaths(event.target.value)}
                            placeholder="/login, /admin"
                            disabled={createSubmitting}
                          />
                        </Field>
                        <Field>
                          <FieldLabel htmlFor="create-crawl-allow-subdomains">
                            Allow subdomains
                          </FieldLabel>
                          <Select
                            value={crawlAllowSubdomains}
                            onValueChange={(value) => {
                              setCrawlAllowSubdomains(value as "" | "true" | "false")
                            }}
                          >
                            <SelectTrigger id="create-crawl-allow-subdomains" className="w-full">
                              {crawlAllowSubdomains ? (
                                <SelectValue />
                              ) : (
                                <span className="text-muted-foreground">Default</span>
                              )}
                            </SelectTrigger>
                            <SelectContent align="start">
                              <SelectGroup>
                                <SelectItem value="">Default</SelectItem>
                                <SelectItem value="true">Yes</SelectItem>
                                <SelectItem value="false">No</SelectItem>
                              </SelectGroup>
                            </SelectContent>
                          </Select>
                        </Field>
                        <Field>
                          <FieldLabel htmlFor="create-crawl-allow-external">
                            Allow external links
                          </FieldLabel>
                          <Select
                            value={crawlAllowExternalLinks}
                            onValueChange={(value) => {
                              setCrawlAllowExternalLinks(value as "" | "true" | "false")
                            }}
                          >
                            <SelectTrigger id="create-crawl-allow-external" className="w-full">
                              {crawlAllowExternalLinks ? (
                                <SelectValue />
                              ) : (
                                <span className="text-muted-foreground">Default</span>
                              )}
                            </SelectTrigger>
                            <SelectContent align="start">
                              <SelectGroup>
                                <SelectItem value="">Default</SelectItem>
                                <SelectItem value="true">Yes</SelectItem>
                                <SelectItem value="false">No</SelectItem>
                              </SelectGroup>
                            </SelectContent>
                          </Select>
                        </Field>
                        <Field>
                          <FieldLabel htmlFor="create-crawl-ignore-robots">
                            Ignore robots.txt
                          </FieldLabel>
                          <Select
                            value={crawlIgnoreRobotsTxt}
                            onValueChange={(value) => {
                              setCrawlIgnoreRobotsTxt(value as "" | "true" | "false")
                            }}
                          >
                            <SelectTrigger id="create-crawl-ignore-robots" className="w-full">
                              {crawlIgnoreRobotsTxt ? (
                                <SelectValue />
                              ) : (
                                <span className="text-muted-foreground">Default</span>
                              )}
                            </SelectTrigger>
                            <SelectContent align="start">
                              <SelectGroup>
                                <SelectItem value="">Default</SelectItem>
                                <SelectItem value="true">Yes</SelectItem>
                                <SelectItem value="false">No</SelectItem>
                              </SelectGroup>
                            </SelectContent>
                          </Select>
                        </Field>
                      </>
                    ) : null}

                    {createType === "extract" ? (
                      <>
                        <Field className="col-span-2">
                          <FieldLabel htmlFor="create-extract-prompt">
                            Prompt (optional)
                          </FieldLabel>
                          <Textarea
                            id="create-extract-prompt"
                            value={extractPrompt}
                            onChange={(event) => setExtractPrompt(event.target.value)}
                            placeholder="Any extra extraction guidance…"
                            disabled={createSubmitting}
                          />
                        </Field>
                        <Field>
                          <FieldLabel htmlFor="create-extract-provider">
                            Provider (optional)
                          </FieldLabel>
                          <Input
                            id="create-extract-provider"
                            value={extractProvider}
                            onChange={(event) => setExtractProvider(event.target.value)}
                            placeholder="openai, anthropic, google"
                            disabled={createSubmitting}
                          />
                        </Field>
                        <Field>
                          <FieldLabel htmlFor="create-extract-model">
                            Model (optional)
                          </FieldLabel>
                          <Input
                            id="create-extract-model"
                            value={extractModel}
                            onChange={(event) => setExtractModel(event.target.value)}
                            placeholder="e.g. gpt-4.1-mini"
                            disabled={createSubmitting}
                          />
                        </Field>
                        <Field>
                          <FieldLabel htmlFor="create-extract-web-search">
                            Enable web search
                          </FieldLabel>
                          <Select
                            value={extractEnableWebSearch}
                            onValueChange={(value) => {
                              setExtractEnableWebSearch(value as "" | "true" | "false")
                            }}
                          >
                            <SelectTrigger id="create-extract-web-search" className="w-full">
                              {extractEnableWebSearch ? (
                                <SelectValue />
                              ) : (
                                <span className="text-muted-foreground">Default</span>
                              )}
                            </SelectTrigger>
                            <SelectContent align="start">
                              <SelectGroup>
                                <SelectItem value="">Default</SelectItem>
                                <SelectItem value="true">Yes</SelectItem>
                                <SelectItem value="false">No</SelectItem>
                              </SelectGroup>
                            </SelectContent>
                          </Select>
                        </Field>
                      </>
                    ) : null}

                    {createType === "batch" ? null : null}
                  </div>
                ) : null}

                {createError ? <FieldError>{createError}</FieldError> : null}
              </FieldGroup>

              <SheetFooter className="p-0 flex-row justify-end">
                <Button
                  variant="ghost"
                  size="xs"
                  type="button"
                  onClick={() => {
                    createAbortController?.abort()
                    setCreateOpen(false)
                    resetCreateForm()
                  }}
                  disabled={createSubmitting}
                >
                  Cancel
                </Button>
                <Button size="xs" type="submit" disabled={createSubmitting}>
                  {createSubmitting ? "Creating…" : "Create"}
                </Button>
              </SheetFooter>

              {createSubmitting ? (
                <div className="text-muted-foreground text-[11px]">
                  This runs via <code>/v1/scrape</code> and may take a bit for large pages.
                </div>
              ) : null}
            </form>
          </SheetContent>
        </Sheet>

        {filtersOpen ? (
          <div className="border-border rounded-none border px-3 py-2">
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor="jobs-type">Type (optional)</FieldLabel>
                <Input
                  id="jobs-type"
                  value={typeFilter}
                  onChange={(event) => setTypeFilter(event.target.value)}
                  placeholder="e.g. crawl, scrape, extract"
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="jobs-status">Status (optional)</FieldLabel>
                <Input
                  id="jobs-status"
                  value={statusFilter}
                  onChange={(event) => setStatusFilter(event.target.value)}
                  placeholder="pending, running, completed, failed"
                />
                <FieldDescription>
                  Leave blank to see all statuses.
                </FieldDescription>
              </Field>
              <Field>
                <FieldLabel htmlFor="jobs-sync">Sync (optional)</FieldLabel>
                <Input
                  id="jobs-sync"
                  value={syncFilter}
                  onChange={(event) => {
                    const v = event.target.value.trim()
                    if (v === "" || v === "true" || v === "false") {
                      setSyncFilter(v as "" | "true" | "false")
                    }
                  }}
                  placeholder="true / false"
                />
              </Field>
              <Field orientation="horizontal">
                <Button
                  size="xs"
                  type="button"
                  onClick={() => refreshJobs({ reset: true })}
                  disabled={loading}
                >
                  Apply
                </Button>
                <Button
                  variant="ghost"
                  size="xs"
                  type="button"
                  onClick={() => {
                    setTypeFilter("")
                    setStatusFilter("")
                    setSyncFilter("")
                    refreshJobs({ reset: true })
                  }}
                  disabled={loading}
                >
                  Clear
                </Button>
              </Field>
            </FieldGroup>
          </div>
        ) : null}

        <div>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Status</TableHead>
                <TableHead>Type</TableHead>
                <TableHead>API key</TableHead>
                <TableHead>URL</TableHead>
                <TableHead>Created</TableHead>
                <TableHead>Expires</TableHead>
                <TableHead className="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {jobs.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={7} className="text-xs text-muted-foreground">
                    {loading ? "Loading jobs…" : "No jobs found."}
                  </TableCell>
                </TableRow>
              ) : (
                jobs.map((job) => (
                  <TableRow key={job.id}>
                    <TableCell>
                      <Badge variant={statusBadgeVariant(job.status)}>
                        {job.status}
                      </Badge>
                    </TableCell>
                    <TableCell className="font-mono text-[11px]">
                      {job.type}
                    </TableCell>
                    <TableCell className="text-xs">
                      <span className="truncate block max-w-[140px]">
                        {formatActor(job)}
                      </span>
                    </TableCell>
                    <TableCell className="max-w-[460px] truncate text-xs">
                      {job.url}
                    </TableCell>
                    <TableCell className="text-xs">
                      {new Date(job.createdAt).toLocaleString()}
                    </TableCell>
                    <TableCell className="text-xs">
                      {job.expiresAt ? (
                        new Date(job.expiresAt).toLocaleString()
                      ) : (
                        <span className="text-muted-foreground">—</span>
                      )}
                    </TableCell>
                    <TableCell className="text-right">
                      <DropdownMenu>
                        <DropdownMenuTrigger
                          render={
                            <Button
                              variant="ghost"
                              size="xs"
                              type="button"
                              aria-label="Job actions"
                              disabled={deletingJobId === job.id}
                            >
                              ⋯
                            </Button>
                          }
                        />
                        <DropdownMenuContent align="end" className="min-w-44">
                          <DropdownMenuGroup>
                            <DropdownMenuItem
                              onClick={() => openJobDetail(job.id)}
                            >
                              Details
                            </DropdownMenuItem>
                            <DropdownMenuItem
                              disabled={job.status !== "completed" || downloadingJobId === job.id}
                              onClick={() => downloadJob(job)}
                            >
                              {downloadingJobId === job.id ? "Downloading…" : "Download"}
                            </DropdownMenuItem>
                          </DropdownMenuGroup>
                          <DropdownMenuSeparator />
                          <DropdownMenuGroup>
                            <DropdownMenuItem
                              variant="destructive"
                              disabled={deletingJobId === job.id}
                              onClick={() => deleteJob(job)}
                            >
                              {deletingJobId === job.id ? "Deleting…" : "Delete"}
                            </DropdownMenuItem>
                          </DropdownMenuGroup>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </div>

        {jobs.length > 0 ? (
          <div className="flex justify-center">
            <Button
              variant="outline"
              size="xs"
              type="button"
              onClick={() => refreshJobs({ reset: false })}
              disabled={loading || jobs.length < limit}
            >
              {jobs.length < limit ? "No more jobs" : loading ? "Loading…" : "Load more"}
            </Button>
          </div>
        ) : null}
      </CardContent>

      <Sheet
        open={detailOpen}
        onOpenChange={(open) => {
          setDetailOpen(open)
          if (!open) {
            setSelectedJobId(null)
            setDetail(null)
            setDetailError(null)
          }
        }}
      >
        <SheetContent side="right" className="w-full sm:max-w-lg">
          <SheetHeader>
            <SheetTitle>Job details</SheetTitle>
            <SheetDescription>
              {selectedJobId ? `Job ID: ${selectedJobId}` : "Loading job…"}
            </SheetDescription>
          </SheetHeader>

          <div className="px-4 pb-4 text-xs space-y-3">
            {detailLoading ? (
              <p className="text-muted-foreground">Loading…</p>
            ) : detailError ? (
              <p className="text-destructive">{detailError}</p>
            ) : detail ? (
              <>
                <div className="flex items-center gap-2">
                  <Badge variant={statusBadgeVariant(detail.status)}>
                    {detail.status}
                  </Badge>
                  <Badge variant="secondary">{detail.type}</Badge>
                  {detail.sync ? <Badge variant="outline">sync</Badge> : null}
                </div>

                <div className="space-y-1">
                  <div className="text-muted-foreground">API key</div>
                  <div className="break-words">
                    {detail.apiKeyId
                      ? detail.apiKeyLabel || "Unknown key"
                      : `Web UI (${sessionEmail})`}
                  </div>
                </div>

                <div className="space-y-1">
                  <div className="text-muted-foreground">URL</div>
                  <div className="break-all font-mono text-[11px]">{detail.url}</div>
                </div>

                <div className="space-y-1">
                  <div className="text-muted-foreground">Formats</div>
                  {detail.formats && detail.formats.length > 0 ? (
                    <div className="flex flex-wrap gap-1">
                      {detail.formats.map((format) => (
                        <Badge key={format} variant="secondary">
                          {format}
                        </Badge>
                      ))}
                    </div>
                  ) : (
                    <div className="text-muted-foreground">—</div>
                  )}
                </div>

                <div className="grid grid-cols-2 gap-3">
                  <div className="space-y-1">
                    <div className="text-muted-foreground">Created</div>
                    <div>{new Date(detail.createdAt).toLocaleString()}</div>
                  </div>
                  <div className="space-y-1">
                    <div className="text-muted-foreground">Expires</div>
                    <div>
                      {detail.expiresAt
                        ? new Date(detail.expiresAt).toLocaleString()
                        : "—"}
                    </div>
                  </div>
                  <div className="space-y-1">
                    <div className="text-muted-foreground">Updated</div>
                    <div>{new Date(detail.updatedAt).toLocaleString()}</div>
                  </div>
                  <div className="space-y-1">
                    <div className="text-muted-foreground">Completed</div>
                    <div>
                      {detail.completedAt
                        ? new Date(detail.completedAt).toLocaleString()
                        : "—"}
                    </div>
                  </div>
                  <div className="space-y-1">
                    <div className="text-muted-foreground">Priority</div>
                    <div>{detail.priority}</div>
                  </div>
                </div>

                {detail.error ? (
                  <div className="space-y-1">
                    <div className="text-muted-foreground">Error</div>
                    <div className="text-destructive whitespace-pre-wrap break-words">
                      {detail.error}
                    </div>
                  </div>
                ) : null}
              </>
            ) : (
              <p className="text-muted-foreground">No job selected.</p>
            )}
          </div>
        </SheetContent>
      </Sheet>
    </Card>
  )
}
