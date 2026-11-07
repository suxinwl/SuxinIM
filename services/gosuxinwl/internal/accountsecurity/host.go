package accountsecurity

import (
	"context"
	"database/sql"
	"os"
	"sync"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
)

type readyStore struct {
	store *Store
	once  sync.Once
	err   error
}

var stores sync.Map

// Default initializes additive schema once per host database connection. It
// never reads a private configuration file or places credentials in errors.
func Default(ctx context.Context) (*Store, error) {
	db, e := g.DB().Master()
	if e != nil {
		return nil, e
	}
	key := struct {
		DB     *sql.DB
		Prefix string
	}{db, g.DB().GetConfig().Prefix}
	value, _ := stores.LoadOrStore(key, &readyStore{})
	ready := value.(*readyStore)
	ready.once.Do(func() {
		ready.store, ready.err = New(db, key.Prefix)
		if ready.err == nil {
			ready.store.IMEnabled = os.Getenv("GOSUXIN_IM_ENABLED") == "true"
			ready.err = ready.store.Ensure(ctx)
		}
	})
	return ready.store, ready.err
}
