<script lang="ts">
  import * as api from '../lib/api';
  import { navigate } from '../lib/router.svelte';
  import { logout, store, updateMeLocal } from '../lib/store.svelte';
  import { applyTheme, getThemePref, type ThemePref } from '../lib/theme';
  import { currentSubscription, disablePush, enablePush, isIOS, isStandalone, pushSupported } from '../lib/push';
  import { addPasskeyBegin, addPasskeyFinish } from '../lib/api';
  import { createCredential, describeWebAuthnError } from '../lib/webauthn';
  import { errorToast, toast } from '../lib/toast.svelte';
  import type { Passkey } from '../lib/types';
  import Shell from '../components/Shell.svelte';
  import InstallHint from '../components/InstallHint.svelte';

  let name = $state(store.me?.name ?? '');
  let nameBusy = $state(false);
  async function saveName() {
    const n = name.trim();
    if (!n || n === store.me?.name) return;
    nameBusy = true;
    try {
      updateMeLocal(await api.updateMe(n));
      toast('Name updated', 'success');
    } catch (e) {
      errorToast(e);
    } finally {
      nameBusy = false;
    }
  }

  // ---- passkeys
  let passkeys = $state<Passkey[] | null>(null);
  let pkBusy = $state(false);
  let newKeyName = $state('');
  async function loadPasskeys() {
    try {
      passkeys = await api.listPasskeys();
    } catch (e) {
      errorToast(e);
    }
  }
  $effect(() => {
    void loadPasskeys();
  });
  async function addPasskey() {
    pkBusy = true;
    try {
      const label = newKeyName.trim() || defaultKeyName();
      const opts = await addPasskeyBegin(label);
      const cred = await createCredential(opts);
      await addPasskeyFinish(cred);
      newKeyName = '';
      toast('Passkey added', 'success');
      await loadPasskeys();
    } catch (e) {
      toast(e instanceof api.ApiError ? e.message : describeWebAuthnError(e), 'error');
    } finally {
      pkBusy = false;
    }
  }
  async function removePasskey(id: string) {
    if (!confirm('Remove this passkey? You will no longer be able to sign in with it.')) return;
    try {
      await api.deletePasskey(id);
      await loadPasskeys();
    } catch (e) {
      errorToast(e);
    }
  }
  function defaultKeyName(): string {
    const ua = navigator.userAgent;
    if (/iPhone/.test(ua)) return 'iPhone';
    if (/iPad/.test(ua)) return 'iPad';
    if (/Android/.test(ua)) return 'Android';
    if (/Mac/.test(ua)) return 'Mac';
    if (/Windows/.test(ua)) return 'Windows';
    return 'This device';
  }

  // ---- push
  const supported = pushSupported();
  let pushOn = $state(false);
  let pushBusy = $state(false);
  $effect(() => {
    currentSubscription().then((s) => (pushOn = !!s)).catch(() => {});
  });
  async function togglePush(e: Event) {
    const want = (e.currentTarget as HTMLInputElement).checked;
    pushBusy = true;
    try {
      if (want) await enablePush();
      else await disablePush();
      pushOn = want;
      toast(want ? 'Notifications on' : 'Notifications off', 'success');
    } catch (err) {
      pushOn = !want;
      errorToast(err);
    } finally {
      pushBusy = false;
    }
  }
  const needsInstall = $derived(isIOS() && !isStandalone());

  // ---- theme
  let theme = $state<ThemePref>(getThemePref());
  function setTheme(t: ThemePref) {
    theme = t;
    applyTheme(t);
  }

  async function doLogout() {
    await logout();
    navigate('/', { replace: true });
  }
</script>

<Shell title="Settings">
  <div class="flex flex-col gap-4">
    <form
      class="card card-border bg-base-100"
      onsubmit={(e) => {
        e.preventDefault();
        void saveName();
      }}
    >
      <div class="card-body gap-2">
        <h3 class="card-title text-base">Profile</h3>
        <fieldset class="fieldset">
          <legend class="fieldset-legend">Display name</legend>
          <div class="join w-full">
            <input class="input join-item w-full" bind:value={name} maxlength="60" required />
            <button type="submit" class="btn join-item" disabled={nameBusy || !name.trim() || name.trim() === store.me?.name}>Save</button>
          </div>
        </fieldset>
      </div>
    </form>

    <div class="card card-border bg-base-100">
      <div class="card-body gap-2">
        <h3 class="card-title text-base">Notifications</h3>
        {#if !supported}
          <p class="text-sm opacity-70">This browser does not support push notifications.</p>
        {:else}
          <label class="label cursor-pointer justify-between">
            <span class="text-sm">Push notifications for new expenses and payments</span>
            <input type="checkbox" class="toggle toggle-primary" checked={pushOn} disabled={pushBusy || needsInstall} onchange={togglePush} />
          </label>
        {/if}
        {#if needsInstall}
          <InstallHint />
        {/if}
      </div>
    </div>

    <div class="card card-border bg-base-100">
      <div class="card-body gap-2">
        <h3 class="card-title text-base">Passkeys</h3>
        <p class="text-sm opacity-70">Each device you sign in from needs a passkey. Add one here before switching devices.</p>
        {#if passkeys === null}
          <span class="loading loading-spinner loading-sm"></span>
        {:else}
          <ul class="list">
            {#each passkeys as k (k.id)}
              <li class="list-row items-center px-0 py-2">
                <div class="text-sm font-medium">{k.name || 'Passkey'}</div>
                <div class="text-xs opacity-60">
                  added {new Date(k.created_at).toLocaleDateString()}
                  {#if k.last_used_at} · last used {new Date(k.last_used_at).toLocaleDateString()}{/if}
                </div>
                <button class="btn btn-ghost btn-xs" disabled={passkeys.length <= 1} onclick={() => removePasskey(k.id)}>Remove</button>
              </li>
            {/each}
          </ul>
        {/if}
        <div class="join w-full">
          <input class="input join-item w-full" placeholder={defaultKeyName()} bind:value={newKeyName} maxlength="40" />
          <button class="btn join-item btn-primary" disabled={pkBusy} onclick={addPasskey}>
            {#if pkBusy}<span class="loading loading-spinner loading-sm"></span>{/if}
            Add passkey
          </button>
        </div>
      </div>
    </div>

    <div class="card card-border bg-base-100">
      <div class="card-body gap-2">
        <h3 class="card-title text-base">Appearance</h3>
        <div role="tablist" class="tabs tabs-box w-full">
          <button role="tab" class="tab flex-1 {theme === 'system' ? 'tab-active' : ''}" onclick={() => setTheme('system')}>System</button>
          <button role="tab" class="tab flex-1 {theme === 'light' ? 'tab-active' : ''}" onclick={() => setTheme('light')}>Light</button>
          <button role="tab" class="tab flex-1 {theme === 'dark' ? 'tab-active' : ''}" onclick={() => setTheme('dark')}>Dark</button>
        </div>
      </div>
    </div>

    <button class="btn btn-block" onclick={doLogout}>Sign out</button>

    <p class="text-center text-xs opacity-50">Fairshare {__APP_VERSION__} · built {new Date(__BUILD_TIME__).toLocaleDateString()}</p>
  </div>
</Shell>
