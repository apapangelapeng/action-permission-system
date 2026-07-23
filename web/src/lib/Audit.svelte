<script>
  import { api, relTime } from './api.js'

  let { dir, onauthlost } = $props()
  let items = $state([])

  async function load() {
    try {
      const r = await api('/v1/audit?limit=200')
      if (r.ok) items = r.data
    } catch {
      onauthlost()
    }
  }

  $effect(() => {
    load()
    const poll = setInterval(load, 6000)
    return () => clearInterval(poll)
  })

  function actor(e) {
    if (e.actor_kind === 'system') return 'system'
    if (e.actor_kind === 'bot') return dir.bots[e.actor_id] || e.actor_id
    return dir.users[e.actor_id] || e.actor_id
  }

  function eventPill(t) {
    if (t.endsWith('.allowed') || t.endsWith('.approved') || t.endsWith('.executed') || t.endsWith('.activated') || t.endsWith('.enabled') || t.endsWith('.restored')) return 'ok'
    if (t.endsWith('.denied') || t.endsWith('.failed') || t.endsWith('.rejected') || t.endsWith('.disabled') || t.endsWith('.suspended')) return 'err'
    if (t.endsWith('.pending') || t.endsWith('.proposed')) return 'warn'
    return 'neutral'
  }
</script>

<div class="card tbl-wrap">
  <table class="tbl">
    <thead>
      <tr><th>When</th><th>Actor</th><th>Event</th><th>Subject</th><th>Details</th></tr>
    </thead>
    <tbody>
      {#each items as e (e.id)}
        <tr>
          <td class="muted">{relTime(e.ts)}</td>
          <td>
            {actor(e)}
            <div class="muted small">{e.actor_kind}</div>
          </td>
          <td><span class="pill {eventPill(e.event_type)}">{e.event_type}</span></td>
          <td class="small"><code>{e.subject_id}</code></td>
          <td class="muted mono small details">{Object.keys(e.details).length ? JSON.stringify(e.details) : ''}</td>
        </tr>
      {/each}
    </tbody>
  </table>
</div>

<style>
  .details { max-width: 24rem; overflow-wrap: anywhere; }
</style>
