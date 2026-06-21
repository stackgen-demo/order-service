# Order Service

A minimal Go order management API with SQLite, built to **intentionally produce HTTP 500 errors** from a database schema mismatch. Use it to validate Datadog monitors, alerts, and agent-driven remediation workflows.

Service name in Datadog: **`order-service`**

## What it does

| Endpoint | Method | Result |
|----------|--------|--------|
| `/health` | GET | `200` — service is up |
| `/api/users` | GET | `200` — list users |
| `/api/users` | POST | `201` — create user (works) |
| `/api/orders` | GET | `200` — list orders |
| `/api/orders` | POST | **`500` default** — schema mismatch; other modes via `X-Demo-Fault` header |

### Demo fault injection (`X-Demo-Fault`)

Triggers (CronJob, curl, scripts) select the failure mode. The app Deployment has **no fault env vars**.

| Header value | HTTP | `error.kind` | Use case |
|--------------|------|--------------|----------|
| *(omit)* or `schema` | 500 | `DatabaseSchemaMismatch` | Schema drift RCA → fix `cmd/initdb/main.go` |
| `dependency` | 502 | `DownstreamPaymentFailure` | Downstream outage narrative |
| `timeout` | 504 | `DownstreamPaymentTimeout` | Latency / timeout RCA |
| `panic` | 500 | `UnhandledPanic` | Crash / stack trace in logs |
| `locked` | 500 | `DatabaseLocked` | DB contention analogue |
| `healthy` | 201 | — | Recovery demo (monitor should clear) |

Unknown header → **400** `unknown demo fault`.

```bash
# Local
DEMO_FAULT=dependency ./scripts/trigger-fault.sh
curl -X POST http://localhost:3005/api/orders \
  -H "Content-Type: application/json" \
  -H "X-Demo-Fault: dependency" \
  -d '{"customer_email":"bob@example.com","total_amount":42.50}'

# K8s — patch CronJob fault without redeploying the app
kubectl -n aiden-demo patch cronjob order-service-trigger-fault \
  --type=json -p='[{"op":"replace","path":"/spec/jobTemplate/spec/template/spec/containers/0/env/0/value","value":"dependency"}]'
```

### The intentional bug (schema mode)

The handler in `internal/handlers/orders.go` inserts into:

```sql
customer_email, total_amount, status
```

The database schema in `cmd/initdb/main.go` creates:

```sql
amount, status
```

`POST /api/orders` fails with SQLite `no such column: customer_email` and returns **HTTP 500**.

## Quick start

### Prerequisites

- Go 1.22+
- Make (optional)

### 1. Initialize the database

```bash
cd datadog-5xx-test-service
make init-db
# or: go run ./cmd/initdb
```

### 2. Start the service

```bash
make run
# or: go run ./cmd/server
```

