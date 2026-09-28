package imposition

import (
	"fmt"
	"sort"
	"sync"

	"github.com/wyw14/solo-038/internal/model"
)

type planVersion struct {
	id         model.PlanID
	generation uint64
}

// State keeps every committed plan version immutable. A revision adds a new
// generation; superseded versions remain readable for traceability but are
// never returned as the current layout, so an old job's plate positions can
// never be interpreted as a new plan.
type State struct {
	mu       sync.RWMutex
	versions map[model.PlanID]map[uint64]model.ImpositionPlan
	current  map[model.JobID]planVersion
}

func NewState() *State {
	return &State{
		versions: map[model.PlanID]map[uint64]model.ImpositionPlan{},
		current:  map[model.JobID]planVersion{},
	}
}

// Commit stores a validated plan version and marks it current for its job. It
// refuses to move a plan backwards or to overwrite an existing generation,
// because each revision must be a distinct new version.
func (state *State) Commit(plan model.ImpositionPlan) error {
	if err := plan.Validate(); err != nil {
		return err
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	generations, ok := state.versions[plan.ID]
	if !ok {
		generations = map[uint64]model.ImpositionPlan{}
		state.versions[plan.ID] = generations
	}
	if _, exists := generations[plan.Generation]; exists {
		return fmt.Errorf("plan %s generation %d already exists", plan.ID, plan.Generation)
	}
	if plan.Supersedes != 0 {
		if _, exists := generations[plan.Supersedes]; !exists {
			return fmt.Errorf("plan revision supersedes unknown generation %d", plan.Supersedes)
		}
		if plan.Generation <= plan.Supersedes {
			return fmt.Errorf("plan revision generation must advance beyond %d", plan.Supersedes)
		}
	}
	generations[plan.Generation] = plan
	state.current[plan.JobID] = planVersion{id: plan.ID, generation: plan.Generation}
	return nil
}

// Current returns the latest committed plan version for a job.
func (state *State) Current(jobID model.JobID) (model.ImpositionPlan, bool) {
	state.mu.RLock()
	defer state.mu.RUnlock()
	version, ok := state.current[jobID]
	if !ok {
		return model.ImpositionPlan{}, false
	}
	return state.lockedGet(version.id, version.generation)
}

// Get returns a specific plan version, including superseded ones.
func (state *State) Get(id model.PlanID, generation uint64) (model.ImpositionPlan, bool) {
	state.mu.RLock()
	defer state.mu.RUnlock()
	return state.lockedGet(id, generation)
}

func (state *State) lockedGet(id model.PlanID, generation uint64) (model.ImpositionPlan, bool) {
	generations, ok := state.versions[id]
	if !ok {
		return model.ImpositionPlan{}, false
	}
	plan, ok := generations[generation]
	return plan, ok
}

// Generations lists every committed generation of a plan, oldest first.
func (state *State) Generations(id model.PlanID) []uint64 {
	state.mu.RLock()
	defer state.mu.RUnlock()
	values := make([]uint64, 0, len(state.versions[id]))
	for generation := range state.versions[id] {
		values = append(values, generation)
	}
	sort.Slice(values, func(left, right int) bool { return values[left] < values[right] })
	return values
}

// RequireCurrent fails unless the given identity names the current plan
// version of the job. Callers use this before acting on plate positions so a
// stale reference from an old job or superseded generation is rejected.
func (state *State) RequireCurrent(jobID model.JobID, id model.PlanID, generation uint64) error {
	state.mu.RLock()
	defer state.mu.RUnlock()
	version, ok := state.current[jobID]
	if !ok {
		return fmt.Errorf("job %s has no imposition plan", jobID)
	}
	if version.id != id || version.generation != generation {
		return fmt.Errorf("plate positions belong to plan %s generation %d; current is %s generation %d", id, generation, version.id, version.generation)
	}
	return nil
}
