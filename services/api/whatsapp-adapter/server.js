import http from 'node:http'
import fs from 'node:fs/promises'
import path from 'node:path'
import { Writable } from 'node:stream'
import { Boom } from '@hapi/boom'
import makeWASocket, {
  DisconnectReason,
  fetchLatestBaileysVersion,
  useMultiFileAuthState
} from '@whiskeysockets/baileys'
import pino from 'pino'
import QRCode from 'qrcode'

const PORT = Number(process.env.PORT || 18901)
const AUTH_ROOT = process.env.AUTH_ROOT || '/data/whatsapp-auth'
const API_INTERNAL_URL = process.env.API_INTERNAL_URL || 'http://127.0.0.1:8080'
const LOG_STREAM_INGEST_TOKEN = process.env.LOG_STREAM_INGEST_TOKEN || ''
const logger = pino({ level: process.env.LOG_LEVEL || 'info' }, createLogStream())
const accounts = new Map()

function createLogStream() {
  return new Writable({
    write(chunk, _encoding, callback) {
      const line = chunk.toString()
      process.stdout.write(line)
      forwardLogLine(line)
      callback()
    }
  })
}

function forwardLogLine(line) {
  if (!LOG_STREAM_INGEST_TOKEN || typeof fetch !== 'function') return
  let parsed
  try {
    parsed = JSON.parse(line)
  } catch {
    parsed = { msg: line.trim(), level: 'info' }
  }
  const level = typeof parsed.level === 'number'
    ? pinoLevelName(parsed.level)
    : String(parsed.level || 'info').toLowerCase()
  const ts = typeof parsed.time === 'number' ? new Date(parsed.time).toISOString() : new Date().toISOString()
  const message = String(parsed.msg || parsed.message || line.trim())
  const attrs = { ...parsed }
  delete attrs.time
  delete attrs.level
  delete attrs.msg
  delete attrs.message

  fetch(`${API_INTERNAL_URL}/internal/service-logs/ingest`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'X-Log-Stream-Token': LOG_STREAM_INGEST_TOKEN
    },
    body: JSON.stringify({
      ts,
      source: 'whatsapp-adapter',
      level,
      message,
      attrs
    })
  }).catch(() => {})
}

function pinoLevelName(level) {
  if (level >= 60) return 'fatal'
  if (level >= 50) return 'error'
  if (level >= 40) return 'warn'
  if (level >= 30) return 'info'
  if (level >= 20) return 'debug'
  return 'trace'
}

function json(res, code, body) {
  const payload = JSON.stringify(body)
  res.writeHead(code, { 'Content-Type': 'application/json', 'Content-Length': Buffer.byteLength(payload) })
  res.end(payload)
}

async function readJSON(req) {
  const chunks = []
  for await (const chunk of req) chunks.push(chunk)
  if (chunks.length === 0) return {}
  return JSON.parse(Buffer.concat(chunks).toString('utf8'))
}

function accountState(accountId) {
  if (!accounts.has(accountId)) {
    accounts.set(accountId, {
      accountId,
      socket: null,
      status: 'disconnected',
      qr: '',
      qrDataURL: '',
      selfId: '',
      selfLid: '',
      lastError: '',
      callbackUrl: '',
      selfChatEnabled: false,
      sentMessageIds: new Set(),
      messageStore: new Map(),  // id → proto message, for retry requests
      lidToPhone: new Map(),
      pendingPhones: [],
      connectedAt: 0,           // epoch ms when last connection.open fired
      lastMessageAt: 0          // epoch ms of last forwarded inbound message
    })
  }
  return accounts.get(accountId)
}

async function resolvePhonesLIDs(state) {
  if (!state.socket || !state.pendingPhones.length) return
  const phones = [...state.pendingPhones]
  state.pendingPhones = []
  try {
    const results = await state.socket.onWhatsApp(...phones)
    for (const r of results || []) {
      if (r.jid && r.lid) {
        const phoneBare = r.jid.split('@')[0].split(':')[0]
        const lidBare = r.lid.split('@')[0].split(':')[0]
        if (phoneBare && lidBare) {
          state.lidToPhone.set(lidBare, phoneBare)
          logger.info({ accountId: state.accountId, lid: r.lid, phone: r.jid }, 'LID resolved via onWhatsApp')
        }
      }
    }
    logger.info({ accountId: state.accountId, mapSize: state.lidToPhone.size }, 'LID map updated')
  } catch (err) {
    logger.warn({ err, accountId: state.accountId }, 'LID resolution via onWhatsApp failed')
  }
}

