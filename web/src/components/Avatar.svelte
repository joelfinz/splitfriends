<script lang="ts">
  let { name, size = 'md' }: { name: string; size?: 'sm' | 'md' | 'lg' } = $props();
  const initials = $derived(
    name
      .trim()
      .split(/\s+/)
      .slice(0, 2)
      .map((w) => w[0]?.toUpperCase() ?? '')
      .join('') || '?',
  );
  const cls = $derived(size === 'sm' ? 'w-7 text-xs' : size === 'lg' ? 'w-14 text-xl' : 'w-10 text-sm');
  // Stable hue per name so members are distinguishable.
  const hue = $derived([...name].reduce((h, c) => (h * 31 + c.charCodeAt(0)) % 360, 7));
</script>

<div class="avatar avatar-placeholder">
  <div class="rounded-full {cls} text-neutral-content" style="background: oklch(55% 0.12 {hue})">
    <span>{initials}</span>
  </div>
</div>
