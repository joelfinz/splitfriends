const ZERO_DECIMALS = new Set(['JPY', 'KRW', 'VND', 'CLP', 'ISK', 'HUF', 'TWD', 'UGX', 'XAF', 'XOF', 'PYG', 'RWF']);
const THREE_DECIMALS = new Set(['BHD', 'KWD', 'OMR', 'JOD', 'IQD', 'LYD', 'TND']);

export function decimals(currency: string): number {
  const c = currency.toUpperCase();
  if (ZERO_DECIMALS.has(c)) return 0;
  if (THREE_DECIMALS.has(c)) return 3;
  return 2;
}

/** Minor units -> formatted string with currency symbol. */
export function fmt(minor: number, currency: string, opts: { sign?: boolean } = {}): string {
  const d = decimals(currency);
  const major = minor / Math.pow(10, d);
  try {
    const s = new Intl.NumberFormat(undefined, {
      style: 'currency',
      currency,
      minimumFractionDigits: d,
      maximumFractionDigits: d,
      signDisplay: opts.sign ? 'always' : 'auto',
    }).format(major);
    return s;
  } catch {
    return `${major.toFixed(d)} ${currency}`;
  }
}

/** Absolute value formatted. */
export function fmtAbs(minor: number, currency: string): string {
  return fmt(Math.abs(minor), currency);
}

/** Minor units -> plain decimal string for inputs (no symbol). */
export function toMajor(minor: number, currency: string): string {
  const d = decimals(currency);
  if (d === 0) return String(minor);
  return (minor / Math.pow(10, d)).toFixed(d);
}

/** Decimal string from an input -> minor units. Returns NaN if unparseable. */
export function toMinor(input: string | number, currency: string): number {
  const s = String(input).trim().replace(/,/g, '');
  if (s === '' || s === '-' || s === '.') return NaN;
  if (!/^-?\d*(\.\d*)?$/.test(s)) return NaN;
  const d = decimals(currency);
  const neg = s.startsWith('-');
  const [intPart, fracPart = ''] = s.replace('-', '').split('.');
  const frac = (fracPart + '0'.repeat(d)).slice(0, d);
  const v = parseInt(intPart || '0', 10) * Math.pow(10, d) + (d > 0 ? parseInt(frac || '0', 10) : 0);
  return neg ? -v : v;
}

/** Step attribute for a number input in this currency. */
export function step(currency: string): string {
  const d = decimals(currency);
  return d === 0 ? '1' : `0.${'0'.repeat(d - 1)}1`;
}

export const CURRENCIES = [
  'AED', 'AUD', 'BHD', 'CAD', 'CHF', 'CNY', 'DKK', 'EUR', 'GBP', 'HKD', 'IDR', 'INR', 'JPY', 'KRW', 'KWD', 'LKR',
  'MYR', 'NOK', 'NZD', 'OMR', 'PHP', 'PKR', 'QAR', 'SAR', 'SEK', 'SGD', 'THB', 'TRY', 'USD', 'VND', 'ZAR',
];

/** Split `total` minor units equally among n, distributing remainder cents to the first entries. */
export function splitEqual(total: number, n: number): number[] {
  if (n <= 0) return [];
  const base = Math.floor(total / n);
  let rem = total - base * n;
  const out: number[] = [];
  for (let i = 0; i < n; i++) {
    out.push(base + (rem > 0 ? 1 : 0));
    if (rem > 0) rem--;
  }
  return out;
}

/** Split by weights (shares or basis points), largest-remainder rounding. */
export function splitByWeights(total: number, weights: number[]): number[] {
  const sum = weights.reduce((a, b) => a + b, 0);
  if (sum <= 0) return weights.map(() => 0);
  const raw = weights.map((w) => (total * w) / sum);
  const floored = raw.map(Math.floor);
  let rem = total - floored.reduce((a, b) => a + b, 0);
  const order = raw
    .map((r, i) => ({ i, frac: r - Math.floor(r) }))
    .sort((a, b) => b.frac - a.frac);
  for (const { i } of order) {
    if (rem <= 0) break;
    floored[i] += 1;
    rem--;
  }
  return floored;
}
