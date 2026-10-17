# AGENTS.md

This is a personal fork of [jetify-com/devbox](https://github.com/jetify-com/devbox);
it exists to carry process-compose bugfixes and machine-specific tweaks that
upstream doesn't want. Upstream agent instructions would live here too, so
keep this file minimal.

**Read `AGENTS.local.md` if it exists** (git-ignored, may be absent): it holds
the fork-specific process-compose pin upgrade recipe and machine/environment
notes for this checkout.

Fork-specific invariants that ARE worth knowing even without the local file:

- `go.mod` pins `github.com/f1bonacc1/process-compose` to a
  `github.com/jefft/process-compose` pseudo-version (a fork rebased onto an
  upstream tag). `devbox services` runs the daemon from the same fork: the
  global utility project installs the flake
  `github:jefft/process-compose/<processComposeRev>` (`internal/devbox/util.go`).
  The replace's commit and `processComposeRev` must be the same commit —
  `TestProcessComposeRevMatchesGoMod` enforces it.
- `AGENTS.local.md` is git-ignored on purpose (see `.gitignore`); don't
  "helpfully" commit it.
