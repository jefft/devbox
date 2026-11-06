## What's Changed

* **Rebased onto upstream 0.19.0 — "Devbox is its own launcher"** — release builds of devbox no longer rely on a bash launcher script in front of them: the binary picks the version to run (`DEVBOX_USE_VERSION`, otherwise the current project's pinned version), downloads it on demand, verifies it against `checksums.txt`, and runs it directly, so per-command startup overhead drops sharply and the daily update check no longer blocks commands. `"on_mismatch": "auto"` projects now switch to their pinned version before doing any work. Upstream details: https://github.com/jetify-com/devbox/releases/tag/0.19.0, by @mikeland73 (upstream)

* **No fork behavioral changes** — the forked process-compose daemon pin (`6a4d08cc`), plugin lifecycle patches, service env injection, and fork-specific workflows are carried forward unchanged. The nix flake now builds with Go 1.27, matching upstream.

**Full Changelog**: https://github.com/jefft/devbox/compare/v0.18.5-jethro.2...v0.19.0-jethro.1
