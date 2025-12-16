"use client"

import { useEffect, useMemo, useState } from "react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { ChartContainer, ChartTooltip, ChartTooltipContent } from "@/components/ui/chart"
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Bar, BarChart, CartesianGrid, Cell, XAxis, YAxis } from "recharts"

type UsageWindow = "24h" | "7d" | "30d"

type UsageScope = "all" | "tenant" | "user"

type AdminTenantItem = {
  id: string
  slug: string
  name: string
  type: string
}

type AdminUserItem = {
  id: string
  email: string
  name?: string
}

type AdminUsageResponse = {
  success?: boolean
  scopeType?: string
  scopeTenantId?: string
  scopeUserId?: string
  jobs: number
  documents: number
  users: number
  tenants: number
  tenantsByType?: Record<string, number>
  jobsByType?: Record<string, number>
  documentsByType?: Record<string, number>
  error?: string
}

function toChartRows(map?: Record<string, number>): Array<{ key: string; value: number }> {
  if (!map) return []
  return Object.entries(map)
    .map(([key, value]) => ({ key, value: Number(value) || 0 }))
    .sort((a, b) => b.value - a.value)
}

function colorForKey(key: string) {
  switch (key) {
    case "scrape":
      return "var(--chart-1)"
    case "crawl":
      return "var(--chart-2)"
    case "batch_scrape":
    case "batch":
      return "var(--chart-3)"
    case "extract":
      return "var(--chart-4)"
    case "map":
      return "var(--chart-5)"
    default:
      return "var(--muted-foreground)"
  }
}

function formatWindowLabel(window: UsageWindow) {
  switch (window) {
    case "24h":
      return "Last 24 hours"
    case "7d":
      return "Last 7 days"
    case "30d":
      return "Last 30 days"
  }
}

