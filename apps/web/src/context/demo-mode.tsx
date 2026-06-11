'use client'

import { createContext, useContext } from 'react'

export const DemoModeContext = createContext(false)

export function useDemoMode() {
  return useContext(DemoModeContext)
}
