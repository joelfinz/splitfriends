<script lang="ts">
  import * as api from '../lib/api';
  import { fmt, splitByWeights, splitEqual, step, toMajor, toMinor } from '../lib/money';
  import { navigate } from '../lib/router.svelte';
  import { loadGroup, memberName, store } from '../lib/store.svelte';
  import { actionToast, errorToast } from '../lib/toast.svelte';
  import { CATEGORIES, normalizeCategory, suggestCategory } from '../lib/categories';
  import type { Category, Expense, ExpenseInput, SplitType } from '../lib/types';
  import Shell from '../components/Shell.svelte';
  import Spinner from '../components/Spinner.svelte';
  import Avatar from '../components/Avatar.svelte';
  import EmptyState from '../components/EmptyState.svelte';

  let { groupId, expenseId }: { groupId: string; expenseId?: string } = $props();

  const detail = $derived(store.details[groupId]);
  const group = $derived(detail?.group ?? store.groups[groupId]);
  const currency = $derived(group?.currency ?? 'USD');
  const myId = $derived(store.me?.id ?? '');
  const editing = $derived(!!expenseId);
  const existing = $derived(expenseId ? detail?.expenses.find((e) => e.id === expenseId) : undefined);

  // ---- form state
  let description = $state('');
  let amountStr = $state('');
  let date = $state(new Date().toISOString().slice(0, 10));
  let notes = $state('');
  let category = $state<Category>('other');
  let categoryManual = $state(false); // once the user taps a chip we stop suggesting
  let multiPayer = $state(false);
  let singlePayer = $state('');
  let payerAmounts = $state<Record<string, string>>({});
  let splitType = $state<SplitType>('equal');
  let participants = $state<Record<string, boolean>>({});
  let values = $state<Record<string, string>>({}); // exact: major str, percent: "33.33", shares: "1"
  let busy = $state(false);
  let initialised = $state(false);

  $effect(() => {
    loadGroup(groupId).catch(() => {});
  });

  // Initialise once we know the group (and the expense, when editing).
  $effect(() => {
    if (initialised || !group) return;
    if (editing && !existing) return; // wait for detail to load
    const members = group.members.map((m) => m.user_id);
    if (existing) {
      description = existing.description;
      amountStr = toMajor(existing.amount, currency);
      date = existing.date;
      notes = existing.notes ?? '';
      category = normalizeCategory(existing.category);
      categoryManual = true;
      splitType = existing.split_type;
      multiPayer = existing.payers.length > 1;
      singlePayer = existing.payers[0]?.user_id ?? myId;
      const pa: Record<string, string> = {};
      for (const p of existing.payers) pa[p.user_id] = toMajor(p.amount, currency);
      payerAmounts = pa;
      const parts: Record<string, boolean> = {};
      const vals: Record<string, string> = {};
      for (const s of existing.shares) {
        parts[s.user_id] = true;
        vals[s.user_id] =
          existing.split_type === 'exact' ? toMajor(s.amount, currency) : existing.split_type === 'percent' ? (s.value / 100).toString() : String(s.value);
      }
      for (const id of members) parts[id] ??= false;
      participants = parts;
      values = vals;
    } else {
      singlePayer = myId || members[0] || '';
      const parts: Record<string, boolean> = {};
      for (const id of members) parts[id] = true;
      participants = parts;
    }
    initialised = true;
  });

  // Suggest a category from the description until the user picks one by hand.
  $effect(() => {
    if (categoryManual) return;
    const d = description;
    category = suggestCategory(d) ?? 'other';
  });
  function pickCategory(c: Category) {
    category = c;
    categoryManual = true;
  }

  /** Rebuild the input the server would accept from a stored expense (for "Revert"). */
  function inputFrom(e: Expense): ExpenseInput {
    return {
      description: e.description,
      amount: e.amount,
      date: e.date,
      notes: e.notes ?? '',
      payers: e.payers.map((p) => ({ user_id: p.user_id, amount: p.amount })),
      split_type: e.split_type,
      category: normalizeCategory(e.category),
      shares: e.shares.map((sh) => ({ user_id: sh.user_id, value: sh.value })),
    };
  }

  // ---- derived math
  const amount = $derived(toMinor(amountStr, currency));
  const amountOk = $derived(Number.isFinite(amount) && amount > 0);
  const members = $derived(group?.members ?? []);
  const selected = $derived(members.filter((m) => participants[m.user_id]).map((m) => m.user_id));

  const payers = $derived.by(() => {
    if (!multiPayer) return singlePayer ? [{ user_id: singlePayer, amount: amountOk ? amount : 0 }] : [];
    return members
      .map((m) => ({ user_id: m.user_id, amount: toMinor(payerAmounts[m.user_id] ?? '', currency) }))
      .filter((p) => Number.isFinite(p.amount) && p.amount > 0);
  });
  const payersTotal = $derived(payers.reduce((s, p) => s + p.amount, 0));
  const payersRemaining = $derived(amountOk ? amount - payersTotal : 0);

  type Preview = { user_id: string; amount: number; value: number };
  const preview = $derived.by<Preview[]>(() => {
    if (!amountOk || selected.length === 0) return [];
    switch (splitType) {
      case 'equal': {
        const parts = splitEqual(amount, selected.length);
        return selected.map((id, i) => ({ user_id: id, amount: parts[i], value: 0 }));
      }
      case 'exact':
        return selected.map((id) => {
          const v = toMinor(values[id] ?? '', currency);
          return { user_id: id, amount: Number.isFinite(v) ? v : 0, value: Number.isFinite(v) ? v : 0 };
        });
      case 'percent': {
        const bps = selected.map((id) => Math.round(parseFloat(values[id] ?? '0') * 100) || 0);
        const parts = splitByWeights(amount, bps);
        return selected.map((id, i) => ({ user_id: id, amount: parts[i], value: bps[i] }));
      }
      case 'shares': {
        const w = selected.map((id) => Math.max(0, Math.round(parseFloat(values[id] ?? '1') || 0)));
        const parts = splitByWeights(amount, w);
        return selected.map((id, i) => ({ user_id: id, amount: parts[i], value: w[i] }));
      }
    }
  });
  const previewTotal = $derived(preview.reduce((s, p) => s + p.amount, 0));
  const percentTotal = $derived(splitType === 'percent' ? preview.reduce((s, p) => s + p.value, 0) : 10000);
  const sharesTotal = $derived(splitType === 'shares' ? preview.reduce((s, p) => s + p.value, 0) : 1);

  const splitError = $derived.by(() => {
    if (!amountOk) return '';
    if (selected.length === 0) return 'Pick at least one person.';
    if (splitType === 'exact' && previewTotal !== amount) {
      const diff = amount - previewTotal;
      return diff > 0 ? `${fmt(diff, currency)} left to allocate` : `${fmt(-diff, currency)} over`;
    }
    if (splitType === 'percent' && percentTotal !== 10000) {
      const diff = 10000 - percentTotal;
      return diff > 0 ? `${(diff / 100).toFixed(2)}% left to allocate` : `${(-diff / 100).toFixed(2)}% over`;
    }
    if (splitType === 'shares' && sharesTotal <= 0) return 'Shares must add up to more than zero.';
    return '';
  });
  const payerError = $derived.by(() => {
    if (!amountOk) return '';
    if (payers.length === 0) return 'Choose who paid.';
    if (multiPayer && payersRemaining !== 0) {
      return payersRemaining > 0 ? `${fmt(payersRemaining, currency)} left to assign` : `${fmt(-payersRemaining, currency)} over`;
    }
    return '';
  });
  const valid = $derived(description.trim().length > 0 && amountOk && /^\d{4}-\d{2}-\d{2}$/.test(date) && !splitError && !payerError);

  function toggleAll(on: boolean) {
    const p: Record<string, boolean> = {};
    for (const m of members) p[m.user_id] = on;
    participants = p;
  }
  function setSplit(t: SplitType) {
    splitType = t;
    if (t === 'shares') {
      const v = { ...values };
      for (const id of selected) if (!v[id] || !(parseFloat(v[id]) > 0)) v[id] = '1';
      values = v;
    } else if (t === 'percent') {
      const v = { ...values };
      const each = selected.length ? (10000 / selected.length) : 0;
      let allZero = selected.every((id) => !(parseFloat(v[id] ?? '0') > 0));
      if (allZero) for (const id of selected) v[id] = (Math.floor(each) / 100).toFixed(2);
      values = v;
    } else if (t === 'exact') {
      const v = { ...values };
      const allZero = selected.every((id) => !(parseFloat(v[id] ?? '0') > 0));
      if (allZero && amountOk) {
        const parts = splitEqual(amount, selected.length);
        selected.forEach((id, i) => (v[id] = toMajor(parts[i], currency)));
      }
      values = v;
    }
  }

  async function save() {
    if (!valid) return;
    const body: ExpenseInput = {
      description: description.trim(),
      amount,
      date,
      notes: notes.trim(),
      payers,
      split_type: splitType,
      category,
      shares: preview.map((p) => ({ user_id: p.user_id, value: p.value })),
    };
    busy = true;
    try {
      const label = body.description.length > 28 ? body.description.slice(0, 27) + '…' : body.description;
      if (expenseId && existing) {
        const previous = inputFrom(existing);
        const eid = expenseId;
        await api.updateExpense(groupId, eid, body);
        actionToast(`Saved "${label}"`, 'Revert', async () => {
          await api.updateExpense(groupId, eid, previous);
          await loadGroup(groupId);
        });
      } else {
        const created = await api.createExpense(groupId, body);
        actionToast(`Added "${label}"`, 'Undo', async () => {
          await api.deleteExpense(groupId, created.id);
          await loadGroup(groupId);
        });
      }
      await loadGroup(groupId);
      navigate(`/groups/${groupId}`, { replace: true });
    } catch (e) {
      errorToast(e);
    } finally {
      busy = false;
    }
  }
  async function remove() {
    if (!expenseId || !confirm('Delete this expense?')) return;
    busy = true;
    try {
      const eid = expenseId;
      const label = existing?.description ?? 'expense';
      await api.deleteExpense(groupId, eid);
      actionToast(`Deleted "${label}"`, 'Undo', async () => {
        await api.restoreExpense(groupId, eid);
        await loadGroup(groupId);
      });
      await loadGroup(groupId);
      navigate(`/groups/${groupId}`, { replace: true });
    } catch (e) {
      errorToast(e);
    } finally {
      busy = false;
    }
  }
