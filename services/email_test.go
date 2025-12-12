package services

import "testing"

func TestMaskRecipients(t *testing.T) {
	t.Parallel()

	got := maskRecipients([]string{"john.doe@example.com", "x@y.com", "bad"})
	if got[0] != "j***@example.com" {
		t.Fatalf("unexpected mask: %q", got[0])
	}
	if got[1] != "x***@y.com" {
		t.Fatalf("unexpected mask: %q", got[1])
	}
	if got[2] != "" {
		t.Fatalf("expected empty mask for invalid email, got %q", got[2])
	}
}

func TestRecipientDomains(t *testing.T) {
	t.Parallel()

	got := recipientDomains([]string{"a@EXAMPLE.com", "b@example.com", "c@other.org"})
	if len(got) != 2 {
		t.Fatalf("expected 2 unique domains, got %d: %#v", len(got), got)
	}
}

