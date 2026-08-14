package lockstep

import "testing"

func TestMatchComponent(t *testing.T) {
	m := NewMatchComponent()
	if got := m.Match(1); got == nil || len(got) != MatchCount {
		t.Fatal("expected match result since MatchCount=1")
	}
}

func TestMatchComponentRequeueDeduplicatesAndPreservesOrder(t *testing.T) {
	m := NewMatchComponent()
	m.Requeue([]int64{3, 4})
	m.Requeue([]int64{4, 5})

	got := m.WaitingPlayers()
	want := []int64{4, 5, 3}
	if len(got) != len(want) {
		t.Fatalf("waiting players = %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("waiting players = %v, want %v", got, want)
		}
	}
}
