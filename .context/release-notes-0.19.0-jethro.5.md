## What's Changed

### New Features

- **Startup hook messages in your terminal:** after `devbox services up -b`, custom process-compose hook output — such as a service's URL announced from `on_ready` — is printed in the invoking terminal without an automatic prefix. Service hooks can include their name using `PC_PROCESS_NAME`. Devbox waits up to ten seconds for service readiness and the initial hooks; ordinary service output stays in the logs. Hook events after the command returns, and foreground TUI sessions, behave as before (logs and TUI only).
- **Quieter startup output:** the per-service `Process <name> TCP ports: [...]` listing is gone. It printed empty brackets for log tails and databases, and duplicated the URLs hooks already announce. Listening ports remain available via `process-compose process ports <service>`, the ports API, and the TUI process-info dialog.

The bundled process-compose fork is updated to include the hook-output journal API backing the terminal forwarding.

**Full Changelog:** https://github.com/jefft/devbox/compare/v0.19.0-jethro.4...v0.19.0-jethro.5
