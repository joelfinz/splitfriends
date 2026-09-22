# splitfriends ("Fairshare")

A self-hosted, Splitwise-style shared-expense tracker for friends, trips and flatmates.

- **One binary, one file.** Go server with SQLite. The Svelte PWA is embedded, so the container is a static binary on a distroless base.
- **Passkeys only.** Sign in with Face ID, Touch ID, Windows Hello or a security key. No passwords, no email provider.
- **Realtime.** Every device holds one Server-Sent Events stream; changes show up instantly for everyone in the group.
- **Push notifications.** Web Push (VAPID) for members who are not currently connected. Works on iOS once the app is added to the home screen.
- **Splits that add up.** Equal, exact, percentage and share-based splits, multiple payers, largest-remainder rounding so shares always sum to the total.
- **Balances, pairwise debts and simplified debts** derived from an append-only event log per group.
- **Offline-friendly.** Installable PWA with a cached shell and an IndexedDB snapshot for instant loads.
- **Categories and stats.** Fixed category set with keyword suggestions, plus per-group charts: spend by category, over time, and paid vs share per member.
- **Undo and restore.** Every action offers an undo toast; deleted expenses and payments sit in a per-group trash until restored.
- **Optional admin dashboard** at `/admin`, server-rendered and separate from the app: users, sessions with IP and device, activity timelines, session revocation. Enabled only when `ADMIN_PASSWORD` is set.

## Layout

```
cmd/server         main: HTTP server, SPA fallback, graceful shutdown
internal/config    env config (ADDR, DATA_DIR, RP_ID, ORIGIN, APP_NAME, PUSH_CONTACT, DEV)
internal/db        SQLite open (WAL, single writer), migrations, id helpers
internal/store     all SQL; every mutation appends to the per-group event log in the same tx
internal/ledger    pure money math: split shares, net balances, pairwise debts, simplify
internal/auth      passkeys (WebAuthn) + cookie sessions
internal/realtime  in-memory SSE hub, fan-out by user id
internal/push      VAPID key management + Web Push sending
internal/api       chi routes and handlers
internal/admin     server-rendered admin dashboard (html/template)
internal/clientip  client IP resolution behind proxies
web/               Svelte 5 + Tailwind 4 + daisyUI 5 SPA, service worker, manifest
docs/API.md        the frontend/backend contract
```

## How it works

Each group has a `last_seq`. Every change (expense, payment, member, rename) writes its rows **and** an `events` row in one transaction. Clients open `/api/stream` once; on reconnect the server replays everything after `Last-Event-ID`. Balances are computed from expenses and payments on every read, never stored.

Amounts are integers in minor units. The currency is per group.

## Local development

```sh
# terminal 1: API on :8080 (passkeys need RP_ID=localhost and the Vite origin)
RP_ID=localhost ORIGIN=http://localhost:5173 DEV=1 DATA_DIR=./data go run ./cmd/server

# terminal 2: Vite on :5173, proxies /api
cd web && npm install && npm run dev
```

Tests: `go test ./...` and `cd web && npm run check`.

Full binary with the embedded frontend: `cd web && npm run build && cd .. && go build ./cmd/server`.

## Running it

Passkeys and push both require HTTPS on a real hostname, so run this behind a reverse proxy or tunnel that terminates TLS.

```sh
cp .env.example .env      # set RP_ID, ORIGIN, PUSH_CONTACT
docker compose up -d --build
```

The container binds `127.0.0.1:8080` by default (change `BIND_ADDR` / `BIND_PORT` in `.env`). SQLite lives in the `splitfriends-data` volume. VAPID keys are generated on first start and stored in the database, so push subscriptions survive redeploys.

`deploy.sh` copies the source to a Docker host over SSH and runs the same compose command there. Set `DEPLOY_HOST` in `.env`.

Manual backup of the database volume:

```sh
docker run --rm -v splitfriends_splitfriends-data:/data -v "$PWD":/out alpine \
  tar czf /out/splitfriends-$(date +%F).tgz -C /data .
```

## Configuration

| Variable | Default | Purpose |
|---|---|---|
| `ADDR` | `:8080` | listen address |
| `DATA_DIR` | `./data` | directory for `splitfriends.db` |
| `RP_ID` | `localhost` | WebAuthn relying party id, your public hostname |
| `ORIGIN` | `http://localhost:5173` | public origin; comma-separated list allowed, first is canonical |
| `APP_NAME` | `Fairshare` | shown in passkey prompts |
| `PUSH_CONTACT` | `mailto:admin@example.com` | VAPID subscriber contact |
| `DEV` | unset | request logging when set |
| `ADMIN_PASSWORD` | unset | enables `/admin`; unset means the path does not exist |
| `TRUST_PROXY_HEADERS` | unset | set to `1` behind a proxy or tunnel so client IPs come from `CF-Connecting-IP` / `X-Forwarded-For` |

## Notes

- iOS delivers Web Push only to installed (home-screen) PWAs; the app shows an install hint.
- Many proxies close idle streams after a minute or two; the server sends an SSE comment every 25 s to keep connections alive.
- Leaving a group requires a zero balance.
- Backups are not built in; snapshot the volume as shown above.
- The admin dashboard records client IPs per session and per action. Auth audit rows are purged after 90 days; tell your users if you enable it.

## License

MIT
