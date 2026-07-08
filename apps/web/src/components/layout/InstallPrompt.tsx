'use client'

import { useEffect, useState } from 'react'
import { X, Share, Plus, Download } from 'lucide-react'

type Platform = 'ios' | 'android' | null

function detectMobilePlatform(): Platform {
  if (typeof navigator === 'undefined') return null
  const ua = navigator.userAgent
  if (/iPhone|iPad|iPod/.test(ua)) return 'ios'
  if (/Android/.test(ua)) return 'android'
  return null
}

function isStandalone(): boolean {
  if (typeof window === 'undefined') return false
  return window.matchMedia('(display-mode: standalone)').matches ||
    (window.navigator as any).standalone === true
}

export function InstallPrompt() {
  const [platform, setPlatform] = useState<Platform>(null)
  const [dismissed, setDismissed] = useState(true) // start hidden
  const [deferredPrompt, setDeferredPrompt] = useState<any>(null)
  const [showIOSInstructions, setShowIOSInstructions] = useState(false)

  useEffect(() => {
    // Don't show if already installed or dismissed this session
    if (isStandalone()) return
    if (sessionStorage.getItem('install-prompt-dismissed')) return

    const p = detectMobilePlatform()
    if (!p) return
    setPlatform(p)

    if (p === 'android') {
      const handler = (e: Event) => {
        e.preventDefault()
        setDeferredPrompt(e)
        setDismissed(false)
      }
      window.addEventListener('beforeinstallprompt', handler)
      return () => window.removeEventListener('beforeinstallprompt', handler)
    }

    if (p === 'ios') {
      // Show after a short delay so it doesn't feel intrusive
      const t = setTimeout(() => setDismissed(false), 2000)
      return () => clearTimeout(t)
    }
  }, [])

  const dismiss = () => {
    setDismissed(true)
    setShowIOSInstructions(false)
    sessionStorage.setItem('install-prompt-dismissed', '1')
  }

  const handleAndroidInstall = async () => {
    if (!deferredPrompt) return
    deferredPrompt.prompt()
    const { outcome } = await deferredPrompt.userChoice
    if (outcome === 'accepted') dismiss()
    setDeferredPrompt(null)
  }

  if (dismissed) return null

  return (
    <>
      {/* Banner */}
      <div className="fixed bottom-0 left-0 right-0 z-50 p-3 pb-safe">
        <div className="bg-gray-900 text-white rounded-xl shadow-2xl border border-white/10 overflow-hidden">
          <div className="flex items-start gap-3 p-4">
            <div className="w-10 h-10 bg-accent rounded-xl flex items-center justify-center flex-shrink-0">
              <Download className="w-5 h-5 text-white" />
            </div>
            <div className="flex-1 min-w-0">
              <p className="text-[13px] font-semibold text-white">Add to Home Screen</p>
              <p className="text-[11px] text-white/60 mt-0.5">
                Install Agent Nexus for quick access — no app store needed.
              </p>
            </div>
            <button
              onClick={dismiss}
              className="p-1 text-white/40 hover:text-white/80 flex-shrink-0"
              aria-label="Dismiss"
            >
              <X className="w-4 h-4" />
            </button>
          </div>

          {platform === 'android' && (
            <div className="px-4 pb-4">
              <button
                onClick={handleAndroidInstall}
                className="w-full bg-accent text-white text-[13px] font-medium py-2.5 rounded-lg"
              >
                Install
              </button>
            </div>
          )}

          {platform === 'ios' && !showIOSInstructions && (
            <div className="px-4 pb-4">
              <button
                onClick={() => setShowIOSInstructions(true)}
                className="w-full bg-accent text-white text-[13px] font-medium py-2.5 rounded-lg"
              >
                Show me how
              </button>
            </div>
          )}

          {platform === 'ios' && showIOSInstructions && (
            <div className="px-4 pb-4 space-y-2">
              <div className="flex items-center gap-3 bg-white/[0.06] rounded-lg p-3">
                <div className="w-7 h-7 bg-white/10 rounded-full flex items-center justify-center flex-shrink-0 text-[11px] font-bold text-white/60">1</div>
                <div className="flex items-center gap-2 text-[12px] text-white/80">
                  Tap the <Share className="w-4 h-4 inline text-info flex-shrink-0" /> Share button in Safari
                </div>
              </div>
              <div className="flex items-center gap-3 bg-white/[0.06] rounded-lg p-3">
                <div className="w-7 h-7 bg-white/10 rounded-full flex items-center justify-center flex-shrink-0 text-[11px] font-bold text-white/60">2</div>
                <div className="flex items-center gap-2 text-[12px] text-white/80">
                  Tap <Plus className="w-4 h-4 inline text-white/60 flex-shrink-0" /> <strong>Add to Home Screen</strong>
                </div>
              </div>
              <div className="flex items-center gap-3 bg-white/[0.06] rounded-lg p-3">
                <div className="w-7 h-7 bg-white/10 rounded-full flex items-center justify-center flex-shrink-0 text-[11px] font-bold text-white/60">3</div>
                <div className="text-[12px] text-white/80">Tap <strong>Add</strong> to confirm</div>
              </div>
            </div>
          )}
        </div>
      </div>
    </>
  )
}
