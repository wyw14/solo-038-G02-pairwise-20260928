package imposition

import (
	"fmt"
	"time"

	"github.com/wyw14/solo-038/internal/journal"
	"github.com/wyw14/solo-038/internal/model"
)

const EventKind = "imposition_planned"

// Coordinator plans, versions and journals imposition layouts. The journal
// append happens before the in-memory state is updated, mirroring the rest of
// the press domains, so a plan that is not durable is never acted upon.
type Coordinator struct {
	state *State
	store *journal.Store
}

func NewCoordinator(state *State, store *journal.Store) *Coordinator {
	return &Coordinator{state: state, store: store}
}

// Plan creates generation 1 of a new plan for the job.
func (coordinator *Coordinator) Plan(job model.Job, input model.PlanInput, now time.Time) (model.ImpositionPlan, error) {
	if _, exists := coordinator.state.Current(job.ID); exists {
		return model.ImpositionPlan{}, fmt.Errorf("job %s already has an imposition plan; revise it instead", job.ID)
	}
	plan, err := BuildPlan(job, input, now)
	if err != nil {
		return model.ImpositionPlan{}, err
	}
	if err := coordinator.persist(job, plan, now); err != nil {
		return model.ImpositionPlan{}, err
	}
	if err := coordinator.state.Commit(plan); err != nil {
		return model.ImpositionPlan{}, err
	}
	return plan, nil
}

// Revise creates a new generation of the job's current plan from changed
// inputs. Positions are rebuilt and bound to the new generation, so the old
// generation's positions remain associated only with the old version.
func (coordinator *Coordinator) Revise(job model.Job, input model.PlanInput, now time.Time) (model.ImpositionPlan, error) {
	current, ok := coordinator.state.Current(job.ID)
	if !ok {
		return model.ImpositionPlan{}, fmt.Errorf("job %s has no imposition plan to revise", job.ID)
	}
	revised, err := RevisePlan(current, input, now)
	if err != nil {
		return model.ImpositionPlan{}, err
	}
	if err := coordinator.persist(job, revised, now); err != nil {
		return model.ImpositionPlan{}, err
	}
	if err := coordinator.state.Commit(revised); err != nil {
		return model.ImpositionPlan{}, err
	}
	return revised, nil
}

func (coordinator *Coordinator) persist(job model.Job, plan model.ImpositionPlan, now time.Time) error {
	operation := model.NewOperationID()
	event, err := model.NewEvent(operation, job, EventKind, plan, now)
	if err != nil {
		return err
	}
	if _, err := coordinator.store.Append(event); err != nil {
		return err
	}
	return nil
}

func (coordinator *Coordinator) Current(jobID model.JobID) (model.ImpositionPlan, bool) {
	return coordinator.state.Current(jobID)
}

func (coordinator *Coordinator) Get(id model.PlanID, generation uint64) (model.ImpositionPlan, bool) {
	return coordinator.state.Get(id, generation)
}

// RequireCurrent guards consumers that are about to act on plate positions:
// the positions are only valid for the plan version that produced them.
func (coordinator *Coordinator) RequireCurrent(jobID model.JobID, id model.PlanID, generation uint64) error {
	return coordinator.state.RequireCurrent(jobID, id, generation)
}

// RebuildFromJournal re-reads durable imposition events into a fresh state.
// Every event carries its full plan version, so replay restores each
// generation independently and never folds an old generation's positions into
// the newer layout.
func RebuildFromJournal(store *journal.Store) (*State, error) {
	events, err := store.Events()
	if err != nil {
		return nil, err
	}
	state := NewState()
	for _, event := range events {
		if event.Kind != EventKind {
			continue
		}
		plan, err := model.DecodeEventPayload[model.ImpositionPlan](event)
		if err != nil {
			return nil, err
		}
		if err := state.Commit(plan); err != nil {
			return nil, err
		}
	}
	return state, nil
}
