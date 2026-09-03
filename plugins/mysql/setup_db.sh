#! bash

# Migrate the data dir from the pre-0.0.8 location (.devbox/virtenv/<plugin>/data).
OLD_DATA_DIR="$(dirname "$(dirname "$MYSQL_DATADIR")")/virtenv/$(basename "$MYSQL_DATADIR")/data"
if [ -d "$OLD_DATA_DIR" ] && [ -z "$(ls -A "$MYSQL_DATADIR" 2>/dev/null)" ]; then
  mkdir -p "$(dirname "$MYSQL_DATADIR")"
  rmdir "$MYSQL_DATADIR" 2>/dev/null || true
  mv "$OLD_DATA_DIR" "$MYSQL_DATADIR"
fi

# Check for the system schema ('mysql' db), not the datadir itself: since the
# 0.0.8 directory lifecycle, devbox pre-creates an empty $MYSQL_DATADIR before
# init hooks run, which made the old '[ ! -d ]' guard skip initialization.
if [ ! -d "$MYSQL_DATADIR/mysql" ]; then
  # Install the Database
  mkdir -p $MYSQL_DATADIR
  mysqld --initialize-insecure --basedir=$MYSQL_BASEDIR
fi

# Create run directory for socket files if it doesn't exist
MYSQL_RUN_DIR="$(dirname $MYSQL_UNIX_PORT)"
if [ ! -d "$MYSQL_RUN_DIR" ]; then
  mkdir -p -m 700 "$MYSQL_RUN_DIR"
fi

if [ -e "$MYSQL_CONF" ]; then
  ln -fs "$MYSQL_CONF" "$MYSQL_HOME/my.cnf"
fi
