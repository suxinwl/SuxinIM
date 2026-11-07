module github.com/openimsdk/openim-sdk-core/v3

go 1.26.0

require (
	github.com/coder/websocket v1.8.13
	github.com/golang/protobuf v1.5.4
	github.com/gorilla/websocket v1.4.2
	github.com/jinzhu/copier v0.4.0
	github.com/pkg/errors v0.9.1
	google.golang.org/protobuf v1.35.1 // indirect
	gorm.io/driver/sqlite v1.5.5
)

require (
	golang.org/x/crypto v0.58.0 // indirect
	golang.org/x/mod v0.42.0 // indirect
	golang.org/x/net v0.61.0 // indirect
	golang.org/x/tools v0.52.0 // indirect
)

require (
	github.com/google/go-cmp v0.6.0
	github.com/hashicorp/golang-lru/v2 v2.0.7
	github.com/openimsdk/protocol v0.0.73-alpha.12
	github.com/openimsdk/tools v0.0.50-alpha.80
	github.com/patrickmn/go-cache v2.1.0+incompatible
	golang.org/x/image v0.47.0
	golang.org/x/sync v0.24.0
	gorm.io/gorm v1.25.10
)

require (
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/lestrrat-go/strftime v1.0.6 // indirect
	github.com/mattn/go-sqlite3 v1.14.22 // indirect
	go.uber.org/atomic v1.7.0 // indirect
	go.uber.org/multierr v1.6.0 // indirect
	go.uber.org/zap v1.24.0 // indirect
	golang.org/x/mobile v0.0.0-20261011034528-197770444867
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/text v0.43.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240903143218-8af14fe29dc1 // indirect
	google.golang.org/grpc v1.68.0 // indirect
)

tool golang.org/x/mobile/cmd/gobind

require puty.local/nativestore v0.0.0

require puty.local/localstore v0.0.0

replace github.com/mattn/go-sqlite3 => ../../sdks/native-storage/go-sqlite3

replace puty.local/nativestore => ../../sdks/native-storage/store

replace puty.local/localstore => ../../sdks/local-store-native
