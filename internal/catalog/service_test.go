package catalog_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/cosmofactory/recetas-backend/internal/catalog"
)

func assertDeepEqual(t *testing.T, what string, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s:\ngot  %+v\nwant %+v", what, got, want)
	}
}

func assertVersion(t *testing.T, got, want int64) {
	t.Helper()
	if got != want {
		t.Errorf("got content version %d, want %d", got, want)
	}
}

func assertNotFound(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, catalog.ErrNotFound) {
		t.Errorf("got error %v, want ErrNotFound", err)
	}
}

func TestService_RegionsKeepRepositoryOrder(t *testing.T) {
	t.Parallel()
	svc := catalog.NewService(newFakeRepo())
	for _, l := range locales {
		t.Run(string(l), func(t *testing.T) {
			got, version, err := svc.Regions(context.Background(), l)
			if err != nil {
				t.Fatalf("Regions: %v", err)
			}
			assertVersion(t, version, fakeVersion)
			assertDeepEqual(t, "regions", got, domainRegions(l))
		})
	}
}

func TestService_DishesKeepRepositoryOrder(t *testing.T) {
	t.Parallel()
	svc := catalog.NewService(newFakeRepo())
	for _, l := range locales {
		t.Run(string(l), func(t *testing.T) {
			got, version, err := svc.Dishes(context.Background(), l)
			if err != nil {
				t.Fatalf("Dishes: %v", err)
			}
			assertVersion(t, version, fakeVersion)
			assertDeepEqual(t, "dishes", got, domainDishes(l))
		})
	}
}

func TestService_DishReturnsOneDishWithAllVariations(t *testing.T) {
	t.Parallel()
	svc := catalog.NewService(newFakeRepo())
	for _, key := range []string{"tortilla", "cocido"} {
		t.Run(key, func(t *testing.T) {
			got, version, err := svc.Dish(context.Background(), key, "ru")
			if err != nil {
				t.Fatalf("Dish(%q): %v", key, err)
			}
			assertVersion(t, version, fakeVersion)
			assertDeepEqual(t, "dish", got, domainDishByKey(t, key, "ru"))
		})
	}
}

// Asturias has a tortilla variation (dish order 1) and a cocido one (dish
// order 3); Galicia only the tortilla one.
func TestService_RegionListsEveryVariationInDishOrder(t *testing.T) {
	t.Parallel()
	cases := []struct {
		region        string
		wantRecipeKey []string
	}{
		{"ast", []string{"tortilla-chorizo", "fabada-asturiana"}},
		{"gal", []string{"tortilla-betanzos"}},
	}
	svc := catalog.NewService(newFakeRepo())
	for _, tc := range cases {
		t.Run(tc.region, func(t *testing.T) {
			got, version, err := svc.Region(context.Background(), tc.region, "es")
			if err != nil {
				t.Fatalf("Region(%q): %v", tc.region, err)
			}
			assertVersion(t, version, fakeVersion)
			assertDeepEqual(t, "recipe keys", recipeKeys(got.Variations), tc.wantRecipeKey)
			assertDeepEqual(t, "region detail", got, domainRegionDetail(t, tc.region, "es"))
		})
	}
}

func recipeKeys(vs []catalog.RegionVariation) []string {
	var out []string
	for _, v := range vs {
		out = append(out, v.RecipeKey)
	}
	return out
}

func TestService_RecipeReturnsTheVariationRecipe(t *testing.T) {
	t.Parallel()
	svc := catalog.NewService(newFakeRepo())
	for _, p := range fixtureRecipePairs {
		t.Run(p.dish+"/"+p.region, func(t *testing.T) {
			got, version, err := svc.Recipe(context.Background(), p.dish, p.region, "ru")
			if err != nil {
				t.Fatalf("Recipe: %v", err)
			}
			assertVersion(t, version, fakeVersion)
			assertDeepEqual(t, "recipe", got, mustDomainRecipe(t, p, "ru"))
		})
	}
}

