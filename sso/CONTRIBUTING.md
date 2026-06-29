# Contributing to cotton-id

cotton-id is built **spec-first** with [OpenSpec](https://github.com/Fission-AI/OpenSpec). Every
capability is **proposed, designed, specified, and broken into tasks before any code is written**.
This keeps a production identity provider — where a mistake can leak credentials or mint bad tokens —
honest: the behavior is agreed and reviewable as testable requirements, and the code is built to
satisfy them.

Please read this before opening a change.

---

## The spec-driven-development (SDD) workflow

A change moves through these stages. The artifacts live under
`openspec/changes/<change-name>/`; the archived, accepted specs live under `openspec/specs/`.

```
   propose  ──►  design  ──►  specs  ──►  tasks  ──►  apply  ──►  archive
  proposal.md   design.md   specs/*    tasks.md   write code   merge specs
   the "why"    decisions   SHALL +    checklist  to satisfy   into the
   & scope      & rationale WHEN/THEN  of work    the specs    baseline
```

1. **Propose** — `openspec/changes/<change-name>/proposal.md`. *Why* the change exists, *what*
   changes, the capabilities it adds/modifies, and the impact (new systems, APIs, security surface,
   ops). Keep it additive and scoped.
2. **Design** — `design.md`. The architecture decisions and their rationale, written as numbered
   decisions (D1, D2, …) with alternatives considered, plus risks/trade-offs, migration plan, and
   open questions. This is where genuinely hard calls are argued (e.g. "delegate OIDC to Hydra").
3. **Specs** — `specs/<capability>/spec.md`. The **testable requirements**: `### Requirement: …`
   blocks each stating a `SHALL`, followed by `#### Scenario:` blocks in **WHEN / THEN** form. These
   are the acceptance criteria — code and tests must satisfy them verbatim.
4. **Tasks** — `tasks.md`. A checklist (`- [ ]`) breaking the work into reviewable units, grouped by
   area. Implementers tick boxes as they land work.
5. **Apply** — write the code and tests to satisfy the specs. Stay inside the contract
   ([docs/dev/build-contract.md](docs/dev/build-contract.md)) so independently-built slices compose.
6. **Archive** — once the change is delivered and the tasks are complete, archive it so its specs
   merge into the project's baseline specs.

### OpenSpec CLI

The CLI (`openspec`, v1.4.x) drives and validates the workflow:

```bash
openspec list                       # list active changes
openspec list --specs               # list baseline specs (capabilities)
openspec show <change-name>         # show a change (proposal/design/specs/tasks)
openspec status <change-name>       # artifact completion status for a change
openspec validate <change-name>     # validate a change is well-formed (run before review)
openspec validate --strict          # stricter validation
openspec archive <change-name>      # archive a completed change, merging its specs
openspec view                       # interactive dashboard of specs and changes
```

Always run `openspec validate <change-name>` before requesting review, and again before archiving.

### The current change

The repository is implementing one change: **`bootstrap-platform-and-oidc-login`** — the walking
skeleton (runtime substrate + email/password → consent → token). Its artifacts:

- [`proposal.md`](openspec/changes/bootstrap-platform-and-oidc-login/proposal.md)
- [`design.md`](openspec/changes/bootstrap-platform-and-oidc-login/design.md)
- [`specs/`](openspec/changes/bootstrap-platform-and-oidc-login/specs/) — `platform-foundation`,
  `password-authentication`, `oidc-provider`
- [`tasks.md`](openspec/changes/bootstrap-platform-and-oidc-login/tasks.md)

---

## The build contract

[`docs/dev/build-contract.md`](docs/dev/build-contract.md) is the **binding interface contract**:
exact package names, routes, request/response shapes, DB schema, env vars, and ports. Because slices
are built concurrently (often by different contributors/agents), they only compose if everyone
follows the contract exactly.

- Treat the contract as authoritative for *shapes*; treat the specs as authoritative for *behavior*.
- If a genuine deviation is unavoidable, implement the most sensible thing **and flag it clearly** in
  your PR description so the contract can be reconciled.
- `cmd/cotton-id/main.go` is the **only** file that wires packages together. Package authors expose
  constructors and `Routes(r chi.Router)` / handler structs and **never** edit `main.go`'s siblings.

---

## Building & testing

The stack runs in Docker; the canonical build uses the `golang:1.25` and `node:24` images. Locally,
Go is obtained via `GOTOOLCHAIN=auto` (the repo targets **Go 1.25**, but your machine may have an
older toolchain — `auto` fetches the right one).

### The whole stack

```bash
cp deploy/.env.example deploy/.env
docker compose -f deploy/docker-compose.yml up --build
```

### Backend (Go)

```bash
cd backend
go build ./...        # MUST build clean
go vet ./...          # MUST be vet-clean
gofmt -l .            # MUST be empty (no unformatted files)
go test ./...         # new logic MUST carry tests
```

A `Makefile` aggregates common targets (e.g. `make test`, `make swagger`). After changing any HTTP
handler annotation, regenerate Swagger so it doesn't drift:

```bash
make swagger          # runs `swag init`, regenerating backend/docs/
```

### Frontend (React + TypeScript)

```bash
cd frontend
npm ci
npm run typecheck     # tsc --noEmit — MUST be clean
npm run build         # vite build — MUST be clean
npm test              # Vitest + Testing Library
```

### Quality gates (every contributor)

These mirror build-contract §8 and are enforced in CI:

- Backend `go build ./...`, `go vet ./...`, and `gofmt` clean; new logic carries tests.
- Frontend `tsc --noEmit` and `vite build` clean.
- **Every new endpoint carries `swaggo` annotations** so it appears in Swagger; a CI drift check
  fails if generated docs fall out of sync.
- **No secret values committed**; `.env.example` documents every variable.
- **Security events** (login ok/fail, signup, reset, consent, client registration) are logged with
  structured fields — and never with plaintext passwords or full session tokens.

---

## Coding conventions

### Go

- Idiomatic Go, standard `cmd/` + `internal/` layout, layered by domain (`config`, `database`,
  `httpx`, `auth`, `oidc`, `adminapi`, `observability`, `mailer`).
- `gofmt`-formatted; `go vet`-clean. Errors are wrapped with context (`fmt.Errorf("...: %w", err)`).
- HTTP errors are emitted as RFC 7807 `application/problem+json` via the `httpx` error helpers —
  **never** leak stack traces or internal details to clients.
- Logging via `log/slog` (JSON), always carrying the request id. **Never** log plaintext passwords
  or full session tokens.
- Expose **interfaces at domain seams** (e.g. `Authenticator`, `Mailer`, the rate-limiter) so later
  changes add implementations without editing existing packages.
- New logic carries tests; security-sensitive logic (hashing, sessions, CSRF, rate limiting, claims)
  is tested directly.

### Frontend

- React + TypeScript, **strict** `tsconfig`; ESLint + Prettier clean.
- Design tokens are ported **verbatim** from `_design_ref/` (the approved prototype) into
  `src/styles/tokens.css`; UI primitives are typed reimplementations of the prototype components.
- All user-facing copy goes through the typed RU/EN i18n dictionary — **no hard-coded strings**.
- API access goes through the typed `apiClient` (`credentials: 'include'`, CSRF header injection,
  problem+json parsing) — don't call `fetch` ad hoc.

### Database

- Schema changes are **forward-only**, numbered SQL migrations
  (`backend/migrations/NNNN_name.up.sql` + `.down.sql`). `down` files are for local dev only.
- Match the schema in build-contract §4 exactly (column names, types, nullability, defaults).

### Security mindset

This is an IdP handling credentials and driving token issuance. When in doubt, choose the more
conservative option and document the trade-off. Read [docs/SECURITY.md](docs/SECURITY.md) — it
records the baseline decisions and the **known gaps** every change inherits. Don't silently widen a
gap; if you must, flag it.

---

## Commits & pull requests

- Branch from `main`; keep commits scoped and messages descriptive (Conventional-Commit-style
  prefixes like `feat:`, `fix:`, `chore:`, `docs:` are used in this repo).
- Reference the OpenSpec change your work implements, and tick the relevant boxes in its `tasks.md`.
- Before opening a PR: run the build/test/lint gates above, regenerate Swagger if endpoints changed,
  and run `openspec validate <change-name>`.
- The PR description should call out any deviation from the build contract or any security gap
  touched, so reviewers can reconcile it.

Welcome aboard — spec first, then code.
