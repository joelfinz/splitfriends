import * as api from './api';
import { ApiError } from './api';
import { idbGet, idbSet, idbClear } from './idb';
import { computeNet, computePairwise, simplify } from './ledger';
import { toast } from './toast.svelte';
import type { Expense, Group, GroupDetail, GroupEvent, Member, Payment, User } from './types';

type Snapshot = {
  me: User | null;
  groups: Record<string, Group>;
  details: Record<string, GroupDetail>;
  activity: GroupEvent[];
  lastEventId: number;
};

const SNAPSHOT_KEY = 'snapshot:v1';

let me = $state<User | null>(null);
let groups = $state<Record<string, Group>>({});
let details = $state<Record<string, GroupDetail>>({});
let activity = $state<GroupEvent[]>([]);
let lastEventId = $state(0);
let booted = $state(false); // first auth check done
let online = $state(true); // SSE connected
let hydratedFromCache = $state(false);

let es: EventSource | null = null;
let persistTimer: ReturnType<typeof setTimeout> | null = null;
const refetchTimers = new Map<string, ReturnType<typeof setTimeout>>();

export const store = {
  get me() {
    return me;
  },
  get groups() {
    return groups;
  },
  get groupList(): Group[] {
    return Object.values(groups).sort((a, b) => (a.name.localeCompare(b.name)));
  },
  get details() {
    return details;
  },
  get activity() {
    return activity;
  },
  get lastEventId() {
    return lastEventId;
  },
  get booted() {
    return booted;
  },
  get online() {
    return online;
  },
  get hydratedFromCache() {
    return hydratedFromCache;
  },
};

// ---------- persistence
function schedulePersist(): void {
  if (persistTimer) clearTimeout(persistTimer);
  persistTimer = setTimeout(() => {
    persistTimer = null;
    const snap: Snapshot = {
      me: $state.snapshot(me),
      groups: $state.snapshot(groups),
      details: $state.snapshot(details),
      activity: $state.snapshot(activity).slice(0, 100),
      lastEventId,
    };
    void idbSet(SNAPSHOT_KEY, snap);
  }, 300);
}

async function hydrate(): Promise<void> {
  const snap = await idbGet<Snapshot>(SNAPSHOT_KEY);
  if (!snap || !snap.me) return;
  me = snap.me;
  groups = snap.groups ?? {};
  details = snap.details ?? {};
  activity = snap.activity ?? [];
  lastEventId = snap.lastEventId ?? 0;
  hydratedFromCache = true;
}

// ---------- boot / auth
export async function boot(): Promise<void> {
  await hydrate();
  try {
    const u = await api.me();
    await setSession(u);
  } catch (e) {
    if (e instanceof ApiError && e.status === 401) {
      await clearSession();
    } else if (!me) {
      // network error and no cache: stay logged out but don't wipe anything
    }
  } finally {
    booted = true;
  }
}

export async function setSession(u: User): Promise<void> {
  const changedUser = me?.id !== u.id;
  me = u;
  if (changedUser) {
    groups = {};
    details = {};
    activity = [];
    lastEventId = 0;
  }
  await Promise.all([loadGroups(), loadActivity()]);
  connectStream();
  schedulePersist();
}

export async function clearSession(): Promise<void> {
  disconnectStream();
  me = null;
  groups = {};
  details = {};
  activity = [];
  lastEventId = 0;
  hydratedFromCache = false;
  await idbClear();
}

export async function logout(): Promise<void> {
  try {
    await api.logout();
  } finally {
    await clearSession();
  }
}

export function updateMeLocal(u: User): void {
  me = u;
  schedulePersist();
}

// ---------- loading
export async function loadGroups(): Promise<void> {
  const list = await api.listGroups();
  const next: Record<string, Group> = {};
  for (const g of list) next[g.id] = g;
  groups = next;
  // Drop details for groups we no longer belong to.
  for (const id of Object.keys(details)) if (!next[id]) delete details[id];
  schedulePersist();
}

export async function loadGroup(id: string): Promise<GroupDetail> {
  const d = await api.getGroup(id);
  details[id] = d;
  groups[id] = d.group;
  schedulePersist();
  return d;
}

export async function loadActivity(): Promise<void> {
  activity = await api.activity(50);
  if (activity.length && activity[0].id > lastEventId) lastEventId = activity[0].id;
  schedulePersist();
}

export function removeGroupLocal(id: string): void {
  delete groups[id];
  delete details[id];
  schedulePersist();
}

export function upsertGroupLocal(g: Group): void {
  groups[g.id] = g;
  if (details[g.id]) details[g.id].group = g;
  schedulePersist();
}

// ---------- realtime
export function connectStream(): void {
  if (es || !me) return;
  const url = `/api/stream${lastEventId ? `?last_event_id=${lastEventId}` : ''}`;
  es = new EventSource(url, { withCredentials: true });
  es.onopen = () => {
    online = true;
  };
  es.onerror = () => {
    online = false;
    // If our session died, stop retrying.
    void api.me().catch((e) => {
      if (e instanceof ApiError && e.status === 401) {
        void clearSession();
      }
    });
  };
  es.addEventListener('group_event', (msg) => {
    try {
      const ev = JSON.parse((msg as MessageEvent).data) as GroupEvent;
      applyEvent(ev);
    } catch (e) {
      console.error('bad event', e);
    }
  });
}

export function disconnectStream(): void {
  es?.close();
  es = null;
  online = false;
}

