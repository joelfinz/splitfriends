<script lang="ts">
  import { describeEvent, store } from '../lib/store.svelte';
  import type { GroupEvent } from '../lib/types';
  import Avatar from './Avatar.svelte';
  import EmptyState from './EmptyState.svelte';

  let { events }: { events: GroupEvent[] } = $props();

  function when(iso: string): string {
    const d = new Date(iso);
    const now = new Date();
    const sameDay = d.toDateString() === now.toDateString();
    return sameDay
      ? d.toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' })
      : d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
  }
  function href(ev: GroupEvent): string {
    return store.groups[ev.group_id] ? `/groups/${ev.group_id}` : '/';
  }
</script>

{#if events.length === 0}
  <EmptyState title="No activity yet" body="Expenses, payments and group changes show up here as they happen." />
{:else}
  <ul class="list rounded-box bg-base-100 border border-base-300">
    {#each events as ev (ev.id)}
      <li class="list-row items-center">
        <Avatar name={ev.actor_name || '?'} size="sm" />
        <a href={href(ev)} class="min-w-0">
          <div class="text-sm">{describeEvent(ev, store.groups[ev.group_id])}</div>
          <div class="text-xs opacity-60">{when(ev.created_at)}</div>
        </a>
      </li>
    {/each}
  </ul>
{/if}
