"use client"

import { useEffect, useMemo, useState } from "react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
} from "@/components/ui/chart"
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Bar, BarChart, CartesianGrid, Cell, XAxis, YAxis } from "recharts"

type UsageWindow = "24h" | "7d" | "30d"

type TenantUsageResponse = {
  success?: boolean
  jobs: number
  documents: number
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

export function UsagePanel({ tenantId }: { tenantId?: string }) {
  const [window, setWindow] = useState<UsageWindow>("7d")
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [usage, setUsage] = useState<TenantUsageResponse | null>(null)

  useEffect(() => {
    if (!tenantId) {
      setUsage(null)
      setError(null)
      return
    }

    let cancelled = false

    async function load() {
      setLoading(true)
      setError(null)
      try {
        const res = await fetch(`/v1/tenants/${tenantId}/usage?window=${window}`)
        const data = (await res.json()) as TenantUsageResponse
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
  }, [tenantId, window])

  const jobsByType = useMemo(() => toChartRows(usage?.jobsByType), [usage?.jobsByType])
  const documentsByType = useMemo(
    () => toChartRows(usage?.documentsByType),
    [usage?.documentsByType]
  )

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <Badge variant="secondary">Usage</Badge>
          <span className="text-xs text-muted-foreground">
            {formatWindowLabel(window)}
          </span>
        </div>
        <div className="flex items-center gap-2">
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
            onClick={() => {
              // Trigger reload by re-setting the same window.
              setWindow((w) => w)
            }}
            disabled={loading}
          >
            {loading ? "Refreshing…" : "Refresh"}
          </Button>
        </div>
      </div>

      {error ? <p className="text-xs text-destructive">{error}</p> : null}

      <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
        <Card>
          <CardHeader>
            <CardTitle>Jobs</CardTitle>
            <CardDescription>Total jobs created.</CardDescription>
          </CardHeader>
          <CardContent className="text-2xl font-semibold tabular-nums">
            {usage?.jobs ?? 0}
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>Documents</CardTitle>
            <CardDescription>Results stored from jobs.</CardDescription>
          </CardHeader>
          <CardContent className="text-2xl font-semibold tabular-nums">
            {usage?.documents ?? 0}
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>Tenant</CardTitle>
            <CardDescription>Scoped to active tenant.</CardDescription>
          </CardHeader>
          <CardContent className="text-xs text-muted-foreground break-all">
            {tenantId ?? "—"}
          </CardContent>
        </Card>
      </div>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>Jobs by type</CardTitle>
            <CardDescription>
              Count of jobs created per type in this window.
            </CardDescription>
          </CardHeader>
          <CardContent>
            {jobsByType.length === 0 ? (
              <p className="text-xs text-muted-foreground">
                {loading ? "Loading…" : "No job data for this window."}
              </p>
            ) : (
              <ChartContainer config={{}} className="h-56 w-full">
                <BarChart data={jobsByType} margin={{ left: 12, right: 12 }}>
                  <CartesianGrid vertical={false} />
                  <XAxis dataKey="key" tickLine={false} axisLine={false} />
                  <YAxis tickLine={false} axisLine={false} width={28} />
                  <ChartTooltip
                    cursor={false}
                    content={<ChartTooltipContent hideLabel />}
                  />
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
            <CardDescription>
              Results stored per job type.
            </CardDescription>
          </CardHeader>
          <CardContent>
            {documentsByType.length === 0 ? (
              <p className="text-xs text-muted-foreground">
                {loading ? "Loading…" : "No document data for this window."}
              </p>
            ) : (
              <ChartContainer config={{}} className="h-56 w-full">
                <BarChart
                  data={documentsByType}
                  margin={{ left: 12, right: 12 }}
                >
                  <CartesianGrid vertical={false} />
                  <XAxis dataKey="key" tickLine={false} axisLine={false} />
                  <YAxis tickLine={false} axisLine={false} width={28} />
                  <ChartTooltip
                    cursor={false}
                    content={<ChartTooltipContent hideLabel />}
                  />
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
