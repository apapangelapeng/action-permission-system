const KEY = 'aps_session'

let session = null
try {
  session = JSON.parse(localStorage.getItem(KEY) || 'null')
} catch {
  session = null
}

export function getSession() {
  return session
}

export function setSession(s) {
  session = s
  if (s) localStorage.setItem(KEY, JSON.stringify(s))
  else localStorage.removeItem(KEY)
}

export class AuthError extends Error {}

// api returns {ok, status, data}; a 401 clears the session and throws
// AuthError so the app can drop back to the login screen.
export async function api(path, { method = 'GET', body } = {}) {
  const headers = {}
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  if (session) headers['Authorization'] = `Bearer ${session.token}`

  const res = await fetch(path, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  let data = null
  try {
    data = await res.json()
  } catch {
    /* empty body */
  }
  if (res.status === 401) {
    setSession(null)
    throw new AuthError(data?.error || 'session expired')
  }
  return { ok: res.ok, status: res.status, data }
}

export function relTime(iso) {
  const s = (Date.now() - new Date(iso).getTime()) / 1000
  if (s < 60) return `${Math.max(0, Math.floor(s))}s ago`
  if (s < 3600) return `${Math.floor(s / 60)}m ago`
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`
  return `${Math.floor(s / 86400)}d ago`
}

export function countdown(iso, now) {
  const s = Math.floor((new Date(iso).getTime() - now) / 1000)
  if (s <= 0) return 'expired'
  if (s < 60) return `${s}s`
  return `${Math.floor(s / 60)}m ${s % 60}s`
}
