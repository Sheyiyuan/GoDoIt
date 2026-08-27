import { Eraser, Image, Sparkles } from 'lucide-react'
import type { IconStrategy } from '../types'

interface Props {
  value: IconStrategy
  customPath?: string
  background: string
  onChange: (value: IconStrategy) => void
  onBackgroundChange: (value: string) => void
  onPickCustom: () => void
}

export function IconPicker({ value, customPath, background, onChange, onBackgroundChange, onPickCustom }: Props) {
  const choices: Array<{ value: IconStrategy; label: string; image?: string; Icon?: typeof Sparkles }> = [
    { value: 'default', label: '缺省', Icon: Sparkles },
    { value: 'godot', label: 'Godot', image: '/brand/godot.svg' },
    { value: 'csharp', label: 'C#', image: '/brand/csharp.svg' },
    { value: 'mascot', label: '吉祥物', image: '/mascot.png' },
  ]
  return (
    <div className="icon-picker" role="radiogroup" aria-label="条目图标">
      {choices.map((choice) => <button key={choice.value} type="button" role="radio" aria-checked={value === choice.value} className={value === choice.value ? 'selected' : ''} onClick={() => onChange(choice.value)}>{choice.image ? <img src={choice.image} alt="" /> : choice.Icon ? <choice.Icon /> : null}<span>{choice.label}</span></button>)}
      <button type="button" role="radio" aria-checked={value === 'custom'} className={value === 'custom' ? 'selected' : ''} onClick={onPickCustom}><Image /><span>{customPath ? '已选择' : '自定义'}</span></button>
      <div className="icon-background-control">
        <label><span>背景色</span><input type="color" aria-label="图标背景色" value={background || '#ffffff'} onChange={(event) => onBackgroundChange(event.target.value)} /></label>
        <code>{background || '透明'}</code>
        <button className="icon-button" type="button" aria-label="恢复透明背景" title="恢复透明背景" disabled={!background} onClick={() => onBackgroundChange('')}><Eraser /></button>
      </div>
    </div>
  )
}
