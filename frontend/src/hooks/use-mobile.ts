"use client"

import { useEffect, useState } from "react"

// Simple mobile breakpoint hook used by the shadcn sidebar.
// Returns true when the viewport width is at or below the given breakpoint.
export function useIsMobile(breakpoint = 768): boolean {
  const [isMobile, setIsMobile] = useState(false)

  useEffect(() => {
    if (typeof window === "undefined") return

    const mediaQuery = window.matchMedia(`(max-width: ${breakpoint}px)`)
    setIsMobile(mediaQuery.matches)

    // Subscribe to changes
    const listener = (event: MediaQueryListEvent) => setIsMobile(event.matches)
    mediaQuery.addEventListener("change", listener)

    return () => {
      mediaQuery.removeEventListener("change", listener)
    }
  }, [breakpoint])

  return isMobile
}