function scheduleRefetch(groupId: string, ms = 400): void {
  const t = refetchTimers.get(groupId);
  if (t) clearTimeout(t);
  refetchTimers.set(
    groupId,
    setTimeout(() => {
      refetchTimers.delete(groupId);
      loadGroup(groupId).catch((e) => {
        if (e instanceof ApiError && (e.status === 404 || e.status === 403)) removeGroupLocal(groupId);
      });
    }, ms),
  );
}

function recompute(d: GroupDetail): void {
  d.balances = computeNet(d.expenses, d.payments);
  d.pairwise = computePairwise(d.expenses, d.payments);
  d.simplified = simplify(d.balances);
  const mine = d.balances.find((b) => b.user_id === me?.id);
  d.group.my_balance = mine?.net ?? 0;
  groups[d.group.id] = d.group;
}

export function applyEvent(ev: GroupEvent): void {
  if (ev.id <= lastEventId) return; // already seen (replay)
  lastEventId = ev.id;

  // Activity feed
  activity = [ev, ...activity.filter((a) => a.id !== ev.id)].slice(0, 100);

  const gid = ev.group_id;
  const g = groups[gid];
  const d = details[gid];
  const contiguous = !!g && ev.seq === g.last_seq + 1;

  // Membership events affecting me
  if (ev.type === 'member.left' && ev.payload?.member?.user_id === me?.id) {
    removeGroupLocal(gid);
    return;
  }

  if (!g || !contiguous) {
    // Unknown group or missed events: authoritative refetch.
    if (!g && ev.type === 'group.created' && ev.payload?.group) {
      groups[gid] = { ...ev.payload.group, members: ev.payload.group.members ?? [], my_balance: 0, last_seq: ev.seq };
    }
    scheduleRefetch(gid, 0);
    return;
  }

  g.last_seq = ev.seq;
  if (d) d.group.last_seq = ev.seq;

  switch (ev.type) {
    case 'group.updated': {
      const ng = ev.payload?.group as Partial<Group> | undefined;
      if (ng) {
        g.name = ng.name ?? g.name;
        g.currency = ng.currency ?? g.currency;
        if (d) {
          d.group.name = g.name;
          d.group.currency = g.currency;
        }
      }
      break;
    }
    case 'member.joined': {
      const m = ev.payload?.member as Member | undefined;
      if (m && !g.members.some((x) => x.user_id === m.user_id)) {
        g.members = [...g.members, m];
        if (d) d.group.members = g.members;
      }
      break;
    }
    case 'member.left': {
      const m = ev.payload?.member as Member | undefined;
      if (m) {
        g.members = g.members.filter((x) => x.user_id !== m.user_id);
        if (d) d.group.members = g.members;
      }
      break;
    }
    case 'expense.created':
    case 'expense.updated':
    case 'expense.restored': {
      const e = ev.payload?.expense as Expense | undefined;
      if (d && e) {
        d.expenses = [e, ...d.expenses.filter((x) => x.id !== e.id)];
        recompute(d);
      } else scheduleRefetch(gid);
      break;
    }
    case 'expense.deleted': {
      const id = ev.payload?.expense_id as string | undefined;
      if (d && id) {
        d.expenses = d.expenses.filter((x) => x.id !== id);
        recompute(d);
      } else scheduleRefetch(gid);
      break;
    }
    case 'payment.created':
    case 'payment.restored': {
      const p = ev.payload?.payment as Payment | undefined;
      if (d && p) {
        d.payments = [p, ...d.payments.filter((x) => x.id !== p.id)];
        recompute(d);
      } else scheduleRefetch(gid);
      break;
    }
    case 'payment.deleted': {
      const id = ev.payload?.payment_id as string | undefined;
      if (d && id) {
        d.payments = d.payments.filter((x) => x.id !== id);
        recompute(d);
      } else scheduleRefetch(gid);
      break;
    }
    default:
      break;
  }

  // Money events: reconcile with the server shortly after the optimistic apply.
  if (ev.type.startsWith('expense.') || ev.type.startsWith('payment.')) scheduleRefetch(gid);

  if (ev.actor_id !== me?.id && (ev.type.startsWith('expense.') || ev.type.startsWith('payment.'))) {
    toast(describeEvent(ev, g), 'info');
  }
  schedulePersist();
}

export function describeEvent(ev: GroupEvent, g?: Group): string {
  const who = ev.actor_name || 'Someone';
  const p = ev.payload ?? {};
  const where = g ? ` in ${g.name}` : '';
  switch (ev.type) {
    case 'group.created':
      return `${who} created the group ${p.group?.name ?? ''}`.trim();
    case 'group.updated':
      return `${who} updated the group${where}`;
    case 'member.joined':
      return `${p.member?.name ?? who} joined${where}`;
    case 'member.left':
      return `${p.member?.name ?? who} left${where}`;
    case 'expense.created':
      return `${who} added "${p.expense?.description ?? 'an expense'}"${where}`;
    case 'expense.updated':
      return `${who} edited "${p.expense?.description ?? 'an expense'}"${where}`;
    case 'expense.deleted':
      return `${who} deleted "${p.description ?? 'an expense'}"${where}`;
    case 'expense.restored':
      return `${who} restored "${p.expense?.description ?? 'an expense'}"${where}`;
    case 'payment.created':
      return `${who} recorded a payment${where}`;
    case 'payment.deleted':
      return `${who} deleted a payment${where}`;
    case 'payment.restored':
      return `${who} restored a payment${where}`;
    default:
      return `${who} did something${where}`;
  }
}

export function memberName(g: Group | undefined, userId: string): string {
  if (userId === me?.id) return 'You';
  return g?.members.find((m) => m.user_id === userId)?.name ?? 'Former member';
}
