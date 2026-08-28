#! bash

# Migrate PGDATA from the pre-0.0.4 location (.devbox/virtenv/postgresql/data).
OLD_DATA_DIR="$(dirname "$(dirname "$PGDATA")")/virtenv/$(basename "$PGDATA")/data"
if [ -d "$OLD_DATA_DIR" ] && [ -z "$(ls -A "$PGDATA" 2>/dev/null)" ]; then
  mkdir -p "$(dirname "$PGDATA")"
  rmdir "$PGDATA" 2>/dev/null || true
  mv "$OLD_DATA_DIR" "$PGDATA"
fi
