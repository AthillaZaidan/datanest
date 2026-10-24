import { useState } from 'react'
import type { FormEvent } from 'react'
import { Navigate, useNavigate } from 'react-router'
import type { ApiError } from '../api/client'
import { Logo } from '../components/Layout'
import { Alert, Field } from '../components/kit'
import { ui } from '../components/ui'
import { useAuth } from '../context/AuthContext'

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
    <main className="grid min-h-screen place-items-center bg-[radial-gradient(ellipse_at_top,var(--color-indigo-100),transparent_60%)] px-4 dark:bg-[radial-gradient(ellipse_at_top,rgb(99_102_241/0.15),transparent_60%)]">
      <div className="w-full max-w-sm">
        <div className="mb-6 flex justify-center">
          <Logo />
        </div>
        <div className={`${ui.card} p-6`}>
          <h1 className="text-lg font-semibold">Masuk</h1>
          <p className={`mb-5 ${ui.muted}`}>Kelola dan jelajahi database PostgreSQL tim Anda.</p>
          <form onSubmit={submit} className="space-y-4">
            <Field label="Username">
              <input className={ui.input} autoComplete="username" autoFocus value={username} onChange={(e) => setUsername(e.target.value)} required />
            </Field>
            <Field label="Password">
              <input className={ui.input} type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} required />
            </Field>
            {error && <Alert>{error}</Alert>}
            <button className={`${ui.buttonPrimary} w-full`} disabled={submitting}>
              {submitting ? 'Masuk…' : 'Masuk'}
            </button>
          </form>
        </div>
      </div>
    </main>
  )
}
