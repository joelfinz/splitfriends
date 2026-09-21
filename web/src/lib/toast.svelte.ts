export type ToastKind = 'info' | 'success' | 'error';
export type Toast = { id: number; text: string; kind: ToastKind };

let items = $state<Toast[]>([]);
let seq = 0;

export const toasts = {
  get items() {
    return items;
  },
};

export function toast(text: string, kind: ToastKind = 'info', ms = 3500): void {
  const id = ++seq;
  items = [...items, { id, text, kind }];
  setTimeout(() => dismiss(id), ms);
}

export function dismiss(id: number): void {
  items = items.filter((t) => t.id !== id);
}

export function errorToast(e: unknown): void {
  const msg = (e as { message?: string })?.message ?? String(e);
  toast(msg, 'error', 5000);
}
