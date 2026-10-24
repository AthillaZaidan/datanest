import { Link } from 'react-router'
import { EmptyState } from '../components/kit'
import { ui } from '../components/ui'

export default function NotFound() {
  return (
    <div className={ui.card}>
      <EmptyState title="Halaman tidak ditemukan">
        <Link className={ui.link} to="/databases">Kembali ke Database saya</Link>
      </EmptyState>
    </div>
  )
}
