import { useState } from 'react'
import type { FormEvent } from 'react'
import { Navigate, useNavigate } from 'react-router'
import type { ApiError } from '../api/client'
import { useAuth } from '../context/AuthContext'

export default function Login() {
  const { user, login } = useAuth()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const navigate = useNavigate()
  if (user) return <Navigate to="/databases" replace />
  async function submit(event: FormEvent) { event.preventDefault(); setError(''); setSubmitting(true); try { await login(username, password); navigate('/databases', { replace: true }) } catch (e) { setError((e as ApiError).message) } finally { setSubmitting(false) } }
  return <main className="login"><h1>Datanest</h1><form onSubmit={submit}><label>Username<input autoComplete="username" value={username} onChange={e => setUsername(e.target.value)} required /></label><label>Password<input type="password" autoComplete="current-password" value={password} onChange={e => setPassword(e.target.value)} required /></label>{error && <p role="alert">{error}</p>}<button disabled={submitting}>{submitting ? 'Masuk…' : 'Masuk'}</button></form></main>
}
