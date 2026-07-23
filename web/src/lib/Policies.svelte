<script>
  import { api } from './api.js'

  let { dir, onauthlost } = $props()
  let items = $state([])

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

  function author(p) {
    if (p.created_by_kind === 'bot') return `bot ${dir.bots[p.created_by_id] || p.created_by_id}`
    return dir.users[p.created_by_id] || p.created_by_id
  }
</script>

<div class="stack">
  <p class="muted small">
    Read-only for now — creating and reviewing policies lands in milestone 3. Evaluation order:
    <span class="pill err">deny</span> beats <span class="pill warn">require_approval</span> beats
    <span class="pill ok">allow</span>; anything unmatched goes to a human.
  </p>
  <div class="card tbl-wrap">
    <table class="tbl">
      <thead>
        <tr><th>Policy</th><th>Applies to</th><th>Matcher</th><th>Effect</th><th>Status</th><th>Author</th><th>Priority</th></tr>
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
              {#if p.depth > 0}<div class="muted small">depth {p.depth}</div>{/if}
            </td>
            <td class="small">{author(p)}</td>
            <td class="muted">{p.priority}</td>
          </tr>
        {/each}
      </tbody>
    </table>
  </div>
</div>

<style>
  .desc, .cfg { max-width: 18rem; }
  p { margin: 0; }
</style>
