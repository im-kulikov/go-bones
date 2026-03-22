package service

import (
	"context"
	"errors"
	"testing"
)

func BenchmarkContainsError(b *testing.B) {
	target := context.Canceled
	nested := errors.Join(
		ErrOsSignal,
		errors.Join(context.DeadlineExceeded, target),
	)
	ignore := errors.Join(ErrOsSignal, context.Canceled, context.DeadlineExceeded)

	b.Run("joined", func(b *testing.B) {
		for b.Loop() {
			if !containsError(target, nested, ignore) {
				b.Fatal("expected joined error to contain target")
			}
		}
	})

	b.Run("flat", func(b *testing.B) {
		for b.Loop() {
			if !containsError(target, ErrOsSignal, ignore, context.DeadlineExceeded) {
				b.Fatal("expected flat error list to contain target")
			}
		}
	})
}
