package dish

import "context"

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) CreateDish(ctx context.Context, name, description string) (Dish, error) {
	return s.repo.Create(ctx, name, description)
}

func (s *Service) ListDishes(ctx context.Context) ([]Dish, error) {
	return s.repo.List(ctx)
}

func (s *Service) GetDish(ctx context.Context, id int64) (Dish, error) {
	return s.repo.Get(ctx, id)
}

func (s *Service) CountDishes(ctx context.Context) (int64, error) {
	return s.repo.Count(ctx)
}
