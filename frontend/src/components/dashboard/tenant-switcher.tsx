"use client"

import { useEffect, useState } from "react"

import { HugeiconsIcon } from "@hugeicons/react"
import { Building03Icon, ProfileIcon } from "@hugeicons/core-free-icons"

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
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  useSidebar,
} from "@/components/ui/sidebar"

export interface TenantSwitcherTenant {
  id: string
  slug: string
  name: string
  type: string
  role: string
}

interface TenantSwitcherProps {
  activeTenantName?: string
  activeTenantType?: string
  onTenantSelected?: (tenant: TenantSwitcherTenant) => Promise<void> | void
}

export function TenantSwitcher({ activeTenantName, activeTenantType, onTenantSelected }: TenantSwitcherProps) {
  useSidebar()
  const [tenants, setTenants] = useState<TenantSwitcherTenant[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false

    async function load() {
      setLoading(true)
      setError(null)
      try {
        const res = await fetch("/v1/tenants")
        const data = (await res.json()) as {
          success?: boolean
          tenants?: TenantSwitcherTenant[]
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
      } catch (err) {
        if (!cancelled) {
          setError("Network error while loading tenants")
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
  }, [])

  const activeName =
    activeTenantName || tenants.find((t) => t.type === "personal")?.name ||
    tenants.find((t) => t.role === "tenant_admin")?.name ||
    tenants[0]?.name

  const nonPersonalCount = tenants.filter((t) => t.type !== "personal").length
  const activeTypeLabel =
    activeTenantType === "personal"
      ? "Personal"
      : activeTenantType === "org"
        ? "Organization"
        : activeTenantType || ""

  const activeIcon = activeTenantType === "personal" ? ProfileIcon : Building03Icon

  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <DropdownMenu>
          <DropdownMenuTrigger
            render={
              <SidebarMenuButton
                size="lg"
                className="data-[state=open]:bg-sidebar-accent data-[state=open]:text-sidebar-accent-foreground rounded-lg border border-sidebar-border bg-sidebar group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:gap-0"
                title={activeName || "Select tenant"}
              >
                <div className="bg-sidebar-primary text-sidebar-primary-foreground flex aspect-square size-9 items-center justify-center rounded-md">
                  <HugeiconsIcon icon={activeIcon} strokeWidth={2} />
                </div>
                <div className="grid flex-1 text-left text-sm leading-tight group-data-[collapsible=icon]:hidden">
                  <span className="truncate font-medium">
                    {activeName || "Select tenant"}
                  </span>
                  <span className="truncate text-xs">
                    {loading
                      ? "Loading tenants…"
                      : error || activeTypeLabel || `${nonPersonalCount} tenant(s)`}
                  </span>
                </div>
                <span className="text-muted-foreground ml-auto text-xs group-data-[collapsible=icon]:hidden">⌄</span>
              </SidebarMenuButton>
            }
          />
          <DropdownMenuContent
            className="w-(--anchor-width) min-w-56 rounded-lg"
            align="start"
            side="bottom"
            sideOffset={4}
          >
            <DropdownMenuGroup>
              <DropdownMenuLabel className="text-muted-foreground text-xs">
                Workspaces
              </DropdownMenuLabel>
              {tenants.map((tenant) => {
                const displayName =
                  tenant.type === "personal" ? "Personal" : tenant.name
                const subtitle =
                  tenant.type === "personal"
                    ? "Personal"
                    : tenant.type === "org"
                      ? "Organization"
                      : tenant.type
                const icon = tenant.type === "personal" ? ProfileIcon : Building03Icon

                return (
                  <DropdownMenuItem
                    key={tenant.id}
                    className="gap-2 p-2"
                    onClick={async () => {
                      try {
                        const res = await fetch(`/v1/tenants/${tenant.id}/select`, {
                          method: "POST",
                        })
                        const data = (await res.json()) as {
                          success?: boolean
                          error?: string
                        }
                        if (!res.ok || !data.success) {
                          console.error("Failed to select tenant", data.error)
                          return
                        }
                        if (onTenantSelected) {
                          await onTenantSelected(tenant)
                        } else {
                          window.location.reload()
                        }
                      } catch (err) {
                        console.error("Network error selecting tenant", err)
                      }
                    }}
                  >

                    <div className="flex size-6 items-center justify-center rounded-md border">
                      <HugeiconsIcon icon={icon} strokeWidth={2} />
                    </div>
                    <div className="flex flex-col">
                      <span className="text-xs font-medium">{displayName}</span>
                      <span className="text-muted-foreground text-[10px] uppercase tracking-wide">
                        {subtitle}
                      </span>
                    </div>
                  </DropdownMenuItem>
                )
              })}
            </DropdownMenuGroup>
            <DropdownMenuSeparator />
            <DropdownMenuItem disabled className="p-2 text-xs text-muted-foreground">
              Manage tenants (coming soon)
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </SidebarMenuItem>
    </SidebarMenu>
  )
}
