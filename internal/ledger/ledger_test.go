package ledger

import "testing"

func sum(s []Share) (t int64) {
	for _, x := range s {
		t += x.Amount
	}
	return
}

func TestEqualSplitRounding(t *testing.T) {
	in := []ShareInput{{UserID: "a"}, {UserID: "b"}, {UserID: "c"}}
	s, err := ComputeShares(100, SplitEqual, in)
	if err != nil {
		t.Fatal(err)
	}
	if sum(s) != 100 {
		t.Fatalf("sum=%d", sum(s))
	}
	if s[0].Amount != 34 || s[1].Amount != 33 || s[2].Amount != 33 {
		t.Fatalf("got %+v", s)
	}
}

func TestPercentSplit(t *testing.T) {
	in := []ShareInput{{"a", 3333}, {"b", 3333}, {"c", 3334}}
	s, err := ComputeShares(1000, SplitPercent, in)
	if err != nil || sum(s) != 1000 {
		t.Fatalf("err=%v sum=%d", err, sum(s))
	}
	if _, err := ComputeShares(1000, SplitPercent, []ShareInput{{"a", 5000}}); err != ErrSharesMismatch {
		t.Fatalf("expected mismatch, got %v", err)
	}
}

func TestSharesSplit(t *testing.T) {
	s, err := ComputeShares(1001, SplitShares, []ShareInput{{"a", 1}, {"b", 2}})
	if err != nil || sum(s) != 1001 {
		t.Fatalf("err=%v sum=%d", err, sum(s))
	}
	if s[0].Amount != 334 || s[1].Amount != 667 {
		t.Fatalf("got %+v", s)
	}
}

func TestExactSplit(t *testing.T) {
	if _, err := ComputeShares(100, SplitExact, []ShareInput{{"a", 60}, {"b", 30}}); err != ErrSharesMismatch {
		t.Fatalf("expected mismatch, got %v", err)
	}
	s, err := ComputeShares(100, SplitExact, []ShareInput{{"a", 60}, {"b", 40}})
	if err != nil || s[0].Amount != 60 {
		t.Fatalf("err=%v %+v", err, s)
	}
}

func TestBalancesAndSimplify(t *testing.T) {
	// a pays 300 split equally among a,b,c; b pays 90 split between b and c.
	exp := []Expense{
		{Amount: 300, Payers: []Payer{{"a", 300}}, Shares: []Share{{"a", 0, 100}, {"b", 0, 100}, {"c", 0, 100}}},
		{Amount: 90, Payers: []Payer{{"b", 90}}, Shares: []Share{{"b", 0, 45}, {"c", 0, 45}}},
	}
	b := Balances([]string{"a", "b", "c"}, exp, nil)
	want := map[string]int64{"a": 200, "b": -55, "c": -145}
	for _, x := range b {
		if want[x.UserID] != x.Net {
			t.Fatalf("%s: got %d want %d", x.UserID, x.Net, want[x.UserID])
		}
	}
	pw := Pairwise(exp, nil)
	// b owes a 100, c owes a 100, c owes b 45
	if len(pw) != 3 {
		t.Fatalf("pairwise %+v", pw)
	}
	simp := Simplify(b)
	var total int64
	for _, d := range simp {
		total += d.Amount
	}
	if total != 200 || len(simp) != 2 {
		t.Fatalf("simplified %+v", simp)
	}
	// c pays a 145: c is settled.
	pay := []Payment{{FromUserID: "c", ToUserID: "a", Amount: 145}}
	b2 := Balances([]string{"a", "b", "c"}, exp, pay)
	for _, x := range b2 {
		if x.UserID == "c" && x.Net != 0 {
			t.Fatalf("c should be settled, got %d", x.Net)
		}
	}
}

func TestPairwiseMultiPayer(t *testing.T) {
	// a and b each pay 50 of a 100 bill split equally across a,b,c,d.
	exp := []Expense{{Amount: 100, Payers: []Payer{{"a", 50}, {"b", 50}},
		Shares: []Share{{"a", 0, 25}, {"b", 0, 25}, {"c", 0, 25}, {"d", 0, 25}}}}
	pw := Pairwise(exp, nil)
	var total int64
	for _, d := range pw {
		total += d.Amount
	}
	// c and d each owe 25 split across a and b -> 4 debts, total 50. a and b are even.
	if total != 50 || len(pw) != 4 {
		t.Fatalf("got %+v", pw)
	}
}
