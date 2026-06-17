import http from 'node:http'
import fs from 'node:fs/promises'
import path from 'node:path'
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
const logger = pino({ level: process.env.LOG_LEVEL || 'info' })
const accounts = new Map()

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
      lastError: '',
      callbackUrl: '',
      selfChatEnabled: false,
      sentMessageIds: new Set()
    })
  }
  return accounts.get(accountId)
}

async function startAccount(accountId, opts = {}) {
  const state = accountState(accountId)
  if (opts.callback_url) state.callbackUrl = opts.callback_url
  state.selfChatEnabled = !!opts.self_chat_enabled
  if (state.socket) return state

  await fs.mkdir(path.join(AUTH_ROOT, accountId), { recursive: true })
  const { state: authState, saveCreds } = await useMultiFileAuthState(path.join(AUTH_ROOT, accountId))
  const { version } = await fetchLatestBaileysVersion()

  state.status = 'connecting'
  state.lastError = ''
  const socket = makeWASocket({
    version,
    auth: authState,
    printQRInTerminal: false,
    logger: logger.child({ accountId })
  })
  state.socket = socket

  socket.ev.on('creds.update', saveCreds)

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
      state.lastError = ''
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
  const payload = {
    type: 'message.received',
    account_id: accountId,
    message_id: msg.key.id || '',
    peer: {
      kind: remoteJid.endsWith('@g.us') ? 'group' : 'direct',
      id: remoteJid
    },
    sender: {
      id: participant,
      phone_number: jidPhone(participant),
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
      last_error: state.lastError,
      has_qr: !!state.qr,
      callback_url: state.callbackUrl,
      self_chat_enabled: state.selfChatEnabled
    })
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
    if (state.socket) await state.socket.logout()
    state.socket = null
    state.status = 'disconnected'
    state.qr = ''
    state.qrDataURL = ''
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
    if (messageId) active.sentMessageIds.add(messageId)
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
