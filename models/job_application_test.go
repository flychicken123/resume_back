package models

import (
	"testing"
)

func TestIsValidStatusTransition(t *testing.T) {
	tests := []struct {
		name string
		from string
		to   string
		want bool
	}{
		// Valid transitions
		{name: "applied → screening", from: "applied", to: "screening", want: true},
		{name: "applied → interviewing (skip)", from: "applied", to: "interviewing", want: true},
		{name: "applied → offered (skip)", from: "applied", to: "offered", want: true},
		{name: "applied → rejected", from: "applied", to: "rejected", want: true},
		{name: "applied → withdrawn", from: "applied", to: "withdrawn", want: true},
		{name: "screening → interviewing", from: "screening", to: "interviewing", want: true},
		{name: "screening → offered", from: "screening", to: "offered", want: true},
		{name: "screening → rejected", from: "screening", to: "rejected", want: true},
		{name: "screening → withdrawn", from: "screening", to: "withdrawn", want: true},
		{name: "interviewing → offered", from: "interviewing", to: "offered", want: true},
		{name: "interviewing → rejected", from: "interviewing", to: "rejected", want: true},
		{name: "interviewing → withdrawn", from: "interviewing", to: "withdrawn", want: true},
		{name: "offered → accepted", from: "offered", to: "accepted", want: true},
		{name: "offered → rejected", from: "offered", to: "rejected", want: true},
		{name: "offered → withdrawn", from: "offered", to: "withdrawn", want: true},
		{name: "accepted → withdrawn", from: "accepted", to: "withdrawn", want: true},

		// Invalid transitions
		{name: "rejected is terminal", from: "rejected", to: "applied", want: false},
		{name: "withdrawn is terminal", from: "withdrawn", to: "applied", want: false},
		{name: "accepted → screening (backward)", from: "accepted", to: "screening", want: true},
		{name: "same status", from: "applied", to: "applied", want: false},
		{name: "backwards interviewing → screening", from: "interviewing", to: "screening", want: true},
		{name: "empty from", from: "", to: "applied", want: false},
		{name: "empty to", from: "applied", to: "", want: false},
		{name: "both empty", from: "", to: "", want: false},
		{name: "unknown status", from: "unknown", to: "applied", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := IsValidStatusTransition(tc.from, tc.to)
			if got != tc.want {
				t.Errorf("IsValidStatusTransition(%q, %q) = %v, want %v", tc.from, tc.to, got, tc.want)
			}
		})
	}
}

func TestGetAllowedTransitions(t *testing.T) {
	tests := []struct {
		name     string
		from     string
		wantLen  int
		wantNone bool
	}{
		{name: "applied has 5 transitions", from: "applied", wantLen: 5},
		{name: "screening has 5 transitions", from: "screening", wantLen: 5},
		{name: "interviewing has 5 transitions", from: "interviewing", wantLen: 5},
		{name: "offered has 6 transitions", from: "offered", wantLen: 6},
		{name: "accepted has 5 transitions", from: "accepted", wantLen: 5},
		{name: "rejected is terminal", from: "rejected", wantLen: 0, wantNone: true},
		{name: "withdrawn is terminal", from: "withdrawn", wantLen: 0, wantNone: true},
		{name: "unknown status", from: "unknown", wantLen: 0, wantNone: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := GetAllowedTransitions(tc.from)
			if len(got) != tc.wantLen {
				t.Errorf("GetAllowedTransitions(%q) returned %d transitions, want %d", tc.from, len(got), tc.wantLen)
			}
		})
	}
}
