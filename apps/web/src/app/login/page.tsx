'use client'

import { useState } from 'react'
import { useRouter } from 'next/navigation'
import { authAPI } from '@/lib/api'
import { useAuthStore } from '@/store/auth'
import type { User, Workspace } from '@/types'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Card, CardContent, CardHeader } from '@/components/ui/card'

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
    <div className="min-h-screen flex items-center justify-center bg-background p-4">
      <Card className="w-full max-w-[400px] border-border bg-surface shadow-card p-8 sm:p-10">
        <CardHeader className="p-0 mb-8">
          <div className="flex items-center gap-2.5 mb-4">
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img src="/icon.svg" alt="Agent Nexus" className="w-9 h-9 rounded-lg shadow-sm" />
          </div>
          <h1 className="text-foreground text-2xl font-bold tracking-tight m-0">Agent Nexus</h1>
          <p className="text-muted-foreground mt-1 mb-0 text-sm">
            {mode === 'login' ? 'Sign in to your account' : 'Create a new account'}
          </p>
        </CardHeader>

        <CardContent className="p-0">
          <form onSubmit={handleSubmit} className="space-y-4">
            {mode === 'register' && (
              <div>
                <Label htmlFor="fullName" className="text-muted-foreground">Full Name</Label>
                <Input
                  id="fullName"
                  type="text"
                  value={fullName}
                  onChange={(e) => setFullName(e.target.value)}
                  required
                  placeholder="Jane Smith"
                  className="bg-surface-2 border-border-strong text-foreground placeholder:text-faint"
                />
              </div>
            )}

            <div>
              <Label htmlFor="email" className="text-muted-foreground">Email</Label>
              <Input
                id="email"
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                required
                placeholder="you@example.com"
                className="bg-surface-2 border-border-strong text-foreground placeholder:text-faint"
              />
            </div>

            <div>
              <Label htmlFor="password" className="text-muted-foreground">Password</Label>
              <Input
                id="password"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
                placeholder="••••••••"
                className="bg-surface-2 border-border-strong text-foreground placeholder:text-faint"
              />
            </div>

            {error && (
              <div className="px-3.5 py-2.5 bg-crit/10 border border-crit/30 rounded-md text-crit text-sm">
                {error}
              </div>
            )}

            <Button type="submit" disabled={loading} className="w-full bg-gradient-to-br from-accent to-accent-ink text-white hover:opacity-95">
              {loading ? 'Please wait…' : mode === 'login' ? 'Sign In' : 'Create Account'}
            </Button>
          </form>

          <p className="mt-5 text-center text-muted-foreground text-sm">
            {mode === 'login' ? "Don't have an account? " : 'Already have an account? '}
            <button
              onClick={() => { setMode(mode === 'login' ? 'register' : 'login'); setError('') }}
              className="bg-transparent border-none text-accent dark:text-accent-bright cursor-pointer text-sm underline"
            >
              {mode === 'login' ? 'Sign up' : 'Sign in'}
            </button>
          </p>
        </CardContent>
      </Card>
    </div>
  )
}
