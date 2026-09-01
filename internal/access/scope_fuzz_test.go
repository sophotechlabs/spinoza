package access

import "testing"

func FuzzDecisionAggregation(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0})
	f.Add([]byte{1})
	f.Add([]byte{2})
	f.Add([]byte{2, 2, 2})
	f.Add([]byte{2, 1})
	f.Add([]byte{2, 0})

	f.Fuzz(func(t *testing.T, encoded []byte) {
		decisions := make([]Decision, len(encoded))
		want := allowed
		if len(encoded) == 0 {
			want = unanswered
		}
		for at, value := range encoded {
			switch value % 3 {
			case 0:
				decisions[at] = Decision{}
				if want != denied {
					want = unanswered
				}
			case 1:
				decisions[at] = Decision{Answered: true}
				want = denied
			case 2:
				decisions[at] = Decision{Answered: true, Allowed: true}
			}
		}
		if got := decide(decisions); got != want {
			t.Fatalf("decide(%v) = %v, want %v", encoded, got, want)
		}
	})
}
