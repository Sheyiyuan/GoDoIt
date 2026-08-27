import { create } from 'zustand'
import { api } from '../lib/api'
import type { AppSnapshot, CandidateWarning, EngineChannel, GUISettings, InstanceInfo, OperationRecord, ProgressEnvelope, SDKChannel, SessionInfo } from '../types'
import { operationResultSummary, sanitizeOperationText } from '../lib/operationRecord'

interface ModalState {
  title: string
  body: string
  confirmLabel: string
  tone?: 'danger' | 'primary'
  onConfirm: () => void | Promise<void>
}

interface AppState {
  snapshot: AppSnapshot | null
  loading: boolean
  error: string
  bootstrapRoot: string
  operations: Record<string, OperationRecord>
  dismissedWarnings: Record<string, true>
  modal: ModalState | null
  toast: string
  engineCandidates: Record<string, { status: 'idle' | 'loading' | 'ready' | 'error'; items: EngineChannel[]; warnings: CandidateWarning[]; error?: string }>
  sdkCandidates: { status: 'idle' | 'loading' | 'ready' | 'error'; items: SDKChannel[]; warnings: CandidateWarning[]; error?: string }
  sessions: SessionInfo[]
  sessionsLoading: boolean
  sessionError: string
  sessionRevision: number
  terminalSessions: Record<string, true>
  load: () => Promise<void>
  handleProgress: (event: ProgressEnvelope) => void
  updateGUISettings: (settings: GUISettings) => Promise<void>
  applyInstalledInstance: (instance: InstanceInfo) => void
  clearOperation: (id: string) => void
  dismissWarning: (id: string) => void
  openModal: (modal: ModalState) => void
  closeModal: () => void
  notify: (message: string) => void
  prefetchCandidates: () => Promise<void>
  refreshEngineCandidates: (source?: string) => Promise<void>
  refreshSDKCandidates: () => Promise<void>
  handleSession: (session: SessionInfo) => void
  refreshSessions: () => Promise<void>
  launchSession: (name: string) => Promise<SessionInfo>
  requestStopSession: (id: string) => Promise<SessionInfo>
  forceStopSession: (id: string) => Promise<SessionInfo>
}

let sessionRefresh: Promise<void> | null = null

