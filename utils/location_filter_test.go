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
