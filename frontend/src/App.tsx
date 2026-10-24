import { Navigate, Outlet, Route, Routes } from 'react-router'
import Layout from './components/Layout'
import Login from './pages/Login'
import Databases from './pages/Databases'
import DatabaseDetail from './pages/DatabaseDetail'
import Migrations from './pages/Migrations'
import MigrationEditor from './pages/MigrationEditor'
import AdminUsers from './pages/admin/Users'
import AdminGroups from './pages/admin/Groups'
import AdminDatabases from './pages/admin/Databases'
import NotFound from './pages/NotFound'
import { useAuth } from './context/AuthContext'

function ProtectedRoute() { const { user, loading } = useAuth(); if (loading) return <p>Memuat sesi…</p>; return user ? <Outlet /> : <Navigate to="/login" replace /> }
function AdminRoute() { const { user } = useAuth(); return user?.isAdmin ? <Outlet /> : <Navigate to="/databases" replace /> }
export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route element={<ProtectedRoute />}>
      <Route element={<Layout />}>
        <Route index element={<Navigate to="/databases" replace />} />
        <Route path="databases" element={<Databases />} />
        <Route path="databases/:databaseId" element={<DatabaseDetail />} />
        <Route path="databases/:databaseId/migrations" element={<Migrations />} />
        <Route path="databases/:databaseId/migrations/new" element={<MigrationEditor />} />
        <Route path="migrations/:migrationId" element={<MigrationEditor />} />
        <Route element={<AdminRoute />}><Route path="admin/users" element={<AdminUsers />} /><Route path="admin/groups" element={<AdminGroups />} /><Route path="admin/databases" element={<AdminDatabases />} /></Route>
        <Route path="*" element={<NotFound />} />
      </Route>
      </Route>
    </Routes>
  )
}
