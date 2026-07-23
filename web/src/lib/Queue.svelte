<script>
  import { api, countdown, relTime } from './api.js'

  let { dir, onauthlost } = $props()

  let items = $state([])
  let loaded = $state(false)
  let banner = $state('')
  let notes = $state({})
  let busy = $state({})
  let now = $state(Date.now())

  async function load() {
    try {
      const r = await api('/v1/actions?status=pending&limit=100')
      if (r.ok) {
        items = r.data
        loaded = true
      }
    } catch {
      onauthlost()
    }
  }

  $effect(() => {
    load()
    const poll = setInterval(load, 4000)
    const tick = setInterval(() => (now = Date.now()), 1000)
    return () => {
      clearInterval(poll)
      clearInterval(tick)
    }
  })

  async function decide(item, decision) {
    busy[item.id] = true
    banner = ''
    try {
      const r = await api(`/v1/actions/${item.id}/decision`, {
        method: 'POST',
        body: { decision, note: notes[item.id] || undefined },
      })
      if (r.status === 409) {
        const req = r.data.request
        const who = dir.users[req.decided_by] || 'the expiry sweeper'
        banner = `Too late — request is already ${req.status} (${who}).`
      }
      await load()
    } catch {
      onauthlost()
    } finally {
      busy[item.id] = false
    }
  }

  function policyName(id) {
    return id ? dir.policies[id] || id : null
  }
</script>

<div class="stack">
  {#if banner}
    <div class="card banner">{banner}</div>
  {/if}

  {#if loaded && items.length === 0}
    <div class="card muted">Queue is empty — nothing needs a human right now.</div>
  {/if}

  {#each items as item (item.id)}
    <div class="card stack request">
      <div class="row spread">
        <div class="row">
          <code class="type">{item.action_type}</code>
          {#if policyName(item.matched_policy_id)}
            <span class="pill warn">gated by {policyName(item.matched_policy_id)}</span>
          {:else}
            <span class="pill neutral">no policy matched — fail closed</span>
          {/if}
        </div>
        <div class="row small muted">
          <span>{dir.bots[item.bot_id] || item.bot_id} · {relTime(item.created_at)}</span>
          <span class="pill {countdown(item.expires_at, now) === 'expired' ? 'err' : 'warn'}">
            expires in {countdown(item.expires_at, now)}
          </span>
        </div>
      </div>

      {#if item.summary}
        <p class="summary small muted">Bot says: “{item.summary}” — unverified; judge the payload below.</p>
      {/if}
      <pre class="payload">{JSON.stringify(item.payload, null, 2)}</pre>

      <div class="row">
        <input
          class="input note"
          placeholder="Optional note (recorded in the audit trail)"
          bind:value={notes[item.id]}
        />
        <button class="btn approve" disabled={busy[item.id]} onclick={() => decide(item, 'approve')}>
          Approve
        </button>
        <button class="btn deny" disabled={busy[item.id]} onclick={() => decide(item, 'deny')}>
          Deny
        </button>
      </div>
    </div>
  {/each}
</div>

<style>
  .type { font-size: 0.9rem; font-weight: 600; }
  .summary { margin: 0; font-style: italic; }
  .banner { border-color: var(--warn); color: var(--warn); }
  .note { flex: 1; min-width: 200px; }
  .request p { margin: 0; }
</style>
