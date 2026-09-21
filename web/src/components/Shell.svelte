<script lang="ts">
  import type { Snippet } from 'svelte';
  import { router } from '../lib/router.svelte';
  import { store } from '../lib/store.svelte';

  let {
    title,
    back,
    actions,
    children,
    dock = true,
  }: { title: string; back?: string; actions?: Snippet; children: Snippet; dock?: boolean } = $props();

  const isHome = $derived(router.path === '/' || router.path.startsWith('/groups'));
  const isActivity = $derived(router.path === '/activity');
  const isSettings = $derived(router.path === '/settings');
</script>

<div class="mx-auto flex min-h-dvh w-full max-w-2xl flex-col">
  <header class="navbar safe-top sticky top-0 z-30 bg-base-100/90 backdrop-blur border-b border-base-300 min-h-14">
    <div class="navbar-start">
      {#if back}
        <a href={back} class="btn btn-ghost btn-circle" aria-label="Back">
          <svg xmlns="http://www.w3.org/2000/svg" class="size-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M15 19l-7-7 7-7" /></svg>
        </a>
      {/if}
      <h1 class="truncate px-2 text-lg font-semibold">{title}</h1>
    </div>
    <div class="navbar-end gap-1">
      {#if !store.online && store.me}
        <span class="status status-warning" title="Reconnecting…"></span>
      {/if}
      {#if actions}{@render actions()}{/if}
    </div>
  </header>

  <main class="flex-1 px-4 pb-24 pt-4">
    {@render children()}
  </main>

  {#if dock && store.me}
    <nav class="dock dock-md bg-base-200 border-t border-base-300 max-w-2xl mx-auto">
      <a href="/" class={isHome ? 'dock-active' : ''} aria-label="Groups">
        <svg xmlns="http://www.w3.org/2000/svg" class="size-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M17 20h5v-2a3 3 0 00-5.356-1.857M17 20H7m10 0v-2c0-.656-.126-1.283-.356-1.857M7 20H2v-2a3 3 0 015.356-1.857M7 20v-2c0-.656.126-1.283.356-1.857m0 0a5.002 5.002 0 019.288 0M15 7a3 3 0 11-6 0 3 3 0 016 0zm6 3a2 2 0 11-4 0 2 2 0 014 0zM7 10a2 2 0 11-4 0 2 2 0 014 0z" /></svg>
        <span class="dock-label">Groups</span>
      </a>
      <a href="/activity" class={isActivity ? 'dock-active' : ''} aria-label="Activity">
        <svg xmlns="http://www.w3.org/2000/svg" class="size-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M13 10V3L4 14h7v7l9-11h-7z" /></svg>
        <span class="dock-label">Activity</span>
      </a>
      <a href="/settings" class={isSettings ? 'dock-active' : ''} aria-label="Settings">
        <svg xmlns="http://www.w3.org/2000/svg" class="size-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" /><path stroke-linecap="round" stroke-linejoin="round" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" /></svg>
        <span class="dock-label">Settings</span>
      </a>
    </nav>
  {/if}
</div>
