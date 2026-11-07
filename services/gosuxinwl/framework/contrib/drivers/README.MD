
# Database drivers

Powerful database drivers for package gdb.

## Installation

Let's take `mysql` for example.

```shell
go get github.com/suxinwl/GoSuxin/framework/contrib/drivers/mysql@latest

# Easy for copying:
go get github.com/suxinwl/GoSuxin/framework/contrib/drivers/clickhouse@latest
go get github.com/gogf/gf/contrib/drivers/dm/v2@latest
go get github.com/gogf/gf/contrib/drivers/gaussdb/v2@latest
go get github.com/gogf/gf/contrib/drivers/mariadb/v2@latest
go get github.com/suxinwl/GoSuxin/framework/contrib/drivers/mssql@latest
go get github.com/gogf/gf/contrib/drivers/oceanbase/v2@latest
go get github.com/suxinwl/GoSuxin/framework/contrib/drivers/oracle@latest
go get github.com/suxinwl/GoSuxin/framework/contrib/drivers/pgsql@latest
go get github.com/suxinwl/GoSuxin/framework/contrib/drivers/sqlite@latest
go get github.com/gogf/gf/contrib/drivers/sqlitecgo/v2@latest
go get github.com/gogf/gf/contrib/drivers/tidb/v2@latest
```

Choose and import the driver to your project:

```go
import _ "github.com/suxinwl/GoSuxin/framework/contrib/drivers/mysql"
```

Commonly imported at top of `main.go`:

```go
package main

import (
	_ "github.com/suxinwl/GoSuxin/framework/contrib/drivers/mysql"

	// Other imported packages.
)

func main() {
	// Main logics.
}
```

## Supported Drivers

### MySQL

```go
import _ "github.com/suxinwl/GoSuxin/framework/contrib/drivers/mysql"
```

### MariaDB

```go
import _ "github.com/gogf/gf/contrib/drivers/mariadb/v2"
```

### TiDB

```go
import _ "github.com/gogf/gf/contrib/drivers/tidb/v2"
```

### OceanBase

```go
import _ "github.com/gogf/gf/contrib/drivers/oceanbase/v2"
```

### GaussDB

```go
import _ "github.com/gogf/gf/contrib/drivers/gaussdb/v2"
```

### SQLite

```go
import _ "github.com/suxinwl/GoSuxin/framework/contrib/drivers/sqlite"
```

#### cgo version

When the target is a `32-bit` Windows system, the `cgo` version needs to be used.

```go
import _ "github.com/gogf/gf/contrib/drivers/sqlitecgo/v2"
```

### PostgreSQL

```go
import _ "github.com/suxinwl/GoSuxin/framework/contrib/drivers/pgsql"
```

### SQL Server

```go
import _ "github.com/suxinwl/GoSuxin/framework/contrib/drivers/mssql"
```

Note:

- `InsertIgnore` returns error if there is no primary key or unique index submitted with record.
- It supports server version >= `SQL Server2005`
- It ONLY supports `datetime2` and `datetimeoffset` types for auto handling created_at/updated_at/deleted_at columns, 
  because datetime type does not support microseconds precision when column value is passed as string.

### Oracle

```go
import _ "github.com/suxinwl/GoSuxin/framework/contrib/drivers/oracle"
```

Note:

- It does not support `LastInsertId`.
- `InsertIgnore` returns error if there is no primary key or unique index submitted with record.

### ClickHouse

```go
import _ "github.com/suxinwl/GoSuxin/framework/contrib/drivers/clickhouse"
```

Note:

- It does not support `InsertIgnore/InsertAndGetId` features.
- It does not support `Save/Replace` features.
- It does not support `Transaction` feature.
- It does not support `RowsAffected` feature.

### DM

```go
import _ "github.com/gogf/gf/contrib/drivers/dm/v2"
```

Note:

- `InsertIgnore` returns error if there is no primary key or unique index submitted with record.

## Custom Drivers

It's quick and easy, please refer to current driver source.
It's quite appreciated if any PR for new drivers support into current repo.
