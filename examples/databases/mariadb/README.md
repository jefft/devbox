# mariadb

## mariadb Notes

1. Start the mariadb server using `devbox services up`
1. Create a database using `"mysql --socket-path=$MYSQL_UNIX_PORT --password='' < setup_db.sql"`
1. You can now connect to the database from the command line by running `devbox run connect_db`

## Services

* mariadb

Use `devbox services start|stop [service]` to interact with services

## This plugin sets the following environment variables

* MYSQL_BASEDIR=/<projectDir>/.devbox/nix/profile/default
* MYSQL_HOME=/<projectDir>/.devbox/virtenv/mariadb/run
* MYSQL_DATADIR=/<projectDir>/.devbox/data/mariadb
* MYSQL_UNIX_PORT=/$XDG_RUNTIME_DIR/devbox/<project-hash>/mariadb/mysql.sock (or the temp dir if XDG_RUNTIME_DIR is unset)
* MYSQL_PID_FILE=/$XDG_RUNTIME_DIR/devbox/<project-hash>/mariadb/mysql.pid

To show this information, run `devbox info mariadb`

The default socket path lives outside the project in a short per-user runtime
directory, so it stays under the ~100-character unix socket limit even in
deeply nested projects. You can still point to a different path by setting the
`MYSQL_UNIX_PORT` env variable in your `devbox.json`.
