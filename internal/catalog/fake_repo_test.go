package catalog_test

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/cosmofactory/recetas-backend/internal/catalog"
)

const fakeVersion int64 = 5

// Fake repository methods, for failing them and counting their calls.
const (
	methodContentVersion = "ContentVersion"
	methodRegions        = "Regions"
	methodDishes         = "Dishes"
	methodRecipe         = "Recipe"
)

// fakeRepo serves the catalogtest fixture at a content version. It records
// every call as "<method>[/<locale>[/<dish>/<region>]]" and can fail any
// method with a configured error.
type fakeRepo struct {
	mu      sync.Mutex
	version int64
	empty   bool // no content imported yet: version 0, no rows
	fail    map[string]error
	calls   map[string]int
}

var _ catalog.Repository = (*fakeRepo)(nil)

func newFakeRepo() *fakeRepo {
	return &fakeRepo{version: fakeVersion, fail: map[string]error{}, calls: map[string]int{}}
}

func newEmptyFakeRepo() *fakeRepo {
	r := newFakeRepo()
	r.version, r.empty = 0, true
	return r
}

func (r *fakeRepo) setVersion(v int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.version = v
}

func (r *fakeRepo) failWith(method string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fail[method] = err
}

func (r *fakeRepo) clearFailures() {
	r.mu.Lock()
	defer r.mu.Unlock()
	clear(r.fail)
}

// record counts a call and returns the failure configured for method.
func (r *fakeRepo) record(method string, parts ...string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls[strings.Join(append([]string{method}, parts...), "/")]++
	return r.fail[method]
}

// count returns how often key was called ("Regions/en").
func (r *fakeRepo) count(key string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls[key]
}

func (r *fakeRepo) totalCalls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, c := range r.calls {
		n += c
	}
	return n
}

func (r *fakeRepo) ContentVersion(context.Context) (int64, error) {
	if err := r.record(methodContentVersion); err != nil {
		return 0, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.version, nil
}

func (r *fakeRepo) Regions(_ context.Context, l catalog.Locale) ([]catalog.Region, error) {
	if err := r.record(methodRegions, string(l)); err != nil {
		return nil, err
	}
	if r.empty {
		return nil, nil
	}
	return domainRegions(l), nil
}

func (r *fakeRepo) Dishes(_ context.Context, l catalog.Locale) ([]catalog.Dish, error) {
	if err := r.record(methodDishes, string(l)); err != nil {
		return nil, err
	}
	if r.empty {
		return nil, nil
	}
	return domainDishes(l), nil
}

func (r *fakeRepo) Recipe(_ context.Context, dishKey, regionKey string, l catalog.Locale) (catalog.Recipe, error) {
	if err := r.record(methodRecipe, string(l), dishKey, regionKey); err != nil {
		return catalog.Recipe{}, err
	}
	rc, ok := domainRecipe(dishKey, regionKey, l)
	if r.empty || !ok {
		return catalog.Recipe{}, fmt.Errorf("fake recipe %s/%s: %w", dishKey, regionKey, catalog.ErrNotFound)
	}
	return rc, nil
}
