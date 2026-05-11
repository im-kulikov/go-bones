package logger

import (
	"context"
)

type contextAttrsKeyType string

// contextAttrsKey is a unique key used to store logging attributes in the context.
const contextAttrsKey contextAttrsKeyType = "context-attributes"

// fromContext retrieves logging attributes stored in the given context and returns them as a slice of `Attr`.
// If no attributes are found or the context is nil, it returns an empty slice.
//
// Parameters:
//   - ctx: The context object that may contain logging attributes.
//
// Returns:
//   - An `[]Attr` containing the extracted logging attributes if available; otherwise, an empty slice.
func fromContext(ctx context.Context) []Attr {
	var out []Attr
	if value, ok := ctx.Value(contextAttrsKey).([]Attr); ok {
		out = make([]Attr, len(value))
		copy(out, value)
	}

	return out
}

// AddContextAttrs adds the provided logging attributes to the specified context.
// This enables the attributes to be passed along with the context and automatically added to log records.
//
// Parameters:
//   - top: The base context to extend.
//   - attrs: The attributes to associate with the context.
//
// Returns:
//   - A new context object with the added attributes.
func AddContextAttrs(top context.Context, attrs ...Attr) context.Context {
	if len(attrs) == 0 {
		return top
	}

	current := fromContext(top)
	counter := len(current)
	outputs := make([]Attr, counter+len(attrs))

	copy(outputs[:counter], current)
	copy(outputs[counter:], attrs)

	return context.WithValue(top, contextAttrsKey, outputs)
}

// contextTransformer is a transformer that adds context-specific attributes to a log record.
// It retrieves attributes from the context and attaches them to the provided record.
//
// Parameters:
//   - ctx: The context from which attributes are retrieved.
//   - record: The original log record to enhance.
//
// Returns:
//   - A modified log record that includes the context-specific attributes.
func contextTransformer(ctx context.Context, record Record) Record {
	record.AddAttrs(fromContext(ctx)...)

	return record
}
