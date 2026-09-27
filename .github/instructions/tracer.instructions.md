---
applyTo: "tracer/**"
---

# Tracer: known non-issues

## Do not flag

- Findings about test inputs that look like `******host:4318/v1` in
  `TestEndpointHost` or the insecure-endpoint warning tests. Secret masking
  replaces URL userinfo in your view; the real inputs are full URLs such as
  `https://user:s3cr3t@host:4318/v1`, which parse to host `host`. The tests
  run in CI and pass.
- `endpointHost` returning `[redacted]` for anything that is not an IP
  address or a DNS name is intentional: it is a whitelist, so the warning can
  never log credentials from userinfo, query or opaque URL parts.
