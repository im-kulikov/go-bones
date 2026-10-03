# Kubernetes

A complete, copy-pasteable setup for a service built with `app.Init` / `app.Run`: ops on `:8090`, the API on `config.HTTP` (`:8080`), gRPC on `config.GRPC` (`:9090`).

- [Probes](#probes)
- [Grace period](#grace-period)
- [Full Deployment](#full-deployment)
- [gRPC-only services](#grpc-only-services)
- [Checklist](#checklist)

## Probes

| Probe | Path | Why |
|---|---|---|
| `startupProbe` | `/readyz` | Covers slow starts (migrations, cache warm-up) without weakening liveness |
| `readinessProbe` | `/readyz` | Removes the pod from Service endpoints while dependencies fail or during drain |
| `livenessProbe` | `/livez` | Restarts only when the process itself is broken |

- Liveness never points at `/readyz` or `/healthz`.
- `timeoutSeconds: 1` is enough: endpoints never do I/O.

## Grace period

```text
terminationGracePeriodSeconds  >  health.drain_delay + shutdown_timeout
          45s                  >        5s          +       30s
```

`shutdown_timeout` is per server (`30s` by default) — take the largest one among your servers and keep a margin: the OPS server stops after the others. This holds only if every service returns from `Start` once its context is canceled ([Lifecycle](lifecycle.md#shutdown-and-drain)).

Set `HEALTH_DRAIN_DELAY=5s` so endpoint controllers and kube-proxy/ingress notice the pod is not ready before it stops accepting connections. Don't also add `preStop: sleep` — the delays add up.

## Full Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: orders
spec:
  replicas: 3
  selector:
    matchLabels: { app: orders }
  template:
    metadata:
      labels: { app: orders }
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "8090"
    spec:
      terminationGracePeriodSeconds: 45
      containers:
        - name: app
          image: ghcr.io/acme/orders:1.4.0
          args: ["--config", "/etc/orders/config.yaml"]  # --config comes with config.Base
          env:
            - { name: HEALTH_DRAIN_DELAY, value: "5s" }
            - { name: OTEL_ENABLED, value: "true" }
            - { name: OTEL_EXPORTER_OTLP_ENDPOINT, value: "http://otel-collector.observability:4318" }
            - { name: OTEL_EXPORTER_OTLP_PROTOCOL, value: "http/protobuf" }
            - name: DATABASE_URL        # settings.Database.URL with `env:"DATABASE"` / `env:"URL"`
              valueFrom:
                secretKeyRef: { name: orders-db, key: url }
          ports:
            - { name: http, containerPort: 8080 }
            - { name: grpc, containerPort: 9090 }
            - { name: ops,  containerPort: 8090 }
          startupProbe:
            httpGet: { path: /readyz, port: ops }
            periodSeconds: 2
            failureThreshold: 60   # up to 2 minutes to start
          readinessProbe:
            httpGet: { path: /readyz, port: ops }
            periodSeconds: 5
            timeoutSeconds: 1
            failureThreshold: 2
          livenessProbe:
            httpGet: { path: /livez, port: ops }
            periodSeconds: 10
            timeoutSeconds: 1
            failureThreshold: 3
          resources:
            requests: { cpu: 100m, memory: 128Mi }
            limits:   { memory: 256Mi }
          volumeMounts:
            - { name: config, mountPath: /etc/orders }
      volumes:
        - { name: config, configMap: { name: orders-config } }
---
apiVersion: v1
kind: Service
metadata:
  name: orders
spec:
  selector: { app: orders }
  ports:            # the ops port is intentionally NOT here
    - { name: http, port: 80, targetPort: http }
    - { name: grpc, port: 9090, targetPort: grpc }
```

## gRPC-only services

Use native gRPC probes against the `grpc.health.v1` service registered by `grpc.WithHealth`:

```yaml
readinessProbe:
  grpc: { port: 9090, service: readiness }
livenessProbe:
  grpc: { port: 9090, service: liveness }
```

## Checklist

- [ ] `HEALTH_DRAIN_DELAY` set (≈5s), no `preStop: sleep`
- [ ] `terminationGracePeriodSeconds` > drain + shutdown timeout
- [ ] Only in-process checks have `Liveness` impact
- [ ] Ops port not in any public Service or Ingress
- [ ] `LOGGER_FORMAT` left at its default `json` (`console` is for local runs)
- [ ] Alerts from [health.md](health.md#metrics-and-alerts) installed
