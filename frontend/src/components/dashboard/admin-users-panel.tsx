"use client"

import { useEffect, useMemo, useState } from "react"

import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"
import { Switch } from "@/components/ui/switch"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

interface AdminUser {
  id: string
  email: string
  name?: string
  authProvider: string
  isSystemAdmin: boolean
  isDisabled?: boolean
  disabledAt?: string
  createdAt: string
  updatedAt: string
  defaultTenantId?: string
  themePreference?: string
  passwordSet?: boolean
}

export function AdminUsersPanel() {
  const [query, setQuery] = useState("")
  const [users, setUsers] = useState<AdminUser[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const [offset, setOffset] = useState(0)
  const limit = 50

  const [detailOpen, setDetailOpen] = useState(false)
  const [detailLoading, setDetailLoading] = useState(false)
  const [detailError, setDetailError] = useState<string | null>(null)
  const [selectedUser, setSelectedUser] = useState<AdminUser | null>(null)

  const [editName, setEditName] = useState("")
  const [editAdmin, setEditAdmin] = useState(false)
  const [editDisabled, setEditDisabled] = useState(false)
  const [saving, setSaving] = useState(false)

  const [createOpen, setCreateOpen] = useState(false)
  const [createEmail, setCreateEmail] = useState("")
  const [createName, setCreateName] = useState("")
  const [createPassword, setCreatePassword] = useState("")
  const [createAdmin, setCreateAdmin] = useState(false)
  const [createLoading, setCreateLoading] = useState(false)
  const [createError, setCreateError] = useState<string | null>(null)

  const [resetPassword, setResetPassword] = useState("")
  const [resetLoading, setResetLoading] = useState(false)
  const [resetStatus, setResetStatus] = useState<string | null>(null)

  const canLoadMore = users.length < total

  useEffect(() => {
    refresh({ reset: true })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  async function refresh({ reset }: { reset: boolean }) {
    const nextOffset = reset ? 0 : offset

    setLoading(true)
    setError(null)
    try {
      const url = new URL("/admin/users", window.location.origin)
      if (query.trim()) {
        url.searchParams.set("query", query.trim())
      }
      url.searchParams.set("limit", String(limit))
      url.searchParams.set("offset", String(nextOffset))

      const res = await fetch(url.toString())
      const data = (await res.json()) as {
        success?: boolean
        users?: AdminUser[]
        total?: number
        error?: string
      }
      if (!res.ok || !data.success) {
        setError(data.error || "Unable to load users")
        return
      }

      const next = data.users ?? []
      if (reset) {
        setUsers(next)
        setOffset(next.length)
      } else {
        setUsers((prev) => [...prev, ...next])
        setOffset(nextOffset + next.length)
      }
      setTotal(data.total ?? 0)
    } catch {
      setError("Network error while loading users")
    } finally {
      setLoading(false)
    }
  }

  async function openDetails(user: AdminUser) {
    setDetailOpen(true)
    setDetailLoading(true)
    setDetailError(null)
    setSelectedUser(null)

    try {
      const res = await fetch(`/admin/users/${user.id}`)
      const data = (await res.json()) as {
        success?: boolean
        user?: AdminUser
        error?: string
      }
      if (!res.ok || !data.success || !data.user) {
        setDetailError(data.error || "Unable to load user")
        return
      }
      setSelectedUser(data.user)
      setEditName(data.user.name ?? "")
      setEditAdmin(data.user.isSystemAdmin)
      setEditDisabled(!!data.user.isDisabled)
      setResetPassword("")
      setResetStatus(null)
    } catch {
      setDetailError("Network error while loading user")
    } finally {
      setDetailLoading(false)
    }
  }

  async function saveUser() {
    if (!selectedUser) return

    setSaving(true)
    setDetailError(null)
    try {
      const res = await fetch(`/admin/users/${selectedUser.id}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          name: editName,
          isSystemAdmin: editAdmin,
          isDisabled: editDisabled,
        }),
      })
      const data = (await res.json()) as {
        success?: boolean
        user?: AdminUser
        error?: string
      }
      if (!res.ok || !data.success || !data.user) {
        setDetailError(data.error || "Unable to save user")
        return
      }

      setSelectedUser(data.user)
      setUsers((prev) => prev.map((u) => (u.id === data.user!.id ? data.user! : u)))
    } catch {
      setDetailError("Network error while saving user")
    } finally {
      setSaving(false)
    }
  }

  const summary = useMemo(() => {
    if (loading && users.length === 0) return "Loading…"
    if (error) return "Error loading users."
    return `${users.length} of ${total}`
  }, [error, loading, total, users.length])

  async function createUser() {
    setCreateLoading(true)
    setCreateError(null)
    try {
      const res = await fetch("/admin/users", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          email: createEmail,
          password: createPassword,
          name: createName,
          isSystemAdmin: createAdmin,
        }),
      })
      const data = (await res.json()) as {
        success?: boolean
        user?: AdminUser
        error?: string
      }
      if (!res.ok || !data.success || !data.user) {
        setCreateError(data.error || "Unable to create user")
        return
      }

      setCreateOpen(false)
      setCreateEmail("")
      setCreateName("")
      setCreatePassword("")
      setCreateAdmin(false)

      await refresh({ reset: true })
      await openDetails(data.user)
    } catch {
      setCreateError("Network error while creating user")
    } finally {
      setCreateLoading(false)
    }
  }

  async function resetUserPassword() {
    if (!selectedUser) return

    setResetLoading(true)
    setResetStatus(null)
    setDetailError(null)
    try {
      const res = await fetch(`/admin/users/${selectedUser.id}/reset-password`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ password: resetPassword }),
      })
      const data = (await res.json()) as {
        success?: boolean
        user?: AdminUser
        error?: string
      }
      if (!res.ok || !data.success || !data.user) {
        setDetailError(data.error || "Unable to reset password")
        return
      }

      setSelectedUser(data.user)
      setUsers((prev) => prev.map((u) => (u.id === data.user!.id ? data.user! : u)))
      setResetPassword("")
      setResetStatus("Password reset.")
    } catch {
      setDetailError("Network error while resetting password")
    } finally {
      setResetLoading(false)
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <Badge variant="secondary">Admin</Badge>
          <span className="text-xs text-muted-foreground">{summary}</span>
        </div>
        <div className="flex items-center gap-2">
          <form
            className="flex items-center gap-2"
            onSubmit={(e) => {
              e.preventDefault()
              refresh({ reset: true })
            }}
          >
            <Input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Search users…"
              className="w-[240px]"
            />
            <Button type="submit" size="xs" variant="outline" disabled={loading}>
              Search
            </Button>
          </form>
          <Button
            variant="outline"
            size="xs"
            type="button"
            onClick={() => {
              setCreateError(null)
              setCreateOpen(true)
            }}
          >
            Create user
          </Button>
          <Button
            variant="ghost"
            size="xs"
            type="button"
            onClick={() => refresh({ reset: true })}
            disabled={loading}
          >
            {loading ? "Refreshing…" : "Refresh"}
          </Button>
        </div>
      </div>

      {error ? <p className="text-xs text-destructive">{error}</p> : null}

      <Card>
        <CardHeader>
          <CardTitle className="text-sm">Users</CardTitle>
          <CardDescription className="text-xs">
            View and manage system users.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="border rounded-none">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Email</TableHead>
                  <TableHead>Provider</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Created</TableHead>
                  <TableHead className="text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {users.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={6} className="text-xs text-muted-foreground">
                      {loading ? "Loading users…" : "No users found."}
                    </TableCell>
                  </TableRow>
                ) : (
                  users.map((user) => (
                    <TableRow key={user.id}>
                      <TableCell className="text-xs">
                        {user.name?.trim() ? (
                          user.name
                        ) : (
                          <span className="text-muted-foreground">—</span>
                        )}
                      </TableCell>
                      <TableCell className="font-mono text-[11px]">
                        {user.email}
                      </TableCell>
                      <TableCell className="text-xs">
                        {user.authProvider}
                      </TableCell>
                      <TableCell className="text-xs">
                        <div className="flex flex-wrap items-center gap-1">
                          {user.isDisabled ? (
                            <Badge variant="destructive">Disabled</Badge>
                          ) : null}
                          {user.isSystemAdmin ? (
                            <Badge variant="secondary">System admin</Badge>
                          ) : (
                            <span className="text-muted-foreground">User</span>
                          )}
                        </div>
                      </TableCell>
                      <TableCell className="text-xs">
                        {new Date(user.createdAt).toLocaleString()}
                      </TableCell>
                      <TableCell className="text-right">
                        <DropdownMenu>
                          <DropdownMenuTrigger
                            render={
                              <Button
                                variant="ghost"
                                size="xs"
                                type="button"
                                aria-label="User actions"
                              >
                                ⋯
                              </Button>
                            }
                          />
                          <DropdownMenuContent align="end" className="min-w-44">
                            <DropdownMenuGroup>
                              <DropdownMenuItem onClick={() => openDetails(user)}>
                                Details
                              </DropdownMenuItem>
                            </DropdownMenuGroup>
                            <DropdownMenuSeparator />
                            <DropdownMenuGroup>
                              <DropdownMenuItem onClick={() => openDetails(user)}>
                                Edit
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

          {users.length > 0 ? (
            <div className="mt-3 flex justify-center">
              <Button
                variant="outline"
                size="xs"
                type="button"
                disabled={loading || !canLoadMore}
                onClick={() => refresh({ reset: false })}
              >
                {canLoadMore ? (loading ? "Loading…" : "Load more") : "No more users"}
              </Button>
            </div>
          ) : null}
        </CardContent>
      </Card>

      <Sheet
        open={detailOpen}
        onOpenChange={(open) => {
          setDetailOpen(open)
          if (!open) {
            setSelectedUser(null)
            setDetailError(null)
            setSaving(false)
            setResetPassword("")
            setResetStatus(null)
          }
        }}
      >
        <SheetContent>
          <SheetHeader>
            <SheetTitle>User details</SheetTitle>
            <SheetDescription className="text-xs">
              View and update user settings.
            </SheetDescription>
          </SheetHeader>

          <div className="mt-4 space-y-4">
            {detailError ? (
              <p className="text-xs text-destructive">{detailError}</p>
            ) : null}

            {detailLoading ? (
              <p className="text-xs text-muted-foreground">Loading user…</p>
            ) : selectedUser ? (
              <>
                <Card>
                  <CardHeader>
                    <CardTitle className="text-sm">Identity</CardTitle>
                    <CardDescription className="text-xs">
                      Basic user details.
                    </CardDescription>
                  </CardHeader>
                  <CardContent>
                    <FieldGroup>
                      <Field>
                        <FieldLabel>Display name</FieldLabel>
                        <Input
                          value={editName}
                          onChange={(e) => setEditName(e.target.value)}
                          placeholder="Optional"
                        />
                        <FieldDescription />
                      </Field>
                      <Field>
                        <FieldLabel>Email</FieldLabel>
                        <Input value={selectedUser.email} readOnly />
                        <FieldDescription />
                      </Field>
                      <Field>
                        <FieldLabel>Auth provider</FieldLabel>
                        <Input value={selectedUser.authProvider} readOnly />
                        <FieldDescription />
                      </Field>
                    </FieldGroup>
                  </CardContent>
                </Card>

                <Card>
                  <CardHeader>
                    <CardTitle className="text-sm">Permissions</CardTitle>
                    <CardDescription className="text-xs">
                      Control administrative access.
                    </CardDescription>
                  </CardHeader>
                  <CardContent>
                    <FieldGroup>
                      <Field orientation="horizontal" className="items-start">
                        <div className="flex flex-col">
                          <FieldLabel>System admin</FieldLabel>
                          <FieldDescription>
                            Grants access to admin endpoints and UI.
                          </FieldDescription>
                        </div>
                        <Switch
                          checked={editAdmin}
                          onCheckedChange={(checked) => setEditAdmin(checked)}
                        />
                      </Field>
                      <Field orientation="horizontal" className="items-start">
                        <div className="flex flex-col">
                          <FieldLabel>Disabled</FieldLabel>
                          <FieldDescription>
                            Prevents the user from authenticating.
                          </FieldDescription>
                        </div>
                        <Switch
                          checked={editDisabled}
                          onCheckedChange={(checked) => setEditDisabled(checked)}
                        />
                      </Field>
                      <FieldError />
                    </FieldGroup>
                  </CardContent>
                </Card>

                <Card>
                  <CardHeader>
                    <CardTitle className="text-sm">Security</CardTitle>
                    <CardDescription className="text-xs">
                      Reset password for local users.
                    </CardDescription>
                  </CardHeader>
                  <CardContent>
                    {resetStatus ? (
                      <p className="mb-2 text-xs text-muted-foreground">{resetStatus}</p>
                    ) : null}
                    <FieldGroup>
                      <Field>
                        <FieldLabel>New password</FieldLabel>
                        <Input
                          type="password"
                          value={resetPassword}
                          onChange={(e) => setResetPassword(e.target.value)}
                          placeholder="Enter a new password"
                          disabled={selectedUser.authProvider !== "local"}
                        />
                        <FieldDescription>
                          {selectedUser.authProvider === "local"
                            ? "This will invalidate the old password."
                            : "Password reset is only available for local users."}
                        </FieldDescription>
                      </Field>
                    </FieldGroup>
                    <div className="mt-3 flex justify-end">
                      <Button
                        type="button"
                        variant="outline"
                        size="xs"
                        onClick={resetUserPassword}
                        disabled={
                          resetLoading ||
                          !resetPassword.trim() ||
                          selectedUser.authProvider !== "local"
                        }
                      >
                        {resetLoading ? "Resetting…" : "Reset password"}
                      </Button>
                    </div>
                  </CardContent>
                </Card>

                <Card>
                  <CardHeader>
                    <CardTitle className="text-sm">Metadata</CardTitle>
                    <CardDescription className="text-xs">
                      Read-only information.
                    </CardDescription>
                  </CardHeader>
                  <CardContent className="text-xs space-y-2">
                    <div className="flex justify-between gap-2">
                      <span className="text-muted-foreground">User ID</span>
                      <span className="font-mono text-[11px]">{selectedUser.id}</span>
                    </div>
                    <div className="flex justify-between gap-2">
                      <span className="text-muted-foreground">Created</span>
                      <span>{new Date(selectedUser.createdAt).toLocaleString()}</span>
                    </div>
                    <div className="flex justify-between gap-2">
                      <span className="text-muted-foreground">Updated</span>
                      <span>{new Date(selectedUser.updatedAt).toLocaleString()}</span>
                    </div>
                  </CardContent>
                </Card>
              </>
            ) : null}
          </div>

          <SheetFooter className="mt-4">
            <Button
              type="button"
              onClick={saveUser}
              disabled={detailLoading || saving || !selectedUser}
            >
              {saving ? "Saving…" : "Save"}
            </Button>
          </SheetFooter>
        </SheetContent>
      </Sheet>

      <Sheet
        open={createOpen}
        onOpenChange={(open) => {
          setCreateOpen(open)
          if (!open) {
            setCreateError(null)
            setCreateLoading(false)
          }
        }}
      >
        <SheetContent>
          <SheetHeader>
            <SheetTitle>Create user</SheetTitle>
            <SheetDescription className="text-xs">
              Creates a local user with a personal workspace.
            </SheetDescription>
          </SheetHeader>

          <div className="mt-4 space-y-4">
            {createError ? (
              <p className="text-xs text-destructive">{createError}</p>
            ) : null}

            <Card>
              <CardHeader>
                <CardTitle className="text-sm">User</CardTitle>
                <CardDescription className="text-xs">
                  Basic user information.
                </CardDescription>
              </CardHeader>
              <CardContent>
                <FieldGroup>
                  <Field>
                    <FieldLabel>Email</FieldLabel>
                    <Input
                      value={createEmail}
                      onChange={(e) => setCreateEmail(e.target.value)}
                      placeholder="user@example.com"
                    />
                    <FieldDescription />
                  </Field>
                  <Field>
                    <FieldLabel>Display name</FieldLabel>
                    <Input
                      value={createName}
                      onChange={(e) => setCreateName(e.target.value)}
                      placeholder="Optional"
                    />
                    <FieldDescription />
                  </Field>
                  <Field>
                    <FieldLabel>Initial password</FieldLabel>
                    <Input
                      type="password"
                      value={createPassword}
                      onChange={(e) => setCreatePassword(e.target.value)}
                      placeholder="Required"
                    />
                    <FieldDescription />
                  </Field>
                  <Field orientation="horizontal" className="items-start">
                    <div className="flex flex-col">
                      <FieldLabel>System admin</FieldLabel>
                      <FieldDescription>
                        Grants access to admin endpoints and UI.
                      </FieldDescription>
                    </div>
                    <Switch
                      checked={createAdmin}
                      onCheckedChange={(checked) => setCreateAdmin(checked)}
                    />
                  </Field>
                </FieldGroup>
              </CardContent>
            </Card>
          </div>

          <SheetFooter className="mt-4">
            <Button
              type="button"
              onClick={createUser}
              disabled={
                createLoading ||
                !createEmail.trim() ||
                !createPassword.trim()
              }
            >
              {createLoading ? "Creating…" : "Create"}
            </Button>
          </SheetFooter>
        </SheetContent>
      </Sheet>
    </div>
  )
}
