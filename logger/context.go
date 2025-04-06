package logger

import "context"

type contextAttrsKeyType string

const contextAttrsKey contextAttrsKeyType = "context-attributes"

func fromContext(ctx context.Context) []any {
	var out []any
	if value, ok := ctx.Value(contextAttrsKey).([]Attr); ok {
		for _, attr := range value {
			out = append(out, attr)
		}
	}

	return out
}

// AddContextAttrs allows to add slog.Attr to context.
func AddContextAttrs(top context.Context, attrs ...Attr) context.Context {
	return context.WithValue(top, contextAttrsKey, attrs)
}

func contextTransformer(ctx context.Context, record Record) Record {
	record.Add(fromContext(ctx)...)

	return record
}
