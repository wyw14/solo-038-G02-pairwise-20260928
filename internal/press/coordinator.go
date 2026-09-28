package press

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/wyw14/solo-038/internal/model"
)

type registerSample struct {
	plateGeneration uint64
	offset          float64
}

type Coordinator struct {
	mu              sync.RWMutex
	controller      *Controller
	state           *State
	stopCommand     model.Command
	stopProof       map[string]model.Acknowledgement
	registerSamples []registerSample
}

func NewCoordinator(controller *Controller, state *State) *Coordinator {
	return &Coordinator{controller: controller, state: state, stopProof: map[string]model.Acknowledgement{}}
}

func (coordinator *Coordinator) BeginStop(job model.Job, epoch uint64, now time.Time) model.Command {
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	coordinator.stopCommand = model.NewCommand(job, epoch, model.CommandStop, now)
	coordinator.stopProof = map[string]model.Acknowledgement{}
	return coordinator.stopCommand
}

func (coordinator *Coordinator) AcknowledgeStop(ack model.Acknowledgement) error {
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	if ack.Operation != coordinator.stopCommand.Operation || ack.Epoch != coordinator.stopCommand.Epoch {
		return fmt.Errorf("stop acknowledgement belongs to another command")
	}
	if ack.Value != 0 {
		return fmt.Errorf("press unit has not reached zero speed")
	}
	if _, ok := coordinator.controller.Unit(ack.Unit); !ok {
		return fmt.Errorf("press unit is unknown")
	}
	coordinator.stopProof[ack.Unit] = ack
	return nil
}

func (coordinator *Coordinator) CanSplice() bool {
	coordinator.mu.RLock()
	defer coordinator.mu.RUnlock()
	if !coordinator.stopCommand.Operation.Valid() {
		return false
	}
	for _, unit := range coordinator.controller.Names() {
		proof, ok := coordinator.stopProof[unit]
		if !ok || proof.Operation != coordinator.stopCommand.Operation || proof.Epoch != coordinator.stopCommand.Epoch || proof.Value != 0 {
			return false
		}
	}
	return true
}

func (coordinator *Coordinator) ChangePlate(plate, planID string, planRevision uint64) (uint64, error) {
	generation, err := coordinator.state.ChangePlate(plate, planID, planRevision)
	if err != nil {
		return 0, err
	}
	coordinator.mu.Lock()
	coordinator.registerSamples = nil
	coordinator.mu.Unlock()
	return generation, nil
}

func (coordinator *Coordinator) RecordRegister(plateGeneration uint64, offset float64) error {
	_, active, _, _, _, _ := coordinator.state.Snapshot()
	if plateGeneration != active {
		return fmt.Errorf("register sample belongs to another plate")
	}
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	coordinator.registerSamples = append(coordinator.registerSamples, registerSample{plateGeneration: plateGeneration, offset: offset})
	if len(coordinator.registerSamples) > 20 {
		coordinator.registerSamples = coordinator.registerSamples[len(coordinator.registerSamples)-20:]
	}
	return nil
}

func (coordinator *Coordinator) RegisterStable(tolerance float64, minimum int) bool {
	coordinator.mu.RLock()
	defer coordinator.mu.RUnlock()
	_, active, _, _, _, _ := coordinator.state.Snapshot()
	count := 0
	for _, sample := range coordinator.registerSamples {
		if sample.plateGeneration == active && math.Abs(sample.offset) <= tolerance {
			count++
		}
	}
	return count >= minimum
}
