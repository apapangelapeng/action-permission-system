<script>
  import { api, relTime } from './api.js'

  let { dir, onauthlost } = $props()
  let items = $state([])

  const pillFor = {
    auto_allowed: 'ok',
    executed: 'ok',
    approved: 'ok',
    pending: 'warn',
    denied: 'err',
    failed: 'err',
    expired: 'neutral',
  }

  async function load() {
    try {
      const r = await api('/v1/actions?limit=100')
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
</script>

<div class="card tbl-wrap">
  <table class="tbl">
    <thead>
      <tr><th>When</th><th>Action</th><th>Status</th><th>Decided by</th><th>Policy</th><th>Payload</th></tr>
    </thead>
    <tbody>
      {#each items as item (item.id)}
        <tr>
          <td class="muted">{relTime(item.created_at)}</td>
          <td>
            <code>{item.action_type}</code>
            {#if item.summary}<div class="muted small">{item.summary}</div>{/if}
          </td>
          <td>
            <span class="pill {pillFor[item.status] || 'neutral'}">{item.status}</span>
            {#if item.decision_note}<div class="muted small note">“{item.decision_note}”</div>{/if}
          </td>
          <td>{item.decided_by ? dir.users[item.decided_by] || item.decided_by : '—'}</td>
          <td class="muted">{item.matched_policy_id ? dir.policies[item.matched_policy_id] || item.matched_policy_id : 'fail-closed default'}</td>
          <td>
            <details>
              <summary class="small muted">view</summary>
              <pre class="payload">{JSON.stringify(item.payload, null, 2)}</pre>
            </details>
          </td>
        </tr>
      {/each}
    </tbody>
  </table>
</div>

<style>
  .note { max-width: 16rem; }
  details pre { margin-top: 6px; max-width: 30rem; }
</style>