</script>

<Shell title={editing ? 'Edit expense' : 'Add expense'} back={`/groups/${groupId}`} dock={false}>
  {#if !group || (editing && !detail)}
    <Spinner />
  {:else if editing && !existing}
    <EmptyState title="Expense not found" body="It may have been deleted by someone else.">
      <a href={`/groups/${groupId}`} class="btn">Back to group</a>
    </EmptyState>
  {:else}
    <form
      class="flex flex-col gap-4"
      onsubmit={(e) => {
        e.preventDefault();
        void save();
      }}
    >
      <div class="card card-border bg-base-100">
        <div class="card-body gap-1">
          <fieldset class="fieldset">
            <legend class="fieldset-legend">Description</legend>
            <!-- svelte-ignore a11y_autofocus -->
            <input class="input w-full" placeholder="Dinner at Luigi's" bind:value={description} maxlength="120" required autofocus={!editing} />
          </fieldset>
          <div class="grid grid-cols-2 gap-2">
            <fieldset class="fieldset">
              <legend class="fieldset-legend">Amount ({currency})</legend>
              <input class="input input-lg w-full tabular" type="number" inputmode="decimal" min="0" step={step(currency)} placeholder="0.00" bind:value={amountStr} required />
            </fieldset>
            <fieldset class="fieldset">
              <legend class="fieldset-legend">Date</legend>
              <input class="input input-lg w-full" type="date" bind:value={date} required />
            </fieldset>
          </div>
          <fieldset class="fieldset">
            <legend class="fieldset-legend">Category{!categoryManual && category !== 'other' ? ' · suggested' : ''}</legend>
            <div class="-mx-1 flex gap-1 overflow-x-auto px-1 pb-1" role="radiogroup" aria-label="Category">
              {#each CATEGORIES as c (c.id)}
                <button
                  type="button"
                  role="radio"
                  aria-checked={category === c.id}
                  class="btn btn-xs shrink-0 {category === c.id ? 'btn-neutral' : 'btn-soft'}"
                  onclick={() => pickCategory(c.id)}
                >
                  <span aria-hidden="true">{c.emoji}</span>
                  {c.label}
                </button>
              {/each}
            </div>
          </fieldset>
        </div>
      </div>

      <!-- paid by -->
      <div class="card card-border bg-base-100">
        <div class="card-body gap-2">
          <div class="flex items-center justify-between">
            <h3 class="card-title text-base">Paid by</h3>
            <label class="label cursor-pointer gap-2 text-xs">
              Multiple people
              <input type="checkbox" class="toggle toggle-sm" bind:checked={multiPayer} />
            </label>
          </div>
          {#if !multiPayer}
            <select class="select w-full" bind:value={singlePayer}>
              {#each members as m (m.user_id)}<option value={m.user_id}>{memberName(group, m.user_id)}</option>{/each}
            </select>
          {:else}
            <ul class="flex flex-col gap-2">
              {#each members as m (m.user_id)}
                <li class="flex items-center gap-2">
                  <Avatar name={m.name} size="sm" />
                  <span class="flex-1 truncate text-sm">{memberName(group, m.user_id)}</span>
                  <input class="input input-sm w-28 tabular" type="number" inputmode="decimal" min="0" step={step(currency)} placeholder="0.00" bind:value={payerAmounts[m.user_id]} />
                </li>
              {/each}
            </ul>
            <p class="text-xs {payerError ? 'text-error' : 'text-success'}">{payerError || 'Payments add up.'}</p>
          {/if}
        </div>
      </div>

      <!-- split -->
      <div class="card card-border bg-base-100">
        <div class="card-body gap-2">
          <div class="flex items-center justify-between">
            <h3 class="card-title text-base">Split</h3>
            <div class="flex gap-1">
              <button type="button" class="btn btn-ghost btn-xs" onclick={() => toggleAll(true)}>All</button>
              <button type="button" class="btn btn-ghost btn-xs" onclick={() => toggleAll(false)}>None</button>
            </div>
          </div>
          <div role="tablist" class="tabs tabs-box tabs-sm w-full">
            {#each [['equal', 'Equally'], ['exact', 'Exact'], ['percent', '%'], ['shares', 'Shares']] as [t, label] (t)}
              <button type="button" role="tab" class="tab flex-1 {splitType === t ? 'tab-active' : ''}" onclick={() => setSplit(t as SplitType)}>{label}</button>
            {/each}
          </div>

          <ul class="flex flex-col gap-2">
            {#each members as m (m.user_id)}
              {@const on = !!participants[m.user_id]}
              {@const p = preview.find((x) => x.user_id === m.user_id)}
              <li class="flex items-center gap-2">
                <input type="checkbox" class="checkbox checkbox-sm" bind:checked={participants[m.user_id]} />
                <Avatar name={m.name} size="sm" />
                <span class="flex-1 truncate text-sm {on ? '' : 'opacity-50'}">{memberName(group, m.user_id)}</span>
                {#if on && splitType === 'exact'}
                  <input class="input input-sm w-28 tabular" type="number" inputmode="decimal" min="0" step={step(currency)} placeholder="0.00" bind:value={values[m.user_id]} />
                {:else if on && splitType === 'percent'}
                  <label class="input input-sm w-28">
                    <input class="tabular grow" type="number" inputmode="decimal" min="0" max="100" step="0.01" placeholder="0" bind:value={values[m.user_id]} />
                    <span class="opacity-60">%</span>
                  </label>
                {:else if on && splitType === 'shares'}
                  <input class="input input-sm w-20 tabular" type="number" inputmode="numeric" min="0" step="1" placeholder="1" bind:value={values[m.user_id]} />
                {/if}
                <span class="w-24 text-right text-sm tabular {on ? '' : 'opacity-30'}">{on && p ? fmt(p.amount, currency) : '—'}</span>
              </li>
            {/each}
          </ul>
          {#if amountOk}
            <p class="text-xs {splitError ? 'text-error' : 'text-success'}">
              {splitError || `Split adds up to ${fmt(previewTotal, currency)}.`}
            </p>
          {:else}
            <p class="text-xs opacity-60">Enter an amount to preview the split.</p>
          {/if}
        </div>
      </div>

      <fieldset class="fieldset">
        <legend class="fieldset-legend">Notes</legend>
        <textarea class="textarea w-full" rows="2" placeholder="Optional" bind:value={notes} maxlength="500"></textarea>
      </fieldset>

      <div class="flex gap-2">
        {#if editing}
          <button type="button" class="btn btn-error btn-soft" disabled={busy} onclick={remove}>Delete</button>
        {/if}
        <button type="submit" class="btn btn-primary flex-1" disabled={!valid || busy}>
          {#if busy}<span class="loading loading-spinner loading-sm"></span>{/if}
          {editing ? 'Save changes' : 'Add expense'}
        </button>
      </div>
    </form>
  {/if}
</Shell>
