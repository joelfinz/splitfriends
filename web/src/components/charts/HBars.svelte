<script lang="ts">
  /**
   * Horizontal bar list, 1–2 series per row. Labels are HTML (they never clip),
   * bars are inline SVG. Bar length carries the value; one hue per series.
   */
  export type Row = { key: string; label: string; emoji?: string; values: number[]; note?: string };
  export type Series = { name: string; color: string };

  let {
    rows,
    series,
    format,
    showPct = false,
  }: {
    rows: Row[];
    series: Series[];
    format: (v: number) => string;
    showPct?: boolean;
  } = $props();

  const max = $derived(Math.max(1, ...rows.flatMap((r) => r.values)));
  const total = $derived(rows.reduce((s, r) => s + (r.values[0] ?? 0), 0));
  const barH = 10;
  const gap = 2;
  const svgH = $derived(series.length * barH + (series.length - 1) * gap);
  const pct = (v: number) => `${Math.max(0, Math.min(100, (v / max) * 100))}%`;
</script>

<ul class="flex flex-col gap-2">
  {#each rows as r (r.key)}
    <li class="grid grid-cols-[minmax(0,7rem)_1fr_auto] items-center gap-2">
      <div class="flex min-w-0 items-center gap-1.5 text-sm">
        {#if r.emoji}<span aria-hidden="true">{r.emoji}</span>{/if}
        <span class="truncate">{r.label}</span>
      </div>
      <svg width="100%" height={svgH} role="img" aria-label={`${r.label}: ${r.values.map((v, k) => `${series[k]?.name ?? ''} ${format(v)}`).join(', ')}`} class="block">
        {#each r.values as v, k (k)}
          {@const y = k * (barH + gap)}
          <rect x="0" y={y} width="100%" height={barH} fill="var(--color-base-200)" rx="4" />
          {#if v > 0}
            <rect x="0" y={y} width={pct(v)} height={barH} fill={series[k]?.color ?? 'var(--color-primary)'} rx="4">
              <title>{r.label} · {series[k]?.name ?? ''} {format(v)}</title>
            </rect>
            <!-- square the baseline end -->
            <rect x="0" y={y} width="4" height={barH} fill={series[k]?.color ?? 'var(--color-primary)'} />
          {/if}
        {/each}
      </svg>
      <div class="text-right text-sm tabular leading-tight">
        {#if series.length === 1}
          <div class="font-medium">{format(r.values[0] ?? 0)}</div>
          {#if showPct && total > 0}<div class="text-xs opacity-60">{Math.round(((r.values[0] ?? 0) / total) * 100)}%</div>{/if}
        {:else}
          <div class="text-xs opacity-70">{r.values.map((v) => format(v)).join(' · ')}</div>
          {#if r.note}<div class="text-xs font-medium">{r.note}</div>{/if}
        {/if}
      </div>
    </li>
  {/each}
</ul>
