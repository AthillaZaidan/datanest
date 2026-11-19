import type { ReactNode } from 'react'
import { personTint } from '../utils/people'
import { ui } from './ui'

type Tone = 'neutral' | 'positive' | 'negative' | 'warning'

const pillTones: Record<Tone, string> = {
  neutral: 'bg-neutral-bg text-neutral-ink',
  positive: 'bg-pos-bg text-pos-ink',
  negative: 'bg-neg-bg text-neg-ink',
  warning: 'bg-warn-bg text-warning',
}

export function Pill({ tone = 'neutral', children }: { tone?: Tone; children: ReactNode }) {
  return (
    <span className={`inline-flex h-5 flex-none items-center rounded px-1.5 text-xs font-medium whitespace-nowrap ${pillTones[tone]}`}>
      {children}
    </span>
  )
}

export function LevelPill({ level }: { level?: string }) {
  if (!level || level === 'NONE') return <span className="text-muted">-</span>
  return <Pill>{level}</Pill>
}

export function Avatar({ id, name }: { id: number; name: string }) {
  return (
    <span className="grid size-7 flex-none place-items-center rounded-full text-xs font-semibold uppercase" style={personTint(id)}>
      {name.slice(0, 1)}
    </span>
  )
}

type HeaderProps = { title: ReactNode; subtitle?: ReactNode; badges?: ReactNode; actions?: ReactNode }

export function PageHeader({ title, subtitle, badges, actions }: HeaderProps) {
  return (
    <header className="mb-6 flex flex-wrap items-start justify-between gap-x-6 gap-y-3">
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <h1 className="text-[22px] leading-8 font-semibold tracking-[-0.01em]">{title}</h1>
          {badges}
        </div>
        {subtitle && <p className="mt-1 text-sm text-muted">{subtitle}</p>}
      </div>
      {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
    </header>
  )
}

export function Section({ title, aside, actions, children, className = '' }: { title: ReactNode; aside?: ReactNode; actions?: ReactNode; children: ReactNode; className?: string }) {
  return (
    <section className={className}>
      <div className="mb-2 flex min-h-8 flex-wrap items-center justify-between gap-3">
        <h2 className="text-base font-semibold">
          {title}
          {aside !== undefined && <span className="ml-2 text-sm font-normal text-muted">{aside}</span>}
        </h2>
        {actions && <div className="flex items-center gap-2">{actions}</div>}
      </div>
      {children}
    </section>
  )
}

const alertTones = {
  error: 'bg-neg-bg text-neg-ink',
  warning: 'border border-warn-line bg-warn-bg text-warning',
  success: 'bg-pos-bg text-pos-ink',
  info: 'bg-raised text-ink',
}

export function Alert({ tone = 'error', children, action }: { tone?: keyof typeof alertTones; children: ReactNode; action?: ReactNode }) {
  return (
    <div role={tone === 'error' ? 'alert' : 'status'} className={`mb-4 flex flex-wrap items-center gap-3 rounded-lg px-3 py-2.5 text-sm ${alertTones[tone]}`}>
      <div className="min-w-0 flex-1">{children}</div>
      {action}
    </div>
  )
}

export function Empty({ children, action }: { children: ReactNode; action?: ReactNode }) {
  return (
    <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-dashed border-control px-4 py-3 text-sm text-muted">
      <span>{children}</span>
      {action}
    </div>
  )
}

// Loading keeps the final layout: grey bars at the size of the rows they stand for.
export function Loading({ rows = 4, label = 'Memuat' }: { rows?: number; label?: string }) {
  return (
    <div aria-busy="true" aria-label={label} className="space-y-0">
      {Array.from({ length: rows }, (_, i) => (
        <div key={i} className="flex h-12 items-center gap-6 border-t border-rule first:border-t-0">
          <span className="h-3.5 w-40 rounded bg-raised" />
          <span className="h-3.5 w-24 rounded bg-raised" />
          <span className="ml-auto h-3.5 w-16 rounded bg-raised" />
        </div>
      ))}
    </div>
  )
}

export function Field({ label, hint, children }: { label: ReactNode; hint?: ReactNode; children: ReactNode }) {
  return (
    <label className="block">
      <span className={ui.label}>{label}</span>
      {children}
      {hint && <span className="mt-1.5 block text-xs text-muted">{hint}</span>}
    </label>
  )
}

type TabItem<K extends string> = { key: K; label: ReactNode; count?: number }

export function Tabs<K extends string>({ items, value, onChange, label }: { items: TabItem<K>[]; value: K; onChange: (key: K) => void; label: string }) {
  return (
    <div role="tablist" aria-label={label} className="flex gap-5 overflow-x-auto border-b border-line">
      {items.map((t) => (
        <button
          key={t.key}
          role="tab"
          aria-selected={t.key === value}
          onClick={() => onChange(t.key)}
          className={`-mb-px flex h-9 flex-none items-center gap-1.5 border-b-2 text-sm transition-colors focus-visible:outline-2 focus-visible:outline-accent ${
            t.key === value ? 'border-ink font-semibold text-ink' : 'border-transparent text-muted hover:text-ink'
          }`}
        >
          {t.label}
          {t.count !== undefined && <span className="num text-xs font-normal text-muted">{t.count}</span>}
        </button>
      ))}
    </div>
  )
}

export function Meta({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex items-baseline justify-between gap-4 border-t border-rule py-2.5 first:border-t-0">
      <dt className="text-sm text-muted">{label}</dt>
      <dd className="text-right text-sm">{children}</dd>
    </div>
  )
}
