<script lang="ts">
  import { toasts, dismiss } from '../lib/toast.svelte';
</script>

{#if toasts.items.length}
  <div class="toast toast-top toast-center z-50 mt-[env(safe-area-inset-top)] w-full max-w-sm px-4">
    {#each toasts.items as t (t.id)}
      <div
        role="alert"
        class="alert alert-soft shadow {t.kind === 'error' ? 'alert-error' : t.kind === 'success' ? 'alert-success' : ''}"
      >
        <span class="text-sm">{t.text}</span>
        <button class="btn btn-ghost btn-xs" aria-label="Dismiss" onclick={() => dismiss(t.id)}>✕</button>
      </div>
    {/each}
  </div>
{/if}
