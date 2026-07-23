<script>
  import { api, setSession } from './api.js'

  let { onlogin } = $props()
  let username = $state('')
  let password = $state('')
  let error = $state('')
  let busy = $state(false)

  async function submit(e) {
    e.preventDefault()
    busy = true
    error = ''
    try {
      const r = await api('/v1/login', { method: 'POST', body: { username, password } })
      if (!r.ok) {
        error = r.data?.error || 'login failed'
        return
      }
      setSession({ token: r.data.token, user: r.data.user })
      onlogin(r.data.user)
    } catch (err) {
      error = String(err.message || err)
    } finally {
      busy = false
    }
  }
</script>

<div class="wrap">
  <form class="card box" onsubmit={submit}>
    <h1>Action Permission System</h1>
    <p class="muted small">Sign in to review what the bot wants to do.</p>
    <label>
      <span class="small muted">Username</span>
      <input class="input" bind:value={username} autocomplete="username" required />
    </label>
    <label>
      <span class="small muted">Password</span>
      <input class="input" type="password" bind:value={password} autocomplete="current-password" required />
    </label>
    {#if error}<p class="err small">{error}</p>{/if}
    <button class="btn primary" disabled={busy}>{busy ? 'Signing in…' : 'Sign in'}</button>
    <p class="muted small">Demo accounts: <code>alice</code> or <code>bob</code>, password <code>password123</code></p>
  </form>
</div>

<style>
  .wrap { min-height: 100vh; display: grid; place-items: center; padding: 24px; }
  .box { width: 22rem; display: flex; flex-direction: column; gap: 14px; padding: 28px; }
  h1 { font-size: 1.15rem; }
  label { display: flex; flex-direction: column; gap: 4px; }
  .err { color: var(--err); margin: 0; }
  p { margin: 0; }
</style>
