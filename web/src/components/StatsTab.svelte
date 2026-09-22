<script lang="ts">
  import { CATEGORIES, normalizeCategory } from '../lib/categories';
  import { decimals, fmt } from '../lib/money';
  import { memberName } from '../lib/store.svelte';
  import type { Expense, GroupDetail } from '../lib/types';
  import Columns from './charts/Columns.svelte';
  import HBars from './charts/HBars.svelte';
  import EmptyState from './EmptyState.svelte';

  let { detail, myId }: { detail: GroupDetail; myId: string } = $props();

  type Preset = 'this' | 'last' | '6m' | 'all';
  type Period = { kind: Preset } | { kind: 'month'; key: string };
  let period = $state<Period>({ kind: 'this' });
  let mode = $state<'group' | 'mine'>('group');

  const group = $derived(detail.group);
  const currency = $derived(group.currency);

  // ---- month helpers
  const monthKey = (d: Date) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`;
  function shiftMonth(key: string, delta: number): string {
    const [y, m] = key.split('-').map(Number);
    const d = new Date(y, m - 1 + delta, 1);
    return monthKey(d);
  }
  function monthLabel(key: string, long = false): string {
    const [y, m] = key.split('-').map(Number);
    return new Date(y, m - 1, 1).toLocaleDateString(undefined, long ? { month: 'long', year: 'numeric' } : { month: 'short' });
  }
  const now = monthKey(new Date());
  const lastMonth = shiftMonth(now, -1);
  const rangeEnd = $derived(period.kind === 'month' && period.key > now ? period.key : now);
  const rangeLen = $derived(period.kind === 'all' ? 12 : 6);
  const rangeMonths = $derived.by(() => {
    // Window ending at the current month; for a selected month outside it, end at that month instead.
    let end = rangeEnd;
    if (period.kind === 'month' && shiftMonth(end, -(rangeLen - 1)) > period.key) end = period.key;
    return Array.from({ length: rangeLen }, (_, i) => shiftMonth(end, -(rangeLen - 1 - i)));
  });

  function inPeriod(e: Expense): boolean {
    const k = e.date.slice(0, 7);
    switch (period.kind) {
      case 'this':
        return k === now;
      case 'last':
        return k === lastMonth;
      case '6m':
        return k >= rangeMonths[0] && k <= now;
      case 'all':
        return true;
      case 'month':
        return k === period.key;
    }
  }
  function valueOf(e: Expense): number {
    if (mode === 'group') return e.amount;
    return e.shares.filter((s) => s.user_id === myId).reduce((s, x) => s + x.amount, 0);
  }

  // ---- derived data
  const filtered = $derived(detail.expenses.filter(inPeriod));
  const total = $derived(filtered.reduce((s, e) => s + valueOf(e), 0));

  const byCategory = $derived.by(() => {
    const sums = new Map<string, number>();
    for (const e of filtered) {
      const c = normalizeCategory(e.category);
      sums.set(c, (sums.get(c) ?? 0) + valueOf(e));
    }
    return CATEGORIES.filter((c) => (sums.get(c.id) ?? 0) > 0)
      .map((c) => ({ key: c.id, label: c.label, emoji: c.emoji, values: [sums.get(c.id) ?? 0] }))
      .sort((a, b) => b.values[0] - a.values[0]);
  });

  const byMonth = $derived.by(() => {
    const sums = new Map<string, number>();
    for (const e of detail.expenses) {
      const k = e.date.slice(0, 7);
      sums.set(k, (sums.get(k) ?? 0) + valueOf(e));
    }
    return rangeMonths.map((k) => ({ key: k, label: monthLabel(k), values: [sums.get(k) ?? 0] }));
  });
  const selectedMonth = $derived(period.kind === 'this' ? now : period.kind === 'last' ? lastMonth : period.kind === 'month' ? period.key : null);

  const perMember = $derived.by(() => {
    return group.members
      .map((m) => {
        let paid = 0;
        let share = 0;
        for (const e of filtered) {
          for (const p of e.payers) if (p.user_id === m.user_id) paid += p.amount;
          for (const s of e.shares) if (s.user_id === m.user_id) share += s.amount;
        }
        const diff = paid - share;
        const note = diff === 0 ? 'even' : diff > 0 ? `paid ${fmt(diff, currency)} more` : `share ${fmt(-diff, currency)} larger`;
        return { key: m.user_id, label: memberName(group, m.user_id), values: [paid, share], note, sort: paid + share };
      })
      .filter((r) => r.sort > 0)
      .sort((a, b) => b.sort - a.sort);
  });

  // ---- formatting
  const money = (v: number) => fmt(v, currency);
  function compact(v: number): string {
    const major = v / Math.pow(10, decimals(currency));
    try {
      return new Intl.NumberFormat(undefined, { style: 'currency', currency, notation: 'compact', maximumFractionDigits: 1 }).format(major);
    } catch {
      return money(v);
    }
  }

  const periodLabel = $derived.by(() => {
    switch (period.kind) {
      case 'this':
        return monthLabel(now, true);
      case 'last':
        return monthLabel(lastMonth, true);
      case '6m':
        return `${monthLabel(rangeMonths[0])} – ${monthLabel(now)}`;
      case 'all':
        return 'All time';
      case 'month':
        return monthLabel(period.key, true);
    }
  });
  const presets: { id: Preset; label: string }[] = [
    { id: 'this', label: 'This month' },
    { id: 'last', label: 'Last month' },
    { id: '6m', label: '6 months' },
    { id: 'all', label: 'All time' },
  ];
  function pickMonth(key: string) {
    period = key === now ? { kind: 'this' } : key === lastMonth ? { kind: 'last' } : { kind: 'month', key };
  }
  const series1 = $derived([{ name: mode === 'group' ? 'Spent' : 'My share', color: 'var(--color-primary)' }]);
  const series2 = [
    { name: 'Paid', color: 'var(--color-primary)' },
    { name: 'Share', color: 'var(--color-secondary)' },
  ];
</script>

<div class="flex flex-col gap-4">
  <!-- filters: one row -->
  <div class="flex flex-wrap items-center gap-2">
    <div class="join">
      {#each presets as p (p.id)}
        <button class="btn btn-sm join-item {period.kind === p.id ? 'btn-active' : ''}" onclick={() => (period = { kind: p.id })}>{p.label}</button>
      {/each}
    </div>
    <div class="join ml-auto">
      <button class="btn btn-sm join-item {mode === 'group' ? 'btn-active' : ''}" onclick={() => (mode = 'group')}>Group</button>
      <button class="btn btn-sm join-item {mode === 'mine' ? 'btn-active' : ''}" onclick={() => (mode = 'mine')}>My share</button>
    </div>
  </div>

  <!-- headline -->
  <div class="stats border border-base-300 bg-base-100">
    <div class="stat py-3">
      <div class="stat-title text-xs">{mode === 'group' ? 'Group spent' : 'My share'} · {periodLabel}</div>
      <div class="stat-value text-2xl">{money(total)}</div>
      <div class="stat-desc">{filtered.length} {filtered.length === 1 ? 'expense' : 'expenses'}</div>
    </div>
  </div>

  <!-- over time -->
  <div class="card card-border bg-base-100">
    <div class="card-body gap-2 p-4">
      <h3 class="text-xs font-semibold uppercase tracking-wide opacity-60">Over time</h3>
      <Columns columns={byMonth} series={series1} format={compact} selectedKey={selectedMonth} onSelect={pickMonth} />
      <p class="text-xs opacity-50">Tap a month to focus on it.</p>
    </div>
  </div>

  {#if filtered.length === 0}
    <EmptyState title="Nothing in this period" body="Pick another period above, or add an expense." />
  {:else}
    <!-- by category -->
    <div class="card card-border bg-base-100">
      <div class="card-body gap-3 p-4">
        <h3 class="text-xs font-semibold uppercase tracking-wide opacity-60">By category</h3>
        <HBars rows={byCategory} series={series1} format={money} showPct />
      </div>
    </div>

    <!-- per member -->
    {#if perMember.length}
      <div class="card card-border bg-base-100">
        <div class="card-body gap-3 p-4">
          <div class="flex items-center justify-between">
            <h3 class="text-xs font-semibold uppercase tracking-wide opacity-60">Per member</h3>
            <div class="flex items-center gap-3 text-xs opacity-70" aria-label="Legend">
              {#each series2 as s (s.name)}
                <span class="flex items-center gap-1"><span class="inline-block size-2.5 rounded-full" style="background: {s.color}"></span>{s.name}</span>
              {/each}
            </div>
          </div>
          <HBars rows={perMember} series={series2} format={money} />
          <p class="text-xs opacity-50">Paid is what each person covered; share is what they owed. Both use full amounts regardless of the Group / My share toggle.</p>
        </div>
      </div>
    {/if}
  {/if}
</div>
