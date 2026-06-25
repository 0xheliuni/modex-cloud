# Modex Cloud — Implementation Plan

Secure multi-supplier API-key custody → AGT platform sync.
Reference codebase: `../new-api` (Go/Gin/GORM). Design decisions: see project memory.

## Core security invariants (never violate)

1. **Write-only keys.** No HTTP endpoint ever returns a channel key plaintext — not to the
   uploading supplier, not to admins. There is no `reveal-key` route.
2. **Destroy-by-default.** A channel key ciphertext is transient: sealed on upload, **wiped**
   the instant AGT sync succeeds. After `KeyState=synced` the DB holds no ciphertext for it —
   only `KeyFingerprint` (HMAC) + `KeyLast4`.
3. **Single decrypt site.** `crypto.Vault.Open` is called **only** from the sync worker
   (`service/sync`). It must never appear in any `controller/` file. Enforced by CI lint.
4. **Master key never persists.** Loaded from env at startup into memory only. Never logged,
   never written to disk/DB. Platform AGT tokens are the only long-term sealed secret.
5. **Audit detail never contains plaintext.** Logs record action/who/ip/time, never key bytes.

## Tech stack

- Backend: Go 1.22+, Gin, GORM v2 (SQLite/MySQL/PostgreSQL), go-redis.
- Frontend: React 18 + Vite + Semi Design + i18next (zh/en).
- Deploy: Docker Compose (app + mysql + redis).

## Package layout (flat, mirrors new-api for easy code reuse)

```
modex-cloud/
  crypto/        AES-256-GCM vault + HMAC fingerprint   [Phase 0 — DONE first]
  common/        bcrypt, random tokens, json, env, page  (lift from new-api)
  constant/      channel types, roles, key states
  model/         GORM models: User, Platform, Grant, Channel, AuditLog
  dto/           request/response structs (NO key field on any response)
  service/
    sync/        AGT client + sync worker (the ONLY Open() caller)
    audit/       audit log writer
  middleware/    auth (session + access-token), RBAC, ratelimit, audit
  controller/    admin/* and supplier/* handlers
  router/        route wiring
  web/           React frontend
  main.go
```

## Phases

### Phase 0 — Crypto core  ✅ (this commit)
- `crypto/vault.go`: `New`/`NewFromHex`, `Seal`/`Open`, `Fingerprint`, `Last4`, `Zeroize`, versioned blobs.
- `crypto/vault_test.go`: round-trip, nonce-uniqueness, tamper-detect, wrong-key, fingerprint stability.
- Acceptance: `go test ./crypto/` green once Go installed.

### Phase 1 — Foundation
- `common/`: lift `crypto.go` (bcrypt), `utils.go` (GenerateKey) from new-api; add env loader.
- `constant/`: `ChannelType*` (OpenAI=1,Azure=3,Anthropic=14,Gemini=24,Aws=33,VertexAi=41), roles, key states.
- `model/main.go`: DB init, cross-DB helpers (mirror new-api), automigrate.
- `model/{user,platform,grant,channel,audit}.go`: models + DAO. Channel query DAO must `Omit("enc_key")`.

### Phase 2 — Auth + RBAC  ✅ DONE
- `common/response.go` (AGT envelope), `common/context.go` (ctx keys).
- `middleware/auth.go`: session + access-token auth; `SupplierAuth`/`AdminAuth`/`RootAuth`.
- `controller/auth.go`: login (enum-safe), logout, change-password, self.
- Tests: `middleware/auth_test.go` — 10 RBAC boundary cases green (supplier blocked from
  admin/root, admin `>=` supplier, disabled/bad/missing token rejected).
- Acceptance MET: supplier cannot reach `/api/admin/*`; no path returns a key field.

### Phase 3 — Admin: platforms, users, grants  ✅ DONE
- `crypto/global.go`: process vault + `GlobalSealer()` (write-only `sealerOnly` struct, no Open)
  + `SyncOpener()` (full vault, sync-worker only). TDD test proved the sealer can't be
  downcast to Open — caught a real hole.
- `common/json.go`: Marshal/Unmarshal wrappers + whitelist JSON encode/decode (TEXT, cross-DB).
- `controller/admin_platform.go`: Platform CRUD; AGT token sealed on write, only Last4 returned.
- `controller/admin_user.go`: User CRUD (create supplier accounts, reset password, enable/disable).
- `controller/admin_grant.go`: Grant upsert/list/delete + supplier-role validation.
- Model DAO added: Platform Update/SetModexToken/Delete, User List/UpdateProfile/SetAccessToken/Delete,
  Grant ListAll/Upsert/Delete.
