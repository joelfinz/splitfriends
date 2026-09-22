<script lang="ts">
  import { toasts, dismiss, runAction } from '../lib/toast.svelte';
</script>

{#if toasts.items.length}
  <div class="toast toast-top toast-center z-50 mt-[env(safe-area-inset-top)] w-full max-w-sm px-4">
    {#each toasts.items as t (t.id)}
      <div
        role="alert"
        class="alert alert-soft shadow {t.kind === 'error' ? 'alert-error' : t.kind === 'success' ? 'alert-success' : ''}"
      >
        <span class="min-w-0 flex-1 truncate text-sm">{t.text}</span>
        {#if t.action}
          <button class="btn btn-sm btn-soft" onclick={() => void runAction(t)}>{t.action.label}</button>
        {/if}
        <button class="btn btn-ghost btn-xs" aria-label="Dismiss" onclick={() => dismiss(t.id)}>✕</button>
      </div>
    {/each}
  </div>
{/if}