func TestService_UnknownKeysAreNotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cases := []struct {
		name string
		call func(svc *catalog.Service) error
	}{
		{"region", func(svc *catalog.Service) error { _, _, err := svc.Region(ctx, "nope", "en"); return err }},
		{"dish", func(svc *catalog.Service) error { _, _, err := svc.Dish(ctx, "nope", "en"); return err }},
		{"recipe of unknown dish", func(svc *catalog.Service) error { _, _, err := svc.Recipe(ctx, "nope", "gal", "en"); return err }},
		{"recipe of unknown region", func(svc *catalog.Service) error { _, _, err := svc.Recipe(ctx, "tortilla", "nope", "en"); return err }},
		{"recipe of pair without variation", func(svc *catalog.Service) error { _, _, err := svc.Recipe(ctx, "cocido", "gal", "en"); return err }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertNotFound(t, tc.call(catalog.NewService(newFakeRepo())))
		})
	}
}

func TestService_BeforeAnyImportListsAreEmptyAndDetailsNotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := catalog.NewService(newEmptyFakeRepo())

	regions, version, err := svc.Regions(ctx, "en")
	if err != nil || len(regions) != 0 || version != 0 {
		t.Errorf("Regions: got (%d regions, version %d, %v), want (0, 0, nil)", len(regions), version, err)
	}
	dishes, version, err := svc.Dishes(ctx, "en")
	if err != nil || len(dishes) != 0 || version != 0 {
		t.Errorf("Dishes: got (%d dishes, version %d, %v), want (0, 0, nil)", len(dishes), version, err)
	}
	_, _, err = svc.Region(ctx, "gal", "en")
	assertNotFound(t, err)
	_, _, err = svc.Dish(ctx, "tortilla", "en")
	assertNotFound(t, err)
}

// memoStep is one service call followed by the expected cumulative number of
// repository list loads.
type memoStep struct {
	name        string
	call        func(svc *catalog.Service) error
	bumpVersion int64 // when non-zero, the repository moves to this version first
	wantLoads   map[string]int
}

func TestService_MemoisesListsPerLocaleAndContentVersion(t *testing.T) {
	ctx := context.Background()
	regions := func(l catalog.Locale) func(*catalog.Service) error {
		return func(svc *catalog.Service) error { _, _, err := svc.Regions(ctx, l); return err }
	}
	dishes := func(l catalog.Locale) func(*catalog.Service) error {
		return func(svc *catalog.Service) error { _, _, err := svc.Dishes(ctx, l); return err }
	}
	steps := []memoStep{
		{name: "first regions en loads", call: regions("en"), wantLoads: map[string]int{"Regions/en": 1}},
		{name: "repeat regions en is memoised", call: regions("en"), wantLoads: map[string]int{"Regions/en": 1}},
		{name: "regions ru loads its own list", call: regions("ru"), wantLoads: map[string]int{"Regions/en": 1, "Regions/ru": 1}},
		{name: "first dishes en loads", call: dishes("en"), wantLoads: map[string]int{"Dishes/en": 1}},
		{name: "dish detail uses the memoised list", call: func(svc *catalog.Service) error {
			_, _, err := svc.Dish(ctx, "tortilla", "en")
			return err
		}, wantLoads: map[string]int{"Dishes/en": 1}},
		{name: "region detail uses the memoised lists", call: func(svc *catalog.Service) error {
			_, _, err := svc.Region(ctx, "ast", "en")
			return err
		}, wantLoads: map[string]int{"Regions/en": 1, "Dishes/en": 1}},
		{name: "new content version reloads regions", bumpVersion: 6, call: regions("en"), wantLoads: map[string]int{"Regions/en": 2, "Regions/ru": 1}},
		{name: "new content version reloads dishes", call: dishes("en"), wantLoads: map[string]int{"Dishes/en": 2}},
		{name: "repeat at new version is memoised", call: dishes("en"), wantLoads: map[string]int{"Dishes/en": 2}},
	}
	repo := newFakeRepo()
	svc := catalog.NewService(repo)
	for _, step := range steps {
		runMemoStep(t, svc, repo, step)
	}
}

func runMemoStep(t *testing.T, svc *catalog.Service, repo *fakeRepo, step memoStep) {
	t.Helper()
	if step.bumpVersion != 0 {
		repo.setVersion(step.bumpVersion)
	}
	if err := step.call(svc); err != nil {
		t.Fatalf("%s: %v", step.name, err)
	}
	for key, want := range step.wantLoads {
		if got := repo.count(key); got != want {
			t.Errorf("%s: got %d %s calls, want %d", step.name, got, key, want)
		}
	}
}

