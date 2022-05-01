package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"

	"github.com/ucarion/cli"
)

func main() {
	cli.Run(context.Background(), validate, init_, status, reset, migrate)
}

type rootArgs struct {
	Driver     string `cli:"-D,--driver" value:"mysql|postgres|sqlite3" usage:"database driver to use"`
	DSN        string `cli:"-d,--dsn" value:"dsn" usage:"database connection string"`
	StateTable string `cli:"-s,--state-table" value:"table-name" usage:"name of table for keeping track of which migrations have been run"`
	Migrations string `cli:"-m,--migrations" value:"dir" usage:"directory containing migration sql files"`
	RunInTx    string `cli:"-t,--run-in-transaction" value:"auto|always|never" usage:"run migrations in a transaction; default is 'auto', which uses transactions for postgres and sqlite3"`
}

func (a rootArgs) Description() string {
	return "sql change control"
}

func (a rootArgs) ExtendedDescription() string {
	return strings.TrimSpace(`
sqlcc is a simple tool for running database migrations.

You start using sqlcc by running:

    sqlcc init (see: sqlcc-init.1)

You can then run migrations using:

    sqlcc migrate (see: sqlcc-migrate.1)

If things go wrong, you can inspect sqlcc's state by running:

    sqlcc status (see: sqlcc-status.1)

And you can manually reset sqlcc's state with:

    sqlcc reset (see: sqlcc-reset.1)

To validate that your migrations directory is well-formed, use:

    sqlcc validate (see: sqlcc-validate.1)

For further documentation beyond this manual, see:

    https://github.com/ucarion/sqlcc
`)
}

func (a rootArgs) ExtendedUsage_Driver() string {
	return strings.TrimSpace(`
Database driver to use. Valid values are mysql, postgres, or sqlite3. This
parameter is required.
`)
}

func (a rootArgs) ExtendedUsage_DSN() string {
	return strings.TrimSpace(`
Data source name ("DSN", also known as a "connection string") of the database.
This parameter is required.

Some examples of valid DSNs are:

	root:password@tcp(127.0.0.1)/?multiStatements=true (for mysql)

	postgresql://postgres:password@0.0.0.0:5432?sslmode=disable (for postgres)

	example.db (for sqlite3)

The syntax of these DSNs are documented here:

	https://github.com/go-sql-driver/mysql#dsn-data-source-name (for mysql)

	https://pkg.go.dev/github.com/lib/pq#hdr-Connection_String_Parameters (for postgres)

	https://github.com/mattn/go-sqlite3#connection-string (for sqlite3)

Note in particular that for MySQL, you will very likely want to set

	multiStatements=true

in your DSN, as the example above does. Without this option enabled, you will
get a MySQL syntax error on migrations containing multiple statements.
`)
}

func (a rootArgs) ExtendedUsage_StateTable() string {
	return strings.TrimSpace(`
Name of the table sqlcc will use to keep state. This parameter is required.

In order to keep track of what migrations sqlcc has previously run on a
database, sqlcc writes its last performed operation in a table in that same
database. This flag controls what that table's name is.

For use-cases where migrations are controlling multiple MySQL "databases" or
Postgres "schemas", you may include the database/schema name, using the usual
schema_name.table_name SQL syntax. In such a use-case, you will want to ensure
that your DSN does not specify a database/schema.
`)
}

func (a rootArgs) ExtendedUsage_Migrations() string {
	return strings.TrimSpace(`
Directory containing migrations. This parameter is required.

Migrations are plain SQL files in your migrations directory. The only special
requirement is that their names start with a number, followed by an underscore.
For example, this is a valid migrations directory:

	migrations/00001_foo.sql

	migrations/2_bar.sql

	migrations/003_.sql
`)
}

func (a rootArgs) ExtendedUsage_RunInTx() string {
	return strings.TrimSpace(`
Whether to run operations in a transaction. Valid values are "auto", "never",
and "always". Default is "auto", which enables transactional mode for Postgres
and SQLite, but not MySQL.

When transactional mode is enabled, sqlcc will run all operations, including
executing user migrations, in a single transaction.
`)
}

func (a rootArgs) validate(noDB bool) error {
	if a.Migrations == "" {
		return fmt.Errorf("-m/--migrations is required")
	}

	// if we're not validating db-related state, go no further
	if noDB {
		return nil
	}

	switch a.Driver {
	case "mysql", "postgres", "sqlite3":
		// noop
	case "":
		return fmt.Errorf("-D/--driver is required")
	default:
		return fmt.Errorf("invalid -D/--driver: must be one of mysql, postgres, or sqlite3")
	}

	if a.DSN == "" {
		return fmt.Errorf("-d/--dsn is required")
	}

	if a.StateTable == "" {
		return fmt.Errorf("-s/--state-table is required")
	}

	switch a.RunInTx {
	case "", "auto", "always", "never":
		// noop
	default:
		return fmt.Errorf("invalid -t/--run-in-transaction: must be one of auto, always, or never")
	}

	return nil
}

