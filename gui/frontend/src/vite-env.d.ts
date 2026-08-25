/// <reference types="vite/client" />

interface Window {
  go?: {
    bridge?: {
      App?: Record<string, (...args: unknown[]) => Promise<unknown>>
    }
  }
  runtime?: {
    EventsOn: (event: string, callback: (payload: unknown) => void) => void
    EventsOff: (event: string) => void
    Environment?: () => Promise<{ platform: string; arch: string; buildType: string }>
    WindowMinimise?: () => void
    WindowToggleMaximise?: () => void
    WindowIsMaximised?: () => Promise<boolean>
    Quit?: () => void
  }
}
