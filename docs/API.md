# splitfriends API contract

Base URL: same origin as the SPA. All JSON. All amounts are **integers in minor units** (cents). Dates are `YYYY-MM-DD`. Timestamps are RFC3339 UTC.
Auth is a `sf_session` HttpOnly cookie set by the passkey flows. Unauthenticated requests to protected routes return `401 {"error":"unauthorized"}`.
Errors: `{"error": "<machine_code>", "message": "<human text>"}` with a 4xx/5xx status.

IDs are opaque strings.

## Types

```ts
type User = { id: string; name: string; created_at: string }
type Member = { user_id: string; name: string; joined_at: string }
type Group = {
  id: string; name: string; currency: string; // ISO 4217, e.g. "AED"
  created_by: string; created_at: string;
  members: Member[];
  my_balance: number;   // + means the group owes me, - means I owe
  last_seq: number;     // latest event seq in this group
}
type Payer = { user_id: string; amount: number }
type SplitType = 'equal' | 'exact' | 'percent' | 'shares'
type ShareInput = { user_id: string; value: number }
// equal: value ignored (send 0). exact: value = minor units. percent: value = basis points (10000 = 100%). shares: value = share count.
type Share = { user_id: string; value: number; amount: number } // amount = computed owed minor units
type Expense = {
  id: string; group_id: string; description: string; amount: number; date: string;
  split_type: SplitType; notes: string;
  payers: Payer[]; shares: Share[];
  created_by: string; created_at: string; updated_at: string;
}
type Payment = {
  id: string; group_id: string; from_user_id: string; to_user_id: string; amount: number;
  date: string; notes: string; created_by: string; created_at: string;
}
type Balance = { user_id: string; net: number }           // + is owed, - owes
type Debt = { from_user_id: string; to_user_id: string; amount: number }
type GroupDetail = {
  group: Group; expenses: Expense[]; payments: Payment[];
  balances: Balance[]; pairwise: Debt[]; simplified: Debt[];
}
type Event = {
  id: number;          // global, monotonic. Used as SSE id / Last-Event-ID
  group_id: string; seq: number; // per-group monotonic
  type: 'group.created'|'group.updated'|'member.joined'|'member.left'
       |'expense.created'|'expense.updated'|'expense.deleted'
       |'payment.created'|'payment.deleted';
  actor_id: string; actor_name: string;
  payload: any;        // see below
  created_at: string;
}
```

Event payloads:
- `group.created` / `group.updated`: `{ group: Group }` (without members balance fields)
- `member.joined` / `member.left`: `{ member: Member }`
- `expense.created` / `expense.updated`: `{ expense: Expense }`
- `expense.deleted`: `{ expense_id: string, description: string, amount: number }`
- `payment.created`: `{ payment: Payment }`
- `payment.deleted`: `{ payment_id: string, amount: number }`

## Auth (passkeys)

