'use client'

import { useState } from 'react'
import { useRouter } from 'next/navigation'
import { authAPI } from '@/lib/api'
import { useAuthStore } from '@/store/auth'
import type { User, Workspace } from '@/types'

export default function LoginPage() {
  const router = useRouter()
  const { setAuth } = useAuthStore()
  const [mode, setMode] = useState<'login' | 'register'>('login')
  const [fullName, setFullName] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError('')
    setLoading(true)

    try {
      let data: { access_token: string; user: Record<string, unknown>; workspace_id: string }
      if (mode === 'login') {
        data = await authAPI.login({ email, password })
      } else {
        data = await authAPI.register({ email, password, full_name: fullName })
      }

      setAuth({
        user: data.user as unknown as User,
        workspaceId: data.workspace_id,
        accessToken: data.access_token,
      })

      try {
        const me = await authAPI.me()
        if (me.workspace) {
          useAuthStore.getState().setWorkspace(me.workspace as unknown as Workspace)
        }
      } catch { /* best effort */ }

      router.push('/dashboard')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Something went wrong')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div style={{
      minHeight: '100vh',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      backgroundColor: '#0f0e17',
    }}>
      <div style={{
        width: 400,
        padding: 40,
        backgroundColor: '#1a1825',
        borderRadius: 12,
        border: '1px solid #2d2b3d',
      }}>
        <div style={{ marginBottom: 32 }}>
          <h1 style={{ color: '#fff', fontSize: 24, fontWeight: 700, margin: 0 }}>Agent Nexus</h1>
          <p style={{ color: '#9ca3af', marginTop: 4, marginBottom: 0 }}>
            {mode === 'login' ? 'Sign in to your account' : 'Create a new account'}
          </p>
        </div>

        <form onSubmit={handleSubmit}>
          {mode === 'register' && (
            <div style={{ marginBottom: 16 }}>
              <label style={{ display: 'block', color: '#d1d5db', marginBottom: 6, fontSize: 14 }}>
                Full Name
              </label>
              <input
                type="text"
                value={fullName}
                onChange={(e) => setFullName(e.target.value)}
                required
                placeholder="Jane Smith"
                style={inputStyle}
              />
            </div>
          )}

          <div style={{ marginBottom: 16 }}>
            <label style={{ display: 'block', color: '#d1d5db', marginBottom: 6, fontSize: 14 }}>
              Email
            </label>
            <input
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              required
              placeholder="you@example.com"
              style={inputStyle}
            />
          </div>

          <div style={{ marginBottom: 24 }}>
            <label style={{ display: 'block', color: '#d1d5db', marginBottom: 6, fontSize: 14 }}>
              Password
            </label>
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
              placeholder="••••••••"
              style={inputStyle}
            />
          </div>

          {error && (
            <div style={{
              marginBottom: 16,
              padding: '10px 14px',
              backgroundColor: '#3d1a1a',
              border: '1px solid #7f1d1d',
              borderRadius: 6,
              color: '#f87171',
              fontSize: 14,
            }}>
              {error}
            </div>
          )}

          <button
            type="submit"
            disabled={loading}
            style={{
              width: '100%',
              padding: '10px 0',
              backgroundColor: loading ? '#6d28d9' : '#7c3aed',
              color: '#fff',
              border: 'none',
              borderRadius: 6,
              fontSize: 15,
              fontWeight: 600,
              cursor: loading ? 'not-allowed' : 'pointer',
            }}
          >
            {loading ? 'Please wait…' : mode === 'login' ? 'Sign In' : 'Create Account'}
          </button>
        </form>

        <p style={{ marginTop: 20, textAlign: 'center', color: '#6b7280', fontSize: 14 }}>
          {mode === 'login' ? "Don't have an account? " : 'Already have an account? '}
          <button
            onClick={() => { setMode(mode === 'login' ? 'register' : 'login'); setError('') }}
            style={{
              background: 'none',
              border: 'none',
              color: '#8b83e0',
              cursor: 'pointer',
              fontSize: 14,
              textDecoration: 'underline',
            }}
          >
            {mode === 'login' ? 'Sign up' : 'Sign in'}
          </button>
        </p>
      </div>
    </div>
  )
}

const inputStyle: React.CSSProperties = {
  width: '100%',
  padding: '10px 14px',
  backgroundColor: '#0f0e17',
  border: '1px solid #2d2b3d',
  borderRadius: 6,
  color: '#fff',
  fontSize: 14,
  outline: 'none',
  boxSizing: 'border-box',
}
