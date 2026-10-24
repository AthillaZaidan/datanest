import { useState } from 'react'
import type { FormEvent } from 'react'
import { Navigate, useNavigate } from 'react-router'
import type { ApiError } from '../api/client'
import { Mark } from '../components/icons'
import { Alert, Field } from '../components/kit'
import { ui } from '../components/ui'
import { useAuth } from '../context/AuthContext'
import { demoAccounts } from '../dev/demoAccounts'

export default function Login() {
  const { user, login } = useAuth()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const navigate = useNavigate()
  if (user) return <Navigate to="/databases" replace />

  async function submit(event: FormEvent) {
    event.preventDefault()
    setError('')
    setSubmitting(true)
    try {
      await login(username, password)
      navigate('/databases', { replace: true })
    } catch (e) {
      setError((e as ApiError).message)
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <main className="grid min-h-dvh place-items-center px-4 py-10">
      <div className="w-full max-w-[360px]">
        <div className="mb-6 flex items-center gap-2">
          <Mark size={26} />
          <span className="text-lg font-semibold tracking-[-0.01em]">Datanest</span>
        </div>
        <div className="rounded-2xl border border-line bg-sheet p-6">
          <h1 className="text-[22px] leading-8 font-semibold">Masuk</h1>
          <p className="mt-1 mb-6 text-sm text-muted">Jelajahi database PostgreSQL tim Anda dan jalankan migration schema berversi.</p>
          <form onSubmit={submit} className="space-y-4">
            <Field label="Username">
              <input className={ui.input} autoComplete="username" autoFocus value={username} onChange={(e) => setUsername(e.target.value)} required />
            </Field>
            <Field label="Password">
              <input className={ui.input} type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} required />
            </Field>
            {error && <Alert>{error}</Alert>}
            <button className={`${ui.buttonPrimary} h-10 w-full`} disabled={submitting}>
              {submitting ? 'Masuk…' : 'Masuk'}
            </button>
          </form>
        </div>
        <p className="mt-4 text-xs text-muted">Akun dibuat oleh Platform Admin. Akses database diberikan lewat group.</p>
        {import.meta.env.DEV && (
          <section aria-labelledby="demo" className="mt-6">
            <h2 id="demo" className="mb-2 text-sm font-medium">Akun demo</h2>
            <ul className="space-y-1">
              {demoAccounts.map((a) => (
                <li key={a.username}>
                  <button
                    type="button"
                    className="w-full rounded-lg px-3 py-2 text-left transition-colors hover:bg-active focus-visible:outline-2 focus-visible:outline-accent"
                    onClick={() => {
                      setUsername(a.username)
                      setPassword(a.password)
                      setError('')
                    }}
                  >
                    <span className="flex items-baseline justify-between gap-3">
                      <span className="font-mono text-[13px] font-medium">{a.username}</span>
                      <span className="text-xs text-muted">{a.role}</span>
                    </span>
                    <span className="block text-xs text-muted">{a.access}</span>
                  </button>
                </li>
              ))}
            </ul>
            <p className="mt-2 text-xs text-muted">Hanya tampil saat development. Jalankan seed dulu agar akun ini ada.</p>
          </section>
        )}
      </div>
    </main>
  )
}
