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
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

interface AdminTenant {
  id: string
  slug: string
  name: string
  type: string
  ownerUserId?: string
  createdAt: string
  updatedAt: string
  defaultApiKeyRateLimitPerMinute?: number
}

interface AdminTenantMember {
  userId: string
  email: string
  name?: string
  role: "tenant_admin" | "tenant_member"
  createdAt: string
  updatedAt: string
}

interface AdminUserOption {
  id: string
  email: string
  name?: string
  isSystemAdmin: boolean
  isDisabled?: boolean
}

export function AdminTenantsPanel() {
  const [query, setQuery] = useState("")
  const [tenants, setTenants] = useState<AdminTenant[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const [offset, setOffset] = useState(0)
  const limit = 50
  const canLoadMore = tenants.length < total

  const [detailOpen, setDetailOpen] = useState(false)
  const [detailLoading, setDetailLoading] = useState(false)
  const [detailError, setDetailError] = useState<string | null>(null)
  const [selectedTenant, setSelectedTenant] = useState<AdminTenant | null>(null)

  const [editName, setEditName] = useState("")
  const [editSlug, setEditSlug] = useState("")
  const [saving, setSaving] = useState(false)

  const [members, setMembers] = useState<AdminTenantMember[]>([])
  const [membersLoading, setMembersLoading] = useState(false)

  const [memberQuery, setMemberQuery] = useState("")
  const [userResults, setUserResults] = useState<AdminUserOption[]>([])
  const [userResultsLoading, setUserResultsLoading] = useState(false)
  const [selectedUserId, setSelectedUserId] = useState<string>("")
  const [selectedRole, setSelectedRole] = useState<"tenant_admin" | "tenant_member">(
    "tenant_member"
  )
  const [memberActionLoading, setMemberActionLoading] = useState(false)

  const [createOpen, setCreateOpen] = useState(false)
  const [createSlug, setCreateSlug] = useState("")
  const [createName, setCreateName] = useState("")
  const [createDefaultRateLimit, setCreateDefaultRateLimit] = useState("")
  const [createMembers, setCreateMembers] = useState<Array<{ user: AdminUserOption; role: "tenant_admin" | "tenant_member" }>>([])
  const [createMemberQuery, setCreateMemberQuery] = useState("")
  const [createUserResults, setCreateUserResults] = useState<AdminUserOption[]>([])
  const [createUserResultsLoading, setCreateUserResultsLoading] = useState(false)
  const [createSelectedUserId, setCreateSelectedUserId] = useState<string>("")
  const [createSelectedRole, setCreateSelectedRole] = useState<"tenant_admin" | "tenant_member">("tenant_member")
  const [createLoading, setCreateLoading] = useState(false)
  const [createError, setCreateError] = useState<string | null>(null)

  useEffect(() => {
    refresh({ reset: true })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  async function refresh({ reset }: { reset: boolean }) {
    const nextOffset = reset ? 0 : offset

    setLoading(true)
    setError(null)
    try {
      const url = new URL("/admin/tenants", window.location.origin)
      if (query.trim()) url.searchParams.set("query", query.trim())
      url.searchParams.set("includePersonal", "false")
      url.searchParams.set("limit", String(limit))
      url.searchParams.set("offset", String(nextOffset))

      const res = await fetch(url.toString())
      const data = (await res.json()) as {
        success?: boolean
        tenants?: AdminTenant[]
        total?: number
        error?: string
      }
      if (!res.ok || !data.success) {
        setError(data.error || "Unable to load tenants")
        return
      }

      const next = data.tenants ?? []
      if (reset) {
        setTenants(next)
        setOffset(next.length)
      } else {
        setTenants((prev) => [...prev, ...next])
        setOffset(nextOffset + next.length)
      }
      setTotal(data.total ?? 0)
    } catch {
      setError("Network error while loading tenants")
    } finally {
      setLoading(false)
    }
  }

  async function openDetails(tenant: AdminTenant) {
    setDetailOpen(true)
    setDetailLoading(true)
    setDetailError(null)
    setSelectedTenant(null)
    setMembers([])
    setMemberQuery("")
    setUserResults([])
    setSelectedUserId("")
    setSelectedRole("tenant_member")

    try {
      const res = await fetch(`/admin/tenants/${tenant.id}`)
      const data = (await res.json()) as {
        success?: boolean
        tenant?: AdminTenant
        error?: string
      }
      if (!res.ok || !data.success || !data.tenant) {
        setDetailError(data.error || "Unable to load tenant")
        return
      }

      setSelectedTenant(data.tenant)
      setEditName(data.tenant.name)
      setEditSlug(data.tenant.slug)

      await refreshMembers(data.tenant.id)
    } catch {
      setDetailError("Network error while loading tenant")
    } finally {
      setDetailLoading(false)
    }
  }

  async function refreshMembers(tenantId: string) {
    setMembersLoading(true)
    try {
      const res = await fetch(`/admin/tenants/${tenantId}/members?limit=200&offset=0`)
      const data = (await res.json()) as {
        success?: boolean
        members?: AdminTenantMember[]
        error?: string
      }
      if (!res.ok || !data.success) {
        setDetailError(data.error || "Unable to load members")
        return
      }
      setMembers(data.members ?? [])
    } catch {
      setDetailError("Network error while loading members")
    } finally {
      setMembersLoading(false)
    }
  }

  async function saveTenant() {
    if (!selectedTenant) return
    setSaving(true)
    setDetailError(null)
    try {
      const res = await fetch(`/admin/tenants/${selectedTenant.id}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name: editName, slug: editSlug }),
      })
      const data = (await res.json()) as {
        success?: boolean
        tenant?: AdminTenant
        error?: string
      }
      if (!res.ok || !data.success || !data.tenant) {
        setDetailError(data.error || "Unable to save tenant")
        return
      }

      setSelectedTenant(data.tenant)
      setTenants((prev) => prev.map((t) => (t.id === data.tenant!.id ? data.tenant! : t)))
    } catch {
      setDetailError("Network error while saving tenant")
    } finally {
      setSaving(false)
    }
  }

  async function searchUsers() {
    if (!memberQuery.trim()) {
      setUserResults([])
      return
    }

    setUserResultsLoading(true)
    try {
      const url = new URL("/admin/users", window.location.origin)
      url.searchParams.set("query", memberQuery.trim())
      url.searchParams.set("limit", "10")
      url.searchParams.set("offset", "0")
      const res = await fetch(url.toString())
      const data = (await res.json()) as {
        success?: boolean
        users?: AdminUserOption[]
        error?: string
      }
      if (!res.ok || !data.success) {
        setDetailError(data.error || "Unable to search users")
        return
      }
      setUserResults(data.users ?? [])
    } catch {
      setDetailError("Network error while searching users")
    } finally {
      setUserResultsLoading(false)
    }
  }

  async function addMember() {
    if (!selectedTenant || !selectedUserId) return
    setMemberActionLoading(true)
    setDetailError(null)
    try {
      const res = await fetch(`/admin/tenants/${selectedTenant.id}/members`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ userId: selectedUserId, role: selectedRole }),
      })
      const data = (await res.json()) as { success?: boolean; error?: string }
      if (!res.ok || !data.success) {
        setDetailError(data.error || "Unable to add member")
        return
      }
      await refreshMembers(selectedTenant.id)
    } catch {
      setDetailError("Network error while adding member")
    } finally {
      setMemberActionLoading(false)
    }
  }

  async function updateMemberRole(userId: string, role: "tenant_admin" | "tenant_member") {
    if (!selectedTenant) return
    setMemberActionLoading(true)
    setDetailError(null)
    try {
      const res = await fetch(`/admin/tenants/${selectedTenant.id}/members/${userId}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ role, userId }),
      })
      const data = (await res.json()) as { success?: boolean; error?: string }
      if (!res.ok || !data.success) {
        setDetailError(data.error || "Unable to update member")
        return
      }
      await refreshMembers(selectedTenant.id)
    } catch {
      setDetailError("Network error while updating member")
    } finally {
      setMemberActionLoading(false)
    }
  }

  async function removeMember(userId: string) {
    if (!selectedTenant) return
    setMemberActionLoading(true)
    setDetailError(null)
    try {
      const res = await fetch(`/admin/tenants/${selectedTenant.id}/members/${userId}`, {
        method: "DELETE",
      })
      const data = (await res.json()) as { success?: boolean; error?: string }
      if (!res.ok || !data.success) {
        setDetailError(data.error || "Unable to remove member")
        return
      }
      await refreshMembers(selectedTenant.id)
    } catch {
      setDetailError("Network error while removing member")
    } finally {
      setMemberActionLoading(false)
    }
  }

  async function createTenant() {
    setCreateLoading(true)
    setCreateError(null)
    try {
      const parsedRateLimit = createDefaultRateLimit.trim()
        ? Number(createDefaultRateLimit.trim())
        : undefined
      if (parsedRateLimit !== undefined && (!Number.isFinite(parsedRateLimit) || parsedRateLimit < 0)) {
        setCreateError("Default rate limit must be a number >= 0")
        return
      }

      const res = await fetch("/admin/tenants", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          slug: createSlug,
          name: createName,
          type: "org",
          defaultApiKeyRateLimitPerMinute: parsedRateLimit,
          members: createMembers.map((m) => ({
            userId: m.user.id,
            role: m.role,
          })),
        }),
      })
      const data = (await res.json()) as {
        success?: boolean
        tenant?: AdminTenant
        error?: string
      }
      if (!res.ok || !data.success || !data.tenant) {
        setCreateError(data.error || "Unable to create tenant")
        return
      }

      setCreateOpen(false)
      setCreateSlug("")
      setCreateName("")
      setCreateDefaultRateLimit("")
      setCreateMembers([])
      setCreateMemberQuery("")
      setCreateUserResults([])
      setCreateSelectedUserId("")
      setCreateSelectedRole("tenant_member")
      await refresh({ reset: true })
      await openDetails(data.tenant)
    } catch {
      setCreateError("Network error while creating tenant")
    } finally {
      setCreateLoading(false)
    }
  }

  async function searchUsersForCreate() {
    if (!createMemberQuery.trim()) {
      setCreateUserResults([])
      return
    }

    setCreateUserResultsLoading(true)
    try {
      const url = new URL("/admin/users", window.location.origin)
      url.searchParams.set("query", createMemberQuery.trim())
      url.searchParams.set("limit", "10")
      url.searchParams.set("offset", "0")
      const res = await fetch(url.toString())
      const data = (await res.json()) as {
        success?: boolean
        users?: AdminUserOption[]
        error?: string
      }
      if (!res.ok || !data.success) {
        setCreateError(data.error || "Unable to search users")
        return
      }
      setCreateUserResults(data.users ?? [])
    } catch {
      setCreateError("Network error while searching users")
    } finally {
      setCreateUserResultsLoading(false)
    }
  }

  function addCreateMember() {
    const user = createUserResults.find((u) => u.id === createSelectedUserId)
    if (!user) return
    if (user.isDisabled) return
    setCreateMembers((prev) => {
      if (prev.some((m) => m.user.id === user.id)) return prev
      return [...prev, { user, role: createSelectedRole }]
    })
    setCreateSelectedUserId("")
  }

  function removeCreateMember(userId: string) {
    setCreateMembers((prev) => prev.filter((m) => m.user.id !== userId))
  }

  const summary = useMemo(() => {
    if (loading && tenants.length === 0) return "Loading…"
    if (error) return "Error loading tenants."
    return `${tenants.length} of ${total}`
  }, [error, loading, tenants.length, total])

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
              placeholder="Search tenants…"
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
            Create tenant
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
          <CardTitle className="text-sm">Tenants</CardTitle>
          <CardDescription className="text-xs">
            View and manage tenants and memberships.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="border rounded-none">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Slug</TableHead>
                  <TableHead>Type</TableHead>
                  <TableHead>Created</TableHead>
                  <TableHead className="text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {tenants.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={5} className="text-xs text-muted-foreground">
                      {loading ? "Loading tenants…" : "No tenants found."}
                    </TableCell>
                  </TableRow>
                ) : (
                  tenants.map((tenant) => (
                    <TableRow key={tenant.id}>
                      <TableCell className="text-xs">{tenant.name}</TableCell>
                      <TableCell className="font-mono text-[11px]">{tenant.slug}</TableCell>
                  <TableCell className="text-xs">
                    {tenant.type === "personal" ? (
                      <Badge variant="secondary">Personal</Badge>
                    ) : (
                      <span className="text-muted-foreground">Org</span>
                    )}
                  </TableCell>
                      <TableCell className="text-xs">
                        {new Date(tenant.createdAt).toLocaleString()}
                      </TableCell>
                      <TableCell className="text-right">
                        <DropdownMenu>
                          <DropdownMenuTrigger
                            render={
                              <Button variant="ghost" size="xs" type="button" aria-label="Tenant actions">
                                ⋯
                              </Button>
                            }
                          />
                          <DropdownMenuContent align="end" className="min-w-44">
                            <DropdownMenuGroup>
                              <DropdownMenuItem onClick={() => openDetails(tenant)}>
                                Details
                              </DropdownMenuItem>
                            </DropdownMenuGroup>
                            <DropdownMenuSeparator />
                            <DropdownMenuGroup>
                              <DropdownMenuItem disabled variant="destructive">
                                Delete (coming soon)
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

          {tenants.length > 0 ? (
            <div className="mt-3 flex justify-center">
              <Button
                variant="outline"
                size="xs"
                type="button"
                disabled={loading || !canLoadMore}
                onClick={() => refresh({ reset: false })}
              >
                {canLoadMore ? (loading ? "Loading…" : "Load more") : "No more tenants"}
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
            setSelectedTenant(null)
            setDetailError(null)
            setSaving(false)
            setMembers([])
            setMemberQuery("")
            setUserResults([])
            setSelectedUserId("")
            setSelectedRole("tenant_member")
          }
        }}
      >
        <SheetContent className="w-[min(100vw-2rem,72rem)]">
          <SheetHeader>
            <SheetTitle>Tenant details</SheetTitle>
            <SheetDescription className="text-xs">
              View and update tenant settings and members.
            </SheetDescription>
          </SheetHeader>

          <div className="mt-4 space-y-4">
            {detailError ? <p className="text-xs text-destructive">{detailError}</p> : null}

            {detailLoading ? (
              <p className="text-xs text-muted-foreground">Loading tenant…</p>
            ) : selectedTenant ? (
              <>
                <Card>
                  <CardHeader>
                    <CardTitle className="text-sm">Tenant</CardTitle>
                    <CardDescription className="text-xs">
                      Basic tenant information.
                    </CardDescription>
                  </CardHeader>
                  <CardContent>
                    <FieldGroup>
                      <Field>
                        <FieldLabel>Name</FieldLabel>
                        <Input value={editName} onChange={(e) => setEditName(e.target.value)} />
                        <FieldDescription />
                      </Field>
                      <Field>
                        <FieldLabel>Slug</FieldLabel>
                        <Input value={editSlug} onChange={(e) => setEditSlug(e.target.value)} />
                        <FieldDescription />
                      </Field>
                      <Field>
                        <FieldLabel>Default API key rate limit</FieldLabel>
                        <Input
                          value={
                            selectedTenant.defaultApiKeyRateLimitPerMinute != null
                              ? String(selectedTenant.defaultApiKeyRateLimitPerMinute)
                              : ""
                          }
                          readOnly
                          placeholder="Not set"
                        />
                        <FieldDescription>
                          Used when creating tenant API keys without an explicit rate limit.
                        </FieldDescription>
                      </Field>
                      <Field>
                        <FieldLabel>Type</FieldLabel>
                        <Input value={selectedTenant.type} readOnly />
                        <FieldDescription />
                      </Field>
                    </FieldGroup>
                  </CardContent>
                </Card>

                <Card>
                  <CardHeader>
                    <CardTitle className="text-sm">Members</CardTitle>
                    <CardDescription className="text-xs">
                      Manage who has access to this tenant.
                    </CardDescription>
                  </CardHeader>
                  <CardContent className="space-y-3">
                    <div className="flex items-center justify-between gap-2">
                      <div className="flex items-center gap-2">
                        <Input
                          value={memberQuery}
                          onChange={(e) => setMemberQuery(e.target.value)}
                          placeholder="Search users (email/name)…"
                          className="w-[260px]"
                        />
                        <Button
                          type="button"
                          size="xs"
                          variant="outline"
                          onClick={searchUsers}
                          disabled={userResultsLoading || !memberQuery.trim()}
                        >
                          {userResultsLoading ? "Searching…" : "Search"}
                        </Button>
                      </div>
                      <div className="flex items-center gap-2">
                        <Select value={selectedRole} onValueChange={(v) => setSelectedRole(v as any)}>
                          <SelectTrigger className="w-[160px]">
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent align="end">
                            <SelectGroup>
                              <SelectLabel>Role</SelectLabel>
                              <SelectItem value="tenant_member">Tenant member</SelectItem>
                              <SelectItem value="tenant_admin">Tenant admin</SelectItem>
                            </SelectGroup>
                          </SelectContent>
                        </Select>
                        <Button
                          type="button"
                          size="xs"
                          onClick={addMember}
                          disabled={memberActionLoading || !selectedUserId}
                        >
                          {memberActionLoading ? "Adding…" : "Add"}
                        </Button>
                      </div>
                    </div>

                    {userResults.length > 0 ? (
                      <div className="border rounded-none">
                        <Table>
                          <TableHeader>
                            <TableRow>
                              <TableHead>User</TableHead>
                              <TableHead>Email</TableHead>
                              <TableHead className="text-right">Select</TableHead>
                            </TableRow>
                          </TableHeader>
                          <TableBody>
                            {userResults.map((u) => (
                              <TableRow key={u.id}>
                                <TableCell className="text-xs">
                                  {u.name?.trim() ? u.name : <span className="text-muted-foreground">—</span>}
                                </TableCell>
                                <TableCell className="font-mono text-[11px]">{u.email}</TableCell>
                                <TableCell className="text-right">
                                  <Button
                                    type="button"
                                    size="xs"
                                    variant={selectedUserId === u.id ? "secondary" : "outline"}
                                    onClick={() => setSelectedUserId(u.id)}
                                    disabled={u.isDisabled}
                                  >
                                    {u.isDisabled ? "Disabled" : selectedUserId === u.id ? "Selected" : "Select"}
                                  </Button>
                                </TableCell>
                              </TableRow>
                            ))}
                          </TableBody>
                        </Table>
                      </div>
                    ) : null}

                    <div className="border rounded-none">
                      <Table>
                        <TableHeader>
                          <TableRow>
                            <TableHead>User</TableHead>
                            <TableHead>Email</TableHead>
                            <TableHead>Role</TableHead>
                            <TableHead className="text-right">Actions</TableHead>
                          </TableRow>
                        </TableHeader>
                        <TableBody>
                          {members.length === 0 ? (
                            <TableRow>
                              <TableCell colSpan={4} className="text-xs text-muted-foreground">
                                {membersLoading ? "Loading members…" : "No members."}
                              </TableCell>
                            </TableRow>
                          ) : (
                            members.map((m) => (
                              <TableRow key={m.userId}>
                                <TableCell className="text-xs">
                                  {m.name?.trim() ? m.name : <span className="text-muted-foreground">—</span>}
                                </TableCell>
                                <TableCell className="font-mono text-[11px]">{m.email}</TableCell>
                                <TableCell className="text-xs">
                                  <Select
                                    value={m.role}
                                    onValueChange={(v) => updateMemberRole(m.userId, v as any)}
                                  >
                                    <SelectTrigger className="w-[160px]">
                                      <SelectValue />
                                    </SelectTrigger>
                                    <SelectContent align="start">
                                      <SelectGroup>
                                        <SelectLabel>Role</SelectLabel>
                                        <SelectItem value="tenant_member">Tenant member</SelectItem>
                                        <SelectItem value="tenant_admin">Tenant admin</SelectItem>
                                      </SelectGroup>
                                    </SelectContent>
                                  </Select>
                                </TableCell>
                                <TableCell className="text-right">
                                  <Button
                                    type="button"
                                    size="xs"
                                    variant="destructive"
                                    onClick={() => removeMember(m.userId)}
                                    disabled={memberActionLoading}
                                  >
                                    Remove
                                  </Button>
                                </TableCell>
                              </TableRow>
                            ))
                          )}
                        </TableBody>
                      </Table>
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
                      <span className="text-muted-foreground">Tenant ID</span>
                      <span className="font-mono text-[11px]">{selectedTenant.id}</span>
                    </div>
                    <div className="flex justify-between gap-2">
                      <span className="text-muted-foreground">Created</span>
                      <span>{new Date(selectedTenant.createdAt).toLocaleString()}</span>
                    </div>
                    <div className="flex justify-between gap-2">
                      <span className="text-muted-foreground">Updated</span>
                      <span>{new Date(selectedTenant.updatedAt).toLocaleString()}</span>
                    </div>
                  </CardContent>
                </Card>
              </>
            ) : null}
          </div>

          <SheetFooter className="mt-4">
            <Button type="button" onClick={saveTenant} disabled={saving || detailLoading || !selectedTenant}>
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
            <SheetTitle>Create tenant</SheetTitle>
            <SheetDescription className="text-xs">
              Creates a new tenant (typically an organization).
            </SheetDescription>
          </SheetHeader>

          <div className="mt-4 space-y-4">
            {createError ? <p className="text-xs text-destructive">{createError}</p> : null}

            <Card>
              <CardHeader>
                <CardTitle className="text-sm">Tenant</CardTitle>
                <CardDescription className="text-xs">Basic tenant information.</CardDescription>
              </CardHeader>
              <CardContent>
                <FieldGroup>
                  <Field>
                    <FieldLabel>Name</FieldLabel>
                    <Input value={createName} onChange={(e) => setCreateName(e.target.value)} placeholder="Acme Corp" />
                    <FieldDescription />
                  </Field>
                  <Field>
                    <FieldLabel>Slug</FieldLabel>
                    <Input value={createSlug} onChange={(e) => setCreateSlug(e.target.value)} placeholder="acme-corp" />
                    <FieldDescription />
                  </Field>
                  <Field>
                    <FieldLabel>Type</FieldLabel>
                    <Input value="org" readOnly />
                    <FieldDescription>
                      Personal workspaces are managed via Users.
                    </FieldDescription>
                    <FieldError />
                  </Field>
                  <Field>
                    <FieldLabel>Default API key rate limit</FieldLabel>
                    <Input
                      inputMode="numeric"
                      value={createDefaultRateLimit}
                      onChange={(e) => setCreateDefaultRateLimit(e.target.value)}
                      placeholder="Optional (per minute)"
                    />
                    <FieldDescription>
                      Applied to new tenant API keys when no rate limit is provided.
                    </FieldDescription>
                    <FieldError />
                  </Field>
                </FieldGroup>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className="text-sm">Members</CardTitle>
                <CardDescription className="text-xs">
                  Add existing users and assign tenant roles.
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-3">
                <div className="flex items-center justify-between gap-2">
                  <div className="flex items-center gap-2">
                    <Input
                      value={createMemberQuery}
                      onChange={(e) => setCreateMemberQuery(e.target.value)}
                      placeholder="Search users (email/name)…"
                      className="w-[260px]"
                    />
                    <Button
                      type="button"
                      size="xs"
                      variant="outline"
                      onClick={searchUsersForCreate}
                      disabled={createUserResultsLoading || !createMemberQuery.trim()}
                    >
                      {createUserResultsLoading ? "Searching…" : "Search"}
                    </Button>
                  </div>
                  <div className="flex items-center gap-2">
                    <Select value={createSelectedRole} onValueChange={(v) => setCreateSelectedRole(v as any)}>
                      <SelectTrigger className="w-[160px]">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent align="end">
                        <SelectGroup>
                          <SelectLabel>Role</SelectLabel>
                          <SelectItem value="tenant_member">Tenant member</SelectItem>
                          <SelectItem value="tenant_admin">Tenant admin</SelectItem>
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                    <Button
                      type="button"
                      size="xs"
                      onClick={addCreateMember}
                      disabled={!createSelectedUserId}
                    >
                      Add
                    </Button>
                  </div>
                </div>

                {createUserResults.length > 0 ? (
                  <div className="border rounded-none">
                    <Table>
                      <TableHeader>
                        <TableRow>
                          <TableHead>User</TableHead>
                          <TableHead>Email</TableHead>
                          <TableHead className="text-right">Select</TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {createUserResults.map((u) => (
                          <TableRow key={u.id}>
                            <TableCell className="text-xs">
                              {u.name?.trim() ? u.name : <span className="text-muted-foreground">—</span>}
                            </TableCell>
                            <TableCell className="font-mono text-[11px]">{u.email}</TableCell>
                            <TableCell className="text-right">
                              <Button
                                type="button"
                                size="xs"
                                variant={createSelectedUserId === u.id ? "secondary" : "outline"}
                                onClick={() => setCreateSelectedUserId(u.id)}
                                disabled={u.isDisabled}
                              >
                                {u.isDisabled ? "Disabled" : createSelectedUserId === u.id ? "Selected" : "Select"}
                              </Button>
                            </TableCell>
                          </TableRow>
                        ))}
                      </TableBody>
                    </Table>
                  </div>
                ) : null}

                <div className="border rounded-none">
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>User</TableHead>
                        <TableHead>Email</TableHead>
                        <TableHead>Role</TableHead>
                        <TableHead className="text-right">Remove</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {createMembers.length === 0 ? (
                        <TableRow>
                          <TableCell colSpan={4} className="text-xs text-muted-foreground">
                            No members added yet.
                          </TableCell>
                        </TableRow>
                      ) : (
                        createMembers.map((m) => (
                          <TableRow key={m.user.id}>
                            <TableCell className="text-xs">
                              {m.user.name?.trim() ? m.user.name : <span className="text-muted-foreground">—</span>}
                            </TableCell>
                            <TableCell className="font-mono text-[11px]">{m.user.email}</TableCell>
                            <TableCell className="text-xs">{m.role}</TableCell>
                            <TableCell className="text-right">
                              <Button
                                type="button"
                                size="xs"
                                variant="destructive"
                                onClick={() => removeCreateMember(m.user.id)}
                              >
                                Remove
                              </Button>
                            </TableCell>
                          </TableRow>
                        ))
                      )}
                    </TableBody>
                  </Table>
                </div>
              </CardContent>
            </Card>
          </div>

          <SheetFooter className="mt-4">
            <Button
              type="button"
              onClick={createTenant}
              disabled={createLoading || !createName.trim() || !createSlug.trim()}
            >
              {createLoading ? "Creating…" : "Create"}
            </Button>
          </SheetFooter>
        </SheetContent>
      </Sheet>
    </div>
  )
}
