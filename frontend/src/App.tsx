import { Navigate, Route, Routes } from 'react-router'
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

// TODO(A): guard routes with AuthContext (login required; admin pages need isAdmin).
export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route element={<Layout />}>
        <Route index element={<Navigate to="/databases" replace />} />
        <Route path="databases" element={<Databases />} />
        <Route path="databases/:databaseId" element={<DatabaseDetail />} />
        <Route path="databases/:databaseId/migrations" element={<Migrations />} />
        <Route path="databases/:databaseId/migrations/new" element={<MigrationEditor />} />
        <Route path="migrations/:migrationId" element={<MigrationEditor />} />
        <Route path="admin/users" element={<AdminUsers />} />
        <Route path="admin/groups" element={<AdminGroups />} />
        <Route path="admin/databases" element={<AdminDatabases />} />
        <Route path="*" element={<NotFound />} />
      </Route>
    </Routes>
  )
}
