{
  description = "A flake that outputs MariaDB with custom configuration and aliases to work in Devbox";

  inputs = {
    nixpkgs.url = "{{.URLForInput}}";
  };

  outputs = {self, nixpkgs}:
    let
      mariadb-bin =  nixpkgs.legacyPackages.{{.System}}.symlinkJoin {

        name = "mariadb-wrapped";
        paths = [nixpkgs.legacyPackages.{{ .System }}.{{.PackageAttributePath}}];
        nativeBuildInputs = [ nixpkgs.legacyPackages.{{.System}}.makeWrapper];
        postBuild = ''

          wrapProgram $out/bin/mariadbd \
            --add-flags '--defaults-file=''$MYSQL_CONF --basedir=$out --datadir=''$MYSQL_DATADIR --pid-file=''$MYSQL_PID_FILE --socket=''$MYSQL_UNIX_PORT';

          if [-f $out/bin/mariadbd-safe]; then
            wrapProgram $out/bin/mariadbd_safe \
              --add-flags '--defaults-file=''$MYSQL_CONF --basedir=$out --datadir=''$MYSQL_DATADIR --pid-file=''$MYSQL_PID_FILE --socket=''$MYSQL_UNIX_PORT';
          fi

          if [-f $out/bin/mariadb-install-db]; then
            wrapProgram "$out/bin/mariadb_install_db" \
              --add-flags '--basedir=$out --datadir=''$MYSQL_DATADIR --pid-file=''$MYSQL_PID_FILE --basedir=''$MYSQL_BASEDIR';
          fi

          wrapProgram $out/bin/mariadb-admin \
            --add-flags '--defaults-file=''$MYSQL_CONF --socket=''$MYSQL_UNIX_PORT';

          wrapProgram $out/bin/mariadb \
            --add-flags '--defaults-file=''$MYSQL_CONF --socket=''$MYSQL_UNIX_PORT';

          wrapProgram $out/bin/mariadb-dump \
            --add-flags '--defaults-file=''$MYSQL_CONF --socket=''$MYSQL_UNIX_PORT';

          # Don't wrap 'mysql' or any mysql binaries, as they are correctly left as symlinks

        '';
      };
    in{
      packages.{{.System}} = {
        default = mariadb-bin;
      };
    };
}
