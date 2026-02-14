package types

import (
	"errors"
	"testing"
)

func TestSimpleErrorClassifierClassifyError(t *testing.T) {
	classifier := NewSimpleErrorClassifier()

	cases := []struct {
		name string
		err  error
		want FailoverReason
	}{
		{
			name: "auth",
			err:  errors.New("invalid API key provided"),
			want: FailoverReasonAuth,
		},
		{
			name: "rate limit",
			err:  errors.New("429 too many requests"),
			want: FailoverReasonRateLimit,
		},
		{
			name: "timeout",
			err:  errors.New("context deadline exceeded"),
			want: FailoverReasonTimeout,
		},
		{
			name: "billing",
			err:  errors.New("payment required"),
			want: FailoverReasonBilling,
		},
		{
			name: "unknown",
			err:  errors.New("random backend failure"),
			want: FailoverReasonUnknown,
		},
		{
			name: "nil error",
			err:  nil,
			want: FailoverReasonUnknown,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifier.ClassifyError(tc.err)
			if got != tc.want {
				t.Fatalf("expected reason %q, got %q", tc.want, got)
			}
		})
	}
}

func TestSimpleErrorClassifierIsFailoverError(t *testing.T) {
	classifier := NewSimpleErrorClassifier()

	if classifier.IsFailoverError(nil) {
		t.Fatalf("nil error should not be failover error")
	}
	if !classifier.IsFailoverError(errors.New("forbidden: auth failed")) {
		t.Fatalf("auth errors should be failover errors")
	}
	if classifier.IsFailoverError(errors.New("plain unknown crash")) {
		t.Fatalf("unknown errors should not be failover errors")
	}
}

func TestSimpleErrorClassifierPrecedence(t *testing.T) {
	classifier := NewSimpleErrorClassifier()

	// Both auth and rate-limit keywords exist; classifier should follow auth precedence.
	err := errors.New("401 unauthorized and rate limit exceeded")
	got := classifier.ClassifyError(err)
	if got != FailoverReasonAuth {
		t.Fatalf("expected auth precedence, got %q", got)
	}
}
