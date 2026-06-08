'use client'

import { useState } from 'react'
import { useRouter } from 'next/navigation'
import { useAuthStore } from '@/store/auth'
import { authAPI } from '@/lib/api'

export function UserMenu() {
  const { user, clearAuth } = useAuthStore()
  const router = useRouter()
  const [open, setOpen] = useState(false)

  const initials = user?.full_name
    ? user.full_name.split(' ').map((n: string) => n[0]).join('').toUpperCase().slice(0, 2)
    : '?'

  async function handleLogout() {
    try {
      await authAPI.logout()
    } catch { /* ignore */ }
    clearAuth()
    router.push('/login')
  }

  return (
    <div style={{ position: 'relative' }}>
      <button
        onClick={() => setOpen(!open)}
        style={{
          width: 36,
          height: 36,
          borderRadius: '50%',
          backgroundColor: '#534AB7',
          color: '#fff',
          border: 'none',
          cursor: 'pointer',
          fontSize: 13,
          fontWeight: 600,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
        }}
      >
        {initials}
      </button>

      {open && (
        <>
          <div
            style={{ position: 'fixed', inset: 0, zIndex: 40 }}
            onClick={() => setOpen(false)}
          />
          <div style={{
            position: 'absolute',
            right: 0,
            top: 44,
            zIndex: 50,
            backgroundColor: '#1a1825',
            border: '1px solid #2d2b3d',
            borderRadius: 8,
            padding: 8,
            minWidth: 200,
          }}>
            <div style={{ padding: '8px 12px', borderBottom: '1px solid #2d2b3d', marginBottom: 4 }}>
              <p style={{ color: '#fff', margin: 0, fontSize: 14, fontWeight: 500 }}>{user?.full_name}</p>
              <p style={{ color: '#6b7280', margin: 0, fontSize: 12 }}>{user?.email}</p>
            </div>
            <button
              onClick={handleLogout}
              style={{
                display: 'block',
                width: '100%',
                padding: '8px 12px',
                textAlign: 'left',
                background: 'none',
                border: 'none',
                color: '#f87171',
                cursor: 'pointer',
                fontSize: 14,
                borderRadius: 4,
              }}
            >
              Sign out
            </button>
          </div>
        </>
      )}
    </div>
  )
}
