# redis
GoFrame `gredis.Adapter` implements using `go-redis`.

# Installation
```
go get -u github.com/suxinwl/GoSuxin/framework/contrib/nosql/redis
```

Commonly imported at top of `main.go`:
```go
package main

import (
	_ "github.com/suxinwl/GoSuxin/framework/contrib/nosql/redis"

	// Other imported packages.
)

func main() {
	// Main logics.
}
```
