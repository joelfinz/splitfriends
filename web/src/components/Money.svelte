<script lang="ts">
  import { fmt } from '../lib/money';
  let {
    amount,
    currency,
    signed = false,
    colored = false,
    class: cls = '',
  }: { amount: number; currency: string; signed?: boolean; colored?: boolean; class?: string } = $props();
  const color = $derived(!colored ? '' : amount > 0 ? 'text-success' : amount < 0 ? 'text-error' : 'opacity-60');
</script>

<span class="tabular {color} {cls}">{fmt(signed ? amount : Math.abs(amount), currency, { sign: signed })}</span>