export const useAppStore = create<AppState>((set, get) => ({
  snapshot: null,
  loading: true,
  error: '',
  bootstrapRoot: '',
  operations: {},
  dismissedWarnings: {},
  modal: null,
  toast: '',
  engineCandidates: {},
  sdkCandidates: { status: 'idle', items: [], warnings: [] },
  sessions: [],
  sessionsLoading: false,
  sessionError: '',
  sessionRevision: 0,
  terminalSessions: {},
  load: async () => {
    set({ loading: true, error: '' })
    try {
      const snapshot = await api.Bootstrap()
      set({ snapshot, bootstrapRoot: snapshot.root, loading: false })
    } catch (error) {
      let bootstrapRoot = useAppStore.getState().bootstrapRoot
      try {
        bootstrapRoot = await api.GetRoot()
      } catch {
        // 根目录读取本身不应失败；保留已有值，避免掩盖原始初始化错误。
      }
      set({ loading: false, bootstrapRoot, error: error instanceof Error ? error.message : String(error) })
    }
  },
  handleProgress: (event) => set((state) => {
    const previous = state.operations[event.operation_id]
    if (previous && previous.status !== 'running') return {}
    const progress = event.progress
    const itemKey = progress && (progress.version || progress.filename || progress.source)
      ? `${progress.version || ''}|${progress.filename || ''}|${progress.source || ''}`
      : ''
    const items = previous?.items ? [...previous.items] : []
    if (itemKey) {
      const index = items.findIndex((item) => item.key === itemKey)
      const item = { key: itemKey, version: progress?.version, source: progress?.source, filename: progress?.filename, stage: progress?.stage || 'running', bytes_downloaded: progress?.bytes_downloaded, total_bytes: progress?.total_bytes }
      if (index >= 0) items[index] = { ...items[index], ...item }
      else items.push(item)
    }
    const next: OperationRecord = {
      id: event.operation_id,
      operation: event.operation,
      status: event.status,
      started_at: previous?.started_at || event.timestamp,
      finished_at: event.status === 'running' ? previous?.finished_at : event.timestamp,
      summary: sanitizeOperationText(progress?.message) || previous?.summary,
      error: sanitizeOperationText(event.error),
      result_summary: event.status === 'complete' ? operationResultSummary(event.operation, event.result) : previous?.result_summary || [],
      items,
    }
    return { operations: { ...state.operations, [event.operation_id]: next } }
  }),
  updateGUISettings: async (settings) => {
    await api.SetGUISettings(settings)
    set((state) => state.snapshot ? { snapshot: { ...state.snapshot, gui: settings } } : {})
  },
  applyInstalledInstance: (instance) => set((state) => {
    if (!state.snapshot) return {}
    const instances = state.snapshot.instances
      .filter((item) => item.id !== instance.id && item.name !== instance.name)
      .map((item) => instance.current ? { ...item, current: false } : item)
    instances.push(instance)
    return {
      snapshot: {
        ...state.snapshot,
        instances,
        current: instance.current ? instance : state.snapshot.current,
      },
    }
  }),
  clearOperation: (id) => set((state) => {
    const operations = { ...state.operations }
    delete operations[id]
    return { operations }
  }),
  dismissWarning: (id) => set((state) => ({ dismissedWarnings: { ...state.dismissedWarnings, [id]: true } })),
  openModal: (modal) => set({ modal }),
  closeModal: () => set({ modal: null }),
  notify: (toast) => {
    set({ toast })
    window.setTimeout(() => set((state) => state.toast === toast ? { toast: '' } : state), 2600)
  },
  prefetchCandidates: async () => {
    const state = useAppStore.getState()
    const tasks: Promise<void>[] = []
    const engines = state.engineCandidates['']
    if (!engines || engines.status === 'idle') tasks.push(state.refreshEngineCandidates(''))
    if (state.sdkCandidates.status === 'idle') tasks.push(state.refreshSDKCandidates())
    await Promise.all(tasks)
  },
  refreshEngineCandidates: async (source = '') => {
    const current = useAppStore.getState().engineCandidates[source]
    if (current?.status === 'loading') return
    set((state) => ({ engineCandidates: { ...state.engineCandidates, [source]: { status: 'loading', items: current?.items || [], warnings: current?.warnings || [] } } }))
    try {
      const result = await api.ListAvailableVersions(source)
      set((state) => ({ engineCandidates: { ...state.engineCandidates, [source]: { status: 'ready', items: result.channels, warnings: result.warnings } } }))
    } catch (error) {
      set((state) => ({ engineCandidates: { ...state.engineCandidates, [source]: { status: 'error', items: current?.items || [], warnings: current?.warnings || [], error: error instanceof Error ? error.message : String(error) } } }))
    }
  },
  refreshSDKCandidates: async () => {
    if (useAppStore.getState().sdkCandidates.status === 'loading') return
    set((state) => ({ sdkCandidates: { ...state.sdkCandidates, status: 'loading' } }))
    try {
      const result = await api.ListAvailableSDKs()
      set({ sdkCandidates: { status: 'ready', items: result.channels, warnings: result.warnings } })
    } catch (error) {
      set((state) => ({ sdkCandidates: { status: 'error', items: state.sdkCandidates.items, warnings: state.sdkCandidates.warnings, error: error instanceof Error ? error.message : String(error) } }))
    }
  },
  handleSession: (session) => set((state) => {
    if (session.status === 'exited' || session.status === 'lost') {
      return {
        sessions: state.sessions.filter((item) => item.session_id !== session.session_id),
        sessionError: '',
        sessionRevision: state.sessionRevision + 1,
        terminalSessions: { ...state.terminalSessions, [session.session_id]: true },
      }
    }
    if (state.terminalSessions[session.session_id]) return {}
    const sessions = state.sessions.filter((item) => item.session_id !== session.session_id)
    sessions.push(session)
    sessions.sort((left, right) => right.started_at.localeCompare(left.started_at))
    return { sessions, sessionError: '', sessionRevision: state.sessionRevision + 1 }
  }),
  refreshSessions: async () => {
    if (sessionRefresh) return sessionRefresh
    const revision = get().sessionRevision
    set({ sessionsLoading: true })
    sessionRefresh = (async () => {
      try {
        const result = await api.ListSessions()
        set((state) => {
          if (state.sessionRevision !== revision) return { sessionsLoading: false, sessionError: '' }
          const sessions = result.sessions.filter((item) => (item.status === 'running' || item.status === 'stopping') && !state.terminalSessions[item.session_id])
          return { sessions, sessionsLoading: false, sessionError: '', sessionRevision: state.sessionRevision + 1 }
        })
      } catch (error) {
        set({ sessionsLoading: false, sessionError: error instanceof Error ? error.message : String(error) })
      } finally {
        sessionRefresh = null
      }
    })()
    return sessionRefresh
  },
  launchSession: async (name) => {
    try {
      const session = await api.LaunchSession(name)
      get().handleSession(session)
      return session
    } catch (error) {
      set({ sessionError: error instanceof Error ? error.message : String(error) })
      throw error
    }
  },
  requestStopSession: async (id) => {
    try {
      const session = await api.RequestStopSession(id)
      get().handleSession(session)
      return session
    } catch (error) {
      set({ sessionError: error instanceof Error ? error.message : String(error) })
      throw error
    }
  },
  forceStopSession: async (id) => {
    try {
      const session = await api.ForceStopSession(id)
      get().handleSession(session)
      return session
    } catch (error) {
      set({ sessionError: error instanceof Error ? error.message : String(error) })
      throw error
    }
  },
}))