// ── Credential persistence (survives Railway deploys) ─────────────────────────

const backupTimers = new Map()

async function backupAuthState(accountId) {
  const dir = path.join(AUTH_ROOT, accountId)
  try {
    const entries = await fs.readdir(dir)
    const files = {}
    for (const name of entries) {
      if (!name.endsWith('.json')) continue
      const raw = await fs.readFile(path.join(dir, name), 'utf8')
      try { files[name] = JSON.parse(raw) } catch { files[name] = raw }
    }
    if (Object.keys(files).length === 0) return
    const res = await fetch(`${API_INTERNAL_URL}/internal/whatsapp/${encodeURIComponent(accountId)}/credentials`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ files })
    })
    if (!res.ok) throw new Error(`backup PUT returned ${res.status}`)
    logger.info({ accountId, fileCount: Object.keys(files).length }, 'backed up auth state to DB')
  } catch (err) {
    logger.warn({ err, accountId }, 'failed to backup auth state')
  }
}

function scheduleBackup(accountId) {
  if (backupTimers.has(accountId)) clearTimeout(backupTimers.get(accountId))
  backupTimers.set(accountId, setTimeout(() => {
    backupTimers.delete(accountId)
    backupAuthState(accountId).catch(() => {})
  }, 2000))
}

async function restoreAuthState(accountId) {
  try {
    const res = await fetch(`${API_INTERNAL_URL}/internal/whatsapp/${encodeURIComponent(accountId)}/credentials`)
    if (!res.ok) return false
    const { files } = await res.json()
    if (!files || Object.keys(files).length === 0) return false
    const dir = path.join(AUTH_ROOT, accountId)
    await fs.mkdir(dir, { recursive: true })
    for (const [name, content] of Object.entries(files)) {
      const data = typeof content === 'string' ? content : JSON.stringify(content)
      await fs.writeFile(path.join(dir, name), data)
    }
    logger.info({ accountId, fileCount: Object.keys(files).length }, 'restored auth state from DB')
    return true
  } catch (err) {
    logger.warn({ err, accountId }, 'failed to restore auth state')
    return false
  }
}

async function deleteAuthStateFromDB(accountId) {
  try {
    await fetch(`${API_INTERNAL_URL}/internal/whatsapp/${encodeURIComponent(accountId)}/credentials`, { method: 'DELETE' })
  } catch (_) {}
}

// ──────────────────────────────────────────────────────────────────────────────

