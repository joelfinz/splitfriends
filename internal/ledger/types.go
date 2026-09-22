// Package ledger holds the domain types and the pure money math: computing
// split shares, per-member net balances, pairwise debts and simplified debts.
// It has no I/O so it can be tested exhaustively.
package ledger

type User struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

type Member struct {
	UserID   string `json:"user_id"`
	Name     string `json:"name"`
	JoinedAt string `json:"joined_at"`
}

type Group struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Currency  string   `json:"currency"`
	CreatedBy string   `json:"created_by"`
	CreatedAt string   `json:"created_at"`
	Members   []Member `json:"members"`
	MyBalance int64    `json:"my_balance"`
	LastSeq   int64    `json:"last_seq"`
}

type Payer struct {
	UserID string `json:"user_id"`
	Amount int64  `json:"amount"`
}

type SplitType string

const (
	SplitEqual   SplitType = "equal"
	SplitExact   SplitType = "exact"
	SplitPercent SplitType = "percent"
	SplitShares  SplitType = "shares"
)

type ShareInput struct {
	UserID string `json:"user_id"`
	Value  int64  `json:"value"`
}

type Share struct {
	UserID string `json:"user_id"`
	Value  int64  `json:"value"`
	Amount int64  `json:"amount"`
}

type Expense struct {
	ID          string    `json:"id"`
	GroupID     string    `json:"group_id"`
	Description string    `json:"description"`
	Amount      int64     `json:"amount"`
	Date        string    `json:"date"`
	SplitType   SplitType `json:"split_type"`
	Category    Category  `json:"category"`
	Notes       string    `json:"notes"`
	Payers      []Payer   `json:"payers"`
	Shares      []Share   `json:"shares"`
	CreatedBy   string    `json:"created_by"`
	CreatedAt   string    `json:"created_at"`
	UpdatedAt   string    `json:"updated_at"`
	DeletedAt   string    `json:"deleted_at,omitempty"`
}

type Payment struct {
	ID         string `json:"id"`
	GroupID    string `json:"group_id"`
	FromUserID string `json:"from_user_id"`
	ToUserID   string `json:"to_user_id"`
	Amount     int64  `json:"amount"`
	Date       string `json:"date"`
	Notes      string `json:"notes"`
	CreatedBy  string `json:"created_by"`
	CreatedAt  string `json:"created_at"`
	DeletedAt  string `json:"deleted_at,omitempty"`
}

type Balance struct {
	UserID string `json:"user_id"`
	Net    int64  `json:"net"`
}

type Debt struct {
	FromUserID string `json:"from_user_id"`
	ToUserID   string `json:"to_user_id"`
	Amount     int64  `json:"amount"`
}

type ExpenseInput struct {
	Description string       `json:"description"`
	Amount      int64        `json:"amount"`
	Date        string       `json:"date"`
	Notes       string       `json:"notes"`
	Payers      []Payer      `json:"payers"`
	SplitType   SplitType    `json:"split_type"`
	Category    Category     `json:"category"`
	Shares      []ShareInput `json:"shares"`
}

// Category is a fixed vocabulary so charts stay comparable across groups.
type Category string

var Categories = []Category{"food", "groceries", "drinks", "transport", "accommodation", "entertainment",
	"shopping", "utilities", "health", "travel", "gifts", "other"}

const CategoryOther Category = "other"

func ValidCategory(c Category) bool {
	for _, k := range Categories {
		if k == c {
			return true
		}
	}
	return false
}

// Event is one entry in a group's append-only log. ID is global and
// monotonic (used as the SSE id); Seq is per group.
type Event struct {
	ID        int64  `json:"id"`
	GroupID   string `json:"group_id"`
	Seq       int64  `json:"seq"`
	Type      string `json:"type"`
	ActorID   string `json:"actor_id"`
	ActorName string `json:"actor_name"`
	Payload   any    `json:"payload"`
	CreatedAt string `json:"created_at"`
}

const (
	EvGroupCreated    = "group.created"
	EvGroupUpdated    = "group.updated"
	EvMemberJoined    = "member.joined"
	EvMemberLeft      = "member.left"
	EvExpenseCreated  = "expense.created"
	EvExpenseUpdated  = "expense.updated"
	EvExpenseDeleted  = "expense.deleted"
	EvPaymentCreated  = "payment.created"
	EvPaymentDeleted  = "payment.deleted"
	EvExpenseRestored = "expense.restored"
	EvPaymentRestored = "payment.restored"
)