| Method | Path | Body | Response |
|---|---|---|---|
| GET | `/api/me` | | `User` or 401 |
| PATCH | `/api/me` | `{name}` | `User` |
| POST | `/api/auth/register/begin` | `{name}` | WebAuthn `PublicKeyCredentialCreationOptions` JSON (`{publicKey: {...}}`) |
| POST | `/api/auth/register/finish` | the credential from `navigator.credentials.create` serialized (use `@github/webauthn-json` or `PublicKeyCredential.toJSON()`) | `User`, sets cookie |
| POST | `/api/auth/login/begin` | `{}` | WebAuthn `PublicKeyCredentialRequestOptions` JSON (`{publicKey: {...}}`), discoverable/usernameless |
| POST | `/api/auth/login/finish` | assertion serialized | `User`, sets cookie |
| POST | `/api/auth/logout` | | 204 |
| GET | `/api/auth/passkeys` | | `[{id, name, created_at, last_used_at}]` |
| POST | `/api/auth/passkeys/begin` | `{name}` (label for the new key) | creation options (logged-in user adds another device) |
| POST | `/api/auth/passkeys/finish` | credential | `{id, name, created_at}` |
| DELETE | `/api/auth/passkeys/{id}` | | 204 (400 `last_passkey` if it's the only one) |

Frontend: use `PublicKeyCredential.parseCreationOptionsFromJSON` / `parseRequestOptionsFromJSON` and `.toJSON()` where available, with `@github/webauthn-json` as fallback. The server speaks the standard JSON encoding (base64url).

## Groups

| Method | Path | Body | Response |
|---|---|---|---|
| GET | `/api/groups` | | `Group[]` |
| POST | `/api/groups` | `{name, currency}` | `Group` (201) |
| GET | `/api/groups/{id}` | | `GroupDetail` |
| PATCH | `/api/groups/{id}` | `{name?, currency?}` | `Group` |
| POST | `/api/groups/{id}/leave` | | 204 (400 `nonzero_balance` if the user still owes/is owed) |
| POST | `/api/groups/{id}/invites` | | `{token, url, expires_at}` (201). url = `${origin}/join/${token}` |
| GET | `/api/invites/{token}` | | `{group_id, group_name, currency, inviter_name, member_count, already_member: bool}` (works logged out; already_member false then) |
| POST | `/api/invites/{token}/accept` | | `Group` (auth required) |

## Expenses and payments

| Method | Path | Body | Response |
|---|---|---|---|
| POST | `/api/groups/{id}/expenses` | `ExpenseInput` | `Expense` (201) |
| PATCH | `/api/groups/{id}/expenses/{eid}` | `ExpenseInput` | `Expense` |
| DELETE | `/api/groups/{id}/expenses/{eid}` | | 204 |
| POST | `/api/groups/{id}/payments` | `{from_user_id, to_user_id, amount, date, notes}` | `Payment` (201) |
| DELETE | `/api/groups/{id}/payments/{pid}` | | 204 |

```ts
type ExpenseInput = {
  description: string; amount: number; date: string; notes?: string;
  payers: Payer[];                 // must sum to amount
  split_type: SplitType;
  shares: ShareInput[];            // exact must sum to amount; percent must sum to 10000; shares >0; equal: list of participant user_ids
}
```
Validation errors return 400 with codes: `invalid_amount`, `payers_mismatch`, `shares_mismatch`, `no_participants`, `not_member`, `invalid_date`.

## Events and realtime

| Method | Path | Response |
|---|---|---|
| GET | `/api/groups/{id}/events?since={seq}&limit=200` | `Event[]` with seq > since, ascending |
| GET | `/api/activity?limit=50` | `Event[]` across all my groups, newest first |
| GET | `/api/stream` | `text/event-stream` |

SSE stream:
- Every message is `id: <event.id>\nevent: group_event\ndata: <Event JSON>\n\n`.
- A comment `: ping` every 25s.
- On connect with `Last-Event-ID` header, the server first replays every event with id > Last-Event-ID for groups the user belongs to.
- When the user is added to a new group mid-stream, subsequent events for that group appear on the same stream (server fans out by user id).
- Client should on any `group_event` apply it to local state; if the seq is not exactly `last_seq + 1` for that group, refetch `GET /api/groups/{id}`.

## Push

| Method | Path | Body | Response |
|---|---|---|---|
| GET | `/api/push/vapid` | | `{public_key}` (base64url) |
| POST | `/api/push/subscribe` | `PushSubscription.toJSON()` = `{endpoint, keys:{p256dh, auth}}` | 204 |
| DELETE | `/api/push/subscribe` | `{endpoint}` | 204 |

Push payload (JSON) delivered to the service worker:
```ts
{ title: string; body: string; url: string; tag: string; group_id: string }
```
Push is sent to every member of the group except the actor, only if that member has **no open SSE connection**.

## Static

- SPA served from `/` with history fallback to `index.html` for any non-`/api` path.
- `/sw.js` and `/manifest.webmanifest` must be served from the root (put them in `web/public/`).
- `/join/{token}` is an SPA route.

## Dev

- Go server listens on `:8080` (env `ADDR`). `RP_ID=localhost`, `ORIGIN=http://localhost:5173` in dev so passkeys work through the Vite dev server.
- Vite dev server on `:5173` proxies `/api` to `http://localhost:8080` (SSE needs `proxy: { '/api': { target, changeOrigin: true } }`, no buffering issues with Vite).

---

## v2 additions: categories, restore, trash

### Categories

`Expense` and `ExpenseInput` gain `category: Category`. Fixed set, validated by the server (400 `invalid_category` otherwise). Missing on input = `other`.

```ts
type Category = 'food' | 'groceries' | 'drinks' | 'transport' | 'accommodation' | 'entertainment'
              | 'shopping' | 'utilities' | 'health' | 'travel' | 'gifts' | 'other'
```

Payments have no category.

### Restore and trash

| Method | Path | Response |
|---|---|---|
| POST | `/api/groups/{id}/expenses/{eid}/restore` | `Expense` (200). 404 if not deleted / unknown. 400 `not_member` if a payer or participant has left the group. |
| POST | `/api/groups/{id}/payments/{pid}/restore` | `Payment` (200). Same errors. |
| GET | `/api/groups/{id}/trash` | `{ expenses: (Expense & {deleted_at: string})[], payments: (Payment & {deleted_at: string})[] }`, newest deletion first, max 50 each |

New event types, payload identical to the matching `*.created` event:
- `expense.restored`: `{ expense: Expense }`
- `payment.restored`: `{ payment: Payment }`

Clients apply them exactly like `expense.created` / `payment.created` (upsert).

`Expense` and `Payment` objects may carry `deleted_at?: string` only in the trash listing; everywhere else they are absent (live items only).
