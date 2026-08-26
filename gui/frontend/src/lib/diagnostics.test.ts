import { describe, expect, it } from 'vitest'
import { doctorDiagnostics, visibleReminderCount } from './diagnostics'
import type { DoctorReport } from '../types'

describe('GUI doctor grouping', () => {
  it('excludes optional integration warnings from reminder counts without hiding broken shims', () => {
    const report: DoctorReport = {
      root: '/tmp/gdit', ok_count: 0, warn_count: 2, error_count: 1,
      items: [
        { code: 'shim', status: 'warn', message: 'godot shim 尚未创建' },
        { code: 'templates', status: 'warn', message: '模板尚未安装' },
        { code: 'shim', status: 'error', message: 'godot shim 已损坏' },
      ],
    }
    const entries = doctorDiagnostics(report)
    expect(entries.map((item) => item.group)).toEqual(['integration', 'attention', 'failure'])
    expect(visibleReminderCount(entries, {})).toBe(2)
    expect(visibleReminderCount(entries, { [entries[1].id]: true })).toBe(1)
  })
})
