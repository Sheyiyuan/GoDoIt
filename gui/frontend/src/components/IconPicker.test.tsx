import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { IconPicker } from './IconPicker'

describe('IconPicker', () => {
  it('updates and clears the icon background', () => {
    const onBackgroundChange = vi.fn()
    const { rerender } = render(<IconPicker value="csharp" background="" onChange={vi.fn()} onBackgroundChange={onBackgroundChange} onPickCustom={vi.fn()} />)

    fireEvent.change(screen.getByLabelText('图标背景色'), { target: { value: '#22c55e' } })
    expect(onBackgroundChange).toHaveBeenCalledWith('#22c55e')

    rerender(<IconPicker value="csharp" background="#22c55e" onChange={vi.fn()} onBackgroundChange={onBackgroundChange} onPickCustom={vi.fn()} />)
    fireEvent.click(screen.getByRole('button', { name: '恢复透明背景' }))
    expect(onBackgroundChange).toHaveBeenCalledWith('')
  })
})
