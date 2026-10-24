import { NavLink, Outlet } from 'react-router'
import { useAuth } from '../context/AuthContext'

export default function Layout() {
	const { user, logout } = useAuth()
  return (
    <div className="layout">
      <nav>
        <strong>Datanest</strong>
        <NavLink to="/databases">Databases</NavLink>
		{user?.isAdmin && <><NavLink to="/admin/users">Admin: Users</NavLink><NavLink to="/admin/groups">Admin: Groups</NavLink><NavLink to="/admin/databases">Admin: Databases</NavLink></>}
		<button onClick={() => void logout()}>Logout</button>
      </nav>
      <main>
        <Outlet />
      </main>
    </div>
  )
}
