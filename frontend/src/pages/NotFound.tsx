import { Link } from 'react-router'
import { Empty, PageHeader } from '../components/kit'
import { ui } from '../components/ui'
import { useCrumbs } from '../context/crumbs'

export default function NotFound() {
  useCrumbs([{ label: 'Tidak ditemukan' }])
  return (
    <>
      <PageHeader title="Halaman tidak ditemukan" subtitle="Alamat ini tidak ada atau sudah dipindahkan." />
      <Empty action={<Link className={ui.button} to="/databases">Ke Databases</Link>}>Kembali ke daftar database yang dapat Anda akses.</Empty>
    </>
  )
}
