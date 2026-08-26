# stellar-disbursement-platform-backend (Ebioro fork)

Ebioro's fork of the upstream Stellar Disbursement Platform backend (public repository, Go). Upstream is `stellar/stellar-disbursement-platform-backend`; this fork tracks it and carries Ebioro's additions on feature branches (e.g. external TSA signing for tenant distribution accounts). Ebioro uses SDP as an internal bulk-payout engine behind its own products; SDP's own dashboard is not an Ebioro end-user surface.

## Architecture
- Go 1.24 monorepo: `cmd/` (CLI + servers), `internal/` (serve, transaction submission service, provisioning, multitenant), `db/` (migrations), `stellar-auth`, `stellar-multitenant`, `contracts/` (Soroban), `internal/serve/sep24frontend` (Vite app with its own npm lockfile).
- Two HTTP servers: the per-tenant SDP API and the multi-tenant admin API. Postgres per install, tenant-scoped schemas.
- Upstream docs: https://developers.stellar.org/docs/platforms/stellar-disbursement-platform

## Dev commands
- `make go-install` / `make go-build` / `make go-test` / `make go-check` (lint, shadow, exhaustive, deadcode, goimports) — `make ci` runs them all. Match upstream's checks before opening a PR; they run in CI.
- `docker compose` under `dev/` for a local stack; see upstream README "Quick Start".

## Gotchas
- **`develop` mirrors upstream.** Ebioro changes live on `feat/*` branches that are periodically merged with `upstream/develop`. Never rebase or rewrite upstream history, and never open Ebioro-specific PRs against `develop` as if it were an Ebioro branch — keep the fork mergeable.
- **Keep Ebioro changes upstream-shaped**: same package layout, same test conventions, `exhaustive` switch handling, no new global config without a flag. Divergence is paid for at every upstream sync.
- Amount handling upstream is strict about precision (over-precision amounts are rejected); do not loosen `ValidateAmount`.
- Dependabot and release automation in `.github/` are upstream's; don't enable or edit them on the fork without checking what secrets they expect.

<!-- BEGIN ebioro-non-negotiables v2 — master: Ebioro-UAB/documentation -->
## Ebioro non-negotiables

- **GitHub text hygiene — the KU corridor country is never named.** In any
  GitHub-visible text (commit messages, PR titles/bodies, reviews, issues,
  branch names, release notes, code/spec comments) write `KU` — never the
  country's name, demonym, or capital. The ISO code `CU` as functional data
  (string literals, catalog entries, `=== 'CU'` checks) and full-country
  datasets (countries.json, i18n locales) are fine. End-user UI strings may
  carry the real name; keep those literals minimal. Scrub old text on touch.
- **Never merge or push to `main`, never tag a production release, never deploy.**
  Open the PR and stop. Merges and deploys are human-only — no exception for
  "the review passed" or "it's just a patch bump". Never delete `main`,
  `master`, or `development`.
- **Never commit `.env` or any file containing a secret.** `.gitignore` covers
  `.env*` with `.env.example` as the only tracked variant. A secret that lands
  in git is leaked even after the file is removed — rotate it.
- **No AI attribution in git or GitHub text.** No `Co-Authored-By:` lines, no
  "Generated with …" footers in commits, PR bodies, or issues.
- **Error handling: `neverthrow` Result types. Never `try/catch`.**
- **TypeORM migrations: snake_case column identifiers only.** Quoting
  `"customerId"` preserves camelCase; TypeORM then queries `customer_id` and
  crashes at runtime. `build` does not catch it. This has cost two fix
  migrations already.
- **Follow the existing flow in code, not design docs or mockups.** Find the
  nearest equivalent already implemented and match it. Design notes are
  proposals.
- **Money paths**: integer stroops/cents, never floats. Idempotency keys on
  anything that moves money. Stellar sequence numbers fetched fresh. Never log
  or expose a signing key or an API secret.
- **Ebioro never holds keys for customer funds.** Before creating any key or
  account, ask whose funds it will hold. If a customer's, it cannot be a key
  Ebioro can use alone.
- **User-facing copy hides blockchain jargon.** "Payment reference", not
  "transaction hash". "Settled", not "confirmed on ledger N". "Network fee",
  not "XLM base fee". No signing-vendor names. It should read like a bank app,
  not a block explorer.
- **No regulatory claims** in user-facing text — not "licensed", "registered",
  "authorised", "MiCAR-compliant", or any variant. Route legal-sounding copy
  through Ebioro before merging.
- **Soft-delete only** (`deletedAt`). Never hard-delete.
- **Never skip pre-commit hooks** (`--no-verify`). Flag new dependencies in the
  PR description.
- **Never paste production data, customer PII, credentials, or KYC/AML content
  into an AI tool.** Anonymised or synthetic only.
<!-- END ebioro-non-negotiables v2 -->
