<script lang="ts">
  import Shell from '../components/Shell.svelte';
  import EmptyState from '../components/EmptyState.svelte';
</script>

<Shell title="Not found" back="/">
  <EmptyState title="Page not found" body="That link does not go anywhere.">
    <a href="/" class="btn btn-primary">Go home</a>
  </EmptyState>
</Shell>
