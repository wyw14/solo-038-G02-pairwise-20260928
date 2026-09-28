package press

import (
	"fmt"
	"sync"
)

type State struct {
	mu              sync.RWMutex
	plate           string
	plateGeneration uint64
	planID          string
	planRevision    uint64
	activeColor     string
	phaseByUnit     map[string]float64
}

func NewState() *State {
	return &State{phaseByUnit: map[string]float64{}}
}

func (state *State) ChangePlate(plate, planID string, planRevision uint64) (uint64, error) {
	if plate == "" {
		return 0, fmt.Errorf("plate identity is required")
	}
	if (planID == "") != (planRevision == 0) {
		return 0, fmt.Errorf("plate plan identity is incomplete")
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	state.plate = plate
	state.plateGeneration++
	state.planID = planID
	state.planRevision = planRevision
	state.phaseByUnit = map[string]float64{}
	return state.plateGeneration, nil
}

func (state *State) SetColor(color string) error {
	if color == "" {
		return fmt.Errorf("ink color is required")
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	state.activeColor = color
	return nil
}

func (state *State) SetPhase(unit string, phase float64, plateGeneration uint64) error {
	state.mu.Lock()
	defer state.mu.Unlock()
	if unit == "" || plateGeneration != state.plateGeneration {
		return fmt.Errorf("phase update does not match active plate")
	}
	state.phaseByUnit[unit] = phase
	return nil
}

func (state *State) Snapshot() (string, uint64, string, uint64, string, map[string]float64) {
	state.mu.RLock()
	defer state.mu.RUnlock()
	phases := make(map[string]float64, len(state.phaseByUnit))
	for key, value := range state.phaseByUnit {
		phases[key] = value
	}
	return state.plate, state.plateGeneration, state.planID, state.planRevision, state.activeColor, phases
}
