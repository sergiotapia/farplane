#!/usr/bin/env node
/**
 * Farplane agent bridge — headless CLI relay inside a Lane computer.
 * Listens on BRIDGE_PORT (default 7420). Farplane Go Runtime connects here;
 * browsers never talk to this port directly.
 */

import http from 'node:http'
import { spawn } from 'node:child_process'
import { randomUUID } from 'node:crypto'

const PORT = Number(process.env.BRIDGE_PORT || 7420)
const PROVIDER = process.env.FARPLANE_AGENT_PROVIDER || 'claude_code'

/** @type {Map<string, import('node:http').ServerResponse>} */
const streams = new Map()

/** Pending agent-switch handoff transcript; prepended to the next user turn. */
let pendingHandoffTranscript = null

/** @type {import('node:child_process').ChildProcess | null} */
let activeChild = null
let turnBusy = false

function sendSSE(res, event) {
  res.write(`data: ${JSON.stringify(event)}\n\n`)
}

function broadcast(event) {
  for (const s of streams.values()) sendSSE(s, event)
}

function writeStatus(res, status) {
  sendSSE(res, { type: 'status', status })
}

function killActiveChild() {
  if (!activeChild || activeChild.killed) return false
  try {
    activeChild.kill('SIGTERM')
    setTimeout(() => {
      if (activeChild && !activeChild.killed) {
        try {
          activeChild.kill('SIGKILL')
        } catch {
          // ignore
        }
      }
    }, 2000)
    return true
  } catch {
    return false
  }
}

/**
 * Spawn the chosen provider headlessly and normalize stdout into Farplane events.
 */
async function runUserTurn(body, onEvent) {
  await loadInjectedSecrets()
  let text = body.text || ''
  if (pendingHandoffTranscript) {
    text =
      pendingHandoffTranscript +
      '\n\n---\n\nThe human now says:\n' +
      text
    pendingHandoffTranscript = null
  }
  const provider = body.provider || PROVIDER
  const sessionID = body.provider_session_id || null

  onEvent({ type: 'status', status: 'running' })

  if (provider === 'claude_code') {
    await runClaude(text, sessionID, onEvent)
  } else if (provider === 'codex') {
    await runCodex(text, onEvent)
  } else if (provider === 'opencode') {
    await runOpenCode(text, onEvent)
  } else if (provider === 'oh_my_pi') {
    await runOhMyPi(text, onEvent)
  } else {
    onEvent({
      type: 'assistant_message',
      role: 'assistant',
      body: `Unsupported provider: ${provider}`,
      done: true,
    })
  }

  onEvent({ type: 'status', status: 'idle' })
}

async function loadInjectedSecrets() {
  try {
    const { readFile } = await import('node:fs/promises')
    const raw = await readFile('/run/farplane/secrets.env', 'utf8')
    for (const line of raw.split('\n')) {
      const m = line.match(/^export\s+([A-Z0-9_]+)=(.*)$/)
      if (!m) continue
      let val = m[2].trim()
      if (
        (val.startsWith("'") && val.endsWith("'")) ||
        (val.startsWith('"') && val.endsWith('"'))
      ) {
        val = val.slice(1, -1)
      }
      process.env[m[1]] = val
    }
  } catch {
    // optional file
  }
}

function trackChild(child) {
  activeChild = child
  const clear = () => {
    if (activeChild === child) activeChild = null
  }
  child.on('close', clear)
  child.on('error', clear)
  return child
}

function attachJSONL(child, onEvent, normalizeLine) {
  let buf = ''
  child.stdout.on('data', (chunk) => {
    buf += chunk.toString()
    const lines = buf.split('\n')
    buf = lines.pop() || ''
    for (const line of lines) {
      if (!line.trim()) continue
      try {
        const msg = JSON.parse(line)
        normalizeLine(msg, onEvent)
      } catch {
        onEvent({ type: 'tool_progress', body: line })
      }
    }
  })
  child.stderr.on('data', (chunk) => {
    onEvent({ type: 'tool_progress', body: chunk.toString() })
  })
}

