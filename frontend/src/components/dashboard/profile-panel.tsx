"use client"

import { useEffect, useMemo, useState } from "react"

import { useTheme, type ThemeMode } from "@/components/theme/theme-provider"
import { Button } from "@/components/ui/button"
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
  FieldLabel,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

interface TenantOption {
  id: string
  slug: string
  name: string
  type: string
  role: string
}

interface ProfilePanelProps {
  userEmail: string
  userName?: string
  defaultTenantId?: string
  themePreference?: ThemeMode
  onUpdated?: () => Promise<void> | void
}

export function ProfilePanel({
  userEmail,
  userName,
  defaultTenantId,
  themePreference,
  onUpdated,
}: ProfilePanelProps) {
  const { theme, setTheme } = useTheme()
  const [tenants, setTenants] = useState<TenantOption[]>([])
  const [loadingTenants, setLoadingTenants] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const [name, setName] = useState(userName ?? "")
  const [selectedTheme, setSelectedTheme] = useState<ThemeMode>(
    themePreference ?? theme
  )
  const [selectedDefaultTenantId, setSelectedDefaultTenantId] = useState<
    string | undefined
  >(defaultTenantId)

  useEffect(() => {
    setName(userName ?? "")
  }, [userName])

  useEffect(() => {
    if (themePreference) {
      setSelectedTheme(themePreference)
    }
  }, [themePreference])

  useEffect(() => {
    setSelectedDefaultTenantId(defaultTenantId)
  }, [defaultTenantId])

  useEffect(() => {
    let cancelled = false

    async function loadTenants() {
      setLoadingTenants(true)
      setError(null)
      try {
        const res = await fetch("/v1/tenants")
        const data = (await res.json()) as {
          success?: boolean
          tenants?: TenantOption[]
          error?: string
        }
        if (!res.ok || !data.success) {
          if (!cancelled) {
            setError(data.error || "Unable to load tenants")
          }
          return
        }
        if (!cancelled) {
          const next = [...(data.tenants ?? [])].sort((a, b) => {
            if (a.type === b.type) {
              return a.name.localeCompare(b.name)
            }
            if (a.type === "personal") return -1
            if (b.type === "personal") return 1
            return a.type.localeCompare(b.type)
          })
          setTenants(next)
        }
      } catch {
        if (!cancelled) {
          setError("Network error while loading tenants")
        }
      } finally {
        if (!cancelled) {
          setLoadingTenants(false)
        }
      }
    }

    loadTenants()
    return () => {
      cancelled = true
    }
  }, [])

  const defaultTenantOptions = useMemo(() => {
    return tenants.map((t) => {
      const label = t.type === "personal" ? "Personal" : t.name
      const subtitle =
        t.type === "personal"
          ? "Personal"
          : t.type === "org"
            ? "Organization"
            : t.type
      return { value: t.id, label, subtitle }
    })
  }, [tenants])

  async function saveProfile() {
    setSaving(true)
    setError(null)

    try {
      const res = await fetch("/v1/me", {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          name,
          themePreference: selectedTheme,
          defaultTenantId: selectedDefaultTenantId ?? "",
        }),
      })
      const data = (await res.json()) as { success?: boolean; error?: string }
      if (!res.ok || !data.success) {
        setError(data.error || "Unable to save profile")
        return
      }

      setTheme(selectedTheme)
      if (onUpdated) {
        await onUpdated()
      }
    } catch {
      setError("Network error while saving profile")
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="mx-auto w-full max-w-3xl space-y-4">
      {error ? (
        <div className="text-xs text-destructive">{error}</div>
      ) : null}

      <Card>
        <CardHeader>
          <CardTitle className="text-sm">Profile</CardTitle>
          <CardDescription className="text-xs">
            Manage your personal information and preferences.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
            <Field>
              <FieldLabel>Display name</FieldLabel>
              <Input
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="Optional"
              />
              <FieldDescription>
                Shown in the UI where a friendly name is helpful.
              </FieldDescription>
            </Field>

            <Field>
              <FieldLabel>Email</FieldLabel>
              <Input value={userEmail} readOnly />
              <FieldDescription>
                Used for login and your personal workspace.
              </FieldDescription>
            </Field>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-sm">Preferences</CardTitle>
          <CardDescription className="text-xs">
            Control how the dashboard behaves for your account.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
            <Field>
              <FieldLabel>Theme</FieldLabel>
              <Select
                value={selectedTheme}
                onValueChange={(value) => setSelectedTheme(value as ThemeMode)}
              >
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    <SelectLabel>Theme</SelectLabel>
                    <SelectItem value="system">System</SelectItem>
                    <SelectItem value="dark">Dark</SelectItem>
                    <SelectItem value="light">Light</SelectItem>
                  </SelectGroup>
                </SelectContent>
              </Select>
              <FieldDescription>
                Theme is applied immediately and saved for future sessions.
              </FieldDescription>
            </Field>

            <Field>
              <FieldLabel>Default workspace</FieldLabel>
              <Select
                value={selectedDefaultTenantId ?? ""}
                onValueChange={(value) =>
                  setSelectedDefaultTenantId(value || undefined)
                }
              >
                <SelectTrigger className="w-full">
                  {selectedDefaultTenantId ? (
                    <SelectValue />
                  ) : (
                    <span className="text-muted-foreground text-xs">
                      {loadingTenants ? "Loading…" : "Select a workspace"}
                    </span>
                  )}
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    <SelectLabel>Workspaces</SelectLabel>
                    {defaultTenantOptions.map((opt) => (
                      <SelectItem key={opt.value} value={opt.value}>
                        <div className="flex flex-col">
                          <span className="text-xs font-medium">{opt.label}</span>
                          <span className="text-muted-foreground text-[10px] uppercase tracking-wide">
                            {opt.subtitle}
                          </span>
                        </div>
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
              <FieldDescription>
                Used the next time you log in.
              </FieldDescription>
              <FieldError />
            </Field>
          </div>

          <div className="mt-4 flex justify-end">
            <Button type="button" onClick={saveProfile} disabled={saving}>
              {saving ? "Saving…" : "Save"}
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
