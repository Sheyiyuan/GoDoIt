export function BrandMark({ large = false }: { large?: boolean }) {
  return <span className={`brand-mark ${large ? 'brand-mark-large' : ''}`}><img src="/logo.svg" alt="GoDoIt" /></span>
}
