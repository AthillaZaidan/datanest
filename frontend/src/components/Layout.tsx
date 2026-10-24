import { Fragment, useState } from 'react'
import type { ReactNode } from 'react'
import { Link, NavLink, Outlet, useLocation } from 'react-router'
import { useAuth } from '../context/AuthContext'
import { CrumbContext } from '../context/crumbs'
import type { Crumb } from '../context/crumbs'
import { CloseIcon, DatabaseIcon, LogoutIcon, MenuIcon, Mark, ServerIcon, UserIcon, UsersIcon } from './icons'
import { Avatar } from './kit'
import { ui } from './ui'

/*
  Shell: one tinted L-frame (228px sidebar + 48px top bar, no rule between them) around a white sheet inset 8px
  from the window's right and bottom edges. Below 1024px the frame folds: the sheet becomes the screen under a
  48px bar with a menu button.
*/

const link = ({ isActive }: { isActive: boolean }) =>
  `flex h-8 items-center gap-2.5 rounded-lg px-2.5 text-sm transition-colors ${
    isActive ? 'bg-active text-ink' : 'text-muted hover:bg-active/60 hover:text-ink'
  }`

function NavItem({ to, icon, children }: { to: string; icon: ReactNode; children: ReactNode }) {
  return (
    <NavLink to={to} className={link}>
      {icon}
      {children}
    </NavLink>
  )
}

export function Wordmark() {
  return (
    <Link to="/databases" className="flex items-center gap-2 rounded-md focus-visible:outline-2 focus-visible:outline-accent">
      <Mark />
      <span className="text-[15px] font-semibold tracking-[-0.01em]">Datanest</span>
    </Link>
  )
}

function Nav({ isAdmin }: { isAdmin: boolean }) {
  return (
    <nav className="flex flex-col gap-0.5" aria-label="Navigasi utama">
      <NavItem to="/databases" icon={<DatabaseIcon />}>Databases</NavItem>
      {isAdmin && (
        <>
          <p className="mt-6 mb-1.5 px-2.5 text-xs text-muted">Administrasi</p>
          <NavItem to="/admin/users" icon={<UserIcon />}>Users</NavItem>
          <NavItem to="/admin/groups" icon={<UsersIcon />}>Groups</NavItem>
          <NavItem to="/admin/databases" icon={<ServerIcon />}>Koneksi database</NavItem>
        </>
      )}
    </nav>
  )
}

function Breadcrumb({ crumbs }: { crumbs: Crumb[] }) {
  return (
    <ol className="flex min-w-0 items-center gap-1.5 overflow-hidden text-sm">
      {crumbs.map((c, i) => {
        const last = i === crumbs.length - 1
        const text = <span className={`truncate ${c.mono ? 'font-mono text-[13px]' : ''}`}>{c.label}</span>
        return (
          <Fragment key={i}>
            {i > 0 && <li aria-hidden="true" className={`text-control ${i <= crumbs.length - 2 ? 'hidden sm:block' : ''}`}>/</li>}
            <li
              className={`min-w-0 ${last ? 'text-ink' : 'text-muted'} ${i < crumbs.length - 2 ? 'hidden sm:block' : ''}`}
              aria-current={last ? 'page' : undefined}
            >
              {c.to && !last ? <Link to={c.to} className="hover:text-ink">{text}</Link> : text}
            </li>
          </Fragment>
        )
      })}
    </ol>
  )
}

export default function Layout() {
  const { user, logout } = useAuth()
  const [crumbs, setCrumbs] = useState<Crumb[]>([])
  // The menu belongs to the path it was opened on, so navigating closes it.
  const { pathname } = useLocation()
  const [menuPath, setMenuPath] = useState<string | null>(null)
  const menu = menuPath === pathname
  const setMenu = (open: boolean) => setMenuPath(open ? pathname : null)

  const account = user && (
    <div className="flex items-center gap-2.5">
      <Avatar id={user.id} name={user.username} />
      <div className="leading-tight">
        <p className="text-sm font-medium">{user.username}</p>
        <p className="text-xs text-muted">{user.isAdmin ? 'Platform Admin' : 'User'}</p>
      </div>
      <button className={`${ui.buttonSmGhost} ml-1`} onClick={() => void logout()} title="Logout" aria-label="Logout">
        <LogoutIcon />
      </button>
    </div>
  )

  return (
    <CrumbContext.Provider value={setCrumbs}>
      <div className="flex h-dvh flex-col lg:grid lg:grid-cols-[228px_minmax(0,1fr)] lg:grid-rows-[48px_minmax(0,1fr)]">
        <div className="hidden items-center px-4 lg:flex">
          <Wordmark />
        </div>
        <header className="flex h-12 flex-none items-center gap-3 px-4 lg:pr-6 lg:pl-6">
          <button className={`${ui.buttonSmGhost} -ml-1 size-9 lg:hidden`} onClick={() => setMenu(true)} aria-label="Buka menu">
            <MenuIcon size={18} />
          </button>
          <span className="lg:hidden"><Mark size={20} /></span>
          <Breadcrumb crumbs={crumbs} />
          <div className="ml-auto hidden lg:block">{account}</div>
        </header>
        <aside className="hidden px-3 pt-2 lg:block">
          <Nav isAdmin={!!user?.isAdmin} />
        </aside>
        <main className="min-h-0 flex-1 overflow-y-auto bg-sheet lg:mr-2 lg:mb-2 lg:rounded-2xl lg:border lg:border-line">
          <div className="mx-auto max-w-[1280px] px-4 py-5 sm:px-6 lg:py-6">
            <Outlet />
          </div>
        </main>
      </div>

      {menu && (
        <div className="fixed inset-0 z-50 flex flex-col bg-frame lg:hidden" role="dialog" aria-modal="true" aria-label="Menu">
          <div className="flex h-12 items-center justify-between px-4">
            <Wordmark />
            <button className={`${ui.buttonSmGhost} size-9`} onClick={() => setMenu(false)} aria-label="Tutup menu">
              <CloseIcon size={18} />
            </button>
          </div>
          <div className="flex-1 overflow-y-auto px-3 pt-2">
            <Nav isAdmin={!!user?.isAdmin} />
          </div>
          <div className="border-t border-line px-4 py-3">{account}</div>
        </div>
      )}
    </CrumbContext.Provider>
  )
}
