# Custom plugin example

Shows how to write custom local plugin. Plugins can:

* Install packages
* Create templatized files (including flakes)
* Declare services (using process-compose)

The `include` entry accepts more than plugins: it can also point at another
project — a directory containing a `devbox.json`, or a `devbox.json` file
directly. The including project extends the included one: environment
variables are overridden by the including project, scripts and services are
inherited, and relative paths (like `env_from`) resolve against the included
project's own directory. See the "Including another project" section of
`plugins/README.md` for details.