func TestService_ReturnsTheNewContentVersionAfterAnImport(t *testing.T) {
	t.Parallel()
	repo := newFakeRepo()
	svc := catalog.NewService(repo)
	if _, _, err := svc.Dishes(context.Background(), "en"); err != nil {
		t.Fatalf("Dishes: %v", err)
	}
	repo.setVersion(fakeVersion + 1)

	_, version, err := svc.Dishes(context.Background(), "en")
	if err != nil {
		t.Fatalf("Dishes: %v", err)
	}
	assertVersion(t, version, fakeVersion+1)
}

var errRepoDown = errors.New("connection refused: host=10.0.0.7 password=hunter2")

func missingRuDish() *catalog.MissingTranslationError {
	return &catalog.MissingTranslationError{Entity: "dish", Key: "tortilla", Locale: "ru"}
}

// serviceCall runs one service method; failsAt is the repository method whose
// failure it must surface.
type serviceCall struct {
	name    string
	failsAt string
	call    func(svc *catalog.Service) error
}

func serviceCalls() []serviceCall {
	ctx := context.Background()
	return []serviceCall{
		{"Regions", methodRegions, func(s *catalog.Service) error { _, _, err := s.Regions(ctx, "ru"); return err }},
		{"Region via regions", methodRegions, func(s *catalog.Service) error { _, _, err := s.Region(ctx, "gal", "ru"); return err }},
		{"Region via dishes", methodDishes, func(s *catalog.Service) error { _, _, err := s.Region(ctx, "gal", "ru"); return err }},
		{"Dishes", methodDishes, func(s *catalog.Service) error { _, _, err := s.Dishes(ctx, "ru"); return err }},
		{"Dish", methodDishes, func(s *catalog.Service) error { _, _, err := s.Dish(ctx, "tortilla", "ru"); return err }},
		{"Recipe", methodRecipe, func(s *catalog.Service) error { _, _, err := s.Recipe(ctx, "tortilla", "gal", "ru"); return err }},
		{"Dishes via content version", methodContentVersion, func(s *catalog.Service) error { _, _, err := s.Dishes(ctx, "ru"); return err }},
		{"Recipe via content version", methodContentVersion, func(s *catalog.Service) error { _, _, err := s.Recipe(ctx, "tortilla", "gal", "ru"); return err }},
	}
}

func TestService_RepositoryErrorsPropagate(t *testing.T) {
	t.Parallel()
	for _, sc := range serviceCalls() {
		t.Run(sc.name, func(t *testing.T) {
			repo := newFakeRepo()
			repo.failWith(sc.failsAt, errRepoDown)

			err := sc.call(catalog.NewService(repo))

			if !errors.Is(err, errRepoDown) {
				t.Errorf("got error %v, want it to wrap %v", err, errRepoDown)
			}
			if errors.Is(err, catalog.ErrNotFound) {
				t.Errorf("got ErrNotFound for a repository failure, want a plain error")
			}
		})
	}
}

func TestService_MissingTranslationIsAnErrorNotAFallback(t *testing.T) {
	t.Parallel()
	for _, sc := range serviceCalls() {
		t.Run(sc.name, func(t *testing.T) {
			repo := newFakeRepo()
			repo.failWith(sc.failsAt, missingRuDish())

			err := sc.call(catalog.NewService(repo))

			var missing *catalog.MissingTranslationError
			if !errors.As(err, &missing) {
				t.Fatalf("got error %v, want a *MissingTranslationError", err)
			}
			assertDeepEqual(t, "missing translation", *missing, *missingRuDish())
		})
	}
}

func TestService_FailedLoadsAreNotMemoised(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := newFakeRepo()
	repo.failWith(methodDishes, errRepoDown)
	svc := catalog.NewService(repo)
	if _, _, err := svc.Dishes(ctx, "en"); err == nil {
		t.Fatal("got nil error from a failing repository, want an error")
	}
	repo.clearFailures()

	got, _, err := svc.Dishes(ctx, "en")
	if err != nil {
		t.Fatalf("Dishes after the repository recovered: %v", err)
	}
	assertDeepEqual(t, "dishes", got, domainDishes("en"))
}

func TestMissingTranslationError_NamesEntityKeyAndLocale(t *testing.T) {
	t.Parallel()
	msg := missingRuDish().Error()
	for _, want := range []string{"dish", "tortilla", "ru"} {
		if !strings.Contains(msg, want) {
			t.Errorf("got error text %q, want it to name %q", msg, want)
		}
	}
}
