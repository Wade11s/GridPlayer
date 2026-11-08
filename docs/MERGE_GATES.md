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

Bootstrap exception (first PR only): the repository starts with zero commits and no
branch protection, so no required check can report on a base that does not exist yet.
The bootstrap PR still needs (2)+(3)+(4); after it merges, main gains the workflow and
required checks. If branch protection/costs later prevent checks, merge waits (no
local-green substitution), per §4.2.

Post-merge: full local regression on main before the next PR; red ⇒ freeze merges,
land a minimal revert PR. No force push, no history rewrite, no release, no public publish.
