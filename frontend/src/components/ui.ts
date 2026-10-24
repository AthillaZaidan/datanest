// Shared Tailwind class strings so every screen uses the same controls and tables.
const control =
  'inline-flex items-center justify-center gap-1.5 whitespace-nowrap rounded-md font-medium transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent disabled:pointer-events-none disabled:opacity-50'
const md = 'h-8 px-3 text-sm'
const sm = 'h-7 px-2 text-xs'
const primary = 'bg-ink text-white hover:bg-[#2b2d30] active:bg-black'
const secondary = 'border border-control bg-sheet text-ink hover:bg-raised active:bg-active'
const danger = 'border border-control bg-sheet text-negative hover:bg-neg-bg active:bg-neg-bg'
const ghost = 'text-muted hover:bg-raised hover:text-ink active:bg-active'
const field =
  'rounded-lg border border-line bg-sheet text-sm text-ink placeholder:text-muted focus:border-accent focus:ring-2 focus:ring-accent/20 focus:outline-none disabled:bg-raised'

export const ui = {
  button: `${control} ${md} ${secondary}`,
  buttonPrimary: `${control} ${md} ${primary}`,
  buttonDanger: `${control} ${md} ${danger}`,
  buttonGhost: `${control} ${md} ${ghost}`,
  buttonSm: `${control} ${sm} ${secondary}`,
  buttonSmDanger: `${control} ${sm} ${danger}`,
  buttonSmGhost: `${control} ${sm} ${ghost}`,
  input: `${field} block h-10 w-full px-3`,
  select: `${field} h-8 border-control pr-8 pl-2.5`,
  selectForm: `${field} block h-10 w-full pr-8 pl-3`,
  label: 'mb-2 block text-sm font-medium',
  well: 'rounded-lg bg-raised p-4',
  panel: 'rounded-lg border border-line bg-sheet',
  link: 'font-medium text-ink underline-offset-2 hover:underline',
  table: 'w-full border-collapse text-left text-sm',
  th: 'h-10 px-3 text-xs font-normal whitespace-nowrap text-muted first:pl-0 last:pr-0',
  td: 'h-12 border-t border-rule px-3 align-middle first:pl-0 last:pr-0',
  num: 'num text-right',
  ident: 'font-mono text-[13px]',
  muted: 'text-sm text-muted',
  small: 'text-xs text-muted',
  error: 'text-sm text-negative',
}
