'use client'

import { useState, useRef } from 'react'
import { ZoomIn, ZoomOut, Maximize2 } from 'lucide-react'

export default function ArchitecturePage() {
  const [zoom, setZoom] = useState(100)
  const containerRef = useRef<HTMLDivElement>(null)

  const zoomIn  = () => setZoom((z) => Math.min(z + 15, 200))
  const zoomOut = () => setZoom((z) => Math.max(z - 15, 40))
  const reset   = () => setZoom(100)

  return (
    <div className="flex flex-col h-full">
      {/* Toolbar */}
      <div className="flex items-center justify-between px-6 py-3 border-b border-gray-100 bg-white flex-shrink-0">
        <div>
          <h1 className="text-base font-semibold text-gray-900">System Architecture</h1>
          <p className="text-[11px] text-gray-400 mt-0.5">
            Agent Nexus — component overview, data flows, and infrastructure
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={zoomOut}
            className="p-1.5 rounded-lg border border-gray-200 hover:bg-gray-50 text-gray-600"
            title="Zoom out"
          >
            <ZoomOut className="w-4 h-4" />
          </button>
          <span className="text-[12px] text-gray-500 w-12 text-center">{zoom}%</span>
          <button
            onClick={zoomIn}
            className="p-1.5 rounded-lg border border-gray-200 hover:bg-gray-50 text-gray-600"
            title="Zoom in"
          >
            <ZoomIn className="w-4 h-4" />
          </button>
          <button
            onClick={reset}
            className="p-1.5 rounded-lg border border-gray-200 hover:bg-gray-50 text-gray-600 ml-1"
            title="Reset zoom"
          >
            <Maximize2 className="w-4 h-4" />
          </button>
        </div>
      </div>

      {/* Diagram canvas — scrollable */}
      <div
        ref={containerRef}
        className="flex-1 overflow-auto bg-[#040C1A]"
        style={{ cursor: 'grab' }}
      >
        <div
          style={{
            transformOrigin: 'top left',
            transform: `scale(${zoom / 100})`,
            width: 'max-content',
            padding: '24px',
          }}
        >
          <ArchitectureDiagram />
        </div>
      </div>
    </div>
  )
}

