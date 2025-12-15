"use client"

import { useEffect, useMemo, useState } from "react"

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
  FieldGroup,
  FieldLabel,
  FieldError,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { FieldSeparator } from "@/components/ui/field"

interface LoginFormProps {
  onSuccess: () => void
}

type AuthProvidersResponse = {
  success?: boolean
  auth?: {
    enabled?: boolean
    local?: { enabled?: boolean }
    oidc?: { enabled?: boolean; issuer?: string }
  }
  error?: string
}

export function LoginForm({ onSuccess }: LoginFormProps) {
  const [email, setEmail] = useState("")
  const [password, setPassword] = useState("")
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const [providersLoading, setProvidersLoading] = useState(true)
  const [localEnabled, setLocalEnabled] = useState(true)
  const [oidcEnabled, setOidcEnabled] = useState(false)

  useEffect(() => {
    let cancelled = false

    async function loadProviders() {
      setProvidersLoading(true)
      try {
        const res = await fetch("/auth/providers")
        const data = (await res.json()) as AuthProvidersResponse
        if (cancelled) return
        if (!res.ok || !data.success || !data.auth) {
          setLocalEnabled(true)
          setOidcEnabled(false)
          return
        }
        setLocalEnabled(!!data.auth.local?.enabled)
        setOidcEnabled(!!data.auth.oidc?.enabled)
      } catch {
        if (!cancelled) {
          setLocalEnabled(true)
          setOidcEnabled(false)
        }
      } finally {
        if (!cancelled) setProvidersLoading(false)
      }
    }

    loadProviders()
    return () => {
      cancelled = true
    }
  }, [])

  const description = useMemo(() => {
    if (providersLoading) return "Loading sign-in options…"
    if (oidcEnabled && localEnabled) return "Use your email and password or sign in with SSO."
    if (oidcEnabled && !localEnabled) return "Sign in with SSO to access the dashboard."
    return "Use your email and password to access the dashboard."
  }, [localEnabled, oidcEnabled, providersLoading])

  async function handleSubmit(event: React.FormEvent) {
    event.preventDefault()
    setError(null)
    setSubmitting(true)

    try {
      const res = await fetch("/auth/login", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({ email, password }),
      })

      const data = (await res.json()) as {
        success?: boolean
        code?: string
        error?: string
      }

      if (!res.ok || !data.success) {
        setError(data.error || "Unable to log in with those credentials.")
        return
      }

      onSuccess()
    } catch (err) {
      setError("Network error while logging in.")
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="flex min-h-svh w-full items-center justify-center p-6 md:p-10">
      <div className="w-full max-w-sm">
        <Card>
          <CardHeader>
            <CardTitle>Sign in to Raito</CardTitle>
            <CardDescription>{description}</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="space-y-4">
              {oidcEnabled ? (
                <Button
                  type="button"
                  className="w-full"
                  disabled={submitting}
                  onClick={() => {
                    window.location.assign("/auth/oidc/login")
                  }}
                >
                  Sign in with SSO
                </Button>
              ) : null}

              {oidcEnabled && localEnabled ? <FieldSeparator>or</FieldSeparator> : null}

              {localEnabled ? (
                <form onSubmit={handleSubmit} className="space-y-4">
                  <FieldGroup>
                    <Field>
                      <FieldLabel htmlFor="email">Email</FieldLabel>
                      <Input
                        id="email"
                        type="email"
                        autoComplete="email"
                        required
                        value={email}
                        onChange={(event) => setEmail(event.target.value)}
                        placeholder="you@example.com"
                      />
                    </Field>
                    <Field>
                      <FieldLabel htmlFor="password">Password</FieldLabel>
                      <Input
                        id="password"
                        type="password"
                        autoComplete="current-password"
                        required
                        value={password}
                        onChange={(event) => setPassword(event.target.value)}
                      />
                    </Field>
                    <Field>
                      <Button type="submit" disabled={submitting}>
                        {submitting ? "Signing in..." : "Sign in"}
                      </Button>
                      <FieldDescription>
                        Sessions are backed by secure HTTP-only cookies.
                      </FieldDescription>
                      {error ? <FieldError>{error}</FieldError> : null}
                    </Field>
                  </FieldGroup>
                </form>
              ) : oidcEnabled ? (
                <FieldDescription>Local email/password login is disabled for this server.</FieldDescription>
              ) : (
                <FieldDescription>No login methods are currently enabled.</FieldDescription>
              )}
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
