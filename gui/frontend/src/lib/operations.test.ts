import { describe, expect, it } from 'vitest'
import { runOperation, subscribeProgress } from './operations'
import type { ProgressEnvelope } from '../types'

function emit(event: ProgressEnvelope) {
  window.dispatchEvent(new CustomEvent('gdit:progress', { detail: event }))
}

describe('operation waiters', () => {
  it('handles a terminal arriving before waiter registration', async () => {
    const unsubscribe = subscribeProgress(() => undefined)
    emit({ operation_id: 'early-terminal', operation: 'doctor', status: 'complete', timestamp: '2026-08-26T00:00:00Z', result: { ok: true } })
    await expect(runOperation(Promise.resolve({ operation_id: 'early-terminal' }))).resolves.toEqual({ ok: true })
    unsubscribe()
  })

  it('releases a settled waiter', async () => {
    const first = runOperation<{ sequence: number }>(Promise.resolve({ operation_id: 'reused-fixture-id' }))
    await Promise.resolve()
    emit({ operation_id: 'reused-fixture-id', operation: 'doctor', status: 'complete', timestamp: '2026-08-26T00:00:00Z', result: { sequence: 1 } })
    await expect(first).resolves.toEqual({ sequence: 1 })

    emit({ operation_id: 'reused-fixture-id', operation: 'doctor', status: 'complete', timestamp: '2026-08-26T00:01:00Z', result: { sequence: 2 } })
    await expect(runOperation(Promise.resolve({ operation_id: 'reused-fixture-id' }))).resolves.toEqual({ sequence: 2 })
  })
})
