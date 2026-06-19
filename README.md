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
| `/api/orders` | POST | **`500` — schema mismatch (intentional)** |

### The intentional bug

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

**5xx rate (APM):**

```text
sum:trace.http.request.hits{service:order-service,http.status_code:500}.as_count()
```

**Root cause (logs):**

```text
service:order-service @error.kind:DatabaseSchemaMismatch
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
├── scripts/trigger-5xx.sh
├── docker-compose.yml
├── Dockerfile
├── Makefile
└── README.md
```

## Make targets

| Command | Description |
|---------|-------------|
| `make init-db` | Create SQLite DB with mismatched schema |
| `make run` | Start the API locally |
| `make build` | Build binaries to `bin/` |
| `make trigger-5xx` | Send repeated failing requests |
| `make docker-up` | Start app + Datadog agent |

## License

MIT
