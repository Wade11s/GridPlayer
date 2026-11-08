# Merge Gates (implements MIGRATION_SPEC §5)

Every PR merges only when ALL hold for the exact head SHA:

1. Scope: change is inside the frozen matrix (`docs/BEHAVIOR_MATRIX.md`) or a
   bootstrap/infra doc; no feature expansion.
2. Evidence: PR body lists Issue, base/head SHA, fixture/spec digests, tool versions,
   test results, artifact paths. `not run` ≠ pass.
3. Independent review: fresh `codex exec` context reads spec + frozen acceptance +
   exact diff + repro commands and writes a SHA-bound report in the PR (never the writer).
   No reliable entry ⇒ fail closed.
4. Local macOS media/GUI tests pass on head + latest base (screenshots, decoded-mode logs).
5. Hosted checks real: `ci.yml` (ubuntu, free tier) — `go vet`, `go build ./...`,
   `go test ./internal/...` with libVLC installed via apt. No macOS runner, no paid CI.
   Post-bootstrap, CI is a required status check.
6. SHA re-check immediately before merge (head/base drift ⇒ reviews/results invalid ⇒ re-run).

Bootstrap path (empty repository): the first commit (`029ec3c`) creates `main` because
GitHub cannot open a pull request before a base branch exists. The PoC content landed
in `94753c1` before that PR could exist; it is recorded as pre-gate scaffolding and is
**not** claimed as a gated merge. This PR re-validates the whole tree at its head SHA
with the independent review, local acceptance and hosted checks below; every later
change is PR-first. No required check is waived: same-repo `pull_request` workflows run
from the PR head branch, so the bootstrap PR's own CI is real.

After this PR, enable branch protection with the `ci` check if the repository plan
supports it; if it does not, report the limitation rather than substituting local green.

Post-merge: full local regression on main before the next PR; red ⇒ freeze merges,
land a minimal revert PR. No force push, no history rewrite, no release, no public publish.
