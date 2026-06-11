import { NextResponse } from 'next/server'
import fs from 'fs'
import path from 'path'

export const dynamic = 'force-dynamic'

export async function GET() {
  try {
    const cwd = process.cwd()
    const manifestPath = path.join(cwd, '.next/server/app-paths-manifest.json')
    const manifest = fs.existsSync(manifestPath)
      ? JSON.parse(fs.readFileSync(manifestPath, 'utf8'))
      : null

    const serverAppPath = path.join(cwd, '.next/server/app')
    const listDir = (dir: string): string[] => {
      if (!fs.existsSync(dir)) return []
      const entries = fs.readdirSync(dir, { withFileTypes: true })
      const result: string[] = []
      for (const e of entries) {
        result.push(e.name + (e.isDirectory() ? '/' : ''))
      }
      return result
    }

    return NextResponse.json({
      cwd,
      manifestKeys: manifest ? Object.keys(manifest) : null,
      serverAppContents: listDir(serverAppPath),
    })
  } catch (e) {
    return NextResponse.json({ error: String(e) }, { status: 500 })
  }
}
