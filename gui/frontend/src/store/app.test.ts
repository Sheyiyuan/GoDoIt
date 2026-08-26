import { beforeEach, describe, expect, it } from 'vitest'
import { useAppStore } from './app'
import type { ProgressEnvelope } from '../types'

const running = (progress: ProgressEnvelope['progress']): ProgressEnvelope => ({ operation_id: 'fixture', operation: 'install-entry', status: 'running', timestamp: '2026-08-26T01:00:00Z', progress })

beforeEach(() => useAppStore.setState({ operations: {}, dismissedWarnings: {}, sessions: [], sessionRevision: 0, terminalSessions: {} }))

describe('operation store contract', () => {
  it('merges multiple assets and fallback sources without overwriting subtasks', () => {
    const handle = useAppStore.getState().handleProgress
    handle(running({ stage: 'download', version: '4.7.2-dotnet', filename: 'godot.zip', source: 'godothub', bytes_downloaded: 2, total_bytes: 10 }))
    handle(running({ stage: 'download', version: '4.7.2-dotnet', filename: 'godot.zip', source: 'github', bytes_downloaded: 10, total_bytes: 10 }))
    handle(running({ stage: 'download', version: '8.0.410(sdk)', filename: 'dotnet.tar.gz', source: 'dotnet-official', bytes_downloaded: 20, total_bytes: 20 }))

    const items = useAppStore.getState().operations.fixture.items
    expect(items.map((item) => item.key)).toEqual([
      '4.7.2-dotnet|godot.zip|godothub',
      '4.7.2-dotnet|godot.zip|github',
      '8.0.410(sdk)|dotnet.tar.gz|dotnet-official',
    ])
  })

  it('accepts only the first terminal and retains a safe result summary', () => {
    const handle = useAppStore.getState().handleProgress
    handle(running({ stage: 'queued', message: '读取 /Users/demo/private/project.godot token=plain https://example.test/file?key=plain' }))
    handle({
      operation_id: 'fixture', operation: 'install-entry', status: 'complete', timestamp: '2026-08-26T01:02:00Z',
      result: {
        instance: { name: 'studio', engine: '4.7.2-dotnet', sdk: '8.0.410', current: true, env: { API_TOKEN: 'plain' } },
        installed: [{ kind: 'engine', id: '4.7.2-dotnet', path: '/Users/demo/.gdit/engines/4.7.2-dotnet' }],
        project_dir: '/Users/demo/private',
      },
    })
    handle({ operation_id: 'fixture', operation: 'install-entry', status: 'failed', timestamp: '2026-08-26T01:03:00Z', error: 'late terminal' })

    const item = useAppStore.getState().operations.fixture
    expect(item.status).toBe('complete')
    expect(item.finished_at).toBe('2026-08-26T01:02:00Z')
    expect(item.result_summary).toEqual(['条目：studio', '引擎：4.7.2-dotnet', '.NET SDK：8.0.410', '已设为当前条目', '已安装引擎：4.7.2-dotnet'])
    const serialized = JSON.stringify(item)
    expect(serialized).not.toContain('/Users/demo')
    expect(serialized).not.toContain('plain')
    expect(serialized).not.toContain('example.test')
  })

  it('clears completed history from the current window', () => {
    const handle = useAppStore.getState().handleProgress
    handle({ operation_id: 'fixture', operation: 'doctor', status: 'complete', timestamp: '2026-08-26T01:00:00Z', result: { ok_count: 2, warn_count: 0, error_count: 0, root: '/private/root' } })
    useAppStore.getState().clearOperation('fixture')
    expect(useAppStore.getState().operations).toEqual({})
  })
})

describe('session store contract', () => {
  it('upserts active sessions and removes terminal sessions', () => {
    const handle = useAppStore.getState().handleSession
    const running = { session_id: 'session-1', instance_id: 'instance-1', instance_name: 'studio', engine_id: '4.5.2-standard', pid: 42, started_at: '2026-08-26T01:00:00Z', status: 'running' as const }
    handle(running)
    handle({ ...running, status: 'stopping' })

    expect(useAppStore.getState().sessions).toEqual([{ ...running, status: 'stopping' }])
    handle({ ...running, status: 'exited' })
    handle(running)
    expect(useAppStore.getState().sessions).toEqual([])
  })
})
