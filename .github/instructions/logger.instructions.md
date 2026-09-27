---
applyTo: "logger/**"
---

# Logger pipeline: invariants and known non-issues

`wrappedHandler.Handle` runs the transformers from `prepareTransformers` strictly
in order, one after another. Exporters are side effects of individual steps: the
OTel log bridge calls `Emit` and `openTracingTransform` calls `span.AddEvent` with
the record *as it is at that step*. Later steps cannot change what was already
exported.

Order (see `logger/logger.go`):

1. `contextTransformer` - merges context attributes.
2. `secrets` - first redaction pass.
3. OTel log bridge - exports to OTel Logs.
4. `openTracingTransform` - span events (only when the bridge is off).
5. Caller-supplied transformers.
6. `secrets` again - only when secrets and custom transformers are both set.
7. Final `slog.Handler`.

## Do not flag

- "The OTel bridge / span events run before custom transformers and the second
  `secrets` pass, so secrets added by custom transformers leak to OTel." This is a
  false positive: attributes added in step 5 are never exported to OTel at all,
  masked or not. Covered by `Test_secretTransformer_CustomAttrsAreNotExported`.
  Exporting custom-transformer attributes is a deliberate non-goal; only propose
  moving the exporters if the change also moves redaction before them and updates
  that test.
- The second `secrets` pass is intentional: it masks attributes added by custom
  transformers before the final handler.

## Do flag

- Any exporter or handler path that can see attributes before a `secrets` pass.
- Attributes that bypass the transformers list: `WithAttrs` must redact through
  `wrappedHandler.redactAttrs`.
- `slog.LogValuer` values must be resolved before redaction (`redactAttr` calls
  `Resolve`).
