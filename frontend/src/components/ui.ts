// Shared Tailwind class strings so pages look consistent.
const buttonBase = 'rounded px-3 py-1 text-sm disabled:cursor-not-allowed disabled:opacity-40'

export const ui = {
  h1: 'mb-4 flex flex-wrap items-baseline gap-3 text-2xl font-semibold',
  h2: 'mb-3 text-xl font-semibold',
  h3: 'mt-6 mb-2 text-lg font-semibold',
  link: 'text-sm font-normal text-blue-600 hover:underline dark:text-blue-400',
  button: `${buttonBase} border border-neutral-500/40 hover:bg-neutral-500/10`,
  buttonPrimary: `${buttonBase} bg-blue-600 text-white hover:bg-blue-700`,
  buttonDanger: `${buttonBase} border border-red-600/50 text-red-600 hover:bg-red-600/10 dark:text-red-400`,
  select: 'rounded border border-neutral-500/40 bg-transparent px-2 py-1 text-sm',
  table: 'mb-4 w-full border-collapse text-sm',
  th: 'border border-neutral-500/30 bg-neutral-500/10 px-2 py-1 text-left font-medium',
  td: 'border border-neutral-500/30 px-2 py-1 align-top',
  code: 'font-mono text-xs',
  error: 'text-sm text-red-600 dark:text-red-400',
  muted: 'text-sm text-neutral-500',
}
