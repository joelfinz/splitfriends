<script lang="ts">
  /**
   * Vertical column chart, 1–2 series, tap-to-select. Inline SVG sized from the
   * container width so text stays at real pixel size on phones.
   */
  export type Column = { key: string; label: string; values: number[] };
  export type Series = { name: string; color: string };

  let {
    columns,
    series,
    format,
    selectedKey = null,
    onSelect,
    height = 168,
  }: {
    columns: Column[];
    series: Series[];
    format: (v: number) => string;
    selectedKey?: string | null;
    onSelect?: (key: string) => void;
    height?: number;
  } = $props();

  let width = $state(320);
  const padL = 8;
  const padR = 8;
  const padT = 20; // room for the value label above the selected column
  const padB = 22; // x labels
  const plotH = $derived(height - padT - padB);
  const plotW = $derived(Math.max(0, width - padL - padR));
  const slot = $derived(columns.length ? plotW / columns.length : 0);
  const nSeries = $derived(Math.max(1, series.length));
  const gap = 2; // surface gap between adjacent bars
  const barW = $derived(Math.min(24, Math.max(4, (slot * 0.72 - gap * (nSeries - 1)) / nSeries)));
  const groupW = $derived(barW * nSeries + gap * (nSeries - 1));
  const max = $derived(Math.max(1, ...columns.flatMap((c) => c.values)));
  // Clean tick: round max up to 1/2/5 × 10^n and draw 2 gridlines.
  const niceMax = $derived.by(() => {
    const p = Math.pow(10, Math.floor(Math.log10(max)));
    const m = max / p;
    const n = m <= 1 ? 1 : m <= 2 ? 2 : m <= 5 ? 5 : 10;
    return n * p;
  });
  const y = (v: number) => padT + plotH - (v / niceMax) * plotH;
  const ticks = $derived([niceMax / 2, niceMax]);
  const selected = $derived(columns.find((c) => c.key === selectedKey) ?? null);

  function barPath(x: number, top: number, w: number, bottom: number): string {
    // Rounded 4px top, square at the baseline.
    const r = Math.min(4, w / 2, Math.max(0, bottom - top));
    return `M${x},${bottom} V${top + r} Q${x},${top} ${x + r},${top} H${x + w - r} Q${x + w},${top} ${x + w},${top + r} V${bottom} Z`;
  }
</script>

<div bind:clientWidth={width} class="w-full select-none">
  <svg {width} {height} viewBox="0 0 {width} {height}" role="img" aria-label="Column chart" class="block overflow-visible">
    <!-- gridlines + tick labels -->
    {#each ticks as t (t)}
      <line x1={padL} x2={width - padR} y1={y(t)} y2={y(t)} stroke="var(--color-base-300)" stroke-width="1" />
      <text x={width - padR} y={y(t) - 3} text-anchor="end" font-size="10" fill="currentColor" opacity="0.55">{format(t)}</text>
    {/each}
    <!-- baseline -->
    <line x1={padL} x2={width - padR} y1={y(0)} y2={y(0)} stroke="var(--color-base-content)" stroke-opacity="0.25" stroke-width="1" />

    {#each columns as c, i (c.key)}
      {@const x0 = padL + i * slot + (slot - groupW) / 2}
      {@const isSel = selectedKey === c.key}
      {@const dim = selectedKey !== null && !isSel}
      <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
      <g
        role={onSelect ? 'button' : undefined}
        tabindex={onSelect ? 0 : undefined}
        aria-label={`${c.label}: ${c.values.map((v, k) => `${series[k]?.name ?? ''} ${format(v)}`).join(', ')}`}
        class={onSelect ? 'cursor-pointer' : ''}
        onclick={() => onSelect?.(c.key)}
        onkeydown={(e) => {
          if (onSelect && (e.key === 'Enter' || e.key === ' ')) {
            e.preventDefault();
            onSelect(c.key);
          }
        }}
      >
        <!-- hit target: the whole slot -->
        <rect x={padL + i * slot} y={padT} width={slot} height={plotH + padB} fill="transparent" />
        {#each c.values as v, k (k)}
          {@const x = x0 + k * (barW + gap)}
          {#if v > 0}
            <path d={barPath(x, y(v), barW, y(0))} fill={series[k]?.color ?? 'var(--color-primary)'} opacity={dim ? 0.4 : 1}>
              <title>{c.label} · {series[k]?.name ?? ''} {format(v)}</title>
            </path>
          {/if}
        {/each}
        <text x={padL + i * slot + slot / 2} y={height - 6} text-anchor="middle" font-size="10" fill="currentColor" opacity={isSel ? 0.9 : 0.55} font-weight={isSel ? 600 : 400}>{c.label}</text>
      </g>
    {/each}

    <!-- selective label: only the selected column's total -->
    {#if selected && selected.values.some((v) => v > 0)}
      {@const i = columns.indexOf(selected)}
      {@const total = selected.values.reduce((a, b) => a + b, 0)}
      {@const top = Math.min(...selected.values.filter((v) => v > 0).map(y), y(0))}
      <text x={padL + i * slot + slot / 2} y={top - 6} text-anchor="middle" font-size="11" font-weight="600" fill="currentColor" class="tabular">{format(total)}</text>
    {/if}
  </svg>
</div>
