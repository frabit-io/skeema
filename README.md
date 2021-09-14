# Go La Tengo

[![build status](https://img.shields.io/github/workflow/status/skeema/tengo/Tests/main)](https://github.com/skeema/tengo/actions)
[![code coverage](https://img.shields.io/coveralls/skeema/tengo.svg)](https://coveralls.io/r/skeema/tengo)
[![godoc](https://img.shields.io/badge/godoc-reference-blue.svg)](https://godoc.org/github.com/skeema/tengo)
[![latest release](https://img.shields.io/github/release/skeema/tengo.svg)](https://github.com/skeema/tengo/releases)

Golang library for MySQL and MariaDB database automation

## Features

### Schema introspection and diff

Go La Tengo examines several `information_schema` tables in order to build Go struct values representing schemas (databases), tables, columns, indexes, foreign key constraints, stored procedures, and functions. These values can be diff'ed to generate corresponding DDL statements.

### Instance modeling

The `tengo.Instance` struct models a single database instance. It keeps track of multiple, separate connection pools for using different default schema and session settings. This helps to avoid problems with Go's database/sql methods, which are incompatible with USE statements and SET SESSION statements.

## Status

This is package is battle-tested from years of production use at many companies. The release numbering is still pre-1.0 though as the API is subject to minor changes. Backwards-incompatible changes are generally avoided whenever possible, but no guarantees are made. 

As of September 2021, open source development of this repo is mostly frozen until further notice.

### Supported databases

Tagged releases are tested against the following databases, all running on Linux:

* MySQL 5.5 - 8.0
* Percona Server 5.5 - 8.0
* MariaDB 10.1 - 10.6

Outside of a tagged release, every commit to the main branch is automatically tested against MySQL 5.7 and 8.0.

### Unsupported in table diffs

Go La Tengo **cannot** diff tables containing any of the following MySQL features:

* spatial indexes
* sub-partitioning (two levels of partitioning in the same table)
* special features of non-InnoDB storage engines

Go La Tengo also does not yet support rename operations, e.g. column renames or table renames.

### Ignored object types

The following object types are completely ignored by this package. Their presence won't break anything, but they will not be introspected or represented by the structs in this package.

* views
* triggers
* events
* grants / users / roles

## External Dependencies

* https://github.com/go-sql-driver/mysql (Mozilla Public License 2.0)
* https://github.com/jmoiron/sqlx (MIT License)
* https://github.com/VividCortex/mysqlerr (MIT License)
* https://github.com/fsouza/go-dockerclient (BSD License)
* https://github.com/pmezard/go-difflib/difflib (BSD License)
* https://github.com/nozzle/throttler (Apache License 2.0)
* https://golang.org/x/sync/errgroup (BSD License)

## Credits

Created and maintained by [@evanelias](https://github.com/evanelias).

Additional [contributions](https://github.com/skeema/tengo/graphs/contributors) by:

* [@tomkrouper](https://github.com/tomkrouper)
* [@efixler](https://github.com/efixler)
* [@chrisjpalmer](https://github.com/chrisjpalmer)
* [@alexandre-vaniachine](https://github.com/alexandre-vaniachine)
* [@mhemmings](https://github.com/mhemmings)

Support for stored procedures and functions generously sponsored by [Psyonix](https://psyonix.com).

Support for partitioned tables generously sponsored by [Etsy](https://www.etsy.com).

## License

**Copyright 2021 Skeema LLC**

```text
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
```