- Tests: platform token sealed-never-plain + JSON-leak guard; delete-in-use refused; sealer write-only.

### Phase 4 — Supplier: upload + sync (destroy-by-default)  ✅ DONE
- `service/validate`: whitelist enforcement (type/models/group/base_url), grant narrows
  platform via set intersection; https-only base_url + allow-list prefix match. 5 tests.
- `service/agt`: AGT client — POST wrapped `{mode,multi_key_mode,channel}` -> data.id,
  PUT flat (id top-level), key omitted on metadata-only update (AGT preserves it).
- `service/sync`: the SOLE decryption site. Opens platform token + channel key in memory,
  forwards to AGT, MarkSynced wipes enc_key on success / MarkFailed retains on failure.
- `controller/supplier_channel.go`: list (no key), create (seal+validate+dispatch sync),
  update (preserve-key-on-absent / rotate-on-present), delete (soft), resync (bounded).
- Model DAO: LoadChannelForSync (only enc_key loader), MarkMetadataSynced, UpdateMetadata,
  ReplaceKey, SoftDeleteForUser, CountChannelsForUserPlatform.
- `internal/architecture/invariant_test.go`: build-breaking guards — controllers never
  reference Open/SyncOpener/crypto.Vault (comments stripped); SyncOpener only in service/sync.
- Tests: end-to-end mock-AGT sync proves key forwarded in correct format AND enc_key wiped
  (raw SELECT); failure retains key; all green. Acceptance MET.

### Phase 5 — Frontend + hardening
- ✅ Backend runnable: `config/` (hard-validates MASTER_KEK/SESSION_SECRET, refuses weak),
  `router/` (auth/supplier/admin groups), `main.go` (boot: config→vault→db→seed root→serve),
  `middleware/security.go` (security headers + login rate limit), root-account seeding.
- ✅ Verified end-to-end with curl against a running server: admin login → create platform
  (AGT token sealed, only last4 returned) → create supplier → grant w/ whitelist narrowing →
  supplier login → whitelist rejections (forbidden model/type) → valid upload → list (no key).
  DB-at-rest grep confirmed plaintext key + AGT token ABSENT from the .db file; not in logs.
  AGT client reached the REAL open.naci-tech.com (got "无效的访问令牌" with the fake token).
- ✅ Infra: `.gitignore`, `.env.example`, `.gitleaks.toml`, `Dockerfile` (static CGO-off),
  `docker-compose.yml` (app+mysql), `README.md`. `config/config_test.go` covers the startup gate.
- ✅ `gofmt` clean, `go vet` clean, full `go test ./...` green.
- ⏳ REMAINING: React frontend (login + supplier upload + admin platforms/users/grants/audit),
  i18n zh/en, run gitleaks in CI, external security review.

### Phase 5b — Frontend  ✅ DONE
- React 18 + Vite + Semi Design SPA under `web/` (zh-CN locale). `npm` via npmmirror.
- `web/src/lib/`: api.js (AGT-envelope fetch wrapper, cookie auth), auth.jsx (context),
  constants.js (6 channel types, role/state labels).
- Pages: Login, SupplierChannels (upload key — write-only UX, whitelist-aware form,
  status tags, resync/delete), AdminPlatforms (AGT token sealed, only last4 shown),
  AdminUsers (create supplier accounts + reset password), AdminGrants (supplier↔platform
  + whitelist narrowing), AdminAudit (paged, filterable).
- App.jsx: role-based routing (admin vs supplier menus), session bootstrap, logout.
- `npm run build` green (3274 modules). Served as a SINGLE binary: `embed.go` go:embed
  web/dist + `router/spa.go` (static + history-API fallback, never shadows /api).
- Verified full-stack: one 28MB binary serves SPA shell, assets (correct MIME), client
  routes, AND same-origin API (cookie, no CORS); API 404s stay JSON.
- Dockerfile now multi-stage (node build → go build → static binary).
- ⏳ REMAINING (optional polish): i18n en, gitleaks-in-CI, external security review.

## Open items (defer, non-blocking)
- KEK→DEK envelope encryption: NOT needed (channel keys transient). Revisit only if we ever keep keys.
- Multi-platform publish per channel: out of scope (one-channel-one-platform locked).
