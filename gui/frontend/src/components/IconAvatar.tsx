import { useState } from 'react'
import { Image as ImageIcon } from 'lucide-react'
import type { InstanceInfo } from '../types'

interface Props {
  instance: InstanceInfo
  size?: 'small' | 'medium' | 'large'
}

export function IconAvatar({ instance, size = 'medium' }: Props) {
  const [customFailed, setCustomFailed] = useState(false)
  const resolved = customFailed || instance.icon_missing
    ? (instance.edition === 'dotnet' ? 'csharp' : 'godot')
    : instance.resolved_icon
  const className = `instance-avatar avatar-${resolved} avatar-${size}`
  const style = { backgroundColor: instance.icon_background || 'transparent' }

  if (resolved === 'custom') {
    return <span className={className} style={style}><img src={`/instance-icons/${instance.id}.png`} alt="" onError={() => setCustomFailed(true)} /></span>
  }
  if (resolved === 'mascot') {
    return <span className={className} style={style}><img src="/mascot.png" alt="" /></span>
  }
  if (resolved === 'csharp') {
    return <span className={className} style={style}><img src="/brand/csharp.svg" alt="C#" /></span>
  }
  if (resolved === 'godot') {
    return <span className={className} style={style}><img src="/brand/godot.svg" alt="Godot" /></span>
  }
  return <span className={className} style={style}><ImageIcon aria-hidden="true" /></span>
}
