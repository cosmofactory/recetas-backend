package dish_test

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/cosmosfactory/recetas-backend/internal/dish"
	"github.com/cosmosfactory/recetas-backend/internal/dish/dishdb"
	"github.com/cosmosfactory/recetas-backend/internal/platform/testdb"
)

func newTxRepo(t *testing.T) dish.Repository {
	t.Helper()
	return dish.NewPGRepository(dishdb.New(testdb.Tx(t)))
}

func mustCreate(t *testing.T, repo dish.Repository, name, description string) dish.Dish {
	t.Helper()
	d, err := repo.Create(context.Background(), name, description)
	if err != nil {
		t.Fatalf("Create(%q): got err %v, want nil", name, err)
	}
	return d
}

func TestIntegrationPGRepositoryCreate(t *testing.T) {
	tests := []struct {
		name        string
		dishName    string
		description string
	}{
		{"plain", "Tortilla de patatas", "Potato omelette."},
		{"accented", "Pulpo á feira", "Galician-style octopus with pimentón."},
		{"cyrillic", "Гаспачо", "Холодный суп."},
		{"empty description", "Salmorejo", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newTxRepo(t)

			got := mustCreate(t, repo, tc.dishName, tc.description)

			if got.ID <= 0 {
				t.Errorf("ID: got %d, want > 0", got.ID)
			}
			if got.Name != tc.dishName {
				t.Errorf("Name: got %q, want %q", got.Name, tc.dishName)
			}
			if got.Description != tc.description {
				t.Errorf("Description: got %q, want %q", got.Description, tc.description)
			}
			if got.CreatedAt.IsZero() {
				t.Error("CreatedAt: got zero time, want non-zero")
			}
			if got.UpdatedAt.IsZero() {
				t.Error("UpdatedAt: got zero time, want non-zero")
			}
		})
	}
}

func TestIntegrationPGRepositoryGet(t *testing.T) {
	t.Run("returns the created dish", func(t *testing.T) {
		repo := newTxRepo(t)
		want := mustCreate(t, repo, "Pulpo á feira", "Octopus, paprika, olive oil.")

		got, err := repo.Get(context.Background(), want.ID)
		if err != nil {
			t.Fatalf("Get(%d): got err %v, want nil", want.ID, err)
		}
		if got.ID != want.ID || got.Name != want.Name || got.Description != want.Description {
			t.Errorf("Get(%d): got %+v, want %+v", want.ID, got, want)
		}
		if !got.CreatedAt.Equal(want.CreatedAt) {
			t.Errorf("CreatedAt: got %v, want %v", got.CreatedAt, want.CreatedAt)
		}
		if !got.UpdatedAt.Equal(want.UpdatedAt) {
			t.Errorf("UpdatedAt: got %v, want %v", got.UpdatedAt, want.UpdatedAt)
		}
	})

	t.Run("missing id returns ErrNotFound", func(t *testing.T) {
		repo := newTxRepo(t)

		_, err := repo.Get(context.Background(), math.MaxInt64)
		if !errors.Is(err, dish.ErrNotFound) {
			t.Errorf("Get(MaxInt64): got err %v, want %v", err, dish.ErrNotFound)
		}
	})
}

func TestIntegrationPGRepositoryList(t *testing.T) {
	repo := newTxRepo(t)
	created := []dish.Dish{
		mustCreate(t, repo, "Fabada asturiana", "Bean stew."),
		mustCreate(t, repo, "Escalivada", "Roasted vegetables."),
		mustCreate(t, repo, "Papas arrugadas", "Wrinkly potatoes with mojo."),
	}

	got, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf("List: got err %v, want nil", err)
	}

	byID := make(map[int64]dish.Dish, len(got))
	for _, d := range got {
		byID[d.ID] = d
	}
	for _, want := range created {
		d, ok := byID[want.ID]
		if !ok {
			t.Errorf("List: dish %d (%q) missing", want.ID, want.Name)
			continue
		}
		if d.Name != want.Name || d.Description != want.Description {
			t.Errorf("List: dish %d got %+v, want %+v", want.ID, d, want)
		}
	}

	// ListDishes orders by created_at DESC. Rows created in one transaction
	// share NOW(), so only the non-increasing invariant is checked.
	for i := 1; i < len(got); i++ {
		if got[i].CreatedAt.After(got[i-1].CreatedAt) {
			t.Errorf("List order: got created_at[%d]=%v after created_at[%d]=%v, want created_at DESC",
				i, got[i].CreatedAt, i-1, got[i-1].CreatedAt)
		}
	}
}

func TestIntegrationPGRepositoryCount(t *testing.T) {
	const n = 3
	repo := newTxRepo(t)
	ctx := context.Background()

	before, err := repo.Count(ctx)
	if err != nil {
		t.Fatalf("Count before: got err %v, want nil", err)
	}
	for i := range n {
		mustCreate(t, repo, "Cocido madrileño", string(rune('a'+i)))
	}
	after, err := repo.Count(ctx)
	if err != nil {
		t.Fatalf("Count after: got err %v, want nil", err)
	}

	if after-before != n {
		t.Errorf("Count delta: got %d, want %d", after-before, n)
	}
}

func TestIntegrationTxRollsBackAfterTest(t *testing.T) {
	var id int64
	t.Run("create inside tx", func(t *testing.T) {
		repo := newTxRepo(t)
		id = mustCreate(t, repo, "Leche frita", "Fried milk.").ID
	})

	repo := newTxRepo(t) // skips here too when TEST_DATABASE_URL is unset
	if id == 0 {
		t.Fatal("subtest did not record a dish id")
	}

	_, err := repo.Get(context.Background(), id)
	if !errors.Is(err, dish.ErrNotFound) {
		t.Errorf("Get(%d) after subtest cleanup: got err %v, want %v (row should be rolled back)",
			id, err, dish.ErrNotFound)
	}
}
