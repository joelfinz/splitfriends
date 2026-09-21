package ledger

import "sort"

// Balances returns each member's net position. Positive means the group owes
// them, negative means they owe the group. Members with no activity are
// included with net 0 so the UI can render everyone.
func Balances(memberIDs []string, expenses []Expense, payments []Payment) []Balance {
	net := map[string]int64{}
	for _, m := range memberIDs {
		net[m] = 0
	}
	for _, e := range expenses {
		for _, p := range e.Payers {
			net[p.UserID] += p.Amount
		}
		for _, s := range e.Shares {
			net[s.UserID] -= s.Amount
		}
	}
	for _, p := range payments {
		net[p.FromUserID] += p.Amount
		net[p.ToUserID] -= p.Amount
	}
	out := make([]Balance, 0, len(net))
	for id, n := range net {
		out = append(out, Balance{UserID: id, Net: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Net != out[j].Net {
			return out[i].Net > out[j].Net
		}
		return out[i].UserID < out[j].UserID
	})
	return out
}

// Pairwise returns the un-simplified "A owes B" debts as Splitwise shows them
// by default: within each expense every debtor owes every creditor, netted
// against the reverse direction and against recorded payments.
func Pairwise(expenses []Expense, payments []Payment) []Debt {
	// owed[a][b] = a owes b
	owed := map[string]map[string]int64{}
	add := func(from, to string, amt int64) {
		if from == to || amt == 0 {
			return
		}
		if owed[from] == nil {
			owed[from] = map[string]int64{}
		}
		owed[from][to] += amt
	}
	for _, e := range expenses {
		if e.Amount == 0 {
			continue
		}
		if len(e.Payers) == 1 {
			p := e.Payers[0].UserID
			for _, s := range e.Shares {
				add(s.UserID, p, s.Amount)
			}
			continue
		}
		// Multiple payers: net each person within this expense (paid - owed),
		// then every debtor owes every creditor in proportion to the creditor's
		// net. This keeps co-payers from owing each other rounding cents.
		net := map[string]int64{}
		order := []string{}
		for _, p := range e.Payers {
			if _, ok := net[p.UserID]; !ok {
				order = append(order, p.UserID)
			}
			net[p.UserID] += p.Amount
		}
		for _, s := range e.Shares {
			if _, ok := net[s.UserID]; !ok {
				order = append(order, s.UserID)
			}
			net[s.UserID] -= s.Amount
		}
		var creditors []string
		var weights []int64
		for _, id := range order {
			if net[id] > 0 {
				creditors = append(creditors, id)
				weights = append(weights, net[id])
			}
		}
		k := 0
		for _, id := range order {
			if net[id] >= 0 {
				continue
			}
			parts := make([]Share, len(creditors))
			allocateRotated(-net[id], weights, parts, k)
			k++
			for i, c := range creditors {
				add(id, c, parts[i].Amount)
			}
		}
	}
	for _, p := range payments {
		// Paying someone reduces what you owe them (or increases what they owe you).
		add(p.ToUserID, p.FromUserID, p.Amount)
	}
	// Net the two directions.
	var out []Debt
	done := map[[2]string]bool{}
	for a, m := range owed {
		for b, ab := range m {
			key := [2]string{a, b}
			if a > b {
				key = [2]string{b, a}
			}
			if done[key] {
				continue
			}
			done[key] = true
			ba := int64(0)
			if owed[b] != nil {
				ba = owed[b][a]
			}
			switch {
			case ab > ba:
				out = append(out, Debt{FromUserID: a, ToUserID: b, Amount: ab - ba})
			case ba > ab:
				out = append(out, Debt{FromUserID: b, ToUserID: a, Amount: ba - ab})
			}
		}
	}
	sortDebts(out)
	return out
}

// Simplify collapses the group's net balances into the minimal-ish set of
// transfers using the standard greedy max-creditor/max-debtor matching.
func Simplify(balances []Balance) []Debt {
	type entry struct {
		id  string
		amt int64
	}
	var creditors, debtors []entry
	for _, b := range balances {
		switch {
		case b.Net > 0:
			creditors = append(creditors, entry{b.UserID, b.Net})
		case b.Net < 0:
			debtors = append(debtors, entry{b.UserID, -b.Net})
		}
	}
	sort.Slice(creditors, func(i, j int) bool {
		if creditors[i].amt != creditors[j].amt {
			return creditors[i].amt > creditors[j].amt
		}
		return creditors[i].id < creditors[j].id
	})
	sort.Slice(debtors, func(i, j int) bool {
		if debtors[i].amt != debtors[j].amt {
			return debtors[i].amt > debtors[j].amt
		}
		return debtors[i].id < debtors[j].id
	})
	var out []Debt
	i, j := 0, 0
	for i < len(creditors) && j < len(debtors) {
		c, d := &creditors[i], &debtors[j]
		amt := min(c.amt, d.amt)
		out = append(out, Debt{FromUserID: d.id, ToUserID: c.id, Amount: amt})
		c.amt -= amt
		d.amt -= amt
		if c.amt == 0 {
			i++
		}
		if d.amt == 0 {
			j++
		}
	}
	sortDebts(out)
	return out
}

func sortDebts(d []Debt) {
	sort.Slice(d, func(i, j int) bool {
		if d[i].Amount != d[j].Amount {
			return d[i].Amount > d[j].Amount
		}
		if d[i].FromUserID != d[j].FromUserID {
			return d[i].FromUserID < d[j].FromUserID
		}
		return d[i].ToUserID < d[j].ToUserID
	})
}
