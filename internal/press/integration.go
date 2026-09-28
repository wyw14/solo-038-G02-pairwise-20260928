package press

import (
	"fmt"

	"github.com/wyw14/solo-038/internal/sensor"
)

type RegisterAdjustment struct {
	FrameID         string  `json:"frame_id"`
	Unit            string  `json:"unit"`
	PlateGeneration uint64  `json:"plate_generation"`
	Correction      float64 `json:"correction"`
}

func ApplyRegisterFrame(controller *PhaseController, unit string, frame sensor.CameraFrame, activeJob, activePlate uint64, activePlanID string, activePlanRevision uint64) (RegisterAdjustment, error) {
	if err := frame.Validate(); err != nil {
		return RegisterAdjustment{}, err
	}
	if !frame.BelongsTo(activeJob, activePlate, activePlanID, activePlanRevision) {
		return RegisterAdjustment{}, fmt.Errorf("camera frame does not belong to active press generation")
	}
	correction, err := controller.Correct(unit, frame.Offset, activePlate)
	if err != nil {
		return RegisterAdjustment{}, err
	}
	return RegisterAdjustment{FrameID: frame.ID, Unit: unit, PlateGeneration: activePlate, Correction: correction}, nil
}

func RestorePhases(state *State, plate string, generation uint64, planID string, planRevision uint64, phases map[string]float64) error {
	state.mu.Lock()
	defer state.mu.Unlock()
	if plate == "" || generation == 0 {
		return fmt.Errorf("plate recovery identity is incomplete")
	}
	if (planID == "") != (planRevision == 0) {
		return fmt.Errorf("plate recovery plan identity is incomplete")
	}
	state.plate = plate
	state.plateGeneration = generation
	state.planID = planID
	state.planRevision = planRevision
	state.phaseByUnit = map[string]float64{}
	for unit, phase := range phases {
		state.phaseByUnit[unit] = phase
	}
	return nil
}
