package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/wyw14/solo-038/internal/model"
)

var errInvalidPlanGeneration = errors.New("plan generation must be a positive integer")

type planRequest struct {
	PageSequence []int               `json:"page_sequence"`
	Sheet        model.SheetSize     `json:"sheet"`
	Area         model.PrintableArea `json:"area"`
	Colors       []string            `json:"colors"`
	Columns      int                 `json:"columns"`
	Rows         int                 `json:"rows"`
}

func (request planRequest) input() model.PlanInput {
	return model.PlanInput{
		PageSequence: request.PageSequence,
		Sheet:        request.Sheet,
		Area:         request.Area,
		Colors:       request.Colors,
		Columns:      request.Columns,
		Rows:         request.Rows,
	}
}

func (handler *Handler) createImpositionPlan(writer http.ResponseWriter, request *http.Request) {
	var body planRequest
	if err := decodeJSON(request, &body); err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	plan, err := handler.runtime.PlanImposition(body.input())
	if err != nil {
		writeError(writer, http.StatusConflict, err)
		return
	}
	writeJSON(writer, http.StatusCreated, plan)
}

func (handler *Handler) reviseImpositionPlan(writer http.ResponseWriter, request *http.Request) {
	var body planRequest
	if err := decodeJSON(request, &body); err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	plan, err := handler.runtime.ReviseImposition(body.input())
	if err != nil {
		writeError(writer, http.StatusConflict, err)
		return
	}
	writeJSON(writer, http.StatusCreated, plan)
}

func (handler *Handler) currentImpositionPlan(writer http.ResponseWriter, request *http.Request) {
	plan, err := handler.runtime.CurrentImposition()
	if err != nil {
		writeError(writer, http.StatusNotFound, err)
		return
	}
	writeJSON(writer, http.StatusOK, plan)
}

func (handler *Handler) impositionPlanVersion(writer http.ResponseWriter, request *http.Request) {
	id, err := model.ParsePlanID(chi.URLParam(request, "id"))
	if err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	generation, err := strconv.ParseUint(chi.URLParam(request, "generation"), 10, 64)
	if err != nil || generation == 0 {
		writeError(writer, http.StatusBadRequest, errInvalidPlanGeneration)
		return
	}
	plan, err := handler.runtime.ImpositionVersion(id, generation)
	if err != nil {
		writeError(writer, http.StatusNotFound, err)
		return
	}
	writeJSON(writer, http.StatusOK, plan)
}
