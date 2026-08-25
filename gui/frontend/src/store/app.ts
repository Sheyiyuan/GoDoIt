import { create } from 'zustand'
import { api } from '../lib/api'
import type { AppSnapshot, ProgressEnvelope } from '../types'

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
  operations: Record<string, ProgressEnvelope>
  modal: ModalState | null
  toast: string
  load: () => Promise<void>
  handleProgress: (event: ProgressEnvelope) => void
  openModal: (modal: ModalState) => void
  closeModal: () => void
  notify: (message: string) => void
}

export const useAppStore = create<AppState>((set) => ({
  snapshot: null,
  loading: true,
  error: '',
  operations: {},
  modal: null,
  toast: '',
  load: async () => {
    set({ loading: true, error: '' })
    try {
      const snapshot = await api.Bootstrap()
      set({ snapshot, loading: false })
    } catch (error) {
      set({ loading: false, error: error instanceof Error ? error.message : String(error) })
    }
  },
  handleProgress: (event) => set((state) => ({ operations: { ...state.operations, [event.operation_id]: event } })),
  openModal: (modal) => set({ modal }),
  closeModal: () => set({ modal: null }),
  notify: (toast) => {
    set({ toast })
    window.setTimeout(() => set((state) => state.toast === toast ? { toast: '' } : state), 2600)
  },
}))
