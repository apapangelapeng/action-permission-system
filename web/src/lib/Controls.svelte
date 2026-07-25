<script>
  import { api } from './api.js'

  let { onauthlost } = $props()
  let suspended = $state(null)
  let bots = $state([])
  let busy = $state(false)
  let newBotName = $state('')
  let createdBot = $state(null) // {name, api_key} — key is shown exactly once
  let createError = $state('')

  async function load() {
    try {
      const [s, b] = await Promise.all([api('/v1/system/auto-allow'), api('/v1/bots')])
      if (s.ok) suspended = s.data.suspended
      if (b.ok) bots = b.data
    } catch {
      onauthlost()
    }
  }

  $effect(() => {
    load()
  })

  async function setSuspended(value) {
    busy = true
    try {
      const r = await api('/v1/system/auto-allow', { method: 'PUT', body: { suspended: value } })
      if (r.ok) suspended = r.data.suspended
    } catch {
      onauthlost()
    } finally {
      busy = false
    }
  }

  async function createBot(e) {
    e.preventDefault()
    busy = true
    createError = ''
    try {
      const r = await api('/v1/bots', { method: 'POST', body: { name: newBotName.trim() } })
      if (!r.ok) {
        createError = r.data?.error || 'creation failed'
        return
      }
      createdBot = r.data
      newBotName = ''
      await load()
    } catch {
      onauthlost()
    } finally {
      busy = false
    }
  }

  async function setBot(bot, disabled) {
    busy = true
    try {
      const r = await api(`/v1/bots/${bot.id}/${disabled ? 'disable' : 'enable'}`, { method: 'POST' })
      if (r.ok) await load()
    } catch {
      onauthlost()
    } finally {
      busy = false
    }
  }
</script>

<div class="stack">
  <div class="card stack">
    <div class="row spread">
      <div>
        <h3>Auto-allow</h3>
        <p class="muted small">
          When suspended, every action — even ones policies would allow — goes to a human.
          Denies stay denies. No policies are changed.
        </p>
      </div>
      {#if suspended === null}
        <span class="muted">…</span>
      {:else if suspended}
        <div class="row">
          <span class="pill err">suspended — everything needs a human</span>
          <button class="btn" disabled={busy} onclick={() => setSuspended(false)}>Restore auto-allow</button>
        </div>
      {:else}
        <div class="row">
          <span class="pill ok">active — policies may auto-allow</span>
          <button class="btn deny" disabled={busy} onclick={() => setSuspended(true)}>Suspend all auto-allow</button>
        </div>
      {/if}
    </div>
  </div>

  <div class="card stack">
    <h3>Bots</h3>
    <p class="muted small">
      One API key = one bot: every process presenting a key acts as that bot —
      same policies, same kill switch, same audit attribution. A disabled bot
      gets an instant deny on everything it tries; the attempts are still recorded.
    </p>

    <form class="row" onsubmit={createBot}>
      <input class="input bot-name" placeholder="new bot name (e.g. deploy-bot)" bind:value={newBotName} required />
      <button class="btn primary" disabled={busy}>Create bot + API key</button>
    </form>
    {#if createError}<p class="err small">{createError}</p>{/if}
    {#if createdBot}
      <div class="keybox">
        <b>{createdBot.name}</b> created. Its API key — <b>copy it now, it is shown exactly once</b>:
        <pre class="payload">{createdBot.api_key}</pre>
        <p class="muted small">The bot can start submitting immediately; with no policies covering it, everything it tries goes to the Queue (fail closed).</p>
      </div>
    {/if}
    {#each bots as bot (bot.id)}
      <div class="row spread bot-row">
        <div class="row">
          <b>{bot.name}</b>
          <code class="muted small">{bot.id}</code>
          {#if bot.disabled}<span class="pill err">disabled — deny all</span>{:else}<span class="pill ok">enabled</span>{/if}
        </div>
        {#if bot.disabled}
          <button class="btn" disabled={busy} onclick={() => setBot(bot, false)}>Enable</button>
        {:else}
          <button class="btn deny" disabled={busy} onclick={() => setBot(bot, true)}>Disable</button>
        {/if}
      </div>
    {/each}
  </div>
</div>

<style>
  h3 { font-size: 1rem; }
  p { margin: 4px 0 0; max-width: 34rem; }
  .bot-row { border-top: 1px solid var(--line); padding-top: 10px; }
  .bot-name { max-width: 20rem; }
  .err { color: var(--err); }
  .keybox { border: 1px solid var(--warn); border-radius: 8px; padding: 12px 14px; }
  .keybox pre { margin: 8px 0; }
</style>
