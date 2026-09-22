<script lang="ts">
  import * as api from '../lib/api';
  import { toMinor, toMajor, step, fmt } from '../lib/money';
  import { navigate } from '../lib/router.svelte';
  import { loadGroup, memberName, removeGroupLocal, store, upsertGroupLocal } from '../lib/store.svelte';
  import { actionToast, errorToast, toast } from '../lib/toast.svelte';
  import { ApiError } from '../lib/api';
  import { categoryInfo } from '../lib/categories';
  import type { Debt, Expense, GroupDetail, Invite, Payment, Trash } from '../lib/types';
  import Shell from '../components/Shell.svelte';
  import StatsTab from '../components/StatsTab.svelte';
  import Spinner from '../components/Spinner.svelte';
  import Money from '../components/Money.svelte';
  import Avatar from '../components/Avatar.svelte';
  import EmptyState from '../components/EmptyState.svelte';

  let { id, tab: initialTab = 'expenses' }: { id: string; tab?: string } = $props();

  type Tab = 'expenses' | 'balances' | 'settle' | 'stats';
  // svelte-ignore state_referenced_locally
  let tab = $state<Tab>((['expenses', 'balances', 'settle', 'stats'].includes(initialTab) ? initialTab : 'expenses') as Tab);
  let loadError = $state('');
  let simplified = $state(true);

  const detail = $derived<GroupDetail | undefined>(store.details[id]);
  const group = $derived(detail?.group ?? store.groups[id]);
  const currency = $derived(group?.currency ?? 'USD');
  const myId = $derived(store.me?.id ?? '');

  $effect(() => {
    loadError = '';
    loadGroup(id).catch((e) => {
      if (!store.details[id]) loadError = (e as Error).message;
    });
  });

  // ---- expenses grouped by month (expenses + payments interleaved, newest first)
  type Row = { kind: 'expense'; e: Expense; date: string } | { kind: 'payment'; p: GroupDetail['payments'][number]; date: string };
  const rows = $derived.by<Row[]>(() => {
    if (!detail) return [];
    const r: Row[] = [
      ...detail.expenses.map((e) => ({ kind: 'expense' as const, e, date: e.date })),
      ...detail.payments.map((p) => ({ kind: 'payment' as const, p, date: p.date })),
    ];
    return r.sort((a, b) => (a.date < b.date ? 1 : a.date > b.date ? -1 : 0));
  });
  const months = $derived.by(() => {
    const out: { key: string; label: string; rows: Row[] }[] = [];
    for (const row of rows) {
      const key = row.date.slice(0, 7);
      let m = out.find((x) => x.key === key);
      if (!m) {
        const d = new Date(row.date + 'T00:00:00');
        m = { key, label: d.toLocaleDateString(undefined, { month: 'long', year: 'numeric' }), rows: [] };
        out.push(m);
      }
      m.rows.push(row);
    }
    return out;
  });

  function myShare(e: Expense): number {
    // + means I lent (paid more than my share), - means I borrowed
    const paid = e.payers.filter((p) => p.user_id === myId).reduce((s, p) => s + p.amount, 0);
    const owed = e.shares.filter((s) => s.user_id === myId).reduce((s, x) => s + x.amount, 0);
    return paid - owed;
  }
  function involved(e: Expense): boolean {
    return e.payers.some((p) => p.user_id === myId) || e.shares.some((s) => s.user_id === myId);
  }
  function payerLabel(e: Expense): string {
    if (e.payers.length === 1) return memberName(group, e.payers[0].user_id);
    return `${e.payers.length} people`;
  }
  function dayOf(date: string): string {
    return new Date(date + 'T00:00:00').toLocaleDateString(undefined, { day: 'numeric', month: 'short' });
  }

  // ---- invite
  let inviteDialog = $state<HTMLDialogElement | null>(null);
  let invite = $state<Invite | null>(null);
  let inviteBusy = $state(false);
  async function openInvite() {
    inviteDialog?.showModal();
    if (invite && new Date(invite.expires_at).getTime() > Date.now()) return;
    inviteBusy = true;
    try {
      invite = await api.createInvite(id);
    } catch (e) {
      errorToast(e);
    } finally {
      inviteBusy = false;
    }
  }
  async function copyInvite() {
    if (!invite) return;
    try {
      await navigator.clipboard.writeText(invite.url);
      toast('Link copied', 'success');
    } catch {
      toast('Could not copy. Long-press the link to copy it.', 'error');
    }
  }
  async function shareInvite() {
    if (!invite || !group) return;
    try {
      await navigator.share({ title: `Join ${group.name} on Fairshare`, text: `Join "${group.name}" to split expenses`, url: invite.url });
    } catch {
      /* user cancelled */
    }
  }
  const canShare = typeof navigator !== 'undefined' && typeof navigator.share === 'function';

  // ---- rename / leave / members
  let settingsDialog = $state<HTMLDialogElement | null>(null);
  let newName = $state('');
  let renameBusy = $state(false);
  function openSettings() {
    newName = group?.name ?? '';
    settingsDialog?.showModal();
  }
  async function rename() {
    const n = newName.trim();
    if (!n || n === group?.name) return;
    renameBusy = true;
    try {
      const g = await api.updateGroup(id, { name: n });
      upsertGroupLocal(g);
      toast('Group renamed', 'success');
    } catch (e) {
      errorToast(e);
    } finally {
      renameBusy = false;
    }
  }
  let leaveBusy = $state(false);
  async function leave() {
    if (!confirm(`Leave "${group?.name}"? You can rejoin with an invite link.`)) return;
    leaveBusy = true;
    try {
      await api.leaveGroup(id);
      removeGroupLocal(id);
      settingsDialog?.close();
      navigate('/', { replace: true });
    } catch (e) {
      errorToast(e);
    } finally {
      leaveBusy = false;
    }
  }

  // ---- settle up
  let payFrom = $state('');
  let payTo = $state('');
  let payAmount = $state('');
  let payDate = $state(new Date().toISOString().slice(0, 10));
  let payNotes = $state('');
  let payBusy = $state(false);
  const payMinor = $derived(toMinor(payAmount, currency));
  const payValid = $derived(payFrom && payTo && payFrom !== payTo && Number.isFinite(payMinor) && payMinor > 0 && /^\d{4}-\d{2}-\d{2}$/.test(payDate));

  $effect(() => {
    if (!payFrom && myId) payFrom = myId;
  });

  function prefill(d: Debt) {
    payFrom = d.from_user_id;
    payTo = d.to_user_id;
    payAmount = toMajor(d.amount, currency);
    tab = 'settle';
  }
  async function recordPayment() {
    if (!payValid) return;
    payBusy = true;
    try {
      const p = await api.createPayment(id, { from_user_id: payFrom, to_user_id: payTo, amount: payMinor, date: payDate, notes: payNotes.trim() });
      actionToast(`Recorded ${fmt(p.amount, currency)} payment`, 'Undo', async () => {
        await api.deletePayment(id, p.id);
        await loadGroup(id);
      });
      payAmount = '';
      payNotes = '';
      await loadGroup(id);
      tab = 'balances';
    } catch (e) {
      errorToast(e);
    } finally {
      payBusy = false;
    }
  }
  async function deletePayment(p: Payment) {
    if (!confirm('Delete this payment?')) return;
    try {
      await api.deletePayment(id, p.id);
      actionToast(`Deleted ${fmt(p.amount, currency)} payment`, 'Undo', async () => {
        await api.restorePayment(id, p.id);
        await loadGroup(id);
      });
      await loadGroup(id);
    } catch (e) {
      errorToast(e);
    }
  }

  // ---- recently deleted
  let trash = $state<Trash | null>(null);
  let trashBusy = $state(false);
  let trashOpen = $state(false);
  async function loadTrash() {
    trashBusy = true;
    try {
      trash = await api.getTrash(id);
    } catch (e) {
      errorToast(e);
    } finally {
      trashBusy = false;
    }
  }
  function toggleTrash() {
    trashOpen = !trashOpen;
    if (trashOpen && !trash) void loadTrash();
  }
  async function restore(kind: 'expense' | 'payment', itemId: string) {
    try {
      if (kind === 'expense') await api.restoreExpense(id, itemId);
      else await api.restorePayment(id, itemId);
      if (trash) {
        trash = {
          expenses: trash.expenses.filter((e) => e.id !== itemId),
          payments: trash.payments.filter((p) => p.id !== itemId),
        };
      }
      toast('Restored', 'success');
      await loadGroup(id);
    } catch (e) {
      if (e instanceof ApiError && e.code === 'not_member') toast('Someone on this expense has left the group, so it cannot be restored.', 'error', 5000);
      else errorToast(e);
    }
  }
  function ago(iso: string): string {
    const ms = Date.now() - new Date(iso).getTime();
    const m = Math.round(ms / 60000);
    if (m < 1) return 'just now';
    if (m < 60) return `${m} min ago`;
    const h = Math.round(m / 60);
    if (h < 24) return `${h} h ago`;
    const d = Math.round(h / 24);
    return d === 1 ? 'yesterday' : `${d} days ago`;
  }

  const debts = $derived(detail ? (simplified ? detail.simplified : detail.pairwise) : []);
  const myDebts = $derived(debts.filter((d) => d.from_user_id === myId || d.to_user_id === myId));
  const otherDebts = $derived(debts.filter((d) => d.from_user_id !== myId && d.to_user_id !== myId));
