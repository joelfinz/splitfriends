<script lang="ts">
  import * as api from '../lib/api';
  import { CURRENCIES } from '../lib/money';
  import { navigate } from '../lib/router.svelte';
  import { store, upsertGroupLocal, loadGroups } from '../lib/store.svelte';
  import { errorToast } from '../lib/toast.svelte';
  import Shell from '../components/Shell.svelte';
  import Money from '../components/Money.svelte';
  import EmptyState from '../components/EmptyState.svelte';
  import InstallHint from '../components/InstallHint.svelte';
  import PasskeyAuth from '../components/PasskeyAuth.svelte';
  import Avatar from '../components/Avatar.svelte';

  let dialog = $state<HTMLDialogElement | null>(null);
  let name = $state('');
  let currency = $state(guessCurrency());
  let busy = $state(false);

  function guessCurrency(): string {
    try {
      const region = new Intl.Locale(navigator.language).maximize().region;
      const map: Record<string, string> = { AE: 'AED', US: 'USD', GB: 'GBP', IN: 'INR', SG: 'SGD', AU: 'AUD', CA: 'CAD', JP: 'JPY', DE: 'EUR', FR: 'EUR', NL: 'EUR', ES: 'EUR', IT: 'EUR', IE: 'EUR', SA: 'SAR', QA: 'QAR', KW: 'KWD', BH: 'BHD', OM: 'OMR', MY: 'MYR', PH: 'PHP', PK: 'PKR', LK: 'LKR', NZ: 'NZD', ZA: 'ZAR', CH: 'CHF' };
      return (region && map[region]) || 'USD';
    } catch {
      return 'USD';
    }
  }

  async function create() {
    const n = name.trim();
    if (!n) return;
    busy = true;
    try {
      const g = await api.createGroup(n, currency);
      upsertGroupLocal(g);
      dialog?.close();
      name = '';
      navigate(`/groups/${g.id}`);
    } catch (e) {
      errorToast(e);
    } finally {
      busy = false;
    }
  }

  $effect(() => {
    if (store.me) loadGroups().catch(() => {});
  });

  const total = $derived(store.groupList.reduce((s, g) => s + g.my_balance, 0));
  const currencies = $derived(new Set(store.groupList.map((g) => g.currency)));
</script>

{#if !store.me}
  <div class="mx-auto flex min-h-dvh w-full max-w-md flex-col justify-center gap-8 px-6 py-12">
    <div class="text-center">
      <img src="/icons/icon.svg" alt="" class="mx-auto mb-4 size-20" />
      <h1 class="text-4xl font-bold tracking-tight">Fairshare</h1>
      <p class="mt-2 opacity-70">Split expenses with friends. Realtime, no fuss, no passwords.</p>
    </div>
    <PasskeyAuth />
    <p class="text-center text-xs opacity-60">
      Sign in uses a passkey saved on this device (Face ID, Touch ID, Windows Hello or your password manager).
    </p>
  </div>
{:else}
  <Shell title="Groups">
    {#snippet actions()}
      <button class="btn btn-primary btn-sm" onclick={() => dialog?.showModal()}>New group</button>
    {/snippet}

    <div class="flex flex-col gap-4">
      <InstallHint />

      {#if store.groupList.length > 0 && currencies.size === 1}
        <div class="stats border border-base-300 bg-base-100">
          <div class="stat">
            <div class="stat-title">Overall</div>
            <div class="stat-value text-2xl">
              <Money amount={total} currency={[...currencies][0]} colored />
            </div>
            <div class="stat-desc">{total > 0 ? 'you are owed' : total < 0 ? 'you owe' : 'all settled up'}</div>
          </div>
        </div>
      {/if}

      {#if store.groupList.length === 0}
        <EmptyState title="No groups yet" body="Create a group for a trip, a flat, or a dinner, then invite your friends with a link.">
          <button class="btn btn-primary" onclick={() => dialog?.showModal()}>Create your first group</button>
        </EmptyState>
      {:else}
        <ul class="list rounded-box border border-base-300 bg-base-100">
          {#each store.groupList as g (g.id)}
            <li class="list-row items-center">
              <Avatar name={g.name} />
              <a href={`/groups/${g.id}`} class="min-w-0">
                <div class="truncate font-medium">{g.name}</div>
                <div class="text-xs opacity-60">
                  {g.members.length} {g.members.length === 1 ? 'member' : 'members'} · {g.currency}
                </div>
              </a>
              <a href={`/groups/${g.id}`} class="text-right">
                {#if g.my_balance === 0}
                  <div class="text-xs opacity-60">settled up</div>
                {:else}
                  <div class="text-xs opacity-60">{g.my_balance > 0 ? 'you are owed' : 'you owe'}</div>
                  <Money amount={g.my_balance} currency={g.currency} colored class="font-semibold" />
                {/if}
              </a>
            </li>
          {/each}
        </ul>
      {/if}
    </div>

    <dialog bind:this={dialog} class="modal modal-bottom sm:modal-middle">
      <div class="modal-box">
        <h3 class="text-lg font-bold">New group</h3>
        <form
          class="mt-2 flex flex-col gap-2"
          onsubmit={(e) => {
            e.preventDefault();
            void create();
          }}
        >
          <fieldset class="fieldset">
            <legend class="fieldset-legend">Name</legend>
            <input class="input w-full" placeholder="Lisbon trip" bind:value={name} maxlength="80" required />
          </fieldset>
          <fieldset class="fieldset">
            <legend class="fieldset-legend">Currency</legend>
            <select class="select w-full" bind:value={currency}>
              {#each CURRENCIES as c}<option value={c}>{c}</option>{/each}
            </select>
            <p class="label">All expenses in this group use one currency.</p>
          </fieldset>
          <div class="modal-action">
            <button type="button" class="btn" onclick={() => dialog?.close()}>Cancel</button>
            <button type="submit" class="btn btn-primary" disabled={busy || !name.trim()}>
              {#if busy}<span class="loading loading-spinner loading-sm"></span>{/if}
              Create
            </button>
          </div>
        </form>
      </div>
      <form method="dialog" class="modal-backdrop"><button>close</button></form>
    </dialog>
  </Shell>
{/if}
