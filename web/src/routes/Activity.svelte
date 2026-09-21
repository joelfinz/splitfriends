<script lang="ts">
  import { loadActivity, store } from '../lib/store.svelte';
  import Shell from '../components/Shell.svelte';
  import ActivityList from '../components/ActivityList.svelte';

  $effect(() => {
    loadActivity().catch(() => {});
  });
</script>

<Shell title="Activity">
  <ActivityList events={store.activity} />
</Shell>
