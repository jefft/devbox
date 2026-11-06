## What's Changed

### Changed Defaults

- **Quieter diagnostics:** Devbox now defaults to `--loglevel=WARN`, hiding routine `Info:` messages such as `Running script ...` and `Ensuring packages are installed`. Warnings and errors remain visible. Use `--loglevel=INFO` or root-level `--verbose` to restore informational messages; `--loglevel=ERROR` hides warnings too. Level names are case-insensitive, and an explicit `--loglevel` takes precedence over `--verbose`.
- **Aligned hook messages:** startup lifecycle hook output is printed without automatic `[service]` or `[project]` prefixes. Service hooks can include their name explicitly using `$PC_PROCESS_NAME`.

### New Features

- **Inherited diagnostic levels:** `DEVBOX_LOG_LEVEL` supplies the default threshold and carries the selected level into nested Devbox commands. Invalid levels fail before the command runs. Command results, success confirmations, hook messages, and child-process output are not filtered by this threshold.
- **Optional management-port announcement:** the process-compose API port is hidden during normal background startup and shown at INFO verbosity. This is separate from application listening ports announced by hooks. The existing `devbox version --verbose` option still prints detailed version information.

**Full Changelog:** https://github.com/jefft/devbox/compare/v0.19.0-jethro.5...v0.19.0-jethro.6
