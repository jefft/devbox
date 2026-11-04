## What's Changed

### New Features
- **Project directory exports:** shells, scripts, and custom services receive `DEVBOX_RUNTIME_DIR`, `DEVBOX_LOG_DIR`, and `DEVBOX_DATA_DIR` before project environment expansion. Paths share the plugin directory calculation and replace inherited values from other projects.
- Plugin services retain their plugin-specific directory values. Exporting paths does not create directories.
