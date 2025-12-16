import { useEffect, useState } from "react"

import { DashboardShell, type SessionInfo } from "@/components/dashboard/dashboard-shell"
import { LoginForm } from "@/components/auth/login-form"

export function App() {
  const [session, setSession] = useState<SessionInfo | null>(null)
  const [checkedSession, setCheckedSession] = useState(false)

  useEffect(() => {
    async function loadSession() {
      try {
        const res = await fetch("/auth/session")
        if (!res.ok) {
          setCheckedSession(true)
          return
        }
        const data = (await res.json()) as {
          success?: boolean
          user?: SessionInfo["user"]
          personalTenant?: SessionInfo["personalTenant"]
          activeTenant?: SessionInfo["activeTenant"]
        }
        if (data.success && data.user) {
          setSession({
            user: data.user,
            personalTenant: data.personalTenant,
            activeTenant: data.activeTenant,
          })
        }
      } catch {
        // Ignore session load errors and fall back to login.
      } finally {
        setCheckedSession(true)
      }
    }

    loadSession()
  }, [])

  async function refreshSession() {
    try {
      const res = await fetch("/auth/session")
      if (!res.ok) {
        setSession(null)
        return
      }
      const data = (await res.json()) as {
        success?: boolean
        user?: SessionInfo["user"]
        personalTenant?: SessionInfo["personalTenant"]
        activeTenant?: SessionInfo["activeTenant"]
      }
      if (data.success && data.user) {
        setSession({
          user: data.user,
          personalTenant: data.personalTenant,
          activeTenant: data.activeTenant,
        })
      } else {
        setSession(null)
      }
    } catch {
      setSession(null)
    }
  }

  async function handleLogout() {
    try {
      await fetch("/auth/logout", { method: "POST" })
    } finally {
      setSession(null)
    }
  }

  if (!checkedSession) {
    // Lightweight splash while session is being checked.
    return (
      <div className="bg-background text-foreground flex min-h-svh items-center justify-center text-xs text-muted-foreground">
        Loading…
      </div>
    )
  }

  if (!session) {
    return <LoginForm onSuccess={refreshSession} />
  }

  return (
    <DashboardShell
      session={session}
      onLogout={handleLogout}
      onTenantChanged={refreshSession}
    />
  )
}

export default App
