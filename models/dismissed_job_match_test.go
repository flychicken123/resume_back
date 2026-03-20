package models

import (
	"testing"
)

func TestNewDismissedJobMatchModel(t *testing.T) {
	m := NewDismissedJobMatchModel(nil)
	if m == nil {
		t.Fatal("expected non-nil model")
	}
}

func TestDismiss_NilDB(t *testing.T) {
	m := NewDismissedJobMatchModel(nil)
	err := m.Dismiss(1, 100, "wrong_role")
	if err == nil {
		t.Fatal("expected error for nil db")
	}
}

func TestUndismiss_NilDB(t *testing.T) {
	m := NewDismissedJobMatchModel(nil)
	err := m.Undismiss(1, 100)
	if err == nil {
		t.Fatal("expected error for nil db")
	}
}

func TestListDismissedIDs_NilDB(t *testing.T) {
	m := NewDismissedJobMatchModel(nil)
	ids, err := m.ListDismissedIDs(1)
	if err == nil {
		t.Fatal("expected error for nil db")
	}
	if ids != nil {
		t.Fatal("expected nil ids for nil db")
	}
}

func TestValidDismissReasons(t *testing.T) {
	validReasons := []string{
		"",
		"wrong_role",
		"too_senior_junior",
		"bad_location",
		"low_pay",
		"not_interested_company",
	}

	for _, reason := range validReasons {
		if !IsValidDismissReason(reason) {
			t.Errorf("expected %q to be a valid dismiss reason", reason)
		}
	}

	invalidReasons := []string{
		"invalid",
		"i_dont_like_mondays",
		"random_reason",
	}

	for _, reason := range invalidReasons {
		if IsValidDismissReason(reason) {
			t.Errorf("expected %q to be an invalid dismiss reason", reason)
		}
	}
}
