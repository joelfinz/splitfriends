export type ToastKind = 'info' | 'success' | 'error';
export type ToastAction = { label: string; onClick: () => void | Promise<void> };
export type Toast = { id: number; text: string; kind: ToastKind; action?: ToastAction };

let items = $state<Toast[]>([]);
let seq = 0;
const timers = new Map<number, ReturnType<typeof setTimeout>>();

export const toasts = {
  get items() {
    return items;
  },
};

/** Show a toast. With an action the default lifetime is 7 s so the user has time to react. */
export function toast(text: string, kind: ToastKind = 'info', ms?: number, action?: ToastAction): number {
  const id = ++seq;
  items = [...items, { id, text, kind, action }];
  timers.set(id, setTimeout(() => dismiss(id), ms ?? (action ? 7000 : 3500)));
  return id;
}

/** Convenience for "did X — Undo" toasts. */
export function actionToast(text: string, label: string, onClick: () => void | Promise<void>, kind: ToastKind = 'success'): number {
  return toast(text, kind, undefined, { label, onClick });
}

export function dismiss(id: number): void {
  const t = timers.get(id);
  if (t) clearTimeout(t);
  timers.delete(id);
  items = items.filter((t) => t.id !== id);
}

export async function runAction(t: Toast): Promise<void> {
  dismiss(t.id);
  try {
    await t.action?.onClick();
  } catch (e) {
    errorToast(e);
  }
}

export function errorToast(e: unknown): void {
  const msg = (e as { message?: string })?.message ?? String(e);
  toast(msg, 'error', 5000);
}
