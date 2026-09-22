// Mirrors docs/API.md. All amounts are integers in minor units.
export type User = { id: string; name: string; created_at: string };
export type Member = { user_id: string; name: string; joined_at: string };
export type Group = {
  id: string;
  name: string;
  currency: string;
  created_by: string;
  created_at: string;
  members: Member[];
  my_balance: number;
  last_seq: number;
};
export type Payer = { user_id: string; amount: number };
export type SplitType = 'equal' | 'exact' | 'percent' | 'shares';
export type ShareInput = { user_id: string; value: number };
export type Share = { user_id: string; value: number; amount: number };
export type Category =
  | 'food'
  | 'groceries'
  | 'drinks'
  | 'transport'
  | 'accommodation'
  | 'entertainment'
  | 'shopping'
  | 'utilities'
  | 'health'
  | 'travel'
  | 'gifts'
  | 'other';
export type Expense = {
  id: string;
  group_id: string;
  description: string;
  amount: number;
  date: string;
  split_type: SplitType;
  category: Category;
  notes: string;
  payers: Payer[];
  shares: Share[];
  created_by: string;
  created_at: string;
  updated_at: string;
  /** Only present in the trash listing. */
  deleted_at?: string;
};
export type Payment = {
  id: string;
  group_id: string;
  from_user_id: string;
  to_user_id: string;
  amount: number;
  date: string;
  notes: string;
  created_by: string;
  created_at: string;
  /** Only present in the trash listing. */
  deleted_at?: string;
};
export type Trash = { expenses: Expense[]; payments: Payment[] };
export type Balance = { user_id: string; net: number };
export type Debt = { from_user_id: string; to_user_id: string; amount: number };
export type GroupDetail = {
  group: Group;
  expenses: Expense[];
  payments: Payment[];
  balances: Balance[];
  pairwise: Debt[];
  simplified: Debt[];
};
export type EventType =
  | 'group.created'
  | 'group.updated'
  | 'member.joined'
  | 'member.left'
  | 'expense.created'
  | 'expense.updated'
  | 'expense.deleted'
  | 'expense.restored'
  | 'payment.created'
  | 'payment.deleted'
  | 'payment.restored';
export type GroupEvent = {
  id: number;
  group_id: string;
  seq: number;
  type: EventType;
  actor_id: string;
  actor_name: string;
  payload: any;
  created_at: string;
};
export type ExpenseInput = {
  description: string;
  amount: number;
  date: string;
  notes?: string;
  payers: Payer[];
  split_type: SplitType;
  category?: Category;
  shares: ShareInput[];
};
export type PaymentInput = {
  from_user_id: string;
  to_user_id: string;
  amount: number;
  date: string;
  notes: string;
};
export type Invite = { token: string; url: string; expires_at: string };
export type InvitePreview = {
  group_id: string;
  group_name: string;
  currency: string;
  inviter_name: string;
  member_count: number;
  already_member: boolean;
};
export type Passkey = { id: string; name: string; created_at: string; last_used_at: string | null };
