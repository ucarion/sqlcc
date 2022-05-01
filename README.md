# sqlcc - sql change control

`sqlcc` is a simple tool for performing database migrations and making changes
to SQL databases. It supports MySQL, Postgres, and SQLite.

Database migrations are a risky operation, and they routinely interact with the
development experience, version control, test environments, and production data.
`sqlcc` is designed to be as easy to wrap your mind around as possible, so that
it's easy to debug and build on top of.

To that end, `sqlcc`:

* Has just one command to run migrations: `sqlcc migrate`.
* By default, that command runs in dry-run mode.
* Keeps minimal state, and gives you full read (`sqlcc status`) and write
  (`sqlcc reset`) access to that state.
* Does not have "down" migrations, because they:
   * Rarely work if they're written by hand,
   * Are often disastrous if ever run in production, and
   * Are often better replaced with a follow-on "fix" migration instead.

## Installation

```bash
go install github.com/ucarion/sqlcc
```

## Usage

At a high level, the flow for using `sqlcc` is:

* The first time you use `sqlcc` on a database, you run `sqlcc init`.
* Thereafter, you can apply migrations, including any new migrations you've
  added, using `sqlcc migrate`.
* If your migrations ever fail, you can see what migrations `sqlcc` has run with
  `sqlcc status`, and you can reset its state with `sqlcc reset`.
* If you want to validate your migrations are well-formed without talking to a
  database (for instance, as part of a code-linting step), use `sqlcc validate`.

All of the above `sqlcc` invocations require arguments to connect to your
database and find your migrations. Those invocations will look something like:

```sh
# for mysql
sqlcc -m migrations -s sqlcc -D mysql -d 'root:password@tcp(127.0.0.1)/?multiStatements=true' ...
```

```sh
# for postgres
sqlcc -m migrations -s sqlcc -D postgres -d 'postgresql://postgres:password@0.0.0.0:5432/postgres?sslmode=disable' ...
```

```sh
# for sqlite
sqlcc -m migrations -s sqlcc -D sqlite3 -d 'file:example.db' ...
```

Where `-m migrations` indicates the directory containing your migrations, which
should look something like:

```text
migrations/00001_foo.sql
migrations/00002_bar.sql
migrations/00003_baz.sql
```

You can skip migration version numbers, and don't need to be consistent with
your number format, so these are all valid too:

```text
migrations/004_aaa.sql
migrations/005_.sql
migrations/007_aaa.sql
migrations/8_aaa.sql
```

That's the essentials of `sqlcc`. What follows is a more in-depth discussion of
the details of how `sqlcc` works.

### Connecting to a database

To connect to a database, you'll pass the `--driver` (`-D`) and `--dsn` (`-d`)
flags to `sqlcc`. These invocations will look something like:

* for MySQL: 
  
  ```
  sqlcc -D mysql -d 'root:password@tcp(127.0.0.1)/?multiStatements=true' ...
  ```
  
* for Postgres: 

  ```
  sqlcc -D postgres -d 'postgresql://postgres:password@0.0.0.0:5432?sslmode=disable' ...
  ```
  
* for SQLite:

  ```
  sqlcc -D sqlite3 -d 'file:example.db' ...
  ```
  
The syntax for these DSNs (also known as "connection strings") is documented
here:

* MySQL: https://github.com/go-sql-driver/mysql#dsn-data-source-name
* Postgres: https://pkg.go.dev/github.com/lib/pq#hdr-Connection_String_Parameters
* SQLite: https://github.com/mattn/go-sqlite3#connection-string

Note in particular that for MySQL, you will very likely want to set:

```text
multiStatements=true
```

in your DSN, as the example above does. Without this option enabled, you will
get a MySQL syntax error on migrations containing multiple statements. 

When developing against a local Postgres database, it's quite common to set:

```text
sslmode=disable
```

Whether this is required will depend on how your Postgres deployment is set up.

### State Table

`sqlcc` uses a table in your database to keep track of the last migration run.
You specify that table's name using `--state-table` (`-s`). Under the hood, it
looks like this:

```sql
-- XXX is determined by the -s / --state-table argument
create table XXX (version int not null, dirty bool not null);
```

`sqlcc init` creates this table, and inserts a single row into it. `sqlcc
status` reads that row, and `sqlcc reset` overwrites it. `sqlcc migrate` will
also modify it automatically.

### Managing multiple schemas

