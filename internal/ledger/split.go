package ledger

import (
	"errors"
	"sort"
)

var (
	ErrInvalidAmount  = errors.New("invalid_amount")
	ErrPayersMismatch = errors.New("payers_mismatch")
	ErrSharesMismatch = errors.New("shares_mismatch")
	ErrNoParticipants = errors.New("no_participants")
)

// ComputeShares turns the user's split input into concrete owed amounts that
// sum exactly to total. Rounding remainders go to the earliest participants
// (largest-remainder method), which is deterministic and fair over time.
func ComputeShares(total int64, split SplitType, in []ShareInput) ([]Share, error) {
	if total <= 0 {
		return nil, ErrInvalidAmount
	}
	if len(in) == 0 {
		return nil, ErrNoParticipants
	}
	seen := map[string]bool{}
	for _, s := range in {
		if s.UserID == "" || seen[s.UserID] {
			return nil, ErrSharesMismatch
		}
		seen[s.UserID] = true
	}
	out := make([]Share, len(in))
	for i, s := range in {
		out[i] = Share{UserID: s.UserID, Value: s.Value}
	}

	switch split {
	case SplitEqual:
		weights := make([]int64, len(in))
		for i := range weights {
			weights[i] = 1
			out[i].Value = 0
		}
		allocate(total, weights, out)
	case SplitExact:
		var sum int64
		for i, s := range in {
			if s.Value < 0 {
				return nil, ErrSharesMismatch
			}
			out[i].Amount = s.Value
			sum += s.Value
		}
		if sum != total {
			return nil, ErrSharesMismatch
		}
	case SplitPercent:
		var sum int64
		weights := make([]int64, len(in))
		for i, s := range in {
			if s.Value < 0 {
				return nil, ErrSharesMismatch
			}
			weights[i] = s.Value
			sum += s.Value
		}
		if sum != 10000 {
			return nil, ErrSharesMismatch
		}
		allocate(total, weights, out)
	case SplitShares:
		var sum int64
		weights := make([]int64, len(in))
		for i, s := range in {
			if s.Value <= 0 {
				return nil, ErrSharesMismatch
			}
			weights[i] = s.Value
			sum += s.Value
		}
		if sum == 0 {
			return nil, ErrSharesMismatch
		}
		allocate(total, weights, out)
	default:
		return nil, ErrSharesMismatch
	}
	return out, nil
}

// allocate distributes total across out proportionally to weights using the
// largest-remainder method so the parts sum exactly to total.
func allocate(total int64, weights []int64, out []Share) { allocateRotated(total, weights, out, 0) }

// allocateRotated is allocate with the remainder cents starting at position
// rot (mod n) so repeated allocations spread rounding across recipients.
func allocateRotated(total int64, weights []int64, out []Share, rot int) {
	var wsum int64
	for _, w := range weights {
		wsum += w
	}
	type rem struct {
		i int
		r int64
	}
	rems := make([]rem, len(weights))
	var allocated int64
	for i, w := range weights {
		num := total * w
		q := num / wsum
		out[i].Amount = q
		allocated += q
		rems[i] = rem{i, num % wsum}
	}
	// Stable sort by remainder desc; ties keep input order.
	sort.SliceStable(rems, func(a, b int) bool { return rems[a].r > rems[b].r })
	n := int64(len(rems))
	for k := int64(0); k < total-allocated; k++ {
		out[rems[(k+int64(rot))%n].i].Amount++
	}
}

// ValidatePayers checks the payers sum to the total and have no duplicates.
func ValidatePayers(total int64, payers []Payer) error {
	if len(payers) == 0 {
		return ErrPayersMismatch
	}
	seen := map[string]bool{}
	var sum int64
	for _, p := range payers {
		if p.UserID == "" || seen[p.UserID] || p.Amount <= 0 {
			return ErrPayersMismatch
		}
		seen[p.UserID] = true
		sum += p.Amount
	}
	if sum != total {
		return ErrPayersMismatch
	}
	return nil
}