function runClaude(text, sessionID, onEvent) {
  return new Promise((resolve) => {
    const args = [
      '--bare',
      '--dangerously-skip-permissions',
      '--permission-mode',
      'bypassPermissions',
      '-p',
      text,
      '--output-format',
      'stream-json',
      '--verbose',
      '--include-partial-messages',
    ]
    if (sessionID) {
      args.push('--resume', sessionID)
    }
    const child = trackChild(
      spawn('claude', args, {
        env: process.env,
        stdio: ['ignore', 'pipe', 'pipe'],
      }),
    )
    attachJSONL(child, onEvent, normalizeClaude)
    child.on('close', (code) => {
      if (code !== 0 && code !== null) {
        onEvent({
          type: 'assistant_message',
          role: 'assistant',
          body: `claude exited with code ${code}`,
          done: true,
        })
      }
      resolve()
    })
    child.on('error', (err) => {
      onEvent({
        type: 'assistant_message',
        role: 'assistant',
        body: `claude failed to start: ${err.message}`,
        done: true,
      })
      resolve()
    })
  })
}

function normalizeClaude(msg, onEvent) {
  if (msg.type === 'assistant' && msg.message?.content) {
    const parts = msg.message.content
    const text = Array.isArray(parts)
      ? parts.map((p) => (p.type === 'text' ? p.text : '')).join('')
      : String(parts)
    if (text) {
      onEvent({
        type: 'assistant_message',
        role: 'assistant',
        body: text,
        done: false,
      })
    }
  }
  if (msg.type === 'result' && msg.result) {
    onEvent({
      type: 'assistant_message',
      role: 'assistant',
      body: String(msg.result),
      done: true,
    })
  }
  if (msg.session_id) {
    onEvent({
      type: 'provider_session',
      provider_session_id: msg.session_id,
    })
  }
  if (msg.type === 'content_block_delta' && msg.delta?.text) {
    onEvent({
      type: 'assistant_message',
      role: 'assistant',
      body: msg.delta.text,
      done: false,
    })
  }
}

function runCodex(text, onEvent) {
  return new Promise((resolve) => {
    const child = trackChild(
      spawn(
        'codex',
        [
          '--dangerously-bypass-approvals-and-sandbox',
          'exec',
          '--json',
          text,
        ],
        { env: process.env, stdio: ['ignore', 'pipe', 'pipe'] },
      ),
    )
    attachJSONL(child, onEvent, normalizeCodex)
    child.on('close', () => resolve())
    child.on('error', (err) => {
      onEvent({
        type: 'assistant_message',
        role: 'assistant',
        body: `codex failed: ${err.message}`,
        done: true,
      })
      resolve()
    })
  })
}

function normalizeCodex(msg, onEvent) {
  if (msg.type === 'thread.started' && msg.thread_id) {
    onEvent({
      type: 'provider_session',
      provider_session_id: String(msg.thread_id),
    })
  }
  if (msg.type === 'item.completed' && msg.item) {
    const item = msg.item
    if (item.type === 'agent_message' && item.text) {
      onEvent({
        type: 'assistant_message',
        role: 'assistant',
        body: String(item.text),
        done: true,
      })
    } else if (item.type === 'command_execution' || item.type === 'file_change') {
      onEvent({
        type: 'tool_progress',
        body: JSON.stringify(item),
      })
    } else if (item.message) {
      onEvent({ type: 'tool_progress', body: String(item.message) })
    }
  }
  if (msg.type === 'turn.failed' && msg.error) {
    onEvent({
      type: 'assistant_message',
      role: 'assistant',
      body: String(msg.error.message || msg.error),
      done: true,
    })
  }
}

function runOpenCode(text, onEvent) {
  return new Promise((resolve) => {
    const env = {
      ...process.env,
      OPENCODE_DANGEROUSLY_SKIP_PERMISSIONS: 'true',
      OPENCODE_YOLO: 'true',
    }
    const child = trackChild(
      spawn(
        'opencode',
        [
          'run',
          '--auto',
          '--dangerously-skip-permissions',
          '--format',
          'json',
          text,
        ],
        { env, stdio: ['ignore', 'pipe', 'pipe'] },
      ),
    )
    attachJSONL(child, onEvent, normalizeOpenCode)
    child.on('close', () => resolve())
    child.on('error', (err) => {
      onEvent({
        type: 'assistant_message',
        role: 'assistant',
        body: `opencode failed: ${err.message}`,
        done: true,
      })
      resolve()
    })
  })
}

function normalizeOpenCode(msg, onEvent) {
  if (msg.sessionID) {
    onEvent({
      type: 'provider_session',
      provider_session_id: String(msg.sessionID),
    })
  }
  if (msg.type === 'text' && msg.part?.text) {
    onEvent({
      type: 'assistant_message',
      role: 'assistant',
      body: String(msg.part.text),
      done: false,
    })
  }
  if (msg.type === 'tool_use' || msg.type === 'tool_result') {
    onEvent({
      type: 'tool_progress',
      body: JSON.stringify(msg.part || msg),
    })
  }
  if (msg.type === 'step_finish') {
    onEvent({
      type: 'assistant_message',
      role: 'assistant',
      body: '',
      done: true,
    })
  }
}