`sqlcc` can manage multiple SQL schemas in the same database. A "schema" here
refers to a sort of namespace in a SQL database; somewhat confusingly, [MySQL
considers "schemas" to be a synonym for
"databases"](https://dev.mysql.com/doc/refman/8.0/en/glossary.html#glos_schema).

[SQLite considers schemas to be named database
files](https://www.sqlite.org/lang_attach.html), and `sqlcc` can only connect to
a single SQLite database file, so this feature is not applicable to SQLite.

To manage multiple schemas with `sqlcc`, instead of just one, you will want to
do three things:

1. Make sure the DSN (`-d`) you pass to `sqlcc` isn't pointing at a specific
   schema. For instance, instead of:

   ```text
   root:password@tcp(127.0.0.1)/myschema?multiStatements=true
   postgresql://postgres:password@0.0.0.0:5432/myschema?sslmode=disable
   ```
   
   Use something like:

   ```text
   root:password@tcp(127.0.0.1)/?multiStatements=true
   postgresql://postgres:password@0.0.0.0:5432?sslmode=disable
   ```
   
2. Include the name of your schema in the name of the state table (`-s`,
   `--state-table`) you give `sqlcc`. For instance, instead of:

   ```bash
   sqlcc -s mystatetable ...
   ```
   
   Use:

   ```bash
   sqlcc -s myschema.mystatetable ...
   ```

3. Ensure your database migrations don't assume a schema is already chosen. For
   instance, instead of:

   ```sql
   create table widgets (
     -- [...]
   );
   ```
   
   Use something like:

   ```sql
   create table myschema.widgets (
     -- [...]
   );
   ```

After following those steps, `sqlcc` will run across multiple schemas.

### Running migrations in a transaction

By default, `sqlcc migrate` will run in a single transaction on Postgres and
SQLite. [MySQL does not have transactional
DDL](https://dev.mysql.com/doc/refman/5.7/en/cannot-roll-back.html), so `sqlcc`
does not use transactions by default on MySQL.

You can control `sqlcc`'s use of transactions with `--run-in-transaction`
(`-t`), which can be set to `always`, `never`, or `auto` (the default).

Under the hood, `sqlcc migrate` performs the following database operations:

1. Read the version from the `sqlcc` state table,
2. For each migration to be run:
   1. Update the state table as dirty,
   2. Execute the migration query (i.e. the SQL code in your migration file),
   3. Update the state table as non-dirty, and the version to be the migration's
      version

When in transactional mode, this entire process is wrapped in a `begin`/`commit`
(or `rollback` if an error occurs).

You may want to manually disable transactional mode (using `-t never`) to debug
migration errors locally (doing so will let you see intermediary dirty states if
a migration errors out), or to avoid long-running transactions.

You may want to manually enable transactional mode (using `-t always`) if you're
running migrations in MySQL, and you know in advance that you aren't performing
any operations MySQL cannot roll back. `sqlcc` will not verify that your
migrations are rollback-safe.

### Handling failed migrations

If a migration fails (perhaps due to a SQL syntax error, a foreign key
constraint, or I/O errors when communicating with the database), there are two
possible outcomes:

1. If running in [transactional mode](#running-migrations-in-a-transaction),
   then the database returns to its state prior to you running `sqlcc migrate`;
   the entire `sqlcc migrate` process is aborted.
2. Otherwise, the database will be left in a "dirty" state, with a partially-run
   migration.

This section is about handling the second case. In the first case, there isn't
anything to be done to handle the failure.

`sqlcc migrate` will not perform migrations against a database whose state is
marked as dirty. You will need to "clean up" the state by doing the following:

1. Identify the last "clean" version of the database, using `sqlcc status`,
   which will output something like:

   ```text
   723 (dirty)
   ```

   That means the last cleanly-executed migration was for version 723. The next
   migration after 723 is the one that failed.

2. Manually get the database back to a state that is expected by version 723.

   For instance, if your next migration created two columns, but the second 
   column creation command had a syntax error, then you should drop the first
   column to get back to a state that looks like the one 723 could have left 
   behind.

3. Manually reset `sqlcc`'s state to the last good one. Do so using `sqlcc reset
   N`, where `N` is the last clean state. In the example above that was 723, so
   you'd run:

   ```bash
   sqlcc reset 723
   ```

You can then proceed from there. Usually, before attempting to run `sqlcc
migrate` again, you will want to fix your migrations (e.g. fixing SQL syntax
errors, patching data that prevents index creation, etc.).

Alternatively, in a development environment you could always restore from a
snapshot. Or simply wipe your database entirely, reinitialize `sqlcc`, and
re-run all migrations.

### Validating migrations

`sqlcc` can validate that a migrations directory is well-formed without
connecting to a database. Do so by running `sqlcc validate`:

```bash
sqlcc -m migrations validate
```

This will validate that your migrations directory is well-formed, which means
that every `.sql` file in the directory starts with a series of digits followed
by an underscore. The digits before the underscore are taken to be the
migration's version, and no two migrations can have the same version. It's ok to
skip versions.

`sqlcc validate` is intended to be used in CI environments, as part of a code
linting step.