async function startAccount(accountId, opts = {}) {
  const state = accountState(accountId)
  if (opts.callback_url) state.callbackUrl = opts.callback_url
  if (Array.isArray(opts.contact_phones) && opts.contact_phones.length > 0) {
    state.pendingPhones = opts.contact_phones
  }
  if (state.socket) {
    // Socket already exists. selfChatEnabled is managed via POST /accounts/{id}/config —
    // don't overwrite it here so concurrent reconnect timers can't undo a /config update.
    if (state.status === 'connected' && state.pendingPhones.length > 0) {
      resolvePhonesLIDs(state).catch(() => {})
    }
    return state
  }
  // New connection — apply initial config from opts.
  state.selfChatEnabled = !!opts.self_chat_enabled

  // Restore credentials from DB if the local auth dir is missing (e.g. after a Railway deploy).
  await restoreAuthState(accountId)
  await fs.mkdir(path.join(AUTH_ROOT, accountId), { recursive: true })
  const { state: authState, saveCreds } = await useMultiFileAuthState(path.join(AUTH_ROOT, accountId))
  const { version } = await fetchLatestBaileysVersion()

  state.status = 'connecting'
  state.lastError = ''
  const socket = makeWASocket({
    version,
    auth: authState,
    printQRInTerminal: false,
    logger: logger.child({ accountId }),
    // Skip full chat history sync on link — avoids "Couldn't finish syncing"
    // caused by Baileys 6.x failing to decode WhatsApp's critical_block patches.
    syncFullHistory: false,
    // Required so Baileys can respond to WhatsApp retry requests.
    // Without this, failed decryptions show "Waiting for this message" permanently.
    getMessage: async (key) => {
      const stored = state.messageStore.get(key.id)
      if (stored) return stored
      return { conversation: '' }
    }
  })
  state.socket = socket

  socket.ev.on('creds.update', () => { saveCreds(); scheduleBackup(accountId) })

  // Build LID→phone map so we can resolve @lid JIDs to real phone numbers for contact matching.
  // WhatsApp migrated to LID-based JIDs; contact.id has the phone JID, contact.lid has the LID.
  const updateLidMap = (contacts) => {
    for (const contact of contacts) {
      const cid = contact.id || ''
      const clid = contact.lid || ''
      if (cid && clid && !cid.endsWith('@lid')) {
        const phoneBare = cid.split('@')[0].split(':')[0]
        const lidBare = clid.split('@')[0].split(':')[0]
        if (phoneBare && lidBare) {
          state.lidToPhone.set(lidBare, phoneBare)
          logger.info({ accountId, lid: clid, phone: cid }, 'LID mapped')
        }
      }
    }
  }
  socket.ev.on('contacts.upsert', updateLidMap)
  socket.ev.on('contacts.update', updateLidMap)

  socket.ev.on('connection.update', async ({ connection, qr, lastDisconnect }) => {
    if (qr) {
      state.qr = qr
      state.qrDataURL = await QRCode.toDataURL(qr, { margin: 1, width: 320 })
      state.status = 'qr'
    }
    if (connection === 'open') {
      state.status = 'connected'
      state.qr = ''
      state.qrDataURL = ''
      state.selfId = socket.user?.id || ''
      state.selfLid = socket.user?.lid || ''
      state.lastError = ''
      state.connectedAt = Date.now()
      backupAuthState(accountId).catch(() => {})
      // Newer WhatsApp clients use opaque LID JIDs; Baileys may not expose socket.user.lid.
      // Resolve selfLid via onWhatsApp so the from_me self-chat filter can match correctly.
      if (!state.selfLid && state.selfId) {
        const ownPhone = state.selfId.split('@')[0].split(':')[0]
        if (ownPhone) {
          socket.onWhatsApp(`+${ownPhone}`).then((results) => {
            if (results?.[0]?.lid) {
              state.selfLid = results[0].lid
              logger.info({ accountId, selfLid: state.selfLid }, 'selfLid resolved via onWhatsApp')
            }
          }).catch((err) => {
            logger.warn({ err, accountId }, 'selfLid onWhatsApp resolution failed')
          })
        }
      }
      if (state.pendingPhones.length > 0) {
        resolvePhonesLIDs(state).catch(() => {})
      }
    }
    if (connection === 'close') {
      const statusCode = new Boom(lastDisconnect?.error)?.output?.statusCode
      state.status = 'disconnected'
      state.socket = null
      state.lastError = lastDisconnect?.error?.message || ''
      if (statusCode !== DisconnectReason.loggedOut) {
        setTimeout(() => startAccount(accountId, {
          callback_url: state.callbackUrl,
          self_chat_enabled: state.selfChatEnabled
        }).catch((err) => {
          state.lastError = err.message
          logger.error({ err, accountId }, 'whatsapp reconnect failed')
        }), 3000)
      }
    }
  })

  socket.ev.on('messages.upsert', async ({ messages, type }) => {
    if (type !== 'notify') return
    for (const msg of messages || []) {
      if (!msg.message) continue
      // Cache every message so getMessage() can serve retry requests.
      if (msg.key.id) {
        state.messageStore.set(msg.key.id, msg.message)
        if (state.messageStore.size > 500) {
          state.messageStore.delete(state.messageStore.keys().next().value)
        }
      }
      if (msg.key.fromMe) {
        const sentID = msg.key.id || ''
        if (sentID && state.sentMessageIds.has(sentID)) {
          state.sentMessageIds.delete(sentID)
          logger.info({ accountId, messageId: sentID }, 'ignored adapter-sent whatsapp message')
          continue
        }
        if (!state.selfChatEnabled) {
          logger.info({ accountId, key: msg.key }, 'ignored from_me whatsapp message because self-chat is disabled')
          continue
        }
        // Even with selfChatEnabled=true, only forward messages sent to the account's own
        // JID (genuine self-chat). Messages the owner sends to other people arrive as
        // from_me=true but the peer is a different JID — forwarding those would make the
        // agent reply into every chat on the owner's behalf.
        // WhatsApp uses @lid JIDs in newer clients; compare both phone JID and LID.
        const remoteJid = msg.key.remoteJid || ''
        const ownBare = (state.selfId || '').split('@')[0].split(':')[0]
        const ownLidBare = (state.selfLid || '').split('@')[0].split(':')[0]
        const remoteBare = remoteJid.split('@')[0].split(':')[0]
        const isSelf = (ownBare && remoteBare === ownBare) || (ownLidBare && remoteBare === ownLidBare)
        if (!isSelf) {
          logger.info({ accountId, remoteJid }, 'ignored from_me message to non-self peer')
          continue
        }
      }
      await forwardMessage(accountId, state, msg)
    }
  })

  return state
}

