module puty.local/nativestore

go 1.26.0

require (
	github.com/mattn/go-sqlite3 v1.14.22
	puty.local/localstore v0.0.0
)

require golang.org/x/crypto v0.57.0 // indirect

replace github.com/mattn/go-sqlite3 => ../go-sqlite3

replace puty.local/localstore => ../../local-store-native
