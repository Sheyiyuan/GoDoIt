import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from '../lib/api'
import { useAppStore } from '../store/app'
import { EnvironmentEditor } from './EnvironmentEditor'

afterEach(() => {
  vi.restoreAllMocks()
  useAppStore.setState({ modal: null, toast: '' })
})

describe('EnvironmentEditor', () => {
  it('masks sensitive values and only edits the requested instance scope', async () => {
    vi.spyOn(api, 'GetEnvironment').mockResolvedValue({
      configured: { vars: [
        { key: 'GLOBAL_VALUE', value: 'global', scope: 'global', editable: true, sensitive: false },
        { key: 'DISPLAY_BACKEND', value: 'wayland', scope: 'platform', editable: false, sensitive: false },
        { key: 'INSTANCE_TOKEN', value: 'instance-secret', scope: 'instance', editable: true, sensitive: true },
      ] },
      effective: { vars: [{ key: 'INSTANCE_TOKEN', value: 'instance-secret', origin: 'instance', sensitive: true }], args: [] },
    })
    const setEnv = vi.spyOn(api, 'SetEnvVar').mockResolvedValue()

    render(<EnvironmentEditor name="studio" editableScope="instance" />)
    const list = await screen.findByLabelText('环境变量列表')
    const configuredSection = within(list).getByRole('heading', { name: '配置层' }).closest('section')
    expect(configuredSection).not.toBeNull()
    fireEvent.click(within(configuredSection!).getByRole('button', { name: /INSTANCE_TOKEN/ }))
    expect(screen.getByDisplayValue('••••••••••••')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '显示 INSTANCE_TOKEN' }))
    expect(screen.getByDisplayValue('instance-secret')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: '编辑' }))
    fireEvent.change(screen.getByLabelText('变量值'), { target: { value: 'changed-secret' } })
    fireEvent.click(screen.getByRole('button', { name: '保存' }))
    await waitFor(() => expect(setEnv).toHaveBeenCalledWith('studio', 'INSTANCE_TOKEN', 'changed-secret'))

    fireEvent.click(screen.getByRole('button', { name: /GLOBAL_VALUE/ }))
    expect(screen.queryByRole('button', { name: '编辑' })).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /DISPLAY_BACKEND/ }))
    expect(screen.queryByRole('button', { name: '编辑' })).not.toBeInTheDocument()
  })
})
