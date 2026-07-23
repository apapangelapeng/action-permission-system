<script>
  import { api } from './api.js'

  let { dir, onauthlost } = $props()
  let items = $state([])
  let showForm = $state(false)
  let formError = $state('')
  let busy = $state(false)
  let form = $state({
    name: '',
    description: '',
    action_type_pattern: '',
    matcher_type: 'regex',
    field: '',
    pattern: '',
    value: '',
    effect: 'require_approval',
    priority: 100,
  })

  const effectPill = { allow: 'ok', require_approval: 'warn', deny: 'err' }
  const statusPill = { active: 'ok', pending_approval: 'warn', rejected: 'err', disabled: 'neutral', draft: 'neutral' }

  async function load() {
    try {
      const r = await api('/v1/policies')
      if (r.ok) items = r.data
    } catch {
      onauthlost()
    }
  }

  $effect(() => {
    load()
  })

  async function create(e) {
    e.preventDefault()
    busy = true
    formError = ''
    const matcher_config = { field: form.field }
    if (form.matcher_type === 'regex') matcher_config.pattern = form.pattern
    else matcher_config.value = form.value
    try {
      const r = await api('/v1/policies', {
        method: 'POST',
        body: {
          name: form.name,
          description: form.description,
          action_type_pattern: form.action_type_pattern,
          matcher_type: form.matcher_type,
          matcher_config,
          effect: form.effect,
          priority: Number(form.priority) || 100,
        },
      })
      if (!r.ok) {
        formError = r.data?.error || 'creation failed'
        return
      }
      showForm = false
      form = { ...form, name: '', description: '', action_type_pattern: '', field: '', pattern: '', value: '' }
      await load()
    } catch {
      onauthlost()
    } finally {
      busy = false
    }
  }

  async function disable(p) {
    busy = true
    try {
      const r = await api(`/v1/policies/${p.id}/disable`, { method: 'POST' })
      if (r.ok) await load()
    } catch {
      onauthlost()
    } finally {
      busy = false
    }
  }

  function author(p) {
    if (p.created_by_kind === 'bot') return `bot ${dir.bots[p.created_by_id] || p.created_by_id}`
    return dir.users[p.created_by_id] || p.created_by_id
  }
</script>

<div class="stack">
  <div class="row spread">
    <p class="muted small intro">
      Evaluation order: <span class="pill err">deny</span> beats
      <span class="pill warn">require_approval</span> beats <span class="pill ok">allow</span>;
      anything unmatched goes to a human. Your policies take effect immediately;
      bot proposals wait in the Queue for a decision.
    </p>
    <button class="btn primary" onclick={() => (showForm = !showForm)}>
      {showForm ? 'Cancel' : 'New policy'}
    </button>
  </div>

  {#if showForm}
    <form class="card grid" onsubmit={create}>
      <label><span class="small muted">Name</span>
        <input class="input" bind:value={form.name} placeholder="db-reads-auto" required /></label>
      <label><span class="small muted">Applies to (action type pattern)</span>
        <input class="input" bind:value={form.action_type_pattern} placeholder="db.query, db.*, or *" required /></label>
      <label class="wide"><span class="small muted">Description — shown to whoever reads this rule later</span>
        <input class="input" bind:value={form.description} placeholder="Why this rule exists" /></label>
      <label><span class="small muted">Matcher</span>
        <select class="input" bind:value={form.matcher_type}>
          <option value="regex">regex</option>
          <option value="exact">exact</option>
        </select></label>
      <label><span class="small muted">Payload field to inspect</span>
        <input class="input" bind:value={form.field} placeholder="sql" required /></label>
      {#if form.matcher_type === 'regex'}
        <label><span class="small muted">Pattern</span>
          <input class="input mono" bind:value={form.pattern} placeholder={'(?i)^\\s*SELECT'} required /></label>
      {:else}
        <label><span class="small muted">Exact value</span>
          <input class="input mono" bind:value={form.value} required /></label>
      {/if}
      <label><span class="small muted">Effect on match</span>
        <select class="input" bind:value={form.effect}>
          <option value="allow">allow — runs without asking</option>
          <option value="require_approval">require_approval — a human decides</option>
          <option value="deny">deny — blocked outright</option>
        </select></label>
      <label><span class="small muted">Priority (lower runs first)</span>
        <input class="input" type="number" bind:value={form.priority} /></label>
      {#if formError}<p class="err small wide">{formError}</p>{/if}
      <div class="wide"><button class="btn primary" disabled={busy}>Create policy — active immediately</button></div>
    </form>
  {/if}

  <div class="card tbl-wrap">
    <table class="tbl">
      <thead>
        <tr><th>Policy</th><th>Applies to</th><th>Matcher</th><th>Effect</th><th>Status</th><th>Author</th><th>Priority</th><th></th></tr>
      </thead>
      <tbody>
        {#each items as p (p.id)}
          <tr>
            <td>
              <b>{p.name}</b>
              <div class="muted small desc">{p.description}</div>
            </td>
            <td><code>{p.action_type_pattern}</code></td>
            <td class="small">
              <code>{p.matcher_type}</code>
              <div class="muted mono cfg">{JSON.stringify(p.matcher_config)}</div>
            </td>
            <td><span class="pill {effectPill[p.effect]}">{p.effect}</span></td>
            <td>
              <span class="pill {statusPill[p.status] || 'neutral'}">{p.status}</span>
              {#if p.status === 'pending_approval'}<div class="muted small">decide in Queue</div>{/if}
              {#if p.depth > 0}<div class="muted small">depth {p.depth}</div>{/if}
            </td>
            <td class="small">{author(p)}</td>
            <td class="muted">{p.priority}</td>
            <td>
              {#if p.status === 'active'}
                <button class="btn" disabled={busy} onclick={() => disable(p)}>Disable</button>
              {/if}
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  </div>
</div>

<style>
  .desc, .cfg { max-width: 18rem; }
  .intro { margin: 0; max-width: 40rem; }
  .grid { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
  .grid label { display: flex; flex-direction: column; gap: 4px; }
  .wide { grid-column: 1 / -1; }
  .err { color: var(--err); margin: 0; }
  @media (max-width: 640px) { .grid { grid-template-columns: 1fr; } }
</style>
