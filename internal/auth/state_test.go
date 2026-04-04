package auth

import "testing"

func TestGenerateStateLength(t *testing.T) {
	state, err := GenerateState()
	if err != nil {
		t.Fatalf("GenerateState: %v", err)
	}
	if len(state) != 64 {
		t.Errorf("got length %d, want 64", len(state))
	}
}

func TestGenerateStateUniqueness(t *testing.T) {
	s1, err := GenerateState()
	if err != nil {
		t.Fatal(err)
	}
	s2, err := GenerateState()
	if err != nil {
		t.Fatal(err)
	}
	if s1 == s2 {
		t.Error("two calls produced the same state")
	}
}
