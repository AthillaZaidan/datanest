import { useRef } from 'react'
import { LockIcon } from '../icons'

type Props = { value: string; onChange?: (v: string) => void; readOnly?: boolean; placeholder?: string; label?: string }

// A plain textarea with a line-number gutter; both share one line height and scroll together.
export default function SqlEditor({ value, onChange, readOnly, placeholder, label = 'migration.sql' }: Props) {
  const gutter = useRef<HTMLDivElement>(null)
  const lines = Math.max(value.split('\n').length, 12)

  return (
    <div className="overflow-hidden rounded-lg border border-line focus-within:border-accent focus-within:ring-2 focus-within:ring-accent/20">
      <div className="flex h-9 items-center justify-between border-b border-line bg-raised px-3 text-xs text-muted">
        <span className="font-mono">{label}</span>
        {readOnly ? (
          <span className="inline-flex items-center gap-1"><LockIcon size={12} /> hanya baca</span>
        ) : (
          <span className="num">{value.split('\n').length} baris</span>
        )}
      </div>
      <div className="flex bg-sheet">
        <div
          ref={gutter}
          aria-hidden="true"
          className="num h-80 w-11 flex-none overflow-hidden bg-raised py-3 pr-2.5 text-right font-mono text-xs leading-6 text-muted select-none"
        >
          {Array.from({ length: lines }, (_, i) => (
            <div key={i}>{i + 1}</div>
          ))}
        </div>
        <textarea
          aria-label="SQL migration"
          className="h-80 min-w-0 flex-1 resize-none bg-sheet px-3 py-3 font-mono text-[13px] leading-6 text-ink placeholder:text-muted focus:outline-none read-only:text-neutral-ink"
          value={value}
          readOnly={readOnly}
          spellCheck={false}
          placeholder={placeholder}
          wrap="off"
          onScroll={(e) => {
            if (gutter.current) gutter.current.scrollTop = e.currentTarget.scrollTop
          }}
          onChange={(e) => onChange?.(e.target.value)}
        />
      </div>
    </div>
  )
}
