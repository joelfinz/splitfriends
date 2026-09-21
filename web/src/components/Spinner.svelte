<script lang="ts">
  let { label = 'Loading…' }: { label?: string } = $props();
</script>

<div class="flex flex-col items-center justify-center gap-3 py-16 opacity-70">
  <span class="loading loading-spinner loading-lg"></span>
  <span class="text-sm">{label}</span>
</div>
