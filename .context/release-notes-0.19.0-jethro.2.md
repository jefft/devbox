## What's Changed

### Bug Fixes
- **Local-plugin service restarts work again.** Removed the unused project-include implementation that deleted process-compose’s working directory during restarts. Ordinary local `plugin.json` includes remain supported.

### Maintenance
- Rewrote the fork history to remove the project-include change and its revert, retaining subsequent fixes and adapting their plugin references.

This release replaces the rewritten `main` history; existing clones may need to reset or rebase onto it.
