## What's Changed

### New Features

- **Custom service lifecycle notifications:** the bundled process-compose fork supports `on_start`, `on_ready`, `on_exit`, and `on_ports_changed` command blocks, plus a project-level `on_ready`. Hooks use the configured shell and service environment, with `PC_*` variables and JSON event metadata for process identity, listening ports, and exit details.
- **Listening-port discovery:** port-change hooks report TCP and UDP listeners, including child processes. Discovery runs once per second while subscribed services are running; readiness hooks also receive a port snapshot.
- **Ports after startup:** `devbox services up -b` reports TCP ports for running services and their dependencies. Reporting also runs after the foreground TUI detaches while leaving services running.

Hook command failures and timeouts are logged without failing the service. Configure a readiness probe or `ready_log_line` when a service takes time to bind its listeners.

**Full Changelog:** https://github.com/jefft/devbox/compare/v0.19.0-jethro.3...v0.19.0-jethro.4
