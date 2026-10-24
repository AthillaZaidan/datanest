import { NavLink, Outlet } from 'react-router'
import { useAuth } from '../context/AuthContext'

const link = ({ isActive }: { isActive: boolean }) =>
  `rounded px-2 py-1 hover:bg-neutral-500/15 ${isActive ? 'bg-neutral-500/20 font-semibold' : ''}`

export default function Layout() {
  const { user, logout } = useAuth()
  return (
    <div className="flex min-h-screen">
      <nav className="flex w-52 shrink-0 flex-col gap-1 border-r border-neutral-500/30 p-4">
        <strong className="mb-2 px-2 text-lg">Datanest</strong>
        <NavLink to="/databases" className={link}>Databases</NavLink>
        {user?.isAdmin && <>
          <NavLink to="/admin/users" className={link}>Admin: Users</NavLink>
          <NavLink to="/admin/groups" className={link}>Admin: Groups</NavLink>
          <NavLink to="/admin/databases" className={link}>Admin: Databases</NavLink>
        </>}
        <button className="mt-auto rounded px-2 py-1 text-left hover:bg-neutral-500/15" onClick={() => void logout()}>Logout</button>
      </nav>
      <main className="min-w-0 flex-1 px-8 py-4">
        <Outlet />
      </main>
    </div>
  )
}
