package services

import (
	"errors"
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsRateLimitErr(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"429 substring", errors.New("googleapi: got 429 Too Many Requests"), true},
		{"resource exhausted", errors.New("rpc error: code = ResourceExhausted"), true},
		{"quota exceeded", errors.New("quota exceeded for model gemini-2.0-flash"), true},
		{"rate limit with space", errors.New("rate limit exceeded"), true},
		{"rate_limit underscore", errors.New("rate_limit hit"), true},
		{"case-insensitive", errors.New("RESOURCE_EXHAUSTED"), true},
		{"false positive accurate", errors.New("result was not accurate enough"), false},
		{"false positive integrate", errors.New("failed to integrate response"), false},
		{"generic 500", errors.New("server error 500"), false},
		{"network err", fmt.Errorf("dial failed: %w", &net.OpError{Op: "dial"}), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, IsRateLimitErr(tc.err))
		})
	}
}
