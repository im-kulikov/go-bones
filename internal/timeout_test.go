package internal

import (
	"testing"
	"testing/synctest"
	"time"
)

func TestFallbackTimeout(t *testing.T) {
	tests := []struct {
		name     string
		input    time.Duration
		expected time.Duration
	}{
		{
			name:     "return_input_if_positive",
			input:    5 * time.Second,
			expected: 5 * time.Second,
		},
		{
			name:     "return_default_if_zero",
			input:    0,
			expected: defaultTimeout,
		},
		{
			name:     "return_default_if_negative",
			input:    -5 * time.Second,
			expected: defaultTimeout,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				result := FallbackTimeout(tt.input)
				if result != tt.expected {
					t.Errorf("expected %v, got %v", tt.expected, result)
				}
			})
		})
	}
}