</script>

<Shell title={group?.name ?? 'Group'} back="/">
  {#snippet actions()}
    <button class="btn btn-ghost btn-circle" aria-label="Invite" onclick={openInvite}>
      <svg xmlns="http://www.w3.org/2000/svg" class="size-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M18 9v3m0 0v3m0-3h3m-3 0h-3m-2-5a4 4 0 11-8 0 4 4 0 018 0zM3 20a6 6 0 0112 0v1H3v-1z" /></svg>
    </button>
    <button class="btn btn-ghost btn-circle" aria-label="Group settings" onclick={openSettings}>
      <svg xmlns="http://www.w3.org/2000/svg" class="size-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M12 5v.01M12 12v.01M12 19v.01" /></svg>
    </button>
  {/snippet}

  {#if loadError && !detail}
    <EmptyState title="Could not load group" body={loadError}>
      <a href="/" class="btn">Back to groups</a>
    </EmptyState>
  {:else if !group}
    <Spinner />
  {:else}
    <div class="flex flex-col gap-4">
      <!-- header summary -->
      <div class="flex items-center gap-3">
        <div class="avatar-group -space-x-3">
          {#each group.members.slice(0, 5) as m (m.user_id)}
            <Avatar name={m.name} size="sm" />
          {/each}
          {#if group.members.length > 5}
            <div class="avatar avatar-placeholder"><div class="w-7 rounded-full bg-neutral text-neutral-content text-xs"><span>+{group.members.length - 5}</span></div></div>
          {/if}
        </div>
        <div class="min-w-0 flex-1 text-sm opacity-70">
          {group.members.length} {group.members.length === 1 ? 'member' : 'members'} · {group.currency}
        </div>
        <div class="text-right">
          {#if group.my_balance === 0}
            <div class="text-xs opacity-60">settled up</div>
          {:else}
            <div class="text-xs opacity-60">{group.my_balance > 0 ? 'you are owed' : 'you owe'}</div>
            <Money amount={group.my_balance} currency={group.currency} colored class="font-semibold" />
          {/if}
        </div>
      </div>

      <div role="tablist" class="tabs tabs-box w-full">
        <button role="tab" class="tab flex-1 {tab === 'expenses' ? 'tab-active' : ''}" onclick={() => (tab = 'expenses')}>Expenses</button>
        <button role="tab" class="tab flex-1 {tab === 'balances' ? 'tab-active' : ''}" onclick={() => (tab = 'balances')}>Balances</button>
        <button role="tab" class="tab flex-1 {tab === 'settle' ? 'tab-active' : ''}" onclick={() => (tab = 'settle')}>Settle up</button>
        <button role="tab" class="tab flex-1 {tab === 'stats' ? 'tab-active' : ''}" onclick={() => (tab = 'stats')}>Stats</button>
      </div>

      {#if !detail}
        <Spinner />
      {:else if tab === 'expenses'}
        {#if rows.length === 0}
          <EmptyState title="No expenses yet" body="Add the first one and everyone in the group sees it instantly.">
            <a href={`/groups/${id}/expenses/new`} class="btn btn-primary">Add expense</a>
          </EmptyState>
        {:else}
          {#each months as m (m.key)}
            <div>
              <h3 class="mb-1 px-1 text-xs font-semibold uppercase tracking-wide opacity-60">{m.label}</h3>
              <ul class="list rounded-box border border-base-300 bg-base-100">
                {#each m.rows as row (row.kind === 'expense' ? 'e' + row.e.id : 'p' + row.p.id)}
                  {#if row.kind === 'expense'}
                    {@const mine = myShare(row.e)}
                    {@const cat = categoryInfo(row.e.category)}
                    <li class="list-row items-center">
                      <a href={`/groups/${id}/expenses/${row.e.id}/edit`} class="grid size-10 place-items-center rounded-box bg-base-200 text-lg" title={cat.label} aria-label={cat.label}>
                        <span aria-hidden="true">{cat.emoji}</span>
                      </a>
                      <a href={`/groups/${id}/expenses/${row.e.id}/edit`} class="min-w-0">
                        <div class="truncate font-medium">{row.e.description}</div>
                        <div class="truncate text-xs opacity-60">{dayOf(row.e.date)} · {payerLabel(row.e)} paid {fmt(row.e.amount, currency)}</div>
                      </a>
                      <a href={`/groups/${id}/expenses/${row.e.id}/edit`} class="text-right">
                        {#if !involved(row.e)}
                          <div class="text-xs opacity-50">not involved</div>
                        {:else if mine === 0}
                          <div class="text-xs opacity-50">even</div>
                        {:else}
                          <div class="text-xs opacity-60">{mine > 0 ? 'you lent' : 'you borrowed'}</div>
                          <Money amount={mine} currency={currency} colored class="text-sm font-semibold" />
                        {/if}
                      </a>
                    </li>
                  {:else}
                    <li class="list-row items-center">
                      <div class="grid size-10 place-items-center rounded-box bg-base-200 text-lg" title="Payment" aria-label="Payment"><span aria-hidden="true">💸</span></div>
                      <div class="min-w-0">
                        <div class="truncate text-sm">
                          <span class="font-medium">{memberName(group, row.p.from_user_id)}</span> paid
                          <span class="font-medium">{memberName(group, row.p.to_user_id)}</span>
                        </div>
                        <div class="truncate text-xs opacity-60">{dayOf(row.p.date)} · Payment{row.p.notes ? ` · ${row.p.notes}` : ''}</div>
                      </div>
                      <div class="flex items-center gap-1">
                        <Money amount={row.p.amount} currency={currency} class="text-sm font-semibold" />
                        <button class="btn btn-ghost btn-xs btn-square" aria-label="Delete payment" onclick={() => deletePayment(row.p)}>✕</button>
                      </div>
                    </li>
                  {/if}
                {/each}
              </ul>
            </div>
          {/each}
        {/if}
        <div class="fab">
          <a href={`/groups/${id}/expenses/new`} class="btn btn-lg btn-circle btn-primary shadow-lg" aria-label="Add expense">
            <svg xmlns="http://www.w3.org/2000/svg" class="size-6" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M12 4v16m8-8H4" /></svg>
          </a>
        </div>
      {:else if tab === 'balances'}
        <ul class="list rounded-box border border-base-300 bg-base-100">
          {#each group.members as m (m.user_id)}
            {@const net = detail.balances.find((b) => b.user_id === m.user_id)?.net ?? 0}
            <li class="list-row items-center">
              <Avatar name={m.name} size="sm" />
              <div class="font-medium">{memberName(group, m.user_id)}</div>
              <div class="text-right">
                {#if net === 0}
                  <span class="text-xs opacity-50">settled</span>
                {:else}
                  <div class="text-xs opacity-60">{net > 0 ? 'gets back' : 'owes'}</div>
                  <Money amount={net} currency={currency} colored class="font-semibold" />
                {/if}
              </div>
            </li>
          {/each}
        </ul>

        <div class="flex items-center justify-between px-1">
          <h3 class="text-xs font-semibold uppercase tracking-wide opacity-60">Who pays whom</h3>
          <label class="label cursor-pointer gap-2 text-xs">
            Simplify debts
            <input type="checkbox" class="toggle toggle-sm" bind:checked={simplified} />
          </label>
        </div>
        {#if debts.length === 0}
          <div class="alert alert-success alert-soft text-sm"><span>Everyone is settled up.</span></div>
        {:else}
          <ul class="list rounded-box border border-base-300 bg-base-100">
            {#each [...myDebts, ...otherDebts] as d (d.from_user_id + d.to_user_id)}
              <li class="list-row items-center">
                <Avatar name={memberName(group, d.from_user_id)} size="sm" />
                <div class="text-sm">
                  <span class="font-medium">{memberName(group, d.from_user_id)}</span>
                  {d.from_user_id === myId ? 'owe' : 'owes'}
                  <span class="font-medium">{memberName(group, d.to_user_id)}</span>
                </div>
                <div class="flex items-center gap-2">
                  <Money amount={d.amount} currency={currency} class="font-semibold" />
                  <button class="btn btn-sm btn-soft" onclick={() => prefill(d)}>Settle</button>
                </div>
              </li>
            {/each}
          </ul>
        {/if}
      {:else if tab === 'stats'}
        <StatsTab {detail} {myId} />
      {:else}
        <!-- settle up -->
        {#if detail.simplified.length}
          <div>
            <h3 class="mb-1 px-1 text-xs font-semibold uppercase tracking-wide opacity-60">Suggested</h3>
            <div class="flex flex-col gap-2">
              {#each detail.simplified as d (d.from_user_id + d.to_user_id)}
                <button class="btn btn-block justify-between {d.from_user_id === myId ? 'btn-soft btn-primary' : ''}" onclick={() => prefill(d)}>
                  <span class="truncate">{memberName(group, d.from_user_id)} → {memberName(group, d.to_user_id)}</span>
                  <Money amount={d.amount} currency={currency} />
                </button>
              {/each}
            </div>
          </div>
        {/if}

        <form
          class="card card-border bg-base-100"
          onsubmit={(e) => {
            e.preventDefault();
            void recordPayment();
          }}
        >
          <div class="card-body gap-2">
            <h3 class="card-title text-base">Record a payment</h3>
            <div class="grid grid-cols-1 gap-2 sm:grid-cols-2">
              <fieldset class="fieldset">
                <legend class="fieldset-legend">From</legend>
                <select class="select w-full" bind:value={payFrom}>
                  {#each group.members as m (m.user_id)}<option value={m.user_id}>{memberName(group, m.user_id)}</option>{/each}
                </select>
              </fieldset>
              <fieldset class="fieldset">
                <legend class="fieldset-legend">To</legend>
                <select class="select w-full" bind:value={payTo}>
                  <option value="" disabled>Choose…</option>
                  {#each group.members as m (m.user_id)}<option value={m.user_id}>{memberName(group, m.user_id)}</option>{/each}
                </select>
              </fieldset>
              <fieldset class="fieldset">
                <legend class="fieldset-legend">Amount ({currency})</legend>
                <input class="input w-full tabular" type="number" inputmode="decimal" min="0" step={step(currency)} placeholder="0.00" bind:value={payAmount} />
              </fieldset>
              <fieldset class="fieldset">
                <legend class="fieldset-legend">Date</legend>
                <input class="input w-full" type="date" bind:value={payDate} />
              </fieldset>
            </div>
            <fieldset class="fieldset">
              <legend class="fieldset-legend">Notes</legend>
              <input class="input w-full" placeholder="Optional" bind:value={payNotes} maxlength="200" />
            </fieldset>
            {#if payFrom && payTo && payFrom === payTo}
              <p class="text-xs text-error">Payer and receiver must be different people.</p>
            {/if}
            <div class="card-actions justify-end">
              <button type="submit" class="btn btn-primary" disabled={!payValid || payBusy}>
                {#if payBusy}<span class="loading loading-spinner loading-sm"></span>{/if}
                Record payment
              </button>
            </div>
          </div>
        </form>
      {/if}
    </div>

    <!-- invite dialog -->
    <dialog bind:this={inviteDialog} class="modal modal-bottom sm:modal-middle">
      <div class="modal-box">
        <h3 class="text-lg font-bold">Invite to {group.name}</h3>
        <p class="py-1 text-sm opacity-70">Anyone with this link can join the group. Links expire.</p>
        {#if inviteBusy}
          <div class="py-4 text-center"><span class="loading loading-spinner"></span></div>
        {:else if invite}
          <div class="join mt-2 w-full">
            <input class="input join-item w-full text-xs" readonly value={invite.url} onfocus={(e) => (e.currentTarget as HTMLInputElement).select()} />
            <button class="btn join-item" onclick={copyInvite}>Copy</button>
          </div>
          <p class="mt-1 text-xs opacity-60">Expires {new Date(invite.expires_at).toLocaleString()}</p>
        {/if}
        <div class="modal-action">
          {#if canShare && invite}<button class="btn btn-primary" onclick={shareInvite}>Share…</button>{/if}
          <form method="dialog"><button class="btn">Done</button></form>
        </div>
      </div>
      <form method="dialog" class="modal-backdrop"><button>close</button></form>
    </dialog>

    <!-- settings dialog -->
    <dialog bind:this={settingsDialog} class="modal modal-bottom sm:modal-middle">
      <div class="modal-box">
        <h3 class="text-lg font-bold">Group settings</h3>
        <form
          class="mt-2"
          onsubmit={(e) => {
            e.preventDefault();
            void rename();
          }}
        >
          <fieldset class="fieldset">
            <legend class="fieldset-legend">Name</legend>
            <div class="join w-full">
              <input class="input join-item w-full" bind:value={newName} maxlength="80" required />
              <button type="submit" class="btn join-item" disabled={renameBusy || !newName.trim() || newName.trim() === group.name}>Save</button>
            </div>
          </fieldset>
        </form>

        <h4 class="mt-4 text-xs font-semibold uppercase tracking-wide opacity-60">Members</h4>
        <ul class="list mt-1">
          {#each group.members as m (m.user_id)}
            <li class="list-row items-center py-2">
              <Avatar name={m.name} size="sm" />
              <div class="text-sm">{memberName(group, m.user_id)}</div>
              <div class="text-xs opacity-60">joined {new Date(m.joined_at).toLocaleDateString()}</div>
            </li>
          {/each}
        </ul>

        <div class="collapse collapse-arrow mt-4 border border-base-300 bg-base-200/40">
          <input type="checkbox" checked={trashOpen} onchange={toggleTrash} aria-label="Recently deleted" />
          <div class="collapse-title text-xs font-semibold uppercase tracking-wide opacity-60">Recently deleted</div>
          <div class="collapse-content px-0">
            {#if trashBusy && !trash}
              <div class="py-2 text-center"><span class="loading loading-spinner loading-sm"></span></div>
            {:else if trash && trash.expenses.length === 0 && trash.payments.length === 0}
              <p class="px-4 pb-2 text-sm opacity-60">Nothing here. Deleted expenses and payments can be restored from this list.</p>
            {:else if trash}
              <ul class="list">
                {#each trash.expenses as e (e.id)}
                  {@const cat = categoryInfo(e.category)}
                  <li class="list-row items-center py-2">
                    <div class="grid size-9 place-items-center rounded-box bg-base-200" title={cat.label}><span aria-hidden="true">{cat.emoji}</span></div>
                    <div class="min-w-0">
                      <div class="truncate text-sm font-medium">{e.description}</div>
                      <div class="truncate text-xs opacity-60">{fmt(e.amount, currency)} · {dayOf(e.date)}{e.deleted_at ? ` · deleted ${ago(e.deleted_at)}` : ''}</div>
                    </div>
                    <button class="btn btn-sm btn-soft" onclick={() => restore('expense', e.id)}>Restore</button>
                  </li>
                {/each}
                {#each trash.payments as p (p.id)}
                  <li class="list-row items-center py-2">
                    <div class="grid size-9 place-items-center rounded-box bg-base-200" title="Payment"><span aria-hidden="true">💸</span></div>
                    <div class="min-w-0">
                      <div class="truncate text-sm font-medium">{memberName(group, p.from_user_id)} paid {memberName(group, p.to_user_id)}</div>
                      <div class="truncate text-xs opacity-60">{fmt(p.amount, currency)} · {dayOf(p.date)}{p.deleted_at ? ` · deleted ${ago(p.deleted_at)}` : ''}</div>
                    </div>
                    <button class="btn btn-sm btn-soft" onclick={() => restore('payment', p.id)}>Restore</button>
                  </li>
                {/each}
              </ul>
            {/if}
          </div>
        </div>

        <div class="modal-action justify-between">
          <button class="btn btn-error btn-soft" disabled={leaveBusy} onclick={leave}>
            {#if leaveBusy}<span class="loading loading-spinner loading-sm"></span>{/if}
            Leave group
          </button>
          <form method="dialog"><button class="btn">Close</button></form>
        </div>
      </div>
      <form method="dialog" class="modal-backdrop"><button>close</button></form>
    </dialog>
  {/if}
</Shell>