function runOhMyPi(text, onEvent) {
  return new Promise((resolve) => {
    const child = trackChild(
      spawn(
        'omp',
        [
          '-p',
          '--mode=json',
          '--auto-approve',
          '--approval-mode=yolo',
          text,
        ],
        { env: process.env, stdio: ['ignore', 'pipe', 'pipe'] },
      ),
    )
    attachJSONL(child, onEvent, normalizeOhMyPi)
    child.on('close', () => resolve())
    child.on('error', (err) => {
      onEvent({
        type: 'assistant_message',
        role: 'assistant',
        body: `omp failed: ${err.message}`,
        done: true,
      })
      resolve()
    })
  })
}

function textFromContent(content) {
  if (!Array.isArray(content)) return ''
  return content
    .filter((p) => p && p.type === 'text' && p.text)
    .map((p) => p.text)
    .join('')
}

function normalizeOhMyPi(msg, onEvent) {
  if (msg.type === 'session' && msg.id) {
    onEvent({
      type: 'provider_session',
      provider_session_id: String(msg.id),
    })
  }
  // omp message_update repeats the full text; only emit the final message_end.
  if (msg.type === 'message_end' && msg.message?.role === 'assistant') {
    const text = textFromContent(msg.message.content)
    if (text) {
      onEvent({
        type: 'assistant_message',
        role: 'assistant',
        body: text,
        done: true,
      })
    }
  }
  if (msg.type === 'tool_execution_start' || msg.type === 'tool_execution_end') {
    onEvent({ type: 'tool_progress', body: JSON.stringify(msg) })
  }
}

const server = http.createServer(async (req, res) => {
  const url = new URL(req.url || '/', `http://127.0.0.1:${PORT}`)

  if (req.method === 'GET' && url.pathname === '/health') {
    res.writeHead(200, { 'content-type': 'application/json' })
    res.end(JSON.stringify({ ok: true, provider: PROVIDER, busy: turnBusy }))
    return
  }

  if (req.method === 'POST' && url.pathname === '/interrupt') {
    const killed = killActiveChild()
    turnBusy = false
    broadcast({ type: 'status', status: 'idle', interrupted: true })
    res.writeHead(200, { 'content-type': 'application/json' })
    res.end(JSON.stringify({ ok: true, killed }))
    return
  }

  if (req.method === 'GET' && url.pathname === '/events') {
    const streamID = randomUUID()
    res.writeHead(200, {
      'content-type': 'text/event-stream',
      'cache-control': 'no-cache',
      connection: 'keep-alive',
    })
    streams.set(streamID, res)
    writeStatus(res, turnBusy ? 'running' : 'idle')
    req.on('close', () => streams.delete(streamID))
    return
  }

  if (req.method === 'POST' && url.pathname === '/turn') {
    let raw = ''
    for await (const chunk of req) raw += chunk
    let body
    try {
      body = JSON.parse(raw || '{}')
    } catch {
      res.writeHead(400, { 'content-type': 'application/json' })
      res.end(JSON.stringify({ error: 'invalid json' }))
      return
    }

    if (body.type === 'handoff' && body.transcript) {
      pendingHandoffTranscript = String(body.transcript)
      res.writeHead(202, { 'content-type': 'application/json' })
      res.end(JSON.stringify({ ok: true, handoff: true }))
      broadcast({
        type: 'status',
        status: 'idle',
        handoff_received: true,
      })
      return
    }

    if (turnBusy) {
      res.writeHead(409, { 'content-type': 'application/json' })
      res.end(JSON.stringify({ error: 'turn already running' }))
      return
    }

    res.writeHead(202, { 'content-type': 'application/json' })
    res.end(JSON.stringify({ ok: true }))

    turnBusy = true
    try {
      await runUserTurn(body, broadcast)
    } catch (err) {
      broadcast({
        type: 'assistant_message',
        role: 'assistant',
        body: String(err),
        done: true,
      })
      broadcast({ type: 'status', status: 'idle' })
    } finally {
      turnBusy = false
      activeChild = null
    }
    return
  }

  res.writeHead(404)
  res.end('not found')
})

server.listen(PORT, '0.0.0.0', () => {
  console.log(`farplane agent bridge listening on ${PORT} provider=${PROVIDER}`)
})
