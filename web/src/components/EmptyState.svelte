<script lang="ts">
  import type { Snippet } from 'svelte';
  let { title, body, children }: { title: string; body?: string; children?: Snippet } = $props();
</script>

<div class="card card-border bg-base-100">
  <div class="card-body items-center text-center">
    <h3 class="card-title">{title}</h3>
    {#if body}<p class="opacity-70">{body}</p>{/if}
    {#if children}
      <div class="card-actions mt-2">{@render children()}</div>
    {/if}
  </div>
</div>