Local default: [http://localhost:3000](http://localhost:3000)

### 3. Verify endpoints

```bash
curl http://localhost:3005/health

curl -X POST http://localhost:3005/api/users \
  -H "Content-Type: application/json" \
  -d '{"name":"Alice","email":"alice@example.com"}'

curl -X POST http://localhost:3005/api/orders \
  -H "Content-Type: application/json" \
  -d '{"customer_email":"bob@example.com","total_amount":42.50}'
```

### 4. Generate 5xx traffic

```bash
make trigger-5xx
# or: BASE_URL=http://localhost:3005 make trigger-5xx
```

## Docker + Datadog (APM + Logs)

### 1. Configure API key

```bash
cp .env.example .env
# Set DD_API_KEY and DD_SITE
```

### 2. Start stack

```bash
docker compose up --build
```

API default: **http://localhost:3005** (`API_PORT` in `.env`)

### 3. Trigger errors

```bash
make trigger-5xx
```

## Datadog observability

### Data flow

```text
Go app (dd-trace-go)
  ├─ APM traces  → datadog-agent:8126 → Datadog APM
  └─ stdout logs → Docker log driver  → datadog-agent → Datadog Logs
```

### Root cause logs

On `POST /api/orders` failure:

```json
{
  "level": "error",
  "message": "Order creation failed: database schema mismatch",
  "error": {
    "kind": "DatabaseSchemaMismatch",
    "message": "SQL logic error: no such column: customer_email (1)",
    "root_cause": "Application expects orders.customer_email and orders.total_amount but DB schema only has amount and status"
  },
  "http": { "status_code": 500, "method": "POST", "url": "/api/orders" }
}
```

### Example monitors

Apply the full aiden-demo monitor set (all four checkout services + chaos-monkey;
alerts go to `@webhook-sabith-datadog-testbed`):

```bash
DD_API_KEY=<us3-api-key> DD_APP_KEY=<us3-app-key> ./scripts/apply-datadog-monitors.sh
```

**5xx rate (APM):**

```text
sum:trace.http.request.hits{service:order-service,http.status_code:50*}.as_count()
```

**Root cause (logs):**

```text
service:order-service env:demo DatabaseSchemaMismatch
```

## Agent fix guide (for automated PRs)

An agent should fix the schema mismatch in `cmd/initdb/main.go`:

```go
// Replace the orders table definition with:
`CREATE TABLE IF NOT EXISTS orders (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    customer_email TEXT NOT NULL,
    total_amount REAL NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
)`,
```

Then re-initialize the database:

```bash
rm -f data/app.db
make init-db
```

After the fix, `POST /api/orders` returns **201** and 5xx monitors should recover.

Files involved:

| File | Role |
|------|------|
| `cmd/initdb/main.go` | **Fix here** — DB schema (intentional bug) |
| `internal/handlers/orders.go` | Handler INSERT (correct expectation) |
| `internal/handlers/orders.go` | Structured error logs for Datadog |

## Project structure

```text
datadog-5xx-test-service/
├── cmd/
│   ├── server/main.go       # HTTP server + dd-trace-go
│   └── initdb/main.go       # DB init (intentional schema bug)
├── internal/
│   ├── db/db.go
│   ├── logger/logger.go
│   └── handlers/
│       ├── handlers.go
│       ├── users.go
│       └── orders.go
├── scripts/trigger-fault.sh
├── scripts/trigger-5xx.sh   # alias → trigger-fault.sh (schema default)
├── docker-compose.yml
├── Dockerfile
├── Makefile
└── README.md
```

## CI/CD: Docker image (GitHub Container Registry)

`.github/workflows/docker-publish.yml` builds a multi-arch (`linux/amd64`, `linux/arm64`) image and publishes it to **GHCR** on every push to `main` and on `v*.*.*` tags. Pull requests build the image to verify it compiles but do not push.

Published image:

```text
ghcr.io/stackgen-demo/order-service:latest        # default branch
ghcr.io/stackgen-demo/order-service:main          # branch builds
ghcr.io/stackgen-demo/order-service:1.2.3         # semver tags
ghcr.io/stackgen-demo/order-service:sha-<commit>  # immutable per-commit
```

No secrets are required — the workflow authenticates with the built-in `GITHUB_TOKEN`.

### Make the image public (one-time)

GHCR packages default to private. After the first successful run, make it public so the k8s cluster can pull without credentials:

1. Repo → **Packages** → `order-service` → **Package settings**
2. **Danger Zone** → **Change visibility** → **Public**

## Deploy to Kubernetes

Manifests in [`k8s/`](k8s/) deploy a lean stack into `aiden-demo` (app + one
Datadog Agent Deployment for APM traces and container log collection):

| File | What it creates |
|------|-----------------|
| `k8s/stack.yaml` | Namespace, 1× Datadog Agent Deployment (APM + logs), `aiden-demo` Deployment + Services |
| `k8s/network-policy.yaml` | Namespace isolation — no cross-namespace or control-plane egress; Datadog agent US3 only |
| `k8s/fault-profiles/*.yaml` | Shared `aiden-demo-fault-profile` ConfigMap presets (`quiet` / `normal` / `noisy`) |
| `k8s/chaos-monkey.yaml` | Random checkout + leaf fault injection (reads fault profile) |
| `k8s/datadog-secret.yaml` | Placeholder `datadog-secret` |
| `k8s/trigger-fault-cronjob.yaml` | CronJob sends `X-Demo-Fault` (reads `DEMO_FAULT` from fault profile) |

```bash
./scripts/deploy-aiden-demo-stack.sh   # applies normal fault level by default
./scripts/set-fault-level.sh quiet     # reduce noise between demos
./scripts/set-fault-level.sh noisy     # soak / monitor firing
```

### Network isolation

`k8s/network-policy.yaml` restricts **aiden-demo** so pods can only:

- talk to other pods in **aiden-demo** (checkout mesh + Datadog agent)
- resolve DNS via **kube-system** (UDP/TCP 53 only)

Blocked for app pods: other namespaces, the Kubernetes API, EC2 metadata
(`169.254.169.254`), and private RFC1918 ranges. The **datadog-agent** may
additionally egress to public **HTTPS (443)** for US3 intake.

On EKS, NetworkPolicy enforcement requires the VPC CNI addon with
`enableNetworkPolicy: "true"` (once per cluster):

```bash
aws eks update-addon --cluster-name <cluster> --addon-name vpc-cni \
  --resolve-conflicts PRESERVE \
  --configuration-values '{"enableNetworkPolicy":"true"}'
```

After deploy:

```bash
kubectl -n aiden-demo rollout status deployment/aiden-demo
kubectl -n aiden-demo port-forward svc/aiden-demo 3005:80
curl http://localhost:3005/health
```

Traces go to `datadog-agent:8126`; JSON stdout logs are tailed by the single agent
(preferably on the same node as `aiden-demo`) → Datadog US3 (`service:order-service`, `env:demo`).

In Datadog UI, filter **`env:demo`** (not `production`). Logs Explorer:
`service:order-service env:demo`. APM service page:
https://us3.datadoghq.com/apm/entity/service%3Aorder-service?env=demo#logs

Query errors with: `service:order-service env:demo status:error @error.kind:DatabaseSchemaMismatch`.
If logs stop after a reschedule, restart the agent: `kubectl -n aiden-demo rollout restart deployment/datadog-agent`.

## Make targets

| Command | Description |
|---------|-------------|
| `make init-db` | Create SQLite DB with mismatched schema |
| `make run` | Start the API locally |
| `make build` | Build binaries to `bin/` |
| `make trigger-5xx` | Send repeated failing requests (`DEMO_FAULT=schema`) |
| `make docker-up` | Start app + Datadog agent |

## License

MIT
