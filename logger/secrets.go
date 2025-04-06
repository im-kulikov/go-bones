package logger

import (
	"context"
	"log/slog"
	"time"
)

type secretTransformer struct {
	secrets map[string]bool
}

func (h *secretTransformer) apply(secrets []string) {
	h.secrets = make(map[string]bool, len(secrets))
	for _, secret := range secrets {
		h.secrets[secret] = true
	}
}

// Transform would hide secrets with 'REDACTED' for enabled secrets.
func (h *secretTransformer) Transform(_ context.Context, original slog.Record) slog.Record {
	redacted := original

	if len(h.secrets) > 0 {
		redacted = slog.NewRecord(original.Time, original.Level, original.Message, original.PC)
		for attr := range fetchAllAttributes(original) {
			if hide, ok := h.secrets[attr.Key]; ok && hide {
				attr.Value = slog.StringValue("REDACTED")
			}

			redacted.Add(attr)
		}
	}

	if hide, ok := h.secrets[slog.TimeKey]; ok && hide {
		redacted.Time = time.Time{} // to hide value
	}

	return redacted
}
