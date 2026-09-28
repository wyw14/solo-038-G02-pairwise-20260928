package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/wyw14/solo-038/internal/imposition"
)

func (handler *Handler) activeImpositionPlan(writer http.ResponseWriter, request *http.Request) {
	revision, ok := handler.runtime.ActiveImpositionPlan()
	if !ok {
		writeJSON(writer, http.StatusOK, map[string]any{"plan_id": "", "revision": 0})
		return
	}
	writeJSON(writer, http.StatusOK, revision)
}

func (handler *Handler) listImpositionPlans(writer http.ResponseWriter, request *http.Request) {
	writeJSON(writer, http.StatusOK, handler.runtime.ListImpositionPlans())
}

func (handler *Handler) createImpositionPlan(writer http.ResponseWriter, request *http.Request) {
	var input imposition.Input
	if err := decodeJSON(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	revision, err := handler.runtime.CreateImpositionPlan(input)
	if err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	writeJSON(writer, http.StatusCreated, revision)
}

func (handler *Handler) currentImpositionPlan(writer http.ResponseWriter, request *http.Request) {
	id, ok := planIDParam(writer, request)
	if !ok {
		return
	}
	revision, err := handler.runtime.CurrentImpositionPlan(id)
	if err != nil {
		writeError(writer, planErrorStatus(err), err)
		return
	}
	writeJSON(writer, http.StatusOK, revision)
}

func (handler *Handler) reviseImpositionPlan(writer http.ResponseWriter, request *http.Request) {
	id, ok := planIDParam(writer, request)
	if !ok {
		return
	}
	var input imposition.Input
	if err := decodeJSON(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	revision, err := handler.runtime.ReviseImpositionPlan(id, input)
	if err != nil {
		if errors.Is(err, imposition.ErrPlanNotFound) {
			writeError(writer, http.StatusNotFound, err)
		} else {
			writeError(writer, http.StatusBadRequest, err)
		}
		return
	}
	writeJSON(writer, http.StatusCreated, revision)
}

func (handler *Handler) getImpositionRevision(writer http.ResponseWriter, request *http.Request) {
	id, ok := planIDParam(writer, request)
	if !ok {
		return
	}
	revision, ok := revisionParam(writer, request)
	if !ok {
		return
	}
	found, err := handler.runtime.GetImpositionRevision(id, revision)
	if err != nil {
		writeError(writer, planErrorStatus(err), err)
		return
	}
	writeJSON(writer, http.StatusOK, found)
}

func (handler *Handler) activateImpositionRevision(writer http.ResponseWriter, request *http.Request) {
	id, ok := planIDParam(writer, request)
	if !ok {
		return
	}
	revision, ok := revisionParam(writer, request)
	if !ok {
		return
	}
	found, err := handler.runtime.ActivateImpositionPlan(id, revision)
	if err != nil {
		writeError(writer, planErrorStatus(err), err)
		return
	}
	writeJSON(writer, http.StatusOK, found)
}

func planIDParam(writer http.ResponseWriter, request *http.Request) (imposition.PlanID, bool) {
	value := chi.URLParam(request, "planID")
	if _, err := uuid.Parse(value); err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return "", false
	}
	return imposition.PlanID(value), true
}

func revisionParam(writer http.ResponseWriter, request *http.Request) (uint64, bool) {
	value, err := strconv.ParseUint(chi.URLParam(request, "revision"), 10, 64)
	if err != nil || value == 0 {
		writeError(writer, http.StatusBadRequest, errors.New("revision must be a positive integer"))
		return 0, false
	}
	return value, true
}

// planErrorStatus maps unknown plans/revisions to 404 and every other
// failure (input validation, state conflict) to 409; create/revise handlers
// handle their validation failures directly as 400.
func planErrorStatus(err error) int {
	if errors.Is(err, imposition.ErrPlanNotFound) || errors.Is(err, imposition.ErrRevisionNotFound) {
		return http.StatusNotFound
	}
	return http.StatusConflict
}
