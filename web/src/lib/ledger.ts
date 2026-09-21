// Client-side mirror of the server ledger so optimistic updates can show balances
// before the authoritative refetch lands.
import type { Balance, Debt, Expense, Payment } from './types';

export function computeNet(expenses: Expense[], payments: Payment[]): Balance[] {
  const net = new Map<string, number>();
  const add = (u: string, v: number) => net.set(u, (net.get(u) ?? 0) + v);
  for (const e of expenses) {
    for (const p of e.payers) add(p.user_id, p.amount);
    for (const s of e.shares) add(s.user_id, -s.amount);
  }
  for (const p of payments) {
    add(p.from_user_id, p.amount);
    add(p.to_user_id, -p.amount);
  }
  return [...net.entries()].map(([user_id, n]) => ({ user_id, net: n }));
}

export function computePairwise(expenses: Expense[], payments: Payment[]): Debt[] {
  // owes[a][b] = amount a owes b
  const owes = new Map<string, Map<string, number>>();
  const add = (a: string, b: string, v: number) => {
    if (a === b || v === 0) return;
    if (!owes.has(a)) owes.set(a, new Map());
    const m = owes.get(a)!;
    m.set(b, (m.get(b) ?? 0) + v);
  };
  for (const e of expenses) {
    const total = e.payers.reduce((s, p) => s + p.amount, 0);
    if (total <= 0) continue;
    for (const s of e.shares) {
      for (const p of e.payers) add(s.user_id, p.user_id, Math.round((s.amount * p.amount) / total));
    }
  }
  for (const p of payments) add(p.to_user_id, p.from_user_id, p.amount);
  const out: Debt[] = [];
  const seen = new Set<string>();
  for (const [a, m] of owes) {
    for (const [b] of m) {
      const key = [a, b].sort().join('|');
      if (seen.has(key)) continue;
      seen.add(key);
      const ab = owes.get(a)?.get(b) ?? 0;
      const ba = owes.get(b)?.get(a) ?? 0;
      const d = ab - ba;
      if (d > 0) out.push({ from_user_id: a, to_user_id: b, amount: d });
      else if (d < 0) out.push({ from_user_id: b, to_user_id: a, amount: -d });
    }
  }
  return out.sort((x, y) => y.amount - x.amount);
}

export function simplify(balances: Balance[]): Debt[] {
  const debtors = balances.filter((b) => b.net < 0).map((b) => ({ id: b.user_id, amt: -b.net })).sort((a, b) => b.amt - a.amt);
  const creditors = balances.filter((b) => b.net > 0).map((b) => ({ id: b.user_id, amt: b.net })).sort((a, b) => b.amt - a.amt);
  const out: Debt[] = [];
  let i = 0, j = 0;
  while (i < debtors.length && j < creditors.length) {
    const v = Math.min(debtors[i].amt, creditors[j].amt);
    if (v > 0) out.push({ from_user_id: debtors[i].id, to_user_id: creditors[j].id, amount: v });
    debtors[i].amt -= v;
    creditors[j].amt -= v;
    if (debtors[i].amt === 0) i++;
    if (creditors[j].amt === 0) j++;
  }
  return out;
}
