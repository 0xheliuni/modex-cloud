# Modex Cloud

A secure, multi-supplier custody platform for AI provider API keys. Suppliers
upload official keys/tokens for **Anthropic Claude, AWS Claude (Bedrock), OpenAI,
Azure OpenAI, Google Gemini, and Vertex AI**; the platform forwards them to a
downstream **AGT platform** (new-api-compatible channel API) and **destroys the
local copy** the moment the sync succeeds.

> Built in Go (Gin + GORM), modeled on the `new-api` reference project. Supports
> SQLite / MySQL / PostgreSQL.

---

## Why it's secure: the two core guarantees

### 1. Write-only keys — no read path exists
No HTTP endpoint ever returns a channel key's plaintext — not to the supplier who
uploaded it, not to an admin. There is deliberately **no `reveal-key` route**.
- Keys are sealed with **AES-256-GCM** on upload.
- The only decryption point is the sync worker (`service/sync`), via
  `crypto.SyncOpener()`.
- Controllers get a **write-only `Sealer`** (`crypto.GlobalSealer()`) — a struct
  with no `Open` method, so they *cannot* decrypt even by type assertion.
- An **architecture test** (`internal/architecture`) fails the build if any
  controller references `Open` / `SyncOpener` / `crypto.Vault`.

### 2. Destroy-by-default — nothing to steal after sync
A channel key's ciphertext is **transient**:

```
upload → AES-256-GCM seal → store (key_state=pending)
       → sync worker decrypts in memory → POST to AGT
                ├─ success → WIPE enc_key  (key_state=synced; only fingerprint + last4 remain)
                └─ failure → keep sealed key, bounded retry (key_state=failed)
```

After a successful sync, **a database dump contains no recoverable key.** Verified
by tests that run a raw `SELECT enc_key` post-sync and assert it is empty.

Additional defenses: bcrypt passwords, keyed HMAC-SHA256 key fingerprints
(dedup without plaintext), three-tier RBAC, per-supplier platform authorization
with whitelist narrowing, https-only upstream URLs, login rate limiting, security
headers, append-only audit log (never contains secrets), and the platform's AGT
bearer token itself stored sealed.

---

## Architecture

```
crypto/        AES-256-GCM vault; write-only Sealer vs sync-only Opener
config/        env loading + hard validation (refuses to start without MASTER_KEK)
constant/      channel types (OpenAI=1, Azure=3, Anthropic=14, Gemini=24, AWS=33, Vertex=41), roles, key states
common/        bcrypt, random tokens, JSON wrappers, response envelope, ctx keys
model/         GORM models + DAO: User, Platform, Grant, Channel, AuditLog
service/
  validate/    whitelist enforcement (type/model/group/base_url; grant narrows platform)
  agt/         AGT platform client (POST wrapped / PUT flat, per AGT doc)
  sync/        the SOLE key-decryption site; forwards to AGT then destroys local copy
middleware/    session+token auth, RBAC, rate limit, security headers
controller/    admin (platform/user/grant/audit) + supplier (channel) handlers
router/        route wiring
internal/architecture/  build-breaking security-invariant tests
main.go        boot: config → vault → db → seed root → serve
```

Entity model: **Admin** CRUDs target **Platforms**, creates **supplier User**
accounts, and **grants** supplier↔platform access (optionally narrowing each
platform's whitelist). A **Supplier** logs in, picks a granted platform, and
uploads keys (one channel → one platform).

---

## Quick start (local)

Requires Go 1.22+. In China, set `go env -w GOPROXY=https://goproxy.cn,direct`.

```bash
cp .env.example .env
# generate secrets
echo "MASTER_KEK=$(openssl rand -hex 32)"        >> .env
echo "SESSION_SECRET=$(openssl rand -base64 32)" >> .env
echo "ROOT_PASSWORD=changeme123"                 >> .env

go run .            # SQLite by default; listens on :3000
```

The first run seeds an admin account (prints a generated password if
`ROOT_PASSWORD` is unset).

### Docker

```bash
cp .env.example .env   # fill MASTER_KEK, SESSION_SECRET, MYSQL_ROOT_PASSWORD
docker compose up -d   # app + MySQL
```

---

## API overview

All responses use the AGT envelope: `{success, message, data}`.

| Method | Path | Auth | Purpose |
|---|---|---|---|
| POST | `/api/auth/login` | — | Login (session cookie) |
| POST | `/api/auth/logout` | — | Logout |
| GET  | `/api/auth/self` | any | Own profile |
| POST | `/api/auth/change-password` | any | Change own password |
| GET  | `/api/supplier/platforms` | supplier | My authorized platforms + effective whitelist |
| GET  | `/api/supplier/channels` | supplier | My channels (**never** the key) |
| POST | `/api/supplier/channels` | supplier | Upload a key (write-only) → sync to AGT |
| PUT  | `/api/supplier/channels/:id` | supplier | Edit metadata / rotate key (omit key = keep) |
| DELETE | `/api/supplier/channels/:id` | supplier | Soft-delete |
| POST | `/api/supplier/channels/:id/resync` | supplier | Retry a failed sync |
| GET/POST/PUT/DELETE | `/api/admin/platforms[/:id]` | admin | Target platform CRUD (AGT token sealed) |
| GET/POST/PUT/DELETE | `/api/admin/users[/:id]` | admin | User CRUD; `/:id/reset-password` |
| GET/POST/DELETE | `/api/admin/grants[/:id]` | admin | Authorization CRUD |
| GET | `/api/admin/audit-logs` | admin | Audit trail |

---

## Testing

```bash
go test ./...                       # all packages
go test ./service/sync/ -v          # end-to-end destroy-by-default proof
go test ./internal/architecture/    # security invariants (build-breaking)
```

Pre-commit secret scan:

```bash
gitleaks detect --config .gitleaks.toml --source .
```

---

## Operational notes

- **`MASTER_KEK` is the crown jewel.** Inject it from a secrets manager/KMS, never
  bake it into images or commit it. Rotating it requires re-sealing the platform
  AGT tokens (channel keys are transient, so they're unaffected once synced).
- Terminate **TLS** in front of the app; set `TRUSTED_PROXIES` for correct client IPs.
- For multi-node deployments, replace the in-memory rate limiter and cookie store
  with Redis-backed equivalents.
