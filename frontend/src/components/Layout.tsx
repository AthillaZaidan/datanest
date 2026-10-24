import type { ReactNode } from 'react'
import { NavLink, Outlet } from 'react-router'
import { useAuth } from '../context/AuthContext'
import { DatabaseIcon, LogoutIcon, ServerIcon, UserIcon, UsersIcon } from './icons'
import { ui } from './ui'

const link = ({ isActive }: { isActive: boolean }) =>
  `flex items-center gap-2.5 whitespace-nowrap rounded-lg px-2.5 py-1.5 text-sm transition-colors ${
    isActive
      ? 'bg-indigo-50 font-medium text-indigo-700 dark:bg-indigo-500/10 dark:text-indigo-300'
      : 'text-zinc-600 hover:bg-zinc-100 hover:text-zinc-900 dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-100'
  }`

function NavItem({ to, icon, children }: { to: string; icon: ReactNode; children: ReactNode }) {
  return (
    <NavLink to={to} className={link}>
      {icon}
      {children}
    </NavLink>
  )
}

const section = 'mt-5 mb-1 hidden px-2.5 text-[11px] font-semibold tracking-wider text-zinc-400 uppercase md:block dark:text-zinc-500'

export function Logo() {
  return (
    <span className="flex items-center gap-2">
      <span className="grid size-8 place-items-center rounded-lg bg-indigo-600 text-white shadow-sm">
        <DatabaseIcon size={18} />
      </span>
      <span className="text-base font-semibold tracking-tight">Datanest</span>
    </span>
  )
}

export default function Layout() {
  const { user, logout } = useAuth()
  return (
    <div className="min-h-screen md:flex">
      <aside className="flex shrink-0 flex-col border-b border-zinc-200 bg-white md:sticky md:top-0 md:h-screen md:w-60 md:border-r md:border-b-0 dark:border-zinc-800 dark:bg-zinc-900">
        <div className="flex items-center justify-between px-4 py-4">
          <Logo />
        </div>
        <nav className="flex gap-1 overflow-x-auto px-3 pb-3 md:flex-1 md:flex-col md:overflow-visible">
          <p className={`${section} md:mt-0`}>Workspace</p>
          <NavItem to="/databases" icon={<DatabaseIcon />}>Databases</NavItem>
          {user?.isAdmin && (
            <>
              <p className={section}>Administrasi</p>
              <NavItem to="/admin/users" icon={<UserIcon />}>Users</NavItem>
              <NavItem to="/admin/groups" icon={<UsersIcon />}>Groups</NavItem>
              <NavItem to="/admin/databases" icon={<ServerIcon />}>Koneksi database</NavItem>
            </>
          )}
        </nav>
        <div className="flex items-center gap-3 border-t border-zinc-200 px-4 py-3 dark:border-zinc-800">
          <span className="grid size-8 shrink-0 place-items-center rounded-full bg-zinc-200 text-sm font-semibold text-zinc-700 uppercase dark:bg-zinc-700 dark:text-zinc-200">
            {user?.username.slice(0, 1)}
          </span>
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm font-medium">{user?.username}</p>
            <p className="text-xs text-zinc-500 dark:text-zinc-400">{user?.isAdmin ? 'Platform Admin' : 'User'}</p>
          </div>
          <button className={ui.buttonSmGhost} onClick={() => void logout()} title="Logout" aria-label="Logout">
            <LogoutIcon />
          </button>
        </div>
      </aside>
      <main className="min-w-0 flex-1">
        <div className="mx-auto max-w-6xl px-4 py-6 md:px-8 md:py-8">
          <Outlet />
        </div>
      </main>
    </div>
  )
}
