package utils

import "testing"

func TestIsSupportedJobLocation(t *testing.T) {
	tests := []struct {
		name       string
		location   string
		remoteType string
		want       bool
	}{
		{name: "US city with state code", location: "San Francisco, CA", want: true},
		{name: "US spelled out", location: "Seattle, Washington, United States", want: true},
		{name: "Canadian province code", location: "Toronto, ON", want: true},
		{name: "Canadian province name", location: "Vancouver, British Columbia", want: true},
		{name: "UK city", location: "London, UK", want: true},
		{name: "UK region", location: "Edinburgh, Scotland", want: true},
		{name: "Remote US in remote_type", location: "Remote", remoteType: "Remote - US", want: true},
		{name: "Remote Canada in location", location: "Remote - Canada", want: true},
		{name: "Unsupported country", location: "Berlin, Germany", remoteType: "Hybrid", want: false},
		{name: "Remote global", location: "Remote", remoteType: "Remote - Global", want: false},
		{name: "Empty location", location: "", remoteType: "", want: false},
		{name: "Republic of Ireland", location: "Dublin, Ireland", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsSupportedJobLocation(tt.location, tt.remoteType)
			if got != tt.want {
				t.Fatalf("IsSupportedJobLocation(%q, %q) = %v, want %v", tt.location, tt.remoteType, got, tt.want)
			}
		})
	}
}

func TestLooksCanadianLocation(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "Toronto with province", value: "Toronto, Ontario, Canada", want: true},
		{name: "Remote Canada text", value: "Remote - Canada", want: true},
		{name: "Non Canadian location", value: "Austin, Texas, United States", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LooksCanadianLocation(tt.value); got != tt.want {
				t.Fatalf("LooksCanadianLocation(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func TestLooksUSLocation(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "US city and state", value: "Austin, Texas, United States", want: true},
		{name: "Remote US", value: "Remote - US", want: true},
		{name: "Canadian city", value: "Toronto, Ontario", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LooksUSLocation(tt.value); got != tt.want {
				t.Fatalf("LooksUSLocation(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}
