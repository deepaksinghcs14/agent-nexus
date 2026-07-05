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
    <div className="min-h-screen flex items-center justify-center bg-[#0f0e17] p-4">
      <Card className="w-full max-w-[400px] border-[#2d2b3d] bg-[#1a1825] p-8 sm:p-10">
        <CardHeader className="p-0 mb-8">
          <h1 className="text-white text-2xl font-bold m-0">Agent Nexus</h1>
          <p className="text-gray-400 mt-1 mb-0">
            {mode === 'login' ? 'Sign in to your account' : 'Create a new account'}
          </p>
        </CardHeader>

        <CardContent className="p-0">
          <form onSubmit={handleSubmit} className="space-y-4">
            {mode === 'register' && (
              <div>
                <Label htmlFor="fullName" className="text-gray-300">Full Name</Label>
                <Input
                  id="fullName"
                  type="text"
                  value={fullName}
                  onChange={(e) => setFullName(e.target.value)}
                  required
                  placeholder="Jane Smith"
                  className="bg-[#0f0e17] border-[#2d2b3d] text-white placeholder:text-gray-500"
                />
              </div>
            )}

            <div>
              <Label htmlFor="email" className="text-gray-300">Email</Label>
              <Input
                id="email"
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                required
                placeholder="you@example.com"
                className="bg-[#0f0e17] border-[#2d2b3d] text-white placeholder:text-gray-500"
              />
            </div>

            <div>
              <Label htmlFor="password" className="text-gray-300">Password</Label>
              <Input
                id="password"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
                placeholder="••••••••"
                className="bg-[#0f0e17] border-[#2d2b3d] text-white placeholder:text-gray-500"
              />
            </div>

            {error && (
              <div className="px-3.5 py-2.5 bg-[#3d1a1a] border border-red-900 rounded-md text-red-400 text-sm">
                {error}
              </div>
            )}

            <Button type="submit" disabled={loading} className="w-full">
              {loading ? 'Please wait…' : mode === 'login' ? 'Sign In' : 'Create Account'}
            </Button>
          </form>

          <p className="mt-5 text-center text-gray-500 text-sm">
            {mode === 'login' ? "Don't have an account? " : 'Already have an account? '}
            <button
              onClick={() => { setMode(mode === 'login' ? 'register' : 'login'); setError('') }}
              className="bg-transparent border-none text-accent-muted cursor-pointer text-sm underline"
            >
              {mode === 'login' ? 'Sign up' : 'Sign in'}
            </button>
          </p>
        </CardContent>
      </Card>
    </div>
  )
}
