import { act, cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useAppStore } from '../store/app'
import { Modal } from './Modal'

afterEach(() => {
  cleanup()
  act(() => useAppStore.getState().closeModal())
})

describe('Modal keyboard controls', () => {
  it('closes with Escape and confirms once with Enter', async () => {
    const user = userEvent.setup()
    const confirm = vi.fn()
    render(<Modal />)

    act(() => useAppStore.getState().openModal({ title: '确认操作', body: '固定测试', confirmLabel: '确认', onConfirm: confirm }))
    expect(await screen.findByRole('dialog')).toBeInTheDocument()
    await user.keyboard('{Escape}')
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(confirm).not.toHaveBeenCalled()

    act(() => useAppStore.getState().openModal({ title: '确认操作', body: '固定测试', confirmLabel: '确认', onConfirm: confirm }))
    await user.keyboard('{Enter}')
    expect(confirm).toHaveBeenCalledTimes(1)
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })
})
