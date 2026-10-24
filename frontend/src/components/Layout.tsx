import { NavLink, Outlet } from 'react-router'

export default function Layout() {
  return (
    <div className="layout">
      <nav>
        <strong>Datanest</strong>
        <NavLink to="/databases">Databases</NavLink>
        <NavLink to="/admin/users">Admin: Users</NavLink>
        <NavLink to="/admin/groups">Admin: Groups</NavLink>
        <NavLink to="/admin/databases">Admin: Databases</NavLink>
      </nav>
      <main>
        <Outlet />
      </main>
    </div>
  )
}