export function AdminUsagePanel() {
  const [window, setWindow] = useState<UsageWindow>("7d")
  const [scope, setScope] = useState<UsageScope>("all")
  const [selectedTenantId, setSelectedTenantId] = useState<string>("")
  const [selectedUserId, setSelectedUserId] = useState<string>("")

  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [usage, setUsage] = useState<AdminUsageResponse | null>(null)

  const [tenants, setTenants] = useState<AdminTenantItem[]>([])
  const [users, setUsers] = useState<AdminUserItem[]>([])

  useEffect(() => {
    let cancelled = false

    async function loadOptions() {
      try {
        const [tenantsRes, usersRes] = await Promise.all([
          fetch("/admin/tenants?includePersonal=true&limit=500&offset=0"),
          fetch("/admin/users?limit=500&offset=0"),
        ])
        const tenantsData = (await tenantsRes.json()) as {
          success?: boolean
          tenants?: AdminTenantItem[]
        }
        const usersData = (await usersRes.json()) as { success?: boolean; users?: AdminUserItem[] }
        if (!cancelled) {
          if (tenantsRes.ok && tenantsData.success) {
            setTenants((tenantsData.tenants ?? []).filter((t) => t.type !== "personal"))
          }
          if (usersRes.ok && usersData.success) {
            setUsers(usersData.users ?? [])
          }
        }
      } catch {
        // ignore: filters are optional
      }
    }

    loadOptions()
    return () => {
      cancelled = true
    }
  }, [])

  const selectedTenant = useMemo(
    () => tenants.find((t) => t.id === selectedTenantId) ?? null,
    [selectedTenantId, tenants]
  )

  const selectedUser = useMemo(
    () => users.find((u) => u.id === selectedUserId) ?? null,
    [selectedUserId, users]
  )

  useEffect(() => {
    if (scope === "tenant") {
      setSelectedUserId("")
    }
    if (scope === "user") {
      setSelectedTenantId("")
    }
  }, [scope])

  useEffect(() => {
    if (scope === "tenant" && selectedTenantId && !selectedTenant) {
      setSelectedTenantId("")
    }
  }, [scope, selectedTenant, selectedTenantId])

  useEffect(() => {
    let cancelled = false

    async function load() {
      setLoading(true)
      setError(null)
      try {
        const params = new URLSearchParams()
        params.set("window", window)
        if (scope === "tenant" && selectedTenantId) {
          params.set("tenantId", selectedTenantId)
        }
        if (scope === "user" && selectedUserId) {
          params.set("userId", selectedUserId)
        }

        const res = await fetch(`/admin/usage?${params.toString()}`)
        const data = (await res.json()) as AdminUsageResponse
        if (!res.ok || !data.success) {
          if (!cancelled) {
            setError(data.error || "Unable to load usage")
          }
          return
        }
        if (!cancelled) {
          setUsage(data)
        }
      } catch {
        if (!cancelled) {
          setError("Network error while loading usage")
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
  }, [scope, selectedTenantId, selectedUserId, window])

  const jobsByType = useMemo(() => toChartRows(usage?.jobsByType), [usage?.jobsByType])
  const documentsByType = useMemo(() => toChartRows(usage?.documentsByType), [usage?.documentsByType])

  const tenantSummary = useMemo(() => {
    const personal = usage?.tenantsByType?.personal ?? 0
    const org = usage?.tenantsByType?.org ?? 0
    const other = Object.entries(usage?.tenantsByType ?? {})
      .filter(([k]) => k !== "personal" && k !== "org")
      .reduce((sum, [, v]) => sum + (Number(v) || 0), 0)
    return { personal, org, other }
  }, [usage?.tenantsByType])

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <Badge variant="secondary">Usage</Badge>
          <span className="text-xs text-muted-foreground">{formatWindowLabel(window)}</span>
        </div>
        <div className="flex items-center gap-2">
          <Select value={scope} onValueChange={(v) => setScope(v as UsageScope)}>
            <SelectTrigger className="w-[160px]" aria-label="Usage scope">
              <SelectValue />
            </SelectTrigger>
            <SelectContent align="end">
              <SelectGroup>
                <SelectItem value="all">All tenants</SelectItem>
                <SelectItem value="tenant">Tenant</SelectItem>
                <SelectItem value="user">User</SelectItem>
              </SelectGroup>
            </SelectContent>
          </Select>

          {scope === "tenant" ? (
            <Select value={selectedTenantId} onValueChange={(v) => setSelectedTenantId(v ?? "")}>
              <SelectTrigger className="w-[260px]" aria-label="Select tenant">
                <span
                  className={[
                    "min-w-0 flex flex-1 text-left truncate",
                    selectedTenantId ? "" : "text-muted-foreground",
                  ].join(" ")}
                  title={selectedTenant ? selectedTenant.name : undefined}
                >
                  {selectedTenant ? selectedTenant.name : "Select tenant"}
                </span>
              </SelectTrigger>
              <SelectContent align="end" className="max-h-[320px]">
                <SelectGroup>
                  {tenants.map((t) => (
                    <SelectItem key={t.id} value={t.id} title={t.name}>
                      {t.name}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          ) : null}

          {scope === "user" ? (
            <Select value={selectedUserId} onValueChange={(v) => setSelectedUserId(v ?? "")}>
              <SelectTrigger className="w-[260px]" aria-label="Select user">
                <span
                  className={[
                    "min-w-0 flex flex-1 text-left truncate",
                    selectedUserId ? "" : "text-muted-foreground",
                  ].join(" ")}
                  title={
                    selectedUser
                      ? selectedUser.name?.trim()
                        ? `${selectedUser.name} (${selectedUser.email})`
                        : selectedUser.email
                      : undefined
                  }
                >
                  {selectedUser
                    ? selectedUser.name?.trim()
                      ? `${selectedUser.name} (${selectedUser.email})`
                      : selectedUser.email
                    : "Select user"}
                </span>
              </SelectTrigger>
              <SelectContent align="end" className="max-h-[320px]">
                <SelectGroup>
                  {users.map((u) => (
                    <SelectItem
                      key={u.id}
                      value={u.id}
                      title={
                        u.name?.trim() ? `${u.name} (${u.email})` : u.email
                      }
                    >
                      {u.name?.trim() ? `${u.name} (${u.email})` : u.email}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          ) : null}

          <Select value={window} onValueChange={(v) => setWindow(v as UsageWindow)}>
            <SelectTrigger className="w-[160px]" aria-label="Usage window">
              <SelectValue />
            </SelectTrigger>
            <SelectContent align="end">
              <SelectGroup>
                <SelectItem value="24h">Last 24 hours</SelectItem>
                <SelectItem value="7d">Last 7 days</SelectItem>
                <SelectItem value="30d">Last 30 days</SelectItem>
              </SelectGroup>
            </SelectContent>
          </Select>
          <Button
            variant="ghost"
            size="xs"
            type="button"
            onClick={() => setWindow((w) => w)}
            disabled={loading}
          >
            {loading ? "Refreshing…" : "Refresh"}
          </Button>
        </div>
      </div>

      {error ? <p className="text-xs text-destructive">{error}</p> : null}

      <div className={scope === "all" ? "grid grid-cols-1 gap-4 lg:grid-cols-4" : "grid grid-cols-1 gap-4 md:grid-cols-3"}>
        <Card>
          <CardHeader>
            <CardTitle>Jobs</CardTitle>
            <CardDescription>Total jobs created.</CardDescription>
          </CardHeader>
          <CardContent className="text-2xl font-semibold tabular-nums">{usage?.jobs ?? 0}</CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>Documents</CardTitle>
            <CardDescription>Results stored from jobs.</CardDescription>
          </CardHeader>
          <CardContent className="text-2xl font-semibold tabular-nums">{usage?.documents ?? 0}</CardContent>
        </Card>

        {scope === "all" ? (
          <>
            <Card>
              <CardHeader>
                <CardTitle>Users</CardTitle>
                <CardDescription>Accounts in this instance.</CardDescription>
              </CardHeader>
              <CardContent className="text-2xl font-semibold tabular-nums">{usage?.users ?? 0}</CardContent>
            </Card>
            <Card>
              <CardHeader>
                <CardTitle>Tenants</CardTitle>
                <CardDescription>Workspaces (personal + org).</CardDescription>
              </CardHeader>
              <CardContent className="space-y-1">
                <div className="text-2xl font-semibold tabular-nums">{usage?.tenants ?? 0}</div>
                <div className="text-xs text-muted-foreground">
                  {tenantSummary.org} org, {tenantSummary.personal} personal
                  {tenantSummary.other ? `, ${tenantSummary.other} other` : ""}
                </div>
              </CardContent>
            </Card>
          </>
        ) : scope === "tenant" ? (
          <Card>
            <CardHeader>
              <CardTitle>Tenant</CardTitle>
              <CardDescription>Scoped to selected tenant.</CardDescription>
            </CardHeader>
            <CardContent className="text-xs space-y-1">
              <div>{selectedTenant ? `${selectedTenant.name} (${selectedTenant.type})` : "—"}</div>
              <div className="text-muted-foreground break-all">{selectedTenantId || "—"}</div>
            </CardContent>
          </Card>
        ) : (
          <Card>
            <CardHeader>
              <CardTitle>User</CardTitle>
              <CardDescription>Scoped to user's personal workspace.</CardDescription>
            </CardHeader>
            <CardContent className="text-xs space-y-1">
              <div>{selectedUser ? (selectedUser.name?.trim() ? selectedUser.name : selectedUser.email) : "—"}</div>
              <div className="text-muted-foreground break-all">{selectedUser?.email || "—"}</div>
            </CardContent>
          </Card>
        )}
      </div>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>Jobs by type</CardTitle>
            <CardDescription>Count of jobs created per type in this window.</CardDescription>
          </CardHeader>
          <CardContent>
            {jobsByType.length === 0 ? (
              <p className="text-xs text-muted-foreground">{loading ? "Loading…" : "No job data for this window."}</p>
            ) : (
              <ChartContainer config={{}} className="h-56 w-full">
                <BarChart data={jobsByType} margin={{ left: 12, right: 12 }}>
                  <CartesianGrid vertical={false} />
                  <XAxis dataKey="key" tickLine={false} axisLine={false} />
                  <YAxis tickLine={false} axisLine={false} width={28} />
                  <ChartTooltip cursor={false} content={<ChartTooltipContent hideLabel />} />
                  <Bar dataKey="value" radius={4}>
                    {jobsByType.map((row) => (
                      <Cell key={row.key} fill={colorForKey(row.key)} />
                    ))}
                  </Bar>
                </BarChart>
              </ChartContainer>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Documents by type</CardTitle>
            <CardDescription>Results stored per job type.</CardDescription>
          </CardHeader>
          <CardContent>
            {documentsByType.length === 0 ? (
              <p className="text-xs text-muted-foreground">
                {loading ? "Loading…" : "No document data for this window."}
              </p>
            ) : (
              <ChartContainer config={{}} className="h-56 w-full">
                <BarChart data={documentsByType} margin={{ left: 12, right: 12 }}>
                  <CartesianGrid vertical={false} />
                  <XAxis dataKey="key" tickLine={false} axisLine={false} />
                  <YAxis tickLine={false} axisLine={false} width={28} />
                  <ChartTooltip cursor={false} content={<ChartTooltipContent hideLabel />} />
                  <Bar dataKey="value" radius={4}>
                    {documentsByType.map((row) => (
                      <Cell key={row.key} fill={colorForKey(row.key)} />
                    ))}
                  </Bar>
                </BarChart>
              </ChartContainer>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
