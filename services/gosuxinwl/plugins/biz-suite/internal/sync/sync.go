// Package sync contains the business sync registry. It intentionally has no
// dependency on the host scheduler, so workers and ETL tools can reuse it.
package sync

import (
	"context"
	"fmt"
)

type Job func(context.Context) (int, error)

type Registry struct{ jobs map[string]Job }

func New() *Registry { return &Registry{jobs: make(map[string]Job)} }

func (r *Registry) Register(name string, job Job) { r.jobs[name] = job }

func (r *Registry) Run(ctx context.Context, name string) (int, error) {
	job, ok := r.jobs[name]
	if !ok {
		return 0, fmt.Errorf("unknown sync job %q", name)
	}
	return job(ctx)
}
