// Shared Tailwind class strings so pages look consistent.
const buttonBase =
  'inline-flex items-center justify-center gap-1.5 whitespace-nowrap rounded-lg font-medium transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500 disabled:pointer-events-none disabled:opacity-50'
const md = 'px-3 py-1.5 text-sm'
const sm = 'px-2 py-1 text-xs'
const secondary =
  'border border-zinc-300 bg-white text-zinc-800 shadow-xs hover:bg-zinc-50 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-100 dark:hover:bg-zinc-800'
const primary = 'bg-indigo-600 text-white shadow-xs hover:bg-indigo-500'
const danger =
  'border border-red-200 bg-white text-red-600 hover:bg-red-50 dark:border-red-900/60 dark:bg-zinc-900 dark:text-red-400 dark:hover:bg-red-950/40'
const ghost =
  'text-zinc-600 hover:bg-zinc-100 hover:text-zinc-900 dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-100'
const field =
  'rounded-lg border border-zinc-300 bg-white px-3 py-1.5 text-sm shadow-xs placeholder:text-zinc-400 focus:border-indigo-500 focus:ring-2 focus:ring-indigo-500/20 focus:outline-none disabled:opacity-60 dark:border-zinc-700 dark:bg-zinc-900 dark:placeholder:text-zinc-500'

export const ui = {
  button: `${buttonBase} ${md} ${secondary}`,
  buttonPrimary: `${buttonBase} ${md} ${primary}`,
  buttonDanger: `${buttonBase} ${md} ${danger}`,
  buttonGhost: `${buttonBase} ${md} ${ghost}`,
  buttonSm: `${buttonBase} ${sm} ${secondary}`,
  buttonSmDanger: `${buttonBase} ${sm} ${danger}`,
  buttonSmGhost: `${buttonBase} ${sm} ${ghost}`,
  input: `${field} block w-full`,
  select: `${field} pr-8`,
  selectSm:
    'rounded-md border border-zinc-300 bg-white py-1 pr-7 pl-2 text-xs shadow-xs focus:border-indigo-500 focus:outline-none dark:border-zinc-700 dark:bg-zinc-900',
  label: 'mb-1 block text-xs font-medium text-zinc-600 dark:text-zinc-400',
  card: 'rounded-xl border border-zinc-200 bg-white shadow-xs dark:border-zinc-800 dark:bg-zinc-900',
  link: 'font-medium text-indigo-600 hover:text-indigo-500 dark:text-indigo-400 dark:hover:text-indigo-300',
  tableWrap: 'overflow-x-auto',
  table: 'w-full border-collapse text-left text-sm',
  th: 'border-b border-zinc-200 bg-zinc-50 px-4 py-2 text-xs font-medium tracking-wide text-zinc-500 uppercase dark:border-zinc-800 dark:bg-zinc-900/60 dark:text-zinc-400',
  td: 'border-b border-zinc-100 px-4 py-2.5 align-top dark:border-zinc-800/70',
  tr: 'transition-colors hover:bg-zinc-50 dark:hover:bg-zinc-800/40',
  code: 'font-mono text-xs',
  chip: 'rounded-md bg-zinc-100 px-1.5 py-0.5 font-mono text-xs text-zinc-700 dark:bg-zinc-800 dark:text-zinc-300',
  muted: 'text-sm text-zinc-500 dark:text-zinc-400',
  error: 'text-sm text-red-600 dark:text-red-400',
}
