import { useCallback, useEffect, useRef } from 'react'
import { X } from 'lucide-react'
import { useAppStore } from '../store/app'

export function Modal() {
  const modal = useAppStore((state) => state.modal)
  const close = useAppStore((state) => state.closeModal)
  const confirmButton = useRef<HTMLButtonElement>(null)
  const confirming = useRef(false)

  const confirm = useCallback(async () => {
    if (!modal || confirming.current) return
    confirming.current = true
    close()
    await modal.onConfirm()
  }, [close, modal])

  useEffect(() => {
    if (!modal) return
    confirming.current = false
    confirmButton.current?.focus()
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault()
        close()
      } else if (event.key === 'Enter' && !event.repeat) {
        event.preventDefault()
        void confirm()
      }
    }
    document.addEventListener('keydown', handleKeyDown)
    return () => document.removeEventListener('keydown', handleKeyDown)
  }, [close, confirm, modal])

  if (!modal) return null

  return (
    <div className="modal-backdrop" role="presentation" onMouseDown={(event) => event.target === event.currentTarget && close()}>
      <section className="modal-dialog" role="dialog" aria-modal="true" aria-labelledby="modal-title">
        <header><h2 id="modal-title">{modal.title}</h2><button className="icon-button" type="button" onClick={close} aria-label="关闭"><X /></button></header>
        <p>{modal.body}</p>
        <footer><button className="button secondary" type="button" onClick={close}>取消</button><button ref={confirmButton} className={`button ${modal.tone === 'danger' ? 'danger' : 'primary'}`} type="button" onClick={() => void confirm()}>{modal.confirmLabel}</button></footer>
      </section>
    </div>
  )
}
