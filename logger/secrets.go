package logger

import (
	"context"
	"log/slog"
	"time"
)

// secretTransformer masks specified fields in log records by replacing their values with "REDACTED".
// It also optionally zeros out the record timestamp if "time" is marked as a secret.
type secretTransformer struct {
	secrets map[string]bool
}

// apply populates the transformer's internal map of secret fields to hide.
//
// Parameters:
//   - secrets: A list of field names that should be hidden in log records.
func (h *secretTransformer) apply(secrets []string) {
	h.secrets = make(map[string]bool, len(secrets))
	for _, secret := range secrets {
		h.secrets[secret] = true
	}
}

// Transform inspects a log record and replaces values of specified fields with "REDACTED".
// If the "time" field is marked as secret, the record's timestamp is also reset.
//
// Parameters:
//   - ctx: The context (not used here, but required to satisfy the interface).
//   - original: The original slog.Record to transform.
//
// Returns:
//   - A new slog.Record with secret fields redacted and optionally a reset timestamp.
func (h *secretTransformer) Transform(_ context.Context, original slog.Record) slog.Record {
	redacted := original

	// If there are secrets to hide, create a new record and filter the original attributes.
	if len(h.secrets) > 0 {
		redacted = slog.NewRecord(original.Time, original.Level, original.Message, original.PC)
		for attr := range fetchAllAttributes(original) {
			if hide, ok := h.secrets[attr.Key]; ok && hide {
				attr.Value = slog.StringValue("REDACTED")
			}

			redacted.Add(attr)
		}
	}

	// If the "time" key is marked as secret, remove the timestamp from the record.
	if hide, ok := h.secrets[slog.TimeKey]; ok && hide {
		redacted.Time = time.Time{}
	}

	return redacted
}