async function forwardMessage(accountId, state, msg) {
  if (!state.callbackUrl) return
  const remoteJid = msg.key.remoteJid || ''
  const participant = msg.key.participant || remoteJid
  const text = messageText(msg.message)
  if (!text) return

  // Resolve @lid JIDs to phone-based JIDs so Go can match contacts and deliver replies.
  // Check if the LID is the bot itself (self-chat), then check the contact LID→phone map.
  const resolveJid = (jid) => {
    if (!jid.endsWith('@lid')) return jid
    const lidBare = jid.split('@')[0].split(':')[0]
    const ownLidBare = (state.selfLid || '').split('@')[0].split(':')[0]
    if (ownLidBare && lidBare === ownLidBare) return state.selfId || jid
    const phoneBare = state.lidToPhone?.get(lidBare)
    return phoneBare ? `${phoneBare}@s.whatsapp.net` : jid
  }

  const resolvedParticipant = resolveJid(participant)
  const resolvedRemoteJid = resolveJid(remoteJid)

  const payload = {
    type: 'message.received',
    account_id: accountId,
    message_id: msg.key.id || '',
    peer: {
      kind: remoteJid.endsWith('@g.us') ? 'group' : 'direct',
      id: resolvedRemoteJid
    },
    sender: {
      id: resolvedParticipant,
      phone_number: jidPhone(resolvedParticipant),
      display_name: msg.pushName || ''
    },
    body: text,
    from_me: !!msg.key.fromMe,
    media: [],
    received_at: new Date(Number(msg.messageTimestamp || Date.now() / 1000) * 1000).toISOString()
  }
  try {
    logger.info({
      accountId,
      messageId: payload.message_id,
      fromMe: payload.from_me,
      peer: payload.peer,
      sender: payload.sender
    }, 'forwarding whatsapp message')
    await postJSON(state.callbackUrl, payload)
    state.lastMessageAt = Date.now()
  } catch (err) {
    state.lastError = err.message
    logger.error({ err, accountId }, 'failed to forward whatsapp message')
  }
}

function jidPhone(jid) {
  const bare = String(jid || '').split('@')[0].split(':')[0]
  return bare ? `+${bare.replace(/\D/g, '')}` : ''
}

function messageText(message) {
  return message.conversation ||
    message.extendedTextMessage?.text ||
    message.imageMessage?.caption ||
    message.videoMessage?.caption ||
    ''
}

function peerToJid(peer) {
  const id = peer?.id || ''
  if (id.includes('@')) return id
  return id.replace(/\D/g, '') + '@s.whatsapp.net'
}

async function postJSON(target, body) {
  const res = await fetch(target, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body)
  })
  if (!res.ok) throw new Error(`${res.status} ${await res.text()}`)
  return res.json().catch(() => ({}))
}

