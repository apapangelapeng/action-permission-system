<script>
  import { api, getSession, setSession } from './lib/api.js'
  import Login from './lib/Login.svelte'
  import Queue from './lib/Queue.svelte'
  import Activity from './lib/Activity.svelte'
  import Policies from './lib/Policies.svelte'
  import Audit from './lib/Audit.svelte'
  import Controls from './lib/Controls.svelte'

  let user = $state(getSession()?.user || null)
  let tab = $state('queue')
  let pendingCount = $state(0)
  // dir maps ids → display names so every screen can attribute things.
  let dir = $state({ users: {}, bots: {}, policies: {} })

  const tabs = [
    { id: 'queue', label: 'Queue', component: Queue },
    { id: 'activity', label: 'Activity', component: Activity },
    { id: 'policies', label: 'Policies', component: Policies },
    { id: 'audit', label: 'Audit', component: Audit },
    { id: 'controls', label: 'Controls', component: Controls },
  ]

  function authLost() {
    setSession(null)
    user = null
  }

  async function loadDirectory() {
    try {
      const [u, b, p] = await Promise.all([api('/v1/users'), api('/v1/bots'), api('/v1/policies')])
      const next = { users: {}, bots: {}, policies: {} }
      if (u.ok) for (const x of u.data) next.users[x.id] = x.name
      if (b.ok) for (const x of b.data) next.bots[x.id] = x.name
      if (p.ok) for (const x of p.data) next.policies[x.id] = x.name
      dir = next
    } catch {
      authLost()
    }
  }

  async function pollPending() {
    try {
      const r = await api('/v1/actions?status=pending&limit=100')
      if (r.ok) pendingCount = r.data.length
    } catch {
      authLost()
    }
  }

  $effect(() => {
    if (!user) return
    loadDirectory()
    pollPending()
    const t = setInterval(pollPending, 4000)
    return () => clearInterval(t)
  })

  const ActiveView = $derived(tabs.find((t) => t.id === tab)?.component || Queue)
</script>

{#if !user}
  <Login onlogin={(u) => (user = u)} />
{:else}
  <header>
    <div class="inner">
      <div class="row">
        <span class="brand">Action Permission System</span>
        <nav class="row">
          {#each tabs as t (t.id)}
            <button class="tab" class:active={tab === t.id} onclick={() => (tab = t.id)}>
              {t.label}
              {#if t.id === 'queue' && pendingCount > 0}
                <span class="badge">{pendingCount}</span>
              {/if}
            </button>
          {/each}
        </nav>
      </div>
      <div class="row small">
        <span class="muted">{user.name}</span>
        <button class="btn" onclick={authLost}>Sign out</button>
      </div>
    </div>
  </header>
  <main>
    <ActiveView {dir} onauthlost={authLost} />
  </main>
{/if}

<style>
  header { background: var(--surface); border-bottom: 1px solid var(--line); position: sticky; top: 0; z-index: 5; }
  .inner {
    max-width: 1080px; margin: 0 auto; padding: 10px 20px;
    display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap;
  }
  .brand { font-weight: 700; font-size: 0.95rem; margin-right: 10px; }
  .tab {
    font: inherit; font-size: 0.88rem; padding: 6px 12px; border: none; background: none;
    color: var(--muted); border-radius: 7px; cursor: pointer;
  }
  .tab:hover { background: var(--surface-2); }
  .tab.active { color: var(--text); background: var(--surface-2); font-weight: 600; }
  .tab:focus-visible { outline: 2px solid var(--accent); }
  .badge {
    background: var(--warn-bg); color: var(--warn);
    font-size: 0.7rem; font-weight: 700; border-radius: 999px; padding: 1px 7px; margin-left: 4px;
  }
  main { max-width: 1080px; margin: 0 auto; padding: 20px; }
</style>
