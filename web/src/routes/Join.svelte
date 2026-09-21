<script lang="ts">
  import * as api from '../lib/api';
  import { navigate } from '../lib/router.svelte';
  import { store, upsertGroupLocal } from '../lib/store.svelte';
  import { errorToast } from '../lib/toast.svelte';
  import type { InvitePreview } from '../lib/types';
  import Shell from '../components/Shell.svelte';
  import Spinner from '../components/Spinner.svelte';
  import PasskeyAuth from '../components/PasskeyAuth.svelte';
  import Avatar from '../components/Avatar.svelte';

  let { token }: { token: string } = $props();

  let preview = $state<InvitePreview | null>(null);
  let error = $state('');
  let busy = $state(false);

  async function load() {
    error = '';
    preview = null;
    try {
      preview = await api.getInvite(token);
    } catch (e) {
      error = (e as Error).message || 'This invite is invalid or has expired.';
    }
  }

  $effect(() => {
    void store.me; // reload when auth state changes so already_member is accurate
    void load();
  });

  async function accept() {
    busy = true;
    try {
      const g = await api.acceptInvite(token);
      upsertGroupLocal(g);
      navigate(`/groups/${g.id}`, { replace: true });
    } catch (e) {
      errorToast(e);
    } finally {
      busy = false;
    }
  }
</script>

<Shell title="Join group" back="/" dock={false}>
  {#if error}
    <div role="alert" class="alert alert-error alert-soft"><span>{error}</span></div>
  {:else if !preview}
    <Spinner label="Checking invite…" />
  {:else}
    <div class="card card-border bg-base-100">
      <div class="card-body items-center text-center">
        <Avatar name={preview.group_name} size="lg" />
        <h2 class="card-title mt-2">{preview.group_name}</h2>
        <p class="opacity-70">
          {preview.inviter_name} invited you · {preview.member_count}
          {preview.member_count === 1 ? 'member' : 'members'} · {preview.currency}
        </p>

        {#if store.me}
          {#if preview.already_member}
            <a class="btn btn-primary mt-4" href={`/groups/${preview.group_id}`}>Open group</a>
          {:else}
            <button class="btn btn-primary btn-block mt-4" disabled={busy} onclick={accept}>
              {#if busy}<span class="loading loading-spinner loading-sm"></span>{/if}
              Join as {store.me.name}
            </button>
          {/if}
        {:else}
          <p class="mt-4 text-sm opacity-70">Sign in or create an account to join.</p>
          <div class="w-full"><PasskeyAuth compact /></div>
        {/if}
      </div>
    </div>
  {/if}
</Shell>
