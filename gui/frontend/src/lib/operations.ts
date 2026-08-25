import type { OperationStart, ProgressEnvelope } from '../types'

type Listener = (event: ProgressEnvelope) => void
type Waiter = { resolve: (value: unknown) => void; reject: (error: Error) => void }

const listeners = new Set<Listener>()
const waiters = new Map<string, Waiter>()
const completed = new Map<string, ProgressEnvelope>()
let initialised = false

function dispatch(event: ProgressEnvelope) {
  listeners.forEach((listener) => listener(event))
  if (event.status === 'running') return
  const waiter = waiters.get(event.operation_id)
  if (!waiter) {
    completed.set(event.operation_id, event)
    return
  }
  waiters.delete(event.operation_id)
  settle(waiter, event)
}

function settle(waiter: Waiter, event: ProgressEnvelope) {
  if (event.status === 'complete') {
    waiter.resolve(event.result)
  } else {
    waiter.reject(new Error(event.error || (event.status === 'canceled' ? '操作已取消' : '操作失败')))
  }
}

function initialise() {
  if (initialised || typeof window === 'undefined') return
  initialised = true
  if (window.runtime?.EventsOn) {
    window.runtime.EventsOn('gdit:progress', (payload) => dispatch(payload as ProgressEnvelope))
  }
  window.addEventListener('gdit:progress', ((event: CustomEvent<ProgressEnvelope>) => dispatch(event.detail)) as EventListener)
}

export function subscribeProgress(listener: Listener) {
  initialise()
  listeners.add(listener)
  return () => { listeners.delete(listener) }
}

export async function runOperation<T>(started: Promise<OperationStart>): Promise<T> {
  initialise()
  const operation = await started
  const terminal = completed.get(operation.operation_id)
  if (terminal) {
    completed.delete(operation.operation_id)
    return new Promise<T>((resolve, reject) => settle({ resolve: resolve as (value: unknown) => void, reject }, terminal))
  }
  return new Promise<T>((resolve, reject) => {
    waiters.set(operation.operation_id, { resolve: resolve as (value: unknown) => void, reject })
  })
}
