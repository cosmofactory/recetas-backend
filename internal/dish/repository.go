package dish

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/cosmosfactory/recetas-backend/internal/dish/dishdb"
)

var ErrNotFound = errors.New("dish: not found")

type Repository interface {
	Create(ctx context.Context, name, description string) (Dish, error)
	List(ctx context.Context) ([]Dish, error)
	Get(ctx context.Context, id int64) (Dish, error)
	Count(ctx context.Context) (int64, error)
}

type pgRepository struct {
	q *dishdb.Queries
}

func NewPGRepository(q *dishdb.Queries) Repository {
	return &pgRepository{q: q}
}

func (r *pgRepository) Create(ctx context.Context, name, description string) (Dish, error) {
	row, err := r.q.CreateDish(ctx, dishdb.CreateDishParams{
		Name:        name,
		Description: description,
	})
	if err != nil {
		return Dish{}, fmt.Errorf("create dish: %w", err)
	}
	return toDomain(row), nil
}

func (r *pgRepository) List(ctx context.Context) ([]Dish, error) {
	rows, err := r.q.ListDishes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list dishes: %w", err)
	}
	out := make([]Dish, 0, len(rows))
	for _, row := range rows {
		out = append(out, toDomain(row))
	}
	return out, nil
}

func (r *pgRepository) Get(ctx context.Context, id int64) (Dish, error) {
	row, err := r.q.GetDish(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Dish{}, ErrNotFound
		}
		return Dish{}, fmt.Errorf("get dish: %w", err)
	}
	return toDomain(row), nil
}

func (r *pgRepository) Count(ctx context.Context) (int64, error) {
	numberOfDishes, err := r.q.CountDishes(ctx)
	if err != nil {
		return 0, fmt.Errorf("count dishes: %w", err)
	}
	return numberOfDishes, nil
}

func toDomain(d dishdb.Dish) Dish {
	return Dish{
		ID:          d.ID,
		Name:        d.Name,
		Description: d.Description,
		CreatedAt:   d.CreatedAt.Time,
		UpdatedAt:   d.UpdatedAt.Time,
	}
}
