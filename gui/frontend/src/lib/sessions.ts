import type { SessionInfo } from '../types'

type Listener = (session: SessionInfo) => void

const listeners = new Set<Listener>()
let initialised = false

function dispatch(session: SessionInfo) {
  listeners.forEach((listener) => listener(session))
}

function initialise() {
  if (initialised || typeof window === 'undefined') return
  initialised = true
  if (window.runtime?.EventsOn) {
    window.runtime.EventsOn('gdit:session', (payload) => dispatch(payload as SessionInfo))
  }
  window.addEventListener('gdit:session', ((event: CustomEvent<SessionInfo>) => dispatch(event.detail)) as EventListener)
}

export function subscribeSessions(listener: Listener) {
  initialise()
  listeners.add(listener)
  return () => { listeners.delete(listener) }
}
