package api

import (
	"context"
	"net/http"

	"github.com/cosmofactory/recetas-backend/internal/api/apigen"
	"github.com/cosmofactory/recetas-backend/internal/platform/httpx"
)

// Stub answers every catalog operation with 501, for tests of the adapter
// that need an implementation but no catalog.
type Stub struct{}

var _ apigen.StrictServerInterface = Stub{}

func (Stub) ListRegions(context.Context, apigen.ListRegionsRequestObject) (apigen.ListRegionsResponseObject, error) {
	return notImplemented{}, nil
}

func (Stub) GetRegion(context.Context, apigen.GetRegionRequestObject) (apigen.GetRegionResponseObject, error) {
	return notImplemented{}, nil
}

func (Stub) ListDishes(context.Context, apigen.ListDishesRequestObject) (apigen.ListDishesResponseObject, error) {
	return notImplemented{}, nil
}

func (Stub) GetDish(context.Context, apigen.GetDishRequestObject) (apigen.GetDishResponseObject, error) {
	return notImplemented{}, nil
}

func (Stub) GetRecipe(context.Context, apigen.GetRecipeRequestObject) (apigen.GetRecipeResponseObject, error) {
	return notImplemented{}, nil
}

type notImplemented struct{}

func (notImplemented) write(w http.ResponseWriter) error {
	httpx.WriteError(w, http.StatusNotImplemented, "not_implemented", "not implemented")
	return nil
}

func (r notImplemented) VisitListRegionsResponse(w http.ResponseWriter) error { return r.write(w) }
func (r notImplemented) VisitGetRegionResponse(w http.ResponseWriter) error   { return r.write(w) }
func (r notImplemented) VisitListDishesResponse(w http.ResponseWriter) error  { return r.write(w) }
func (r notImplemented) VisitGetDishResponse(w http.ResponseWriter) error     { return r.write(w) }
func (r notImplemented) VisitGetRecipeResponse(w http.ResponseWriter) error   { return r.write(w) }
