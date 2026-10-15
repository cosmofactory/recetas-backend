package catalog

import (
	"context"
	"errors"
	"log/slog"

	"github.com/cosmofactory/recetas-backend/internal/api/apigen"
)

// defaultLocale serves requests without a lang parameter; the generated
// wrapper doesn't apply the spec default.
const defaultLocale Locale = "en"

// Operation names logged with unexpected errors.
const (
	opListRegions = "listRegions"
	opGetRegion   = "getRegion"
	opListDishes  = "listDishes"
	opGetDish     = "getDish"
	opGetRecipe   = "getRecipe"
)

// Handler implements the catalog operations of the generated strict server.
// The lang enum is validated by internal/api and If-None-Match by its
// conditional-GET middleware.
type Handler struct {
	svc    *Service
	logger *slog.Logger
}

var _ apigen.StrictServerInterface = (*Handler)(nil)

func NewHandler(svc *Service, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

func (h *Handler) ListRegions(ctx context.Context, req apigen.ListRegionsRequestObject) (apigen.ListRegionsResponseObject, error) {
	l := localeOf(req.Params.Lang)
	regions, version, err := h.svc.Regions(ctx, l)
	if err != nil {
		h.logFailure(ctx, opListRegions, err)
		return apigen.ListRegions500JSONResponse{InternalJSONResponse: internalBody()}, nil
	}
	return apigen.ListRegions200JSONResponse{
		Body:    toAPIRegions(regions),
		Headers: apigen.ListRegions200ResponseHeaders(cacheHeaders(listETag(version, l))),
	}, nil
}

func (h *Handler) GetRegion(ctx context.Context, req apigen.GetRegionRequestObject) (apigen.GetRegionResponseObject, error) {
	if !validSlug(req.RegionKey) {
		return apigen.GetRegion404JSONResponse{NotFoundJSONResponse: notFoundBody()}, nil
	}
	l := localeOf(req.Params.Lang)
	detail, version, err := h.svc.Region(ctx, req.RegionKey, l)
	switch {
	case errors.Is(err, ErrNotFound):
		return apigen.GetRegion404JSONResponse{NotFoundJSONResponse: notFoundBody()}, nil
	case err != nil:
		h.logFailure(ctx, opGetRegion, err)
		return apigen.GetRegion500JSONResponse{InternalJSONResponse: internalBody()}, nil
	}
	return apigen.GetRegion200JSONResponse{
		Body:    toAPIRegionDetail(detail),
		Headers: apigen.GetRegion200ResponseHeaders(cacheHeaders(listETag(version, l))),
	}, nil
}

func (h *Handler) ListDishes(ctx context.Context, req apigen.ListDishesRequestObject) (apigen.ListDishesResponseObject, error) {
	l := localeOf(req.Params.Lang)
	dishes, version, err := h.svc.Dishes(ctx, l)
	if err != nil {
		h.logFailure(ctx, opListDishes, err)
		return apigen.ListDishes500JSONResponse{InternalJSONResponse: internalBody()}, nil
	}
	return apigen.ListDishes200JSONResponse{
		Body:    toAPIDishes(dishes),
		Headers: apigen.ListDishes200ResponseHeaders(cacheHeaders(listETag(version, l))),
	}, nil
}

func (h *Handler) GetDish(ctx context.Context, req apigen.GetDishRequestObject) (apigen.GetDishResponseObject, error) {
	if !validSlug(req.DishKey) {
		return apigen.GetDish404JSONResponse{NotFoundJSONResponse: notFoundBody()}, nil
	}
	l := localeOf(req.Params.Lang)
	dish, version, err := h.svc.Dish(ctx, req.DishKey, l)
	switch {
	case errors.Is(err, ErrNotFound):
		return apigen.GetDish404JSONResponse{NotFoundJSONResponse: notFoundBody()}, nil
	case err != nil:
		h.logFailure(ctx, opGetDish, err)
		return apigen.GetDish500JSONResponse{InternalJSONResponse: internalBody()}, nil
	}
	return apigen.GetDish200JSONResponse{
		Body:    toAPIDish(dish),
		Headers: apigen.GetDish200ResponseHeaders(cacheHeaders(listETag(version, l))),
	}, nil
}

func (h *Handler) GetRecipe(ctx context.Context, req apigen.GetRecipeRequestObject) (apigen.GetRecipeResponseObject, error) {
	if !validSlug(req.DishKey) || !validSlug(req.RegionKey) {
		return apigen.GetRecipe404JSONResponse{NotFoundJSONResponse: notFoundBody()}, nil
	}
	l := localeOf(req.Params.Lang)
	rc, version, err := h.svc.Recipe(ctx, req.DishKey, req.RegionKey, l)
	switch {
	case errors.Is(err, ErrNotFound):
		return apigen.GetRecipe404JSONResponse{NotFoundJSONResponse: notFoundBody()}, nil
	case err != nil:
		h.logFailure(ctx, opGetRecipe, err)
		return apigen.GetRecipe500JSONResponse{InternalJSONResponse: internalBody()}, nil
	}
	return apigen.GetRecipe200JSONResponse{
		Body:    toAPIRecipe(rc),
		Headers: apigen.GetRecipe200ResponseHeaders(cacheHeaders(recipeETag(version, l, rc.DishKey, rc.RegionKey))),
	}, nil
}

// logFailure records an unexpected error; the client only gets internalBody.
func (h *Handler) logFailure(ctx context.Context, op string, err error) {
	h.logger.ErrorContext(ctx, "catalog request failed", "op", op, "err", err)
}

func localeOf(lang *apigen.Lang) Locale {
	if lang == nil {
		return defaultLocale
	}
	return Locale(*lang)
}
