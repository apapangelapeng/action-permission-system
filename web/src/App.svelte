<script>
  // Milestone 2 replaces this with login, the approval queue, policy screens,
  // and the audit browser.
  let health = $state('checking…')

  fetch('/healthz')
    .then((r) => r.json())
    .then((h) => (health = `service ${h.status} · database ${h.db}`))
    .catch(() => (health = 'health check unreachable'))
</script>

<main>
  <h1>Action Permission System</h1>
  <p>Dashboard scaffold — real screens arrive in milestone 2.</p>
  <p>{health}</p>
</main>
