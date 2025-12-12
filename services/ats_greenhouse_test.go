package services

import "testing"

func TestGreenhouseTokenFromURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		url     string
		want    string
		wantErr bool
	}{
		{
			name: "job_boards_job_url",
			url:  "https://job-boards.greenhouse.io/affirm/jobs/7492221003",
			want: "affirm",
		},
		{
			name: "boards_job_url_with_query",
			url:  "https://boards.greenhouse.io/affirm/jobs/7492221003?gh_src=abc",
			want: "affirm",
		},
		{
			name: "boards_root",
			url:  "https://boards.greenhouse.io/affirm",
			want: "affirm",
		},
		{
			name: "job_boards_root_trailing_slash",
			url:  "https://job-boards.greenhouse.io/affirm/",
			want: "affirm",
		},
		{
			name:    "jobs_without_board",
			url:     "https://boards.greenhouse.io/jobs/7492221003",
			wantErr: true,
		},
		{
			name:    "short_link_requires_redirect",
			url:     "https://grnh.se/abcd1234",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := greenhouseTokenFromURL(tc.url)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got token=%q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("token mismatch: got=%q want=%q", got, tc.want)
			}
		})
	}
}

func TestGreenhouseLooksLikeJobID(t *testing.T) {
	t.Parallel()

	if !greenhouseLooksLikeJobID("7492221003") {
		t.Fatalf("expected numeric token to look like job id")
	}
	if greenhouseLooksLikeJobID("affirm") {
		t.Fatalf("expected non-numeric token not to look like job id")
	}
	if greenhouseLooksLikeJobID("123abc") {
		t.Fatalf("expected mixed token not to look like job id")
	}
}

