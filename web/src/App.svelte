<script lang="ts">
  import { onMount } from 'svelte';
  import { match, router } from './lib/router.svelte';
  import { boot, store } from './lib/store.svelte';
  import Toasts from './components/Toasts.svelte';
  import Spinner from './components/Spinner.svelte';
  import Home from './routes/Home.svelte';
  import Activity from './routes/Activity.svelte';
  import Group from './routes/Group.svelte';
  import ExpenseForm from './routes/ExpenseForm.svelte';
  import Join from './routes/Join.svelte';
  import Settings from './routes/Settings.svelte';
  import NotFound from './routes/NotFound.svelte';

  onMount(() => {
    void boot();
  });

  const p = $derived(router.path);
  const join = $derived(match('/join/:token', p));
  const group = $derived(match('/groups/:id', p));
  const expenseNew = $derived(match('/groups/:id/expenses/new', p));
  const expenseEdit = $derived(match('/groups/:id/expenses/:eid/edit', p));
  const tab = $derived(new URLSearchParams(router.search).get('tab') ?? undefined);
</script>

<Toasts />

{#if !store.booted && !store.me}
  <div class="flex min-h-dvh items-center justify-center"><Spinner label="Starting…" /></div>
{:else if join}
  <Join token={join.token} />
{:else if !store.me}
  <Home />
{:else if p === '/'}
  <Home />
{:else if p === '/activity'}
  <Activity />
{:else if p === '/settings'}
  <Settings />
{:else if expenseNew}
  <ExpenseForm groupId={expenseNew.id} />
{:else if expenseEdit}
  {#key expenseEdit.eid}
    <ExpenseForm groupId={expenseEdit.id} expenseId={expenseEdit.eid} />
  {/key}
{:else if group}
  {#key group.id}
    <Group id={group.id} {tab} />
  {/key}
{:else}
  <NotFound />
{/if}
