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
  upstream tag). The `require` version and the
  `internal/devbox/util.go` `processComposeVersion` const must stay equal —
  `TestProcessComposeVersionMatchesGoMod` enforces it.
- `AGENTS.local.md` is git-ignored on purpose (see `.gitignore`); don't
  "helpfully" commit it.
