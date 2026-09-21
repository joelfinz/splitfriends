<script lang="ts">
  import * as api from '../lib/api';
  import { setSession } from '../lib/store.svelte';
  import { createCredential, describeWebAuthnError, getAssertion, passkeysSupported } from '../lib/webauthn';
  import type { User } from '../lib/types';

  let { onDone, compact = false }: { onDone?: (u: User) => void; compact?: boolean } = $props();

  let mode = $state<'choose' | 'register'>('choose');
  let name = $state('');
  let busy = $state(false);
  let error = $state('');
  const supported = passkeysSupported();

  async function signIn() {
    busy = true;
    error = '';
    try {
      const opts = await api.loginBegin();
      const assertion = await getAssertion(opts);
      const u = await api.loginFinish(assertion);
      await setSession(u);
      onDone?.(u);
    } catch (e) {
      error = e instanceof api.ApiError ? e.message : describeWebAuthnError(e);
    } finally {
      busy = false;
    }
  }

  async function register() {
    const n = name.trim();
    if (n.length < 1) {
      error = 'Please enter your name.';
      return;
    }
    busy = true;
    error = '';
    try {
      const opts = await api.registerBegin(n);
      const cred = await createCredential(opts);
      const u = await api.registerFinish(cred);
      await setSession(u);
      onDone?.(u);
    } catch (e) {
      error = e instanceof api.ApiError ? e.message : describeWebAuthnError(e);
    } finally {
      busy = false;
    }
  }
</script>

<div class="flex flex-col gap-3">
  {#if !supported}
    <div role="alert" class="alert alert-warning alert-soft text-sm">
      This browser does not support passkeys. Try Safari, Chrome, or Edge on a recent OS.
    </div>
  {/if}

  {#if mode === 'choose'}
    <button class="btn btn-primary btn-block {compact ? '' : 'btn-lg'}" disabled={busy || !supported} onclick={signIn}>
      {#if busy}<span class="loading loading-spinner loading-sm"></span>{/if}
      Sign in with passkey
    </button>
    <button class="btn btn-block {compact ? '' : 'btn-lg'}" disabled={busy || !supported} onclick={() => (mode = 'register')}>
      Create account
    </button>
  {:else}
    <form
      class="flex flex-col gap-3"
      onsubmit={(e) => {
        e.preventDefault();
        void register();
      }}
    >
      <fieldset class="fieldset">
        <legend class="fieldset-legend">Your name</legend>
        <!-- svelte-ignore a11y_autofocus -->
        <input
          class="input input-lg w-full"
          placeholder="e.g. Sam"
          bind:value={name}
          autocomplete="name"
          autofocus
          maxlength="60"
          required
        />
        <p class="label">Friends see this name in shared groups.</p>
      </fieldset>
      <button class="btn btn-primary btn-block" type="submit" disabled={busy}>
        {#if busy}<span class="loading loading-spinner loading-sm"></span>{/if}
        Create passkey
      </button>
      <button class="btn btn-ghost btn-block" type="button" disabled={busy} onclick={() => (mode = 'choose')}>Back</button>
    </form>
  {/if}

  {#if error}
    <div role="alert" class="alert alert-error alert-soft text-sm"><span>{error}</span></div>
  {/if}
</div>
