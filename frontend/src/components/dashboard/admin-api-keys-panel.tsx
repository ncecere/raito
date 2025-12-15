import { useCallback, useEffect, useMemo, useState } from "react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Switch } from "@/components/ui/switch"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"

interface AdminAPIKeyItem {
  id: string
  label: string
  isAdmin: boolean
  rateLimitPerMinute?: number
  tenantId?: string
  tenantName?: string
  tenantSlug?: string
  tenantType?: string
  userId?: string
  userEmail?: string
  userName?: string
  createdAt: string
  revokedAt?: string
}

export function AdminAPIKeysPanel() {
  const [queryInput, setQueryInput] = useState("")
  const [query, setQuery] = useState("")
  const [includeRevoked, setIncludeRevoked] = useState(false)

  const [keys, setKeys] = useState<AdminAPIKeyItem[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [revokingId, setRevokingId] = useState<string | null>(null)

  const limit = 50

  const canLoadMore = useMemo(() => keys.length < total, [keys.length, total])

  const buildUrl = useCallback(
    (offset: number) => {
      const params = new URLSearchParams()
      params.set("query", query.trim())
      params.set("includeRevoked", String(includeRevoked))
      params.set("limit", String(limit))
      params.set("offset", String(offset))
      return `/admin/api-keys?${params.toString()}`
    },
    [includeRevoked, query]
  )

  const refresh = useCallback(
    async ({ reset }: { reset: boolean }) => {
      const offset = reset ? 0 : keys.length
      setLoading(true)
      setError(null)
      try {
        const res = await fetch(buildUrl(offset))
        const data = (await res.json()) as {
          success?: boolean
          keys?: AdminAPIKeyItem[]
          total?: number
          error?: string
        }
        if (!res.ok || !data.success) {
          setError(data.error || "Unable to load API keys")
          return
        }

        setTotal(data.total ?? 0)
        if (reset) {
          setKeys(data.keys ?? [])
        } else {
          setKeys((prev) => [...prev, ...(data.keys ?? [])])
        }
      } catch (err) {
        setError(err instanceof Error ? err.message : "Unable to load API keys")
      } finally {
        setLoading(false)
      }
    },
    [buildUrl, keys.length]
  )

  useEffect(() => {
    refresh({ reset: true })
  }, [includeRevoked, query, refresh])

  const revokeKey = useCallback(
    async (id: string) => {
      setRevokingId(id)
      setError(null)
      try {
        const res = await fetch(`/admin/api-keys/${id}`, { method: "DELETE" })
        const data = (await res.json()) as {
          success?: boolean
          revokedAt?: string
          error?: string
        }
        if (!res.ok || !data.success) {
          setError(data.error || "Unable to revoke API key")
          return
        }
        setKeys((prev) =>
          prev.map((k) => (k.id === id ? { ...k, revokedAt: data.revokedAt } : k))
        )
      } catch (err) {
        setError(err instanceof Error ? err.message : "Unable to revoke API key")
      } finally {
        setRevokingId(null)
      }
    },
    []
  )

  const scopeBadge = (key: AdminAPIKeyItem) => {
    if (key.isAdmin) {
      return <Badge variant="destructive">Admin</Badge>
    }
    if (key.tenantId) {
      return <Badge variant="secondary">Tenant</Badge>
    }
    return <Badge variant="outline">Global</Badge>
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex items-start justify-between gap-3">
          <div className="space-y-1">
            <CardTitle className="text-sm">API keys</CardTitle>
            <CardDescription className="text-xs">
              View and revoke API keys across the system.
            </CardDescription>
          </div>
          <div className="flex items-center gap-2">
            <Input
              value={queryInput}
              onChange={(e) => setQueryInput(e.target.value)}
              placeholder="Search label, tenant, user…"
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
            <div className="flex items-center gap-2 pl-2">
              <span className="text-xs text-muted-foreground">Include revoked</span>
              <Switch
                checked={includeRevoked}
                onCheckedChange={(checked) => setIncludeRevoked(checked)}
              />
            </div>
            <Button
              type="button"
              size="xs"
              variant="ghost"
              onClick={() => refresh({ reset: true })}
              disabled={loading}
            >
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
                <TableHead>Label</TableHead>
                <TableHead>Scope</TableHead>
                <TableHead>Tenant</TableHead>
                <TableHead>User</TableHead>
                <TableHead className="text-right">Rate limit</TableHead>
                <TableHead>Created</TableHead>
                <TableHead>Status</TableHead>
                <TableHead className="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {keys.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={8} className="text-xs text-muted-foreground">
                    {loading ? "Loading API keys…" : "No API keys found."}
                  </TableCell>
                </TableRow>
              ) : (
                keys.map((key) => (
                  <TableRow key={key.id}>
                    <TableCell className="text-xs">{key.label}</TableCell>
                    <TableCell>{scopeBadge(key)}</TableCell>
                    <TableCell className="text-xs">
                      {key.tenantId ? (
                        <div className="flex flex-col">
                          <span>{key.tenantName || key.tenantSlug || "Unknown tenant"}</span>
                          {key.tenantType ? (
                            <span className="text-muted-foreground">{key.tenantType}</span>
                          ) : null}
                        </div>
                      ) : (
                        <span className="text-muted-foreground">—</span>
                      )}
                    </TableCell>
                    <TableCell className="text-xs">
                      {key.userEmail ? (
                        <div className="flex flex-col">
                          <span>{key.userName || "—"}</span>
                          <span className="font-mono text-[11px]">{key.userEmail}</span>
                        </div>
                      ) : (
                        <span className="text-muted-foreground">—</span>
                      )}
                    </TableCell>
                    <TableCell className="text-right text-xs">
                      {key.rateLimitPerMinute != null ? key.rateLimitPerMinute : "—"}
                    </TableCell>
                    <TableCell className="text-xs">
                      {new Date(key.createdAt).toLocaleString()}
                    </TableCell>
                    <TableCell className="text-xs">
                      {key.revokedAt ? (
                        <span className="text-muted-foreground">revoked</span>
                      ) : (
                        <span>active</span>
                      )}
                    </TableCell>
                    <TableCell className="text-right">
                      <Button
                        type="button"
                        size="xs"
                        variant="destructive"
                        onClick={() => revokeKey(key.id)}
                        disabled={!!key.revokedAt || revokingId === key.id}
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

        <div className="flex items-center justify-between">
          <p className="text-xs text-muted-foreground">
            Showing {keys.length} of {total}
          </p>
          <Button
            variant="outline"
            size="xs"
            type="button"
            disabled={loading || !canLoadMore}
            onClick={() => refresh({ reset: false })}
          >
            {canLoadMore ? (loading ? "Loading…" : "Load more") : "No more keys"}
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}
