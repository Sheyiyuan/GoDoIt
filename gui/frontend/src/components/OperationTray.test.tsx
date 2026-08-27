import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { useAppStore } from '../store/app'
import { OperationCenter } from './OperationCenter'
import { OperationTray } from './OperationTray'

afterEach(() => {
  cleanup()
  act(() => useAppStore.setState({ operations: {} }))
})

describe('OperationTray download progress', () => {
  it('shows downloaded and total sizes for every file', () => {
    act(() => useAppStore.setState({
      operations: {
        fixture: {
          id: 'fixture',
          operation: 'install-entry',
          status: 'running',
          started_at: '2026-08-26T00:00:00Z',
          summary: '正在下载',
          result_summary: [],
          items: [
            { key: 'engine', version: '4.7.2-dotnet', source: 'godothub', filename: 'godot.zip', stage: 'download', bytes_downloaded: 1024 ** 2, total_bytes: 4 * 1024 ** 2 },
            { key: 'sdk', version: '8.0.410(sdk)', source: 'dotnet-official', filename: 'dotnet-sdk.tar.gz', stage: 'download', bytes_downloaded: 3 * 1024 ** 2, total_bytes: 6 * 1024 ** 2 },
          ],
        },
      },
    }))

    const open = vi.fn()
    render(<OperationTray onOpen={open} />)

    expect(screen.getByText('安装条目 40%')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /安装条目 40%/ }))
    expect(open).toHaveBeenCalledOnce()
  })

  it('keeps unknown sizes stable in the full operation center', () => {
    act(() => useAppStore.setState({
      operations: {
        fixture: {
          id: 'fixture', operation: 'install-entry', status: 'running', started_at: '2026-08-26T00:00:00Z', summary: '正在下载', result_summary: [],
          items: [
            { key: 'engine', version: '4.7.2-standard', source: 'github', filename: 'godot.zip', stage: 'download', bytes_downloaded: 1024, total_bytes: 0 },
            { key: 'template', version: '4.7.2-standard(template)', source: 'github', filename: 'templates.tpz', stage: 'download', bytes_downloaded: 2048, total_bytes: 4096 },
          ],
        },
      },
    }))

    render(<MemoryRouter><OperationCenter open onClose={() => undefined} /></MemoryRouter>)

    expect(screen.getByText('1 KiB / 大小未知')).toBeInTheDocument()
    expect(screen.getByText('2 KiB / 4 KiB')).toBeInTheDocument()
    expect(screen.getAllByRole('progressbar')).toHaveLength(2)
    expect(screen.queryByText(/安装条目 \d+%/)).not.toBeInTheDocument()
  })
})
