package service

import (
	"time"

	"github.com/wyw14/solo-038/internal/imposition"
)

// CreateImpositionPlan opens a new plan lineage at revision 1. The colour
// units are taken from the configured press and frozen on the plan.
func (runtime *Runtime) CreateImpositionPlan(input imposition.Input) (imposition.Revision, error) {
	return runtime.imposition.Create(input, runtime.pressUnits.Names(), time.Now().UTC())
}

// ReviseImpositionPlan appends a new revision; older revisions stay intact
// and retrievable.
func (runtime *Runtime) ReviseImpositionPlan(id imposition.PlanID, input imposition.Input) (imposition.Revision, error) {
	return runtime.imposition.Revise(id, input, time.Now().UTC())
}

func (runtime *Runtime) CurrentImpositionPlan(id imposition.PlanID) (imposition.Revision, error) {
	return runtime.imposition.Current(id)
}

func (runtime *Runtime) GetImpositionRevision(id imposition.PlanID, revision uint64) (imposition.Revision, error) {
	return runtime.imposition.Get(id, revision)
}

func (runtime *Runtime) ListImpositionPlans() []imposition.PlanSummary {
	return runtime.imposition.List()
}

// ActivateImpositionPlan marks a revision as the one the next plate change
// binds to. It does not change the mounted plate itself.
func (runtime *Runtime) ActivateImpositionPlan(id imposition.PlanID, revision uint64) (imposition.Revision, error) {
	target, err := runtime.imposition.Activate(id, revision)
	if err != nil {
		return imposition.Revision{}, err
	}
	runtime.mu.Lock()
	runtime.activePlanID = string(id)
	runtime.activePlanRev = revision
	runtime.mu.Unlock()
	return target, nil
}

// ActivePlan returns the revision the next plate change binds. Zero values
// mean no plan is active and plate changes run unbound.
func (runtime *Runtime) ActivePlan() (string, uint64, bool) {
	runtime.mu.RLock()
	defer runtime.mu.RUnlock()
	return runtime.activePlanID, runtime.activePlanRev, runtime.activePlanID != ""
}

// ActiveImpositionPlan returns the full active revision, or a zero revision
// with an empty plan identity when nothing is active.
func (runtime *Runtime) ActiveImpositionPlan() (imposition.Revision, bool) {
	id, revision, ok := runtime.ActivePlan()
	if !ok {
		return imposition.Revision{}, false
	}
	found, err := runtime.imposition.Get(imposition.PlanID(id), revision)
	if err != nil {
		return imposition.Revision{}, false
	}
	return found, true
}
