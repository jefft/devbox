# mysql

## mysql Notes

1. Start the mysql server using `devbox services up`
1. Create a database using `"mysql -u root --password='' < setup_db.sql"`
1. You can now connect to the database from the command line by running `devbox run connect_db`

## Services

* mysql

Use `devbox services start|stop [service]` to interact with services

## This plugin sets the following environment variables

* MYSQL_BASEDIR=&lt;projectDir>/.devbox/nix/profile/default
* MYSQL_HOME=&lt;projectDir>/.devbox/virtenv/mysql/run
* MYSQL_DATADIR=&lt;projectDir>/.devbox/data/mysql
* MYSQL_UNIX_PORT=&lt;$XDG_RUNTIME_DIR>/devbox/&lt;project-hash>/mysql/mysql.sock (or the temp dir if XDG_RUNTIME_DIR is unset)
* MYSQL_PID_FILE=&lt;$XDG_RUNTIME_DIR>/devbox/&lt;project-hash>/mysql/mysql.pid

To show this information, run `devbox info mysql`

The default socket path lives outside the project in a short per-user runtime
directory, so it stays under the ~100-character unix socket limit even in
deeply nested projects. You can still point to a different path by setting the
`MYSQL_UNIX_PORT` env variable in your `devbox.json`.