async function route(req, res) {
  const url = new URL(req.url, `http://${req.headers.host}`)
  const parts = url.pathname.split('/').filter(Boolean)

  if (req.method === 'GET' && url.pathname === '/health') {
    return json(res, 200, { status: 'ok' })
  }

  if (parts[0] !== 'accounts' || !parts[1]) return json(res, 404, { error: 'not found' })

  const accountId = decodeURIComponent(parts[1])
  const state = accountState(accountId)

  if (req.method === 'GET' && parts[2] === 'status') {
    return json(res, 200, {
      account_id: accountId,
      status: state.status,
      self_id: state.selfId,
      self_lid: state.selfLid,
      last_error: state.lastError,
      has_qr: !!state.qr,
      qr_data_url: state.qrDataURL || '',
      callback_url: state.callbackUrl,
      self_chat_enabled: state.selfChatEnabled,
      lid_map_size: state.lidToPhone?.size || 0,
      connected_at: state.connectedAt || 0,
      last_message_at: state.lastMessageAt || 0
    })
  }

  // Force-close the current socket and reconnect without losing credentials.
  if (req.method === 'POST' && parts[2] === 'reconnect') {
    const callbackUrl = state.callbackUrl
    const selfChatEnabled = state.selfChatEnabled
    if (state.socket) {
      try { state.socket.end(undefined) } catch (_) {}
      state.socket = null
      state.status = 'disconnected'
    }
    await startAccount(accountId, { callback_url: callbackUrl, self_chat_enabled: selfChatEnabled })
    return json(res, 200, { account_id: accountId, status: state.status })
  }

  if (req.method === 'POST' && parts[2] === 'login' && parts[3] === 'start') {
    const body = await readJSON(req)
    const started = await startAccount(accountId, body)
    return json(res, 200, { account_id: accountId, status: started.status, has_qr: !!started.qr })
  }

  if (req.method === 'POST' && parts[2] === 'config') {
    const body = await readJSON(req)
    if (body.callback_url) state.callbackUrl = body.callback_url
    if (Object.prototype.hasOwnProperty.call(body, 'self_chat_enabled')) {
      state.selfChatEnabled = !!body.self_chat_enabled
    }
    return json(res, 200, {
      account_id: accountId,
      status: state.status,
      callback_url: state.callbackUrl,
      self_chat_enabled: state.selfChatEnabled
    })
  }

  if (req.method === 'GET' && parts[2] === 'login' && parts[3] === 'qr') {
    return json(res, 200, {
      account_id: accountId,
      status: state.status,
      qr: state.qr,
      qr_data_url: state.qrDataURL,
      self_id: state.selfId,
      last_error: state.lastError
    })
  }

  if (req.method === 'POST' && parts[2] === 'logout') {
    if (state.socket) {
      try { await state.socket.logout() } catch (_) {}
    }
    state.socket = null
    state.status = 'disconnected'
    state.qr = ''
    state.qrDataURL = ''
    state.selfId = ''
    state.selfLid = ''
    state.lastError = ''
    // Delete stored credentials (disk + DB) so the next /login/start gets a fresh QR code.
    const authDir = path.join(AUTH_ROOT, accountId)
    await fs.rm(authDir, { recursive: true, force: true })
    await deleteAuthStateFromDB(accountId)
    return json(res, 200, { account_id: accountId, status: state.status })
  }

  if (req.method === 'POST' && parts[2] === 'send') {
    const body = await readJSON(req)
    const active = state.socket ? state : await startAccount(accountId)
    if (!active.socket || active.status !== 'connected') {
      return json(res, 409, { error: 'account is not connected', status: active.status })
    }
    const sent = await active.socket.sendMessage(peerToJid(body.peer), { text: body.text || '' })
    const messageId = sent?.key?.id || ''
    if (messageId) {
      active.sentMessageIds.add(messageId)
      if (sent.message) {
        active.messageStore.set(messageId, sent.message)
        if (active.messageStore.size > 500) {
          active.messageStore.delete(active.messageStore.keys().next().value)
        }
      }
    }
    return json(res, 200, { status: 'sent', message_id: messageId })
  }

  return json(res, 404, { error: 'not found' })
}

http.createServer((req, res) => {
  route(req, res).catch((err) => {
    logger.error({ err }, 'request failed')
    json(res, 500, { error: err.message })
  })
}).listen(PORT, '0.0.0.0', () => {
  logger.info({ port: PORT }, 'whatsapp adapter listening')
})
