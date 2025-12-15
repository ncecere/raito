"use client"

import { useCallback, useEffect, useMemo, useState } from "react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger } from "@/components/ui/select"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"

type UsageWindow = "24h" | "7d" | "30d"
type ActorType = "" | "session" | "api_key"

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

type AdminAuditEvent = {
  id: number
  createdAt: string
  action: string
  actorUserId?: string
  actorUserEmail?: string
  actorUserName?: string
  actorApiKeyId?: string
  actorApiKeyLabel?: string
  tenantId?: string
  tenantName?: string
  tenantSlug?: string
  tenantType?: string
  resourceType?: string
  resourceId?: string
  ip?: string
  userAgent?: string
  metadata?: any
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

function formatActor(ev: AdminAuditEvent) {
  if (ev.actorApiKeyId) {
    return ev.actorApiKeyLabel ? `API key: ${ev.actorApiKeyLabel}` : "API key"
  }
  if (ev.actorUserEmail) {
    return ev.actorUserName?.trim() ? `${ev.actorUserName} (${ev.actorUserEmail})` : ev.actorUserEmail
  }
  return "—"
}

function formatTenant(ev: AdminAuditEvent) {
  if (!ev.tenantId) return "—"
  if (ev.tenantName) return ev.tenantName
  if (ev.tenantSlug) return ev.tenantSlug
  return ev.tenantId
}

function summarizeMeta(meta: any) {
  if (!meta || typeof meta !== "object") return ""
  try {
    const entries = Object.entries(meta)
    if (entries.length === 0) return ""
    return entries
      .slice(0, 3)
      .map(([k, v]) => `${k}=${typeof v === "string" ? v : JSON.stringify(v)}`)
      .join(" ")
  } catch {
    return ""
  }
}

export function AdminAuditPanel() {
  const [queryInput, setQueryInput] = useState("")
  const [query, setQuery] = useState("")

  const [action, setAction] = useState("")
  const [actorType, setActorType] = useState<ActorType>("")
  const [window, setWindow] = useState<UsageWindow>("7d")

  const [tenantId, setTenantId] = useState("")
  const [userId, setUserId] = useState("")

  const [tenants, setTenants] = useState<AdminTenantItem[]>([])
  const [users, setUsers] = useState<AdminUserItem[]>([])

  const [events, setEvents] = useState<AdminAuditEvent[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const limit = 50
  const canLoadMore = useMemo(() => events.length < total, [events.length, total])

  useEffect(() => {
    let cancelled = false

    async function loadOptions() {
      try {
        const [tenantsRes, usersRes] = await Promise.all([
          fetch("/admin/tenants?includePersonal=false&limit=500&offset=0"),
          fetch("/admin/users?limit=500&offset=0"),
        ])
        const tenantsData = (await tenantsRes.json()) as { success?: boolean; tenants?: AdminTenantItem[] }
        const usersData = (await usersRes.json()) as { success?: boolean; users?: AdminUserItem[] }
        if (!cancelled) {
          if (tenantsRes.ok && tenantsData.success) {
            setTenants(tenantsData.tenants ?? [])
          }
          if (usersRes.ok && usersData.success) {
            setUsers(usersData.users ?? [])
          }
        }
      } catch {
        // ignore
      }
    }

    loadOptions()
    return () => {
      cancelled = true
    }
  }, [])

  const selectedTenant = useMemo(
    () => tenants.find((t) => t.id === tenantId) ?? null,
    [tenantId, tenants]
  )
  const selectedUser = useMemo(() => users.find((u) => u.id === userId) ?? null, [userId, users])

  const buildUrl = useCallback(
    (offset: number) => {
      const params = new URLSearchParams()
      params.set("query", query.trim())
      if (action) params.set("action", action)
      if (actorType) params.set("actorType", actorType)
      if (tenantId) params.set("tenantId", tenantId)
      if (userId) params.set("userId", userId)
      params.set("window", window)
      params.set("limit", String(limit))
      params.set("offset", String(offset))
      return `/admin/audit?${params.toString()}`
    },
    [action, actorType, query, tenantId, userId, window]
  )

  const refresh = useCallback(
    async ({ reset }: { reset: boolean }) => {
      const offset = reset ? 0 : events.length
      setLoading(true)
      setError(null)
      try {
        const res = await fetch(buildUrl(offset))
        const data = (await res.json()) as {
          success?: boolean
          total?: number
          events?: AdminAuditEvent[]
          error?: string
        }
        if (!res.ok || !data.success) {
          setError(data.error || "Unable to load audit events")
          return
        }
        setTotal(data.total ?? 0)
        if (reset) {
          setEvents(data.events ?? [])
        } else {
          setEvents((prev) => [...prev, ...(data.events ?? [])])
        }
      } catch (err) {
        setError(err instanceof Error ? err.message : "Unable to load audit events")
      } finally {
        setLoading(false)
      }
    },
    [buildUrl, events.length]
  )

  useEffect(() => {
    refresh({ reset: true })
  }, [action, actorType, query, tenantId, userId, window, refresh])

  return (
    <Card>
      <CardHeader>
        <div className="flex items-start justify-between gap-3">
          <div className="space-y-1">
            <CardTitle className="text-sm">Audit log</CardTitle>
            <CardDescription className="text-xs">
              Search security and administrative events.
            </CardDescription>
          </div>

          <div className="flex flex-wrap items-center justify-end gap-2">
            <Input
              value={queryInput}
              onChange={(e) => setQueryInput(e.target.value)}
              placeholder="Search action, user, tenant, resource…"
              className="w-[260px]"
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.preventDefault()
                  setQuery(queryInput)
                }
              }}
            />
            <Button
              type="button"
              size="xs"
              variant="outline"
              onClick={() => setQuery(queryInput)}
              disabled={loading}
            >
              Search
            </Button>

            <Select value={action} onValueChange={(v) => setAction(v ?? "")}>
              <SelectTrigger className="w-[220px]" aria-label="Action filter">
                <span className={["min-w-0 flex flex-1 text-left truncate", action ? "" : "text-muted-foreground"].join(" ")}>
                  {action || "All actions"}
                </span>
              </SelectTrigger>
              <SelectContent align="end" className="max-h-[320px]">
                <SelectGroup>
                  <SelectItem value="">All actions</SelectItem>
                  <SelectItem value="admin.user.create">admin.user.create</SelectItem>
                  <SelectItem value="admin.user.update">admin.user.update</SelectItem>
                  <SelectItem value="admin.user.reset_password">admin.user.reset_password</SelectItem>
                  <SelectItem value="admin.tenant.create">admin.tenant.create</SelectItem>
                  <SelectItem value="admin.tenant.update">admin.tenant.update</SelectItem>
                  <SelectItem value="admin.tenant.member.add">admin.tenant.member.add</SelectItem>
                  <SelectItem value="admin.tenant.member.update">admin.tenant.member.update</SelectItem>
                  <SelectItem value="admin.tenant.member.remove">admin.tenant.member.remove</SelectItem>
                  <SelectItem value="admin.api_key.revoke">admin.api_key.revoke</SelectItem>
                </SelectGroup>
              </SelectContent>
            </Select>

            <Select value={actorType} onValueChange={(v) => setActorType((v ?? "") as ActorType)}>
              <SelectTrigger className="w-[160px]" aria-label="Actor type filter">
                <span className={["min-w-0 flex flex-1 text-left truncate", actorType ? "" : "text-muted-foreground"].join(" ")}>
                  {actorType === "api_key" ? "API key" : actorType === "session" ? "Session" : "Any actor"}
                </span>
              </SelectTrigger>
              <SelectContent align="end">
                <SelectGroup>
                  <SelectItem value="">Any actor</SelectItem>
                  <SelectItem value="session">Session</SelectItem>
                  <SelectItem value="api_key">API key</SelectItem>
                </SelectGroup>
              </SelectContent>
            </Select>

            <Select value={tenantId} onValueChange={(v) => setTenantId(v ?? "")}>
              <SelectTrigger className="w-[220px]" aria-label="Tenant filter">
                <span
                  className={[
                    "min-w-0 flex flex-1 text-left truncate",
                    tenantId ? "" : "text-muted-foreground",
                  ].join(" ")}
                  title={selectedTenant ? selectedTenant.name : undefined}
                >
                  {selectedTenant ? selectedTenant.name : "All tenants"}
                </span>
              </SelectTrigger>
              <SelectContent align="end" className="max-h-[320px]">
                <SelectGroup>
                  <SelectItem value="">All tenants</SelectItem>
                  {tenants.map((t) => (
                    <SelectItem key={t.id} value={t.id} title={t.name}>
                      {t.name}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>

            <Select value={userId} onValueChange={(v) => setUserId(v ?? "")}>
              <SelectTrigger className="w-[220px]" aria-label="User filter">
                <span
                  className={[
                    "min-w-0 flex flex-1 text-left truncate",
                    userId ? "" : "text-muted-foreground",
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
                    : "All users"}
                </span>
              </SelectTrigger>
              <SelectContent align="end" className="max-h-[320px]">
                <SelectGroup>
                  <SelectItem value="">All users</SelectItem>
                  {users.map((u) => (
                    <SelectItem
                      key={u.id}
                      value={u.id}
                      title={u.name?.trim() ? `${u.name} (${u.email})` : u.email}
                    >
                      {u.name?.trim() ? `${u.name} (${u.email})` : u.email}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>

            <Select value={window} onValueChange={(v) => setWindow(v as UsageWindow)}>
              <SelectTrigger className="w-[160px]" aria-label="Time window">
                <span className="min-w-0 flex flex-1 text-left truncate">{formatWindowLabel(window)}</span>
              </SelectTrigger>
              <SelectContent align="end">
                <SelectGroup>
                  <SelectItem value="24h">Last 24 hours</SelectItem>
                  <SelectItem value="7d">Last 7 days</SelectItem>
                  <SelectItem value="30d">Last 30 days</SelectItem>
                </SelectGroup>
              </SelectContent>
            </Select>

            <Button variant="ghost" size="xs" type="button" onClick={() => refresh({ reset: true })} disabled={loading}>
              {loading ? "Refreshing…" : "Refresh"}
            </Button>
          </div>
        </div>
      </CardHeader>

      <CardContent className="space-y-3">
        {error ? <p className="text-xs text-destructive">{error}</p> : null}

        <div className="border rounded-none">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Time</TableHead>
                <TableHead>Action</TableHead>
                <TableHead>Actor</TableHead>
                <TableHead>Tenant</TableHead>
                <TableHead>Resource</TableHead>
                <TableHead className="text-right">IP</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {events.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={6} className="text-xs text-muted-foreground">
                    {loading ? "Loading audit events…" : "No audit events found."}
                  </TableCell>
                </TableRow>
              ) : (
                events.map((ev) => (
                  <TableRow key={ev.id}>
                    <TableCell className="text-xs whitespace-nowrap">
                      {new Date(ev.createdAt).toLocaleString()}
                    </TableCell>
                    <TableCell className="text-xs">
                      <div className="flex items-center gap-2">
                        <Badge variant="secondary">{ev.action}</Badge>
                        {ev.metadata ? (
                          <span className="text-muted-foreground truncate max-w-[360px]" title={JSON.stringify(ev.metadata)}>
                            {summarizeMeta(ev.metadata)}
                          </span>
                        ) : null}
                      </div>
                    </TableCell>
                    <TableCell className="text-xs">{formatActor(ev)}</TableCell>
                    <TableCell className="text-xs">{formatTenant(ev)}</TableCell>
                    <TableCell className="text-xs text-muted-foreground">
                      {ev.resourceType ? (
                        <span title={ev.resourceId ? `${ev.resourceType}:${ev.resourceId}` : ev.resourceType}>
                          {ev.resourceType}
                          {ev.resourceId ? `:${ev.resourceId.slice(0, 8)}` : ""}
                        </span>
                      ) : (
                        "—"
                      )}
                    </TableCell>
                    <TableCell className="text-right text-xs font-mono text-[11px]">
                      {ev.ip || "—"}
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </div>

        <div className="flex items-center justify-between">
          <p className="text-xs text-muted-foreground">
            Showing {events.length} of {total}
          </p>
          <Button
            variant="outline"
            size="xs"
            type="button"
            disabled={loading || !canLoadMore}
            onClick={() => refresh({ reset: false })}
          >
            {canLoadMore ? (loading ? "Loading…" : "Load more") : "No more events"}
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}