function ArchitectureDiagram() {
  return (
    <svg xmlns="http://www.w3.org/2000/svg" viewBox="-24 -14 1448 908" width="1448" height="908" overflow="visible" font-family="'Inter','SF Pro Display',system-ui,sans-serif">
      <defs>
        <filter id="gp" x="-40%" y="-40%" width="180%" height="180%">
          <feGaussianBlur stdDeviation="5" result="b"/>
          <feMerge><feMergeNode in="b"/><feMergeNode in="b"/><feMergeNode in="SourceGraphic"/></feMerge>
        </filter>
        <filter id="gc" x="-40%" y="-40%" width="180%" height="180%">
          <feGaussianBlur stdDeviation="5" result="b"/>
          <feMerge><feMergeNode in="b"/><feMergeNode in="b"/><feMergeNode in="SourceGraphic"/></feMerge>
        </filter>
        <filter id="ga" x="-40%" y="-40%" width="180%" height="180%">
          <feGaussianBlur stdDeviation="5" result="b"/>
          <feMerge><feMergeNode in="b"/><feMergeNode in="b"/><feMergeNode in="SourceGraphic"/></feMerge>
        </filter>
        <filter id="gg" x="-40%" y="-40%" width="180%" height="180%">
          <feGaussianBlur stdDeviation="5" result="b"/>
          <feMerge><feMergeNode in="b"/><feMergeNode in="b"/><feMergeNode in="SourceGraphic"/></feMerge>
        </filter>
        <filter id="gr" x="-40%" y="-40%" width="180%" height="180%">
          <feGaussianBlur stdDeviation="5" result="b"/>
          <feMerge><feMergeNode in="b"/><feMergeNode in="b"/><feMergeNode in="SourceGraphic"/></feMerge>
        </filter>
        <filter id="gp2" x="-60%" y="-60%" width="220%" height="220%">
          <feGaussianBlur stdDeviation="10" result="b"/>
          <feMerge><feMergeNode in="b"/><feMergeNode in="b"/><feMergeNode in="b"/><feMergeNode in="SourceGraphic"/></feMerge>
        </filter>
        <filter id="tg" x="-20%" y="-20%" width="140%" height="140%">
          <feGaussianBlur stdDeviation="2" result="b"/>
          <feMerge><feMergeNode in="b"/><feMergeNode in="SourceGraphic"/></feMerge>
        </filter>
        <radialGradient id="bg" cx="50%" cy="35%" r="65%">
          <stop offset="0%" stopColor="#0B1630"/>
          <stop offset="100%" stopColor="#020611"/>
        </radialGradient>
        <linearGradient id="hdrP" x1="0%" y1="0%" x2="100%" y2="0%">
          <stop offset="0%" stopColor="#3B0764"/>
          <stop offset="100%" stopColor="#6D28D9"/>
        </linearGradient>
        <linearGradient id="hdrP2" x1="0%" y1="0%" x2="100%" y2="0%">
          <stop offset="0%" stopColor="#2E1065"/>
          <stop offset="100%" stopColor="#4C1D95"/>
        </linearGradient>
        <linearGradient id="hdrC" x1="0%" y1="0%" x2="100%" y2="0%">
          <stop offset="0%" stopColor="#082F49"/>
          <stop offset="100%" stopColor="#0E7490"/>
        </linearGradient>
        <linearGradient id="hdrA" x1="0%" y1="0%" x2="100%" y2="0%">
          <stop offset="0%" stopColor="#451A03"/>
          <stop offset="100%" stopColor="#B45309"/>
        </linearGradient>
        <linearGradient id="hdrG" x1="0%" y1="0%" x2="100%" y2="0%">
          <stop offset="0%" stopColor="#052E16"/>
          <stop offset="100%" stopColor="#047857"/>
        </linearGradient>
        <linearGradient id="hdrR" x1="0%" y1="0%" x2="100%" y2="0%">
          <stop offset="0%" stopColor="#4C0519"/>
          <stop offset="100%" stopColor="#9F1239"/>
        </linearGradient>
        <linearGradient id="titleG" x1="0%" y1="0%" x2="100%" y2="0%">
          <stop offset="0%" stopColor="#A78BFA"/>
          <stop offset="50%" stopColor="#E879F9"/>
          <stop offset="100%" stopColor="#60A5FA"/>
        </linearGradient>
        <pattern id="dots" x="0" y="0" width="32" height="32" patternUnits="userSpaceOnUse">
          <circle cx="16" cy="16" r="0.8" fill="#1E3A5F" opacity="0.5"/>
        </pattern>
        <marker id="ap" markerWidth="7" markerHeight="7" refX="6" refY="3.5" orient="auto">
          <path d="M0,0 L0,7 L7,3.5 z" fill="#A78BFA"/>
        </marker>
        <marker id="ac" markerWidth="7" markerHeight="7" refX="6" refY="3.5" orient="auto">
          <path d="M0,0 L0,7 L7,3.5 z" fill="#22D3EE"/>
        </marker>
        <marker id="aa" markerWidth="7" markerHeight="7" refX="6" refY="3.5" orient="auto">
          <path d="M0,0 L0,7 L7,3.5 z" fill="#FCD34D"/>
        </marker>
        <marker id="ag" markerWidth="7" markerHeight="7" refX="6" refY="3.5" orient="auto">
          <path d="M0,0 L0,7 L7,3.5 z" fill="#34D399"/>
        </marker>
        <marker id="ar" markerWidth="7" markerHeight="7" refX="6" refY="3.5" orient="auto">
          <path d="M0,0 L0,7 L7,3.5 z" fill="#FB7185"/>
        </marker>
      </defs>
      <style>{`
        @keyframes dash{from{stroke-dashoffset:26}to{stroke-dashoffset:0}}
        @keyframes pborder{0%,100%{stroke-opacity:.5}50%{stroke-opacity:1}}
        @keyframes pdot{0%,100%{r:3;opacity:.7}50%{r:5;opacity:1}}
        @keyframes blink{0%,100%{opacity:1}50%{opacity:.25}}
        @keyframes scan{from{transform:translateY(-4px)}to{transform:translateY(884px)}}
        @keyframes tglow{0%,100%{opacity:.8}50%{opacity:1}}
        @keyframes orb{0%,100%{transform:scale(1);opacity:.15}50%{transform:scale(1.08);opacity:.28}}
        .fp{stroke-dasharray:8 5;animation:dash 1.7s linear infinite}
        .fc{stroke-dasharray:8 5;animation:dash 1.3s linear infinite}
        .fa{stroke-dasharray:8 5;animation:dash 1.9s linear infinite}
        .fg{stroke-dasharray:8 5;animation:dash 2.1s linear infinite}
        .fr{stroke-dasharray:8 5;animation:dash 1.5s linear infinite}
        .pb{animation:pborder 3s ease-in-out infinite}
        .pd{animation:pdot 2.2s ease-in-out infinite}
        .blink{animation:blink 2s ease-in-out infinite}
        .tg{animation:tglow 2.5s ease-in-out infinite}
        .orb{animation:orb 4s ease-in-out infinite}
      `}</style>

      {/* Background */}
      <rect x="-24" y="-14" width="1448" height="908" fill="url(#bg)"/>
      <rect x="-24" y="-14" width="1448" height="908" fill="url(#dots)"/>

      {/* Ambient orbs */}
      <ellipse cx="700" cy="380" rx="480" ry="320" fill="none" stroke="#4C1D95" strokeWidth="80" className="orb" filter="url(#gp2)" opacity=".12"/>
      <ellipse cx="1120" cy="480" rx="200" ry="180" fill="#0E4F6B" className="orb" opacity=".06" filter="url(#gc)"/>
      <ellipse cx="200" cy="400" rx="180" ry="200" fill="#4C1D95" className="orb" opacity=".06" filter="url(#gp)"/>

      {/* Scanline */}
      <rect x="0" y="0" width="1400" height="3" fill="#A78BFA" opacity=".04">
        <animateTransform attributeName="transform" type="translate" from="0,-3" to="0,883" dur="9s" repeatCount="indefinite"/>
      </rect>

      {/* ── TITLE ── */}
      <text x="700" y="46" textAnchor="middle" fontSize="28" fontWeight="900" fill="url(#titleG)" letterSpacing="4" filter="url(#gp)" className="tg">AGENT NEXUS</text>
      <text x="700" y="66" textAnchor="middle" fontSize="11" fill="#475569" letterSpacing="4">ARCHITECTURE OVERVIEW · SELF-HOSTED AI AGENT ORCHESTRATION PLATFORM</text>

      {/* ── FRONTEND BOX ── */}
      <rect x="14" y="76" width="344" height="528" rx="14" fill="none" stroke="#7C3AED" strokeWidth="2.5" filter="url(#gp)" className="pb"/>
      <rect x="14" y="76" width="344" height="528" rx="14" fill="#0B0E24" stroke="#4C1D95" strokeWidth="1.2"/>
      <rect x="14" y="76" width="344" height="46" rx="14" fill="url(#hdrP)"/>
      <rect x="14" y="106" width="344" height="16" fill="url(#hdrP)"/>
      <circle cx="34" cy="99" r="5" fill="#E879F9" filter="url(#gp)"/>
      <text x="186" y="103" textAnchor="middle" fontSize="12" fontWeight="800" fill="white" letterSpacing="2" filter="url(#tg)">NEXT.JS 14 FRONTEND</text>
      <text x="186" y="118" textAnchor="middle" fontSize="8.5" fill="#C4B5FD" letterSpacing="2">APP ROUTER · REACT QUERY · ZUSTAND</text>
      {/* Build */}
      <rect x="26" y="130" width="150" height="258" rx="8" fill="#0D1035" stroke="#4C1D95" strokeWidth="1"/>
      <text x="101" y="147" textAnchor="middle" fontSize="8" fontWeight="700" fill="#A78BFA" letterSpacing="3">BUILD</text>
      {[['Agents'],['Workflows · Visual Builder'],['Tools (native / HTTP)'],['MCP Servers'],['Connectors (RAG)'],['Webhook Triggers'],['API Tokens'],['Provider Settings'],['Nexus AI (meta-agent)']].map(([label], i) => (
        <g key={label}>
          <rect x="34" y={154 + i * 24} width="134" height="20" rx="5" fill="#13174A" stroke="#312E81" strokeWidth=".8"/>
          <text x="101" y={168 + i * 24} textAnchor="middle" fontSize="9" fill="#C7D2FE">{label}</text>
        </g>
      ))}
      {/* Observe */}
      <rect x="186" y="130" width="158" height="100" rx="8" fill="#0D1A14" stroke="#065F46" strokeWidth="1"/>
      <text x="265" y="147" textAnchor="middle" fontSize="8" fontWeight="700" fill="#34D399" letterSpacing="3">OBSERVE</text>
      {[['Runs + Traces (SSE)'],['Memory Explorer'],['Playground']].map(([label], i) => (
        <g key={label}>
          <rect x="194" y={154 + i * 24} width="142" height="20" rx="5" fill="#0A1F16" stroke="#065F46" strokeWidth=".8"/>
          <text x="265" y={168 + i * 24} textAnchor="middle" fontSize="9" fill="#6EE7B7">{label}</text>
        </g>
      ))}
      {/* Admin */}
      <rect x="186" y="240" width="158" height="100" rx="8" fill="#1A0813" stroke="#7F1D1D" strokeWidth="1"/>
      <text x="265" y="257" textAnchor="middle" fontSize="8" fontWeight="700" fill="#FB7185" letterSpacing="3">ADMIN</text>
      {[['Users & Workspaces'],['Audit Logs'],['Policies · Usage']].map(([label], i) => (
        <g key={label}>
          <rect x="194" y={264 + i * 24} width="142" height="20" rx="5" fill="#1F0A10" stroke="#7F1D1D" strokeWidth=".8"/>
          <text x="265" y={278 + i * 24} textAnchor="middle" fontSize="9" fill="#FDA4AF">{label}</text>
        </g>
      ))}
      {/* Info badges */}
      {[
        { y: 356, color: '#A78BFA', dot: '#34D399', text: 'SSE streams · JWT Bearer · React Query v5' },
        { y: 384, color: '#818CF8', dot: '#A78BFA', text: '@xyflow/react v12 · BFS executor · conditional/parallel/loop' },
        { y: 412, color: '#818CF8', dot: null,      text: 'Dashboard · 6 stat cards · webhook trigger metrics' },
        { y: 440, color: '#818CF8', dot: null,      text: 'OAuth providers · live model discovery · API tokens' },
        { y: 468, color: '#818CF8', dot: null,      text: 'Playground · Nexus AI meta-agent · 7 built-in tools' },
        { y: 496, color: '#6D7EE0', dot: null,      text: 'TypeScript · Tailwind CSS · pnpm · Next.js App Router' },
      ].map(({ y, color, dot, text }) => (
        <g key={y}>
          <rect x="26" y={y} width="318" height="22" rx="7" fill="#13174A" stroke="#4C1D95" strokeWidth=".8"/>
          {dot && <circle cx="42" cy={y + 11} r="3.5" fill={dot} filter="url(#gp)" className="blink"/>}
          <text x="186" y={y + 15} textAnchor="middle" fontSize="8.5" fill={color}>{text}</text>
        </g>
      ))}

      {/* ── API SERVER BOX ── */}
      <rect x="406" y="76" width="542" height="648" rx="16" fill="none" stroke="#7C3AED" strokeWidth="4" filter="url(#gp2)" className="pb"/>
      <rect x="406" y="76" width="542" height="648" rx="16" fill="#09091E" stroke="#4C1D95" strokeWidth="1.5"/>
      <rect x="406" y="76" width="542" height="48" rx="16" fill="url(#hdrP2)"/>
      <rect x="406" y="110" width="542" height="14" fill="url(#hdrP2)"/>
      <circle cx="428" cy="100" r="5" fill="#A78BFA" filter="url(#gp)"/>
      <circle cx="444" cy="100" r="5" fill="#E879F9" filter="url(#gp)"/>
      <text x="677" y="105" textAnchor="middle" fontSize="13" fontWeight="800" fill="white" letterSpacing="2" filter="url(#tg)">GO API SERVER · CHI v5 ROUTER</text>
      <text x="677" y="120" textAnchor="middle" fontSize="8.5" fill="#C4B5FD" letterSpacing="2">services/api · pgx/v5 · JWT · slog · :8080</text>
      {/* Middleware */}
      <rect x="418" y="134" width="518" height="38" rx="8" fill="#0D1035" stroke="#4C1D95" strokeWidth="1"/>
      <text x="677" y="149" textAnchor="middle" fontSize="8.5" fontWeight="700" fill="#A78BFA" letterSpacing="3">MIDDLEWARE</text>
      <text x="677" y="164" textAnchor="middle" fontSize="9" fill="#6D7EE0">JWT Authentication · Workspace Context · Admin RBAC · CORS</text>
      {/* Handler label */}
      <text x="424" y="188" fontSize="8" fontWeight="700" fill="#4B5563" letterSpacing="3">HANDLERS</text>
      {/* Handlers grid - row 1 */}
      {[
        { x: 418, label: 'Auth',        sub: 'login / register',   fill: '#0D1035', stroke: '#312E81', color: '#A78BFA' },
        { x: 522, label: 'Agents',      sub: 'CRUD + tools',       fill: '#0D1035', stroke: '#312E81', color: '#A78BFA' },
        { x: 626, label: 'Workflows',   sub: 'CRUD + graph',       fill: '#0D1035', stroke: '#312E81', color: '#A78BFA' },
        { x: 730, label: 'Tools',       sub: 'native/HTTP/MCP',    fill: '#0D1035', stroke: '#312E81', color: '#A78BFA' },
        { x: 834, label: 'Runs',        sub: 'start/SSE/cancel',   fill: '#0D1035', stroke: '#312E81', color: '#A78BFA' },
      ].map(({ x, label, sub, fill, stroke, color }) => (
        <g key={label}>
          <rect x={x} y="193" width="96" height="34" rx="7" fill={fill} stroke={stroke} strokeWidth="1"/>
          <text x={x + 48} y="208" textAnchor="middle" fontSize="8.5" fontWeight="600" fill={color}>{label}</text>
          <text x={x + 48} y="221" textAnchor="middle" fontSize="7.5" fill="#64748B">{sub}</text>
        </g>
      ))}
      {/* Handlers row 2 */}
      {[
        { x: 418, label: 'Invoke',      sub: 'agent + workflow',   fill: '#0D1035', stroke: '#312E81', color: '#A78BFA' },
        { x: 522, label: 'Connectors',  sub: 'RAG / indexing',     fill: '#0D1035', stroke: '#312E81', color: '#A78BFA' },
        { x: 626, label: 'Memory',      sub: 'vector retrieval',   fill: '#0D1035', stroke: '#312E81', color: '#A78BFA' },
        { x: 730, label: 'Admin',       sub: 'users/ws/audit',     fill: '#111827', stroke: '#7F1D1D', color: '#FB7185' },
        { x: 834, label: 'MCP Client',  sub: 'HTTP + stdio',       fill: '#0A1F16', stroke: '#065F46', color: '#34D399' },
      ].map(({ x, label, sub, fill, stroke, color }) => (
        <g key={label}>
          <rect x={x} y="233" width="96" height="34" rx="7" fill={fill} stroke={stroke} strokeWidth="1"/>
          <text x={x + 48} y="248" textAnchor="middle" fontSize="8.5" fontWeight="600" fill={color}>{label}</text>
          <text x={x + 48} y="261" textAnchor="middle" fontSize="7.5" fill="#64748B">{sub}</text>
        </g>
      ))}
      {/* Handlers row 3 */}
      <rect x="418" y="273" width="96" height="34" rx="7" fill="#130D0A" stroke="#78350F" strokeWidth="1"/>
      <text x="466" y="288" textAnchor="middle" fontSize="8.5" fontWeight="600" fill="#FCD34D">Nexus AI</text>
      <text x="466" y="301" textAnchor="middle" fontSize="7.5" fill="#64748B">meta-agent / 7 tools</text>
      <rect x="522" y="273" width="96" height="34" rx="7" fill="#130D1A" stroke="#7C1D7C" strokeWidth="1"/>
      <text x="570" y="288" textAnchor="middle" fontSize="8.5" fontWeight="600" fill="#E879F9">Workspace</text>
      <text x="570" y="301" textAnchor="middle" fontSize="7.5" fill="#64748B">members / settings</text>
      <rect x="626" y="273" width="96" height="34" rx="7" fill="#1A0813" stroke="#7F1D1D" strokeWidth="1"/>
      <text x="674" y="288" textAnchor="middle" fontSize="8.5" fontWeight="600" fill="#FB7185">Webhook Ingress</text>
      <text x="674" y="301" textAnchor="middle" fontSize="7.5" fill="#64748B">public · HMAC-SHA256</text>
      <rect x="730" y="273" width="190" height="34" rx="7" fill="#0D1035" stroke="#312E81" strokeWidth="1"/>
      <text x="825" y="288" textAnchor="middle" fontSize="8.5" fontWeight="600" fill="#818CF8">Conversations · Approval · API Tokens</text>
      <text x="825" y="301" textAnchor="middle" fontSize="7.5" fill="#64748B">Messages · Providers · Policies</text>
      {/* Invoke Engine */}
      <rect x="418" y="317" width="518" height="84" rx="10" fill="#0D1035" stroke="#4C1D95" strokeWidth="1.5"/>
      <text x="677" y="334" textAnchor="middle" fontSize="9" fontWeight="700" fill="#A78BFA" letterSpacing="3" filter="url(#tg)">INVOKE ENGINE</text>
      <text x="677" y="348" textAnchor="middle" fontSize="8.5" fill="#64748B">executeRun / executeGroupRun · async goroutines · step recording</text>
      <circle cx="432" cy="329" r="4" fill="#A78BFA" filter="url(#gp)" className="blink"/>
      {[['Memory retrieval','426'],['Context (RAG)','550'],['Tool execution','674'],['BFS workflow · SSE stream','798']].map(([label, x]) => (
        <g key={label}>
          <rect x={Number(x)} y="354" width={label.length > 14 ? 128 : 116} height="20" rx="5" fill="#13174A" stroke="#312E81" strokeWidth=".8"/>
          <text x={Number(x) + (label.length > 14 ? 64 : 58)} y="368" textAnchor="middle" fontSize="8.5" fill="#C7D2FE">{label}</text>
        </g>
      ))}
      {/* AI Adapter */}
      <rect x="418" y="411" width="254" height="68" rx="10" fill="#130D0A" stroke="#92400E" strokeWidth="1.5"/>
      <text x="545" y="428" textAnchor="middle" fontSize="9" fontWeight="700" fill="#FCD34D" letterSpacing="3">AI ADAPTER LAYER</text>
      <text x="545" y="442" textAnchor="middle" fontSize="8.5" fill="#64748B">Unified streaming interface · token counting</text>
      <rect x="426" y="449" width="112" height="20" rx="5" fill="#1A1008" stroke="#92400E" strokeWidth=".8"/>
      <text x="482" y="463" textAnchor="middle" fontSize="8.5" fill="#FDE68A">Anthropic · Claude</text>
      <rect x="546" y="449" width="118" height="20" rx="5" fill="#1A1008" stroke="#92400E" strokeWidth=".8"/>
      <text x="605" y="463" textAnchor="middle" fontSize="8.5" fill="#FDE68A">OpenAI · Gemini · Ollama</text>
      {/* Repository */}
      <rect x="682" y="411" width="254" height="68" rx="10" fill="#061A2A" stroke="#0E7490" strokeWidth="1.5"/>
      <text x="809" y="428" textAnchor="middle" fontSize="9" fontWeight="700" fill="#22D3EE" letterSpacing="3">REPOSITORY LAYER</text>
      <text x="809" y="442" textAnchor="middle" fontSize="8.5" fill="#64748B">pgx/v5 typed queries per domain</text>
      <rect x="690" y="449" width="114" height="20" rx="5" fill="#06182A" stroke="#0E7490" strokeWidth=".8"/>
      <text x="747" y="463" textAnchor="middle" fontSize="8.5" fill="#67E8F9">agents · runs · tools</text>
      <rect x="812" y="449" width="116" height="20" rx="5" fill="#06182A" stroke="#0E7490" strokeWidth=".8"/>
      <text x="870" y="463" textAnchor="middle" fontSize="8.5" fill="#67E8F9">memory · connectors</text>
      {/* Migration */}
      <rect x="418" y="489" width="254" height="52" rx="10" fill="#052E16" stroke="#047857" strokeWidth="1.5"/>
      <text x="545" y="506" textAnchor="middle" fontSize="9" fontWeight="700" fill="#34D399" letterSpacing="3">MIGRATION RUNNER</text>
      <text x="545" y="521" textAnchor="middle" fontSize="8.5" fill="#64748B">{'//go:embed SQL · 14 migrations · baseline detect'}</text>
      <circle cx="432" cy="505" r="3.5" fill="#34D399" filter="url(#gg)" className="blink"/>
      {/* Tool Registry */}
      <rect x="682" y="489" width="254" height="52" rx="10" fill="#0D1035" stroke="#4C1D95" strokeWidth="1.5"/>
      <text x="809" y="506" textAnchor="middle" fontSize="9" fontWeight="700" fill="#A78BFA" letterSpacing="3">TOOL REGISTRY</text>
      <text x="809" y="521" textAnchor="middle" fontSize="8.5" fill="#64748B">risk levels · approval gates · MCP mirroring</text>
      {/* Public routes */}
      <rect x="418" y="551" width="518" height="26" rx="8" fill="#1A0813" stroke="#7F1D1D" strokeWidth=".8"/>
      <text x="677" y="568" textAnchor="middle" fontSize="8.5" fill="#FB7185">Public (no auth):  GET /health  ·  POST /webhook/{'{id}'}  ·  POST /auth/login</text>
      {/* Audit */}
      <rect x="418" y="585" width="518" height="26" rx="8" fill="#130D0A" stroke="#78350F" strokeWidth=".8"/>
      <text x="677" y="602" textAnchor="middle" fontSize="8.5" fill="#FCD34D">Audit trail: webhook_trigger.created / updated / deleted / fired</text>
      {/* Encryption */}
      <rect x="418" y="619" width="254" height="26" rx="8" fill="#061A2A" stroke="#0E7490" strokeWidth=".8"/>
      <text x="545" y="636" textAnchor="middle" fontSize="8.5" fill="#67E8F9">AES-256-GCM · API keys at rest</text>
      {/* Cost */}
      <rect x="682" y="619" width="254" height="26" rx="8" fill="#0D1035" stroke="#312E81" strokeWidth=".8"/>
      <text x="809" y="636" textAnchor="middle" fontSize="8.5" fill="#818CF8">Token counting · cost estimate · pgvector 1536-dim</text>
      {/* Go version */}
      <rect x="418" y="653" width="518" height="26" rx="8" fill="#0D1035" stroke="#312E81" strokeWidth=".8"/>
      <text x="677" y="670" textAnchor="middle" fontSize="8.5" fill="#64748B">Go 1.26+ · chi/v5 · pgx/v5 · google/uuid · jackc/pgx · AES-GCM · bcrypt</text>

      {/* ── POSTGRESQL ── */}
      <rect x="1002" y="194" width="384" height="152" rx="14" fill="none" stroke="#0891B2" strokeWidth="2.5" filter="url(#gc)" className="pb" opacity=".6"/>
      <rect x="1002" y="194" width="384" height="152" rx="14" fill="#05131D" stroke="#0E7490" strokeWidth="1.2"/>
      <rect x="1002" y="194" width="384" height="42" rx="14" fill="url(#hdrC)"/>
      <rect x="1002" y="222" width="384" height="14" fill="url(#hdrC)"/>
      <circle cx="1022" cy="215" r="4" fill="#22D3EE" filter="url(#gc)"/>
      <text x="1194" y="219" textAnchor="middle" fontSize="12" fontWeight="800" fill="white" letterSpacing="1" filter="url(#tg)">POSTGRESQL 16 + pgvector</text>
      <text x="1194" y="232" textAnchor="middle" fontSize="8.5" fill="#67E8F9" letterSpacing="1.5">DOCKER VOLUME · PERSISTENT STORAGE</text>
      {[
        [1010,1194,'users · workspaces · agents',         'workflows + workflow_graph'],
        [1010,1194,'runs · run_steps · conversations',     'tools · mcp_servers · mcp_tools'],
        [1010,1194,'memories (vector 1536-dim)',           'webhook_triggers · schema_migrations'],
      ].map(([,, a, b], i) => (
        <g key={i}>
          <rect x="1010" y={244 + i * 22} width="176" height="18" rx="5" fill="#061A2A" stroke="#0E7490" strokeWidth=".7"/>
          <text x="1098" y={257 + i * 22} textAnchor="middle" fontSize="7.5" fill="#67E8F9">{a}</text>
          <rect x="1194" y={244 + i * 22} width="184" height="18" rx="5" fill="#061A2A" stroke="#0E7490" strokeWidth=".7"/>
          <text x="1286" y={257 + i * 22} textAnchor="middle" fontSize="7.5" fill="#67E8F9">{b}</text>
        </g>
      ))}
      <rect x="1010" y="310" width="368" height="26" rx="5" fill="#04293A" stroke="#0E7490" strokeWidth=".7"/>
      <text x="1194" y="327" textAnchor="middle" fontSize="8" fill="#22D3EE">connector_chunks · admin_audit_logs · policies · api_tokens</text>

      {/* ── AI PROVIDERS ── */}
      <rect x="1002" y="362" width="384" height="148" rx="14" fill="none" stroke="#D97706" strokeWidth="2.5" filter="url(#ga)" className="pb" opacity=".6"/>
      <rect x="1002" y="362" width="384" height="148" rx="14" fill="#120C04" stroke="#92400E" strokeWidth="1.2"/>
      <rect x="1002" y="362" width="384" height="42" rx="14" fill="url(#hdrA)"/>
      <rect x="1002" y="390" width="384" height="14" fill="url(#hdrA)"/>
      <circle cx="1022" cy="383" r="4" fill="#FCD34D" filter="url(#ga)"/>
      <text x="1194" y="387" textAnchor="middle" fontSize="12" fontWeight="800" fill="white" letterSpacing="1" filter="url(#tg)">AI MODEL PROVIDERS</text>
      <text x="1194" y="400" textAnchor="middle" fontSize="8.5" fill="#FDE68A" letterSpacing="1.5">UNIFIED ADAPTER · STREAMING · COST TRACKING</text>
      {[['Anthropic',1010],['OpenAI',1100],['Gemini',1190],['Ollama (local)',1278]].map(([label, x]) => (
        <g key={label}>
          <rect x={Number(x)} y="410" width={label === 'Ollama (local)' ? 100 : 82} height="26" rx="8" fill="#1C1004" stroke="#D97706" strokeWidth="1"/>
          <text x={Number(x) + (label === 'Ollama (local)' ? 50 : 41)} y="427" textAnchor="middle" fontSize="9" fontWeight="700" fill="#FCD34D">{label}</text>
        </g>
      ))}
      <rect x="1010" y="444" width="368" height="18" rx="5" fill="#1A1004" stroke="#92400E" strokeWidth=".7"/>
      <text x="1194" y="457" textAnchor="middle" fontSize="8.5" fill="#FDE68A">API key + OAuth · live model discovery · streaming completions</text>
      <rect x="1010" y="468" width="368" height="26" rx="5" fill="#1A1004" stroke="#92400E" strokeWidth=".7"/>
      <text x="1194" y="485" textAnchor="middle" fontSize="8.5" fill="#FDE68A">Claude · GPT-4 · Gemini 1.5 · local LLMs · per-run cost estimate</text>

      {/* ── MCP SERVERS ── */}
      <rect x="1002" y="526" width="384" height="118" rx="14" fill="none" stroke="#059669" strokeWidth="2.5" filter="url(#gg)" className="pb" opacity=".6"/>
      <rect x="1002" y="526" width="384" height="118" rx="14" fill="#041A10" stroke="#065F46" strokeWidth="1.2"/>
      <rect x="1002" y="526" width="384" height="42" rx="14" fill="url(#hdrG)"/>
      <rect x="1002" y="554" width="384" height="14" fill="url(#hdrG)"/>
      <circle cx="1022" cy="547" r="4" fill="#34D399" filter="url(#gg)"/>
      <text x="1194" y="551" textAnchor="middle" fontSize="12" fontWeight="800" fill="white" letterSpacing="1" filter="url(#tg)">MCP SERVERS</text>
      <text x="1194" y="564" textAnchor="middle" fontSize="8.5" fill="#6EE7B7" letterSpacing="1.5">MODEL CONTEXT PROTOCOL · TOOL DISCOVERY</text>
      <rect x="1010" y="576" width="176" height="22" rx="6" fill="#052E16" stroke="#065F46" strokeWidth=".8"/>
      <text x="1098" y="591" textAnchor="middle" fontSize="8.5" fill="#6EE7B7">HTTP transport (remote)</text>
      <rect x="1194" y="576" width="184" height="22" rx="6" fill="#052E16" stroke="#065F46" strokeWidth=".8"/>
      <text x="1286" y="591" textAnchor="middle" fontSize="8.5" fill="#6EE7B7">stdio transport (local process)</text>
      <rect x="1010" y="606" width="368" height="26" rx="5" fill="#052E16" stroke="#065F46" strokeWidth=".7"/>
      <text x="1194" y="623" textAnchor="middle" fontSize="8.5" fill="#34D399">Docker host rewrite · UUID mirroring · risk levels · approval gate</text>

      {/* ── WEBHOOK SOURCES ── */}
      <rect x="14" y="624" width="344" height="140" rx="14" fill="none" stroke="#E11D48" strokeWidth="2.5" filter="url(#gr)" className="pb" opacity=".6"/>
      <rect x="14" y="624" width="344" height="140" rx="14" fill="#130009" stroke="#7F1D1D" strokeWidth="1.2"/>
      <rect x="14" y="624" width="344" height="42" rx="14" fill="url(#hdrR)"/>
      <rect x="14" y="652" width="344" height="14" fill="url(#hdrR)"/>
      <circle cx="34" cy="645" r="4" fill="#FB7185" filter="url(#gr)"/>
      <text x="186" y="649" textAnchor="middle" fontSize="12" fontWeight="800" fill="white" letterSpacing="1" filter="url(#tg)">WEBHOOK / EVENT SOURCES</text>
      <text x="186" y="663" textAnchor="middle" fontSize="8.5" fill="#FDA4AF" letterSpacing="1.5">INBOUND HTTP → ASYNC AGENT RUNS</text>
      {[['GitHub',26],['Zapier',108],['IFTTT',190],['Any HTTP',272]].map(([label, x]) => (
        <g key={label}>
          <rect x={Number(x)} y="676" width="74" height="24" rx="7" fill="#1F0A12" stroke="#9F1239" strokeWidth=".8"/>
          <text x={Number(x) + 37} y="692" textAnchor="middle" fontSize="9" fontWeight="700" fill="#FB7185">{label}</text>
        </g>
      ))}
      <rect x="26" y="708" width="328" height="20" rx="5" fill="#1F0A12" stroke="#7F1D1D" strokeWidth=".7"/>
      <text x="190" y="722" textAnchor="middle" fontSize="8.5" fill="#FDA4AF">POST /webhook/{'{id}'}  ·  HMAC-SHA256  ·  Go template mapping</text>
      <rect x="26" y="734" width="328" height="20" rx="5" fill="#1F0A12" stroke="#7F1D1D" strokeWidth=".7"/>
      <text x="190" y="748" textAnchor="middle" fontSize="8.5" fill="#FDA4AF">trigger_id on runs · fire-and-forget dispatch · audit logged</text>

      {/* ── DOCKER INFRA ── */}
      <rect x="406" y="748" width="542" height="72" rx="12" fill="#0A0A1E" stroke="#312E81" strokeWidth="1.5" filter="url(#gp)" opacity=".7"/>
      <circle cx="424" cy="777" r="4" fill="#34D399" filter="url(#gg)" className="blink"/>
      <circle cx="438" cy="777" r="4" fill="#FCD34D" filter="url(#ga)" className="blink"/>
      <circle cx="452" cy="777" r="4" fill="#22D3EE" filter="url(#gc)" className="blink"/>
      <text x="677" y="772" textAnchor="middle" fontSize="11" fontWeight="700" fill="#A78BFA" letterSpacing="2">DOCKER COMPOSE INFRASTRUCTURE</text>
      <text x="677" y="788" textAnchor="middle" fontSize="9" fill="#6D7EE0">agent-nexus-api :8080  ·  agent-nexus-web :3000  ·  agent-nexus-postgres :5432</text>
      <text x="677" y="806" textAnchor="middle" fontSize="8.5" fill="#4B5563">Auto-migration on startup  ·  Go 1.26+  ·  Node 22+  ·  pnpm workspaces  ·  Build 22</text>

      {/* ── LEGEND ── */}
      <rect x="1002" y="660" width="384" height="108" rx="12" fill="#09091E" stroke="#312E81" strokeWidth="1"/>
      <text x="1194" y="678" textAnchor="middle" fontSize="9" fontWeight="700" fill="#64748B" letterSpacing="3">LEGEND</text>
      <line x1="1014" y1="692" x2="1058" y2="692" stroke="#A78BFA" strokeWidth="2" strokeDasharray="7 4" markerEnd="url(#ap)"/>
      <text x="1066" y="696" fontSize="9" fill="#94A3B8">REST / SSE  (authenticated)</text>
      <line x1="1014" y1="712" x2="1058" y2="712" stroke="#22D3EE" strokeWidth="2" markerEnd="url(#ac)"/>
      <text x="1066" y="716" fontSize="9" fill="#94A3B8">pgx/v5 database queries</text>
      <line x1="1014" y1="732" x2="1058" y2="732" stroke="#FCD34D" strokeWidth="2" strokeDasharray="5 4" markerEnd="url(#aa)"/>
      <text x="1066" y="736" fontSize="9" fill="#94A3B8">AI API calls  (streaming)</text>
      <line x1="1014" y1="752" x2="1058" y2="752" stroke="#34D399" strokeWidth="2" strokeDasharray="5 4" markerEnd="url(#ag)"/>
      <text x="1066" y="756" fontSize="9" fill="#94A3B8">MCP protocol</text>
      <line x1="1194" y1="692" x2="1238" y2="692" stroke="#FB7185" strokeWidth="2" strokeDasharray="7 4" markerEnd="url(#ar)"/>
      <text x="1246" y="696" fontSize="9" fill="#94A3B8">Inbound webhook</text>
      <circle cx="1212" cy="712" r="4" fill="#A78BFA" filter="url(#gp)"/>
      <text x="1246" y="716" fontSize="9" fill="#94A3B8">Connection node</text>
      <circle cx="1212" cy="732" r="3.5" fill="#34D399" className="blink" filter="url(#gg)"/>
      <text x="1246" y="736" fontSize="9" fill="#94A3B8">Live indicator</text>

      {/* ═══════════════════════════════════════════════════════════ */}
      {/* ANIMATED CONNECTION LINES                                  */}
      {/* ═══════════════════════════════════════════════════════════ */}

      {/* 1. Frontend ↔ API  (purple) */}
      <line x1="358" y1="350" x2="406" y2="350" stroke="#A78BFA" strokeWidth="6" opacity=".15" filter="url(#gp)"/>
      <line x1="358" y1="350" x2="406" y2="350" stroke="#A78BFA" strokeWidth="2" className="fp" markerEnd="url(#ap)"/>
      <line x1="406" y1="358" x2="358" y2="358" stroke="#7C3AED" strokeWidth="1.5" className="fp" markerEnd="url(#ap)" opacity=".5"/>
      <circle cx="358" cy="354" r="4" fill="#A78BFA" filter="url(#gp)" className="pd"/>
      <circle cx="406" cy="354" r="4" fill="#A78BFA" filter="url(#gp)"/>
      <rect x="361" y="336" width="40" height="14" rx="4" fill="#13174A"/>
      <text x="381" y="347" textAnchor="middle" fontSize="8" fill="#A78BFA">REST/SSE</text>
      <circle r="3.5" fill="#E879F9" opacity=".9" filter="url(#gp)">
        <animateMotion dur="1.4s" repeatCount="indefinite" path="M358,350 L406,350"/>
      </circle>
      <circle r="2.5" fill="#A78BFA" opacity=".6">
        <animateMotion dur="1.4s" begin="0.7s" repeatCount="indefinite" path="M358,350 L406,350"/>
      </circle>

      {/* 2. API → PostgreSQL (cyan) */}
      <line x1="948" y1="280" x2="1002" y2="280" stroke="#22D3EE" strokeWidth="6" opacity=".15" filter="url(#gc)"/>
      <line x1="948" y1="280" x2="1002" y2="280" stroke="#22D3EE" strokeWidth="2" className="fc" markerEnd="url(#ac)"/>
      <circle cx="948" cy="280" r="4" fill="#22D3EE" filter="url(#gc)" className="pd"/>
      <circle cx="1002" cy="280" r="4" fill="#22D3EE" filter="url(#gc)"/>
      <rect x="951" y="264" width="44" height="14" rx="4" fill="#061A2A"/>
      <text x="973" y="275" textAnchor="middle" fontSize="8" fill="#22D3EE">pgx/v5</text>
      <circle r="3.5" fill="#67E8F9" opacity=".9" filter="url(#gc)">
        <animateMotion dur="1.1s" repeatCount="indefinite" path="M948,280 L1002,280"/>
      </circle>
      <circle r="2.5" fill="#22D3EE" opacity=".6">
        <animateMotion dur="1.1s" begin="0.55s" repeatCount="indefinite" path="M948,280 L1002,280"/>
      </circle>

      {/* 3. API → AI Providers (amber) */}
      <line x1="948" y1="450" x2="1002" y2="450" stroke="#FCD34D" strokeWidth="6" opacity=".15" filter="url(#ga)"/>
      <line x1="948" y1="450" x2="1002" y2="450" stroke="#FCD34D" strokeWidth="2" className="fa" markerEnd="url(#aa)"/>
      <circle cx="948" cy="450" r="4" fill="#FCD34D" filter="url(#ga)" className="pd"/>
      <circle cx="1002" cy="450" r="4" fill="#FCD34D" filter="url(#ga)"/>
      <rect x="950" y="434" width="48" height="14" rx="4" fill="#1A1004"/>
      <text x="974" y="445" textAnchor="middle" fontSize="8" fill="#FCD34D">AI calls</text>
      <circle r="3.5" fill="#FDE68A" opacity=".9" filter="url(#ga)">
        <animateMotion dur="1.7s" repeatCount="indefinite" path="M948,450 L1002,450"/>
      </circle>
      <circle r="2.5" fill="#FCD34D" opacity=".6">
        <animateMotion dur="1.7s" begin="0.85s" repeatCount="indefinite" path="M948,450 L1002,450"/>
      </circle>

      {/* 4. API → MCP Servers (green) */}
      <line x1="948" y1="590" x2="1002" y2="590" stroke="#34D399" strokeWidth="6" opacity=".15" filter="url(#gg)"/>
      <line x1="948" y1="590" x2="1002" y2="590" stroke="#34D399" strokeWidth="2" className="fg" markerEnd="url(#ag)"/>
      <circle cx="948" cy="590" r="4" fill="#34D399" filter="url(#gg)" className="pd"/>
      <circle cx="1002" cy="590" r="4" fill="#34D399" filter="url(#gg)"/>
      <rect x="950" y="574" width="42" height="14" rx="4" fill="#052E16"/>
      <text x="971" y="585" textAnchor="middle" fontSize="8" fill="#34D399">MCP</text>
      <circle r="3.5" fill="#6EE7B7" opacity=".9" filter="url(#gg)">
        <animateMotion dur="2s" repeatCount="indefinite" path="M948,590 L1002,590"/>
      </circle>
      <circle r="2.5" fill="#34D399" opacity=".6">
        <animateMotion dur="2s" begin="1s" repeatCount="indefinite" path="M948,590 L1002,590"/>
      </circle>

      {/* 5. Webhook → API (rose, curved) */}
      <path d="M358,695 C382,695 406,672 406,652" fill="none" stroke="#FB7185" strokeWidth="6" opacity=".12" filter="url(#gr)"/>
      <path d="M358,695 C382,695 406,672 406,652" fill="none" stroke="#FB7185" strokeWidth="2" className="fr" markerEnd="url(#ar)"/>
      <circle cx="358" cy="695" r="4" fill="#FB7185" filter="url(#gr)" className="pd"/>
      <circle cx="406" cy="652" r="4" fill="#FB7185" filter="url(#gr)"/>
      <rect x="348" y="669" width="58" height="14" rx="4" fill="#130009"/>
      <text x="377" y="680" textAnchor="middle" fontSize="8" fill="#FB7185">HTTP POST</text>
      <circle r="3.5" fill="#FDA4AF" opacity=".9" filter="url(#gr)">
        <animateMotion dur="1.3s" repeatCount="indefinite" path="M358,695 C382,695 406,672 406,652"/>
      </circle>
      <circle r="2.5" fill="#FB7185" opacity=".6">
        <animateMotion dur="1.3s" begin="0.65s" repeatCount="indefinite" path="M358,695 C382,695 406,672 406,652"/>
      </circle>

      {/* Version badge */}
      <rect x="1270" y="14" width="118" height="26" rx="10" fill="#0D1035" stroke="#4C1D95" strokeWidth="1" filter="url(#gp)" opacity=".8"/>
      <text x="1329" y="31" textAnchor="middle" fontSize="10" fontWeight="700" fill="#A78BFA">Build 22 · v0.1.0</text>
    </svg>
  )
}
