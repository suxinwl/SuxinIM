package enterprise

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var ErrNotFound = errors.New("record not found")
var ErrConflict = errors.New("record changed; retry")

// Document lives exclusively in the enterprise database. Version makes concurrent
// state transitions safe across multiple processes, rather than just goroutines.
type Document struct {
	ID      string    `bson:"_id"`
	Kind    string    `bson:"kind"`
	Owner   string    `bson:"owner"`
	Version int64     `bson:"version"`
	Data    []byte    `bson:"data"`
	Updated time.Time `bson:"updated"`
}
type Store interface {
	Get(context.Context, string) (Document, error)
	List(context.Context, string, string) ([]Document, error)
	Save(context.Context, Document, int64) error
	Delete(context.Context, string, int64) error
	Ping(context.Context) error
}
type MongoStore struct{ c *mongo.Collection }

func NewMongoStore(ctx context.Context, c *mongo.Client, db string) (*MongoStore, error) {
	s := &MongoStore{c: c.Database(db).Collection("enterprise_documents")}
	_, err := s.c.Indexes().CreateMany(ctx, []mongo.IndexModel{{Keys: bson.D{{Key: "kind", Value: 1}, {Key: "owner", Value: 1}, {Key: "updated", Value: -1}}}})
	return s, err
}
func (s *MongoStore) Get(ctx context.Context, id string) (Document, error) {
	var d Document
	err := s.c.FindOne(ctx, bson.M{"_id": id}).Decode(&d)
	if errors.Is(err, mongo.ErrNoDocuments) {
		err = ErrNotFound
	}
	return d, err
}
func (s *MongoStore) List(ctx context.Context, kind, owner string) ([]Document, error) {
	f := bson.M{"kind": kind}
	if owner != "" {
		f["owner"] = owner
	}
	cur, err := s.c.Find(ctx, f, options.Find().SetSort(bson.D{{Key: "updated", Value: -1}, {Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var ds []Document
	err = cur.All(ctx, &ds)
	return ds, err
}
func (s *MongoStore) Save(ctx context.Context, d Document, old int64) error {
	d.Version = old + 1
	d.Updated = time.Now().UTC()
	if old == 0 {
		_, err := s.c.InsertOne(ctx, d)
		if mongo.IsDuplicateKeyError(err) {
			return ErrConflict
		}
		return err
	}
	r, err := s.c.ReplaceOne(ctx, bson.M{"_id": d.ID, "version": old}, d)
	if err == nil && r.MatchedCount != 1 {
		return ErrConflict
	}
	return err
}
func (s *MongoStore) Delete(ctx context.Context, id string, v int64) error {
	r, err := s.c.DeleteOne(ctx, bson.M{"_id": id, "version": v})
	if err == nil && r.DeletedCount != 1 {
		return ErrConflict
	}
	return err
}
func (s *MongoStore) Ping(ctx context.Context) error { return s.c.Database().Client().Ping(ctx, nil) }

// MemoryStore is for deterministic domain and HTTP tests; production has no
// volatile-storage fallback if MongoDB is unavailable.
type MemoryStore struct {
	mu   sync.RWMutex
	docs map[string]Document
}

func NewMemoryStore() *MemoryStore { return &MemoryStore{docs: map[string]Document{}} }
func (s *MemoryStore) Get(_ context.Context, id string) (Document, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.docs[id]
	if !ok {
		return d, ErrNotFound
	}
	d.Data = append([]byte(nil), d.Data...)
	return d, nil
}
func (s *MemoryStore) List(_ context.Context, kind, owner string) ([]Document, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ds := []Document{}
	for _, d := range s.docs {
		if d.Kind == kind && (owner == "" || d.Owner == owner) {
			d.Data = append([]byte(nil), d.Data...)
			ds = append(ds, d)
		}
	}
	return ds, nil
}
func (s *MemoryStore) Save(_ context.Context, d Document, v int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.docs[d.ID]
	if (v == 0 && ok) || (v > 0 && (!ok || old.Version != v)) {
		return ErrConflict
	}
	d.Version = v + 1
	d.Updated = time.Now().UTC()
	d.Data = append([]byte(nil), d.Data...)
	s.docs[d.ID] = d
	return nil
}
func (s *MemoryStore) Delete(_ context.Context, id string, v int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.docs[id]
	if !ok || d.Version != v {
		return ErrConflict
	}
	delete(s.docs, id)
	return nil
}
func (s *MemoryStore) Ping(context.Context) error { return nil }
func document(id, kind, owner string, v any) Document {
	b, _ := json.Marshal(v)
	return Document{ID: id, Kind: kind, Owner: owner, Data: b}
}
func load[T any](ctx context.Context, s Store, id string) (T, Document, error) {
	var v T
	d, err := s.Get(ctx, id)
	if err == nil {
		err = json.Unmarshal(d.Data, &v)
	}
	return v, d, err
}
func mutate[T any](ctx context.Context, s Store, id string, fn func(*T) error) (T, error) {
	var zero T
	for i := 0; i < 16; i++ {
		v, d, err := load[T](ctx, s, id)
		if err != nil {
			return zero, err
		}
		if err = fn(&v); err != nil {
			return zero, err
		}
		d.Data, _ = json.Marshal(v)
		err = s.Save(ctx, d, d.Version)
		if errors.Is(err, ErrConflict) {
			continue
		}
		return v, err
	}
	return zero, ErrConflict
}
