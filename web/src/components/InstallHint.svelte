<script lang="ts">
  import { isIOS, isStandalone } from '../lib/push';
  const KEY = 'fs_install_hint_dismissed';
  let dismissed = $state(false);
  try {
    dismissed = localStorage.getItem(KEY) === '1';
  } catch {
    dismissed = false;
  }
  const show = $derived(!dismissed && isIOS() && !isStandalone());
  function hide() {
    dismissed = true;
    try {
      localStorage.setItem(KEY, '1');
    } catch {
      /* ignore */
    }
  }
</script>

{#if show}
  <div role="alert" class="alert alert-info alert-soft text-sm">
    <span>
      <strong>Install Fairshare</strong> for notifications on iPhone: tap the Share button, then
      <strong>Add to Home Screen</strong>.
    </span>
    <button class="btn btn-ghost btn-xs" onclick={hide}>Got it</button>
  </div>
{/if}
