import { createContext, useContext, useEffect } from 'react'

export type Crumb = { label: string; to?: string; mono?: boolean }

export const CrumbContext = createContext<(crumbs: Crumb[]) => void>(() => undefined)

// Each screen names where it sits; the shell shows it in the top bar.
export function useCrumbs(crumbs: Crumb[]) {
  const set = useContext(CrumbContext)
  const key = JSON.stringify(crumbs)
  useEffect(() => {
    set(JSON.parse(key) as Crumb[])
  }, [set, key])
}
