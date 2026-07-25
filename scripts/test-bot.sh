#!/bin/bash
# Hand-drive the Action Permission System from the terminal.
#
#   ./scripts/test-bot.sh read              # SELECT        -> auto-allowed by policy
#   ./scripts/test-bot.sh drop              # DROP TABLE    -> denied by policy
#   ./scripts/test-bot.sh write             # UPDATE payments -> pending, a human decides
#   ./scripts/test-bot.sh email             # email.send    -> pending (fail-closed default)
#   ./scripts/test-bot.sh get | post        # http.request GET / POST
#   ./scripts/test-bot.sh propose           # bot proposes a policy -> pending
#   ./scripts/test-bot.sh queue             # list what's waiting for a human
#   ./scripts/test-bot.sh poll <id>         # current state of a request
#   ./scripts/test-bot.sh approve <id> [alice|bob] [note...]
#   ./scripts/test-bot.sh deny    <id> [alice|bob] [note...]
#   ./scripts/test-bot.sh done    <id>      # bot reports executed (consumes the approval)
set -euo pipefail

BASE="${APS_URL:-http://localhost:8080}"
KEY="${APS_BOT_KEY:-aps_95e649720496234ad390a2fc324c9df91f60a8934d84ff6d}"
J='Content-Type: application/json'

pretty() { python3 -m json.tool; }

submit() { # $1 = json body
  curl -s -X POST "$BASE/v1/actions" -H "X-API-Key: $KEY" -H "$J" -d "$1" | pretty
}

login() { # $1 = username -> token on stdout
  curl -s -X POST "$BASE/v1/login" -H "$J" \
    -d "{\"username\":\"$1\",\"password\":\"password123\"}" \
    | python3 -c 'import json,sys;print(json.load(sys.stdin)["token"])'
}

decide() { # $1 decision, $2 id, $3 user, $4 note
  local token; token=$(login "$3")
  curl -s -w "\nHTTP %{http_code}\n" -X POST "$BASE/v1/actions/$2/decision" \
    -H "Authorization: Bearer $token" -H "$J" \
    -d "{\"decision\":\"$1\",\"note\":\"$4\"}"
}

cmd="${1:-help}"
case "$cmd" in
  read)
    submit '{"type":"db.query","payload":{"sql":"SELECT id, status FROM orders LIMIT 5"},"summary":"peek at recent orders"}' ;;
  drop)
    submit '{"type":"db.query","payload":{"sql":"DROP TABLE order_archive"},"summary":"cleanup (bad idea)"}' ;;
  write)
    submit '{"type":"db.query","payload":{"sql":"UPDATE payments SET status = '\''resolved'\'' WHERE id = 4242"},"summary":"resolve payment 4242"}' ;;
  email)
    submit '{"type":"email.send","payload":{"to":"customer@example.com","subject":"Your refund","body":"Refund for order #8812 is on the way."},"summary":"refund notification"}' ;;
  get)
    submit '{"type":"http.request","payload":{"method":"GET","host":"api.example.com","path":"/orders/today"},"summary":"fetch orders"}' ;;
  post)
    submit '{"type":"http.request","payload":{"method":"POST","host":"hooks.slack.com","path":"/services/T00/B00/xyz","body":{"text":"ops summary"}},"summary":"post to slack"}' ;;
  propose)
    submit '{"type":"aps.policy.create","payload":{"name":"emails-to-customers","description":"Proposed by bot: emails to @example.com customers are routine.","action_type_pattern":"email.send","matcher_type":"regex","matcher_config":{"field":"to","pattern":"@example\\.com$"},"effect":"allow"},"summary":"proposal: stop asking about customer emails"}' ;;
  queue)
    token=$(login alice)
    curl -s "$BASE/v1/actions?status=pending" -H "Authorization: Bearer $token" \
      | python3 -c '
import json, sys
rows = json.load(sys.stdin)
if not rows: print("queue is empty")
for r in rows:
    print("%s  %-22s  expires %s  %s" % (r["id"], r["action_type"], r["expires_at"], r.get("summary") or ""))' ;;
  poll)
    curl -s "$BASE/v1/actions/${2:?usage: poll <id>}" -H "X-API-Key: $KEY" | pretty ;;
  approve|deny)
    id="${2:?usage: $cmd <id> [alice|bob] [note...]}"
    user="${3:-alice}"; shift $(( $# > 2 ? 3 : 2 ))
    decide "$cmd" "$id" "$user" "${*:-}" ;;
  done)
    curl -s -w "\nHTTP %{http_code}\n" -X POST "$BASE/v1/actions/${2:?usage: done <id>}/executed" \
      -H "X-API-Key: $KEY" -H "$J" -d '{"success":true}' ;;
  *)
    grep '^#   ' "$0" | sed 's/^#   //' ;;
esac
