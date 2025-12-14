package services

import "testing"

func TestFormatJobDescriptionSnippet_DecodesAndStripsHTML(t *testing.T) {
	raw := `&lt;div class=&quot;content-intro&quot;&gt;Hello&lt;br/&gt;World&lt;/div&gt;`
	got := formatJobDescriptionSnippet(raw, 500)
	if got != "Hello World" {
		t.Fatalf("expected %q, got %q", "Hello World", got)
	}
}

func TestFormatJobDescriptionSnippet_StripsRawHTML(t *testing.T) {
	raw := `<div>One<br>Two</div>`
	got := formatJobDescriptionSnippet(raw, 500)
	if got != "One Two" {
		t.Fatalf("expected %q, got %q", "One Two", got)
	}
}

func TestFormatLocation_DedupesRemote(t *testing.T) {
	got := formatLocation("Remote - United States  Remote", "Remote")
	if got != "Remote - United States" {
		t.Fatalf("expected %q, got %q", "Remote - United States", got)
	}
}
