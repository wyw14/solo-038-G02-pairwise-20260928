package service

import (
	"fmt"
	"time"

	"github.com/wyw14/solo-038/internal/model"
)

// PlanImposition creates generation 1 of an imposition plan for the current
// job from the finished page sequence, sheet size and printable area.
func (runtime *Runtime) PlanImposition(input model.PlanInput) (model.ImpositionPlan, error) {
	current, ok := runtime.jobs.Current()
	if !ok {
		return model.ImpositionPlan{}, fmt.Errorf("there is no active job")
	}
	return runtime.impositionPlan.Plan(current, input, time.Now())
}

// ReviseImposition lays out a new generation of the current job's plan. The
// previous version is retained; the returned positions belong solely to the
// new generation.
func (runtime *Runtime) ReviseImposition(input model.PlanInput) (model.ImpositionPlan, error) {
	current, ok := runtime.jobs.Current()
	if !ok {
		return model.ImpositionPlan{}, fmt.Errorf("there is no active job")
	}
	return runtime.impositionPlan.Revise(current, input, time.Now())
}

// CurrentImposition returns the current plan version of the current job.
func (runtime *Runtime) CurrentImposition() (model.ImpositionPlan, error) {
	current, ok := runtime.jobs.Current()
	if !ok {
		return model.ImpositionPlan{}, fmt.Errorf("there is no active job")
	}
	plan, ok := runtime.impositionPlan.Current(current.ID)
	if !ok {
		return model.ImpositionPlan{}, fmt.Errorf("current job has no imposition plan")
	}
	return plan, nil
}

// ImpositionVersion returns a specific, possibly superseded, plan version.
func (runtime *Runtime) ImpositionVersion(id model.PlanID, generation uint64) (model.ImpositionPlan, error) {
	plan, ok := runtime.impositionPlan.Get(id, generation)
	if !ok {
		return model.ImpositionPlan{}, fmt.Errorf("imposition plan version not found")
	}
	return plan, nil
}
