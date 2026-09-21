import type {
  Expense,
  ExpenseInput,
  Group,
  GroupDetail,
  GroupEvent,
  Invite,
  InvitePreview,
  Passkey,
  Payment,
  PaymentInput,
  User,
} from './types';

export class ApiError extends Error {
  status: number;
  code: string;
  constructor(status: number, code: string, message: string) {
    super(message);
    this.status = status;
    this.code = code;
  }
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: body !== undefined ? { 'Content-Type': 'application/json' } : undefined,
    body: body !== undefined ? JSON.stringify(body) : undefined,
    credentials: 'same-origin',
  });
  if (res.status === 204) return undefined as T;
  const text = await res.text();
  let data: any = null;
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      data = null;
    }
  }
  if (!res.ok) {
    const code = data?.error ?? `http_${res.status}`;
    const message = data?.message ?? humanize(code, res.status);
    throw new ApiError(res.status, code, message);
  }
  return data as T;
}

function humanize(code: string, status: number): string {
  const known: Record<string, string> = {
    unauthorized: 'You need to sign in.',
    invalid_amount: 'The amount is not valid.',
    payers_mismatch: 'Payer amounts must add up to the total.',
    shares_mismatch: 'Splits must add up to the total.',
    no_participants: 'Pick at least one person to split with.',
    not_member: 'That person is not in this group.',
    invalid_date: 'The date is not valid.',
    nonzero_balance: 'Settle your balance before leaving the group.',
    last_passkey: 'You cannot remove your only passkey.',
  };
  return known[code] ?? (status >= 500 ? 'Something went wrong on the server.' : `Request failed (${code}).`);
}

const get = <T>(p: string) => request<T>('GET', p);
const post = <T>(p: string, b?: unknown) => request<T>('POST', p, b ?? {});
const patch = <T>(p: string, b: unknown) => request<T>('PATCH', p, b);
const del = <T>(p: string, b?: unknown) => request<T>('DELETE', p, b);

// ---- auth
export const me = () => get<User>('/api/me');
export const updateMe = (name: string) => patch<User>('/api/me', { name });
export const registerBegin = (name: string) => post<any>('/api/auth/register/begin', { name });
export const registerFinish = (credential: unknown) => post<User>('/api/auth/register/finish', credential);
export const loginBegin = () => post<any>('/api/auth/login/begin', {});
export const loginFinish = (assertion: unknown) => post<User>('/api/auth/login/finish', assertion);
export const logout = () => post<void>('/api/auth/logout');
export const listPasskeys = () => get<Passkey[]>('/api/auth/passkeys');
export const addPasskeyBegin = (name: string) => post<any>('/api/auth/passkeys/begin', { name });
export const addPasskeyFinish = (credential: unknown) => post<Passkey>('/api/auth/passkeys/finish', credential);
export const deletePasskey = (id: string) => del<void>(`/api/auth/passkeys/${encodeURIComponent(id)}`);

// ---- groups
export const listGroups = () => get<Group[]>('/api/groups');
export const createGroup = (name: string, currency: string) => post<Group>('/api/groups', { name, currency });
export const getGroup = (id: string) => get<GroupDetail>(`/api/groups/${encodeURIComponent(id)}`);
export const updateGroup = (id: string, body: { name?: string; currency?: string }) =>
  patch<Group>(`/api/groups/${encodeURIComponent(id)}`, body);
export const leaveGroup = (id: string) => post<void>(`/api/groups/${encodeURIComponent(id)}/leave`);
export const createInvite = (id: string) => post<Invite>(`/api/groups/${encodeURIComponent(id)}/invites`);
export const getInvite = (token: string) => get<InvitePreview>(`/api/invites/${encodeURIComponent(token)}`);
export const acceptInvite = (token: string) => post<Group>(`/api/invites/${encodeURIComponent(token)}/accept`);

// ---- expenses & payments
export const createExpense = (gid: string, body: ExpenseInput) =>
  post<Expense>(`/api/groups/${encodeURIComponent(gid)}/expenses`, body);
export const updateExpense = (gid: string, eid: string, body: ExpenseInput) =>
  patch<Expense>(`/api/groups/${encodeURIComponent(gid)}/expenses/${encodeURIComponent(eid)}`, body);
export const deleteExpense = (gid: string, eid: string) =>
  del<void>(`/api/groups/${encodeURIComponent(gid)}/expenses/${encodeURIComponent(eid)}`);
export const createPayment = (gid: string, body: PaymentInput) =>
  post<Payment>(`/api/groups/${encodeURIComponent(gid)}/payments`, body);
export const deletePayment = (gid: string, pid: string) =>
  del<void>(`/api/groups/${encodeURIComponent(gid)}/payments/${encodeURIComponent(pid)}`);

// ---- events
export const groupEvents = (gid: string, since: number, limit = 200) =>
  get<GroupEvent[]>(`/api/groups/${encodeURIComponent(gid)}/events?since=${since}&limit=${limit}`);
export const activity = (limit = 50) => get<GroupEvent[]>(`/api/activity?limit=${limit}`);

// ---- push
export const vapidKey = () => get<{ public_key: string }>('/api/push/vapid');
export const pushSubscribe = (sub: PushSubscriptionJSON) => post<void>('/api/push/subscribe', sub);
export const pushUnsubscribe = (endpoint: string) => del<void>('/api/push/subscribe', { endpoint });