func (a rootArgs) withTx(ctx context.Context, f func(queryer) error) error {
	db, err := sql.Open(a.Driver, a.DSN)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}

	return withTx(ctx, a.runInTx(), db, f)
}

func (a rootArgs) runInTx() bool {
	switch a.RunInTx {
	case "always":
		return true
	case "never":
		return false
	case "", "auto":
		switch a.Driver {
		case "mysql":
			return false
		case "postgres", "sqlite3":
			return true
		default:
			panic("unreachable")
		}
	default:
		panic("unreachable")
	}
}

type validateArgs struct {
	RootArgs rootArgs `cli:"validate,subcmd"`
}

func (a validateArgs) Description() string {
	return "validate sqlcc migrations"
}

func (a validateArgs) ExtendedDescription() string {
	return strings.TrimSpace(`
sqlcc validate checks that the migrations directory is well-formed.

See the documentation for --migrations in sqlcc.1 for details on what makes a
well-formed migrations dir.
`)
}

func validate(_ context.Context, args validateArgs) error {
	if err := args.RootArgs.validate(true); err != nil {
		return err
	}

	_, err := parseMigrations(args.RootArgs.Migrations)
	return err
}

type initArgs struct {
	RootArgs rootArgs `cli:"init,subcmd"`
}

func (a initArgs) Description() string {
	return "validate sqlcc migrations"
}

func (a initArgs) ExtendedDescription() string {
	return strings.TrimSpace(`
sqlcc init creates a new sqlcc state table.
`)
}

func init_(ctx context.Context, args initArgs) error {
	if err := args.RootArgs.validate(false); err != nil {
		return err
	}

	return args.RootArgs.withTx(ctx, func(q queryer) error {
		return initState(ctx, args.RootArgs.StateTable, q)
	})
}

type statusArgs struct {
	RootArgs rootArgs `cli:"status,subcmd"`
}

func (a statusArgs) Description() string {
	return "get sqlcc version state"
}

func (a statusArgs) ExtendedDescription() string {
	return strings.TrimSpace(`
sqlcc gets the current state from a sqlcc state table.

Outputs to stdout the current version followed by the string " (dirty)" if it is
marked as dirty.
`)
}

func status(ctx context.Context, args statusArgs) error {
	if err := args.RootArgs.validate(false); err != nil {
		return err
	}

	var s state
	if err := args.RootArgs.withTx(ctx, func(q queryer) error {
		var err error
		s, err = getState(ctx, args.RootArgs.StateTable, q)
		return err
	}); err != nil {
		return err
	}

	if s.dirty {
		fmt.Printf("%d (dirty)\n", s.version)
	} else {
		fmt.Printf("%d\n", s.version)
	}

	return nil
}

type resetArgs struct {
	RootArgs rootArgs `cli:"reset,subcmd"`
	Version  uint     `cli:"version"`
	Dirty    bool     `cli:"--dirty"`
}

func (a resetArgs) Description() string {
	return "set sqlcc version state"
}

func (a resetArgs) ExtendedDescription() string {
	return strings.TrimSpace(`
sqlcc reset the current state from a sqlcc state table.

Outputs to stdout the current version followed by the string " (dirty)" if it is
marked as dirty.
`)
}

func reset(ctx context.Context, args resetArgs) error {
	if err := args.RootArgs.validate(false); err != nil {
		return err
	}

	return args.RootArgs.withTx(ctx, func(q queryer) error {
		return setState(ctx, args.RootArgs.StateTable, q, state{
			version: int(args.Version),
			dirty:   args.Dirty,
		})
	})
}

type migrateArgs struct {
	RootArgs rootArgs `cli:"migrate,subcmd"`
	Force    bool     `cli:"-f,--force"`
}

func migrate(ctx context.Context, args migrateArgs) error {
	if err := args.RootArgs.validate(false); err != nil {
		return err
	}

	if !args.Force {
		_, _ = fmt.Fprintln(os.Stderr, "running in dry-run mode because '--force' was not provided")
	}

	migrations, err := parseMigrations(args.RootArgs.Migrations)
	if err != nil {
		return err
	}

	return args.RootArgs.withTx(ctx, func(q queryer) error {
		state, err := getState(ctx, args.RootArgs.StateTable, q)
		if err != nil {
			return err
		}

		if state.dirty {
			return fmt.Errorf("state is dirty, will not migrate")
		}

		// advance to first migration after current state
		var i int
		for i < len(migrations) && migrations[i].version <= state.version {
			i++
		}

		// run all migrations thereafter
		for i < len(migrations) {
			fmt.Println(migrations[i].name)

			if args.Force {
				state.dirty = true
				if err := setState(ctx, args.RootArgs.StateTable, q, state); err != nil {
					return err
				}

				if _, err := q.ExecContext(ctx, migrations[i].query); err != nil {
					return fmt.Errorf("exec %q: %w", migrations[i].name, err)
				}

				state.dirty = false
				state.version = migrations[i].version
				if err := setState(ctx, args.RootArgs.StateTable, q, state); err != nil {
					return err
				}
			}

			i++
		}

		return nil
	})
}
