package sensor

import (
	"fmt"
	"time"
)

type CameraFrame struct {
	ID              string    `json:"id"`
	Camera          string    `json:"camera"`
	JobGeneration   uint64    `json:"job_generation"`
	PlateGeneration uint64    `json:"plate_generation"`
	PlanID          string    `json:"plan_id"`
	PlanRevision    uint64    `json:"plan_revision"`
	WebMeter        float64   `json:"web_meter"`
	Offset          float64   `json:"offset"`
	CapturedAt      time.Time `json:"captured_at"`
	ReceivedAt      time.Time `json:"received_at"`
}

func (frame CameraFrame) Validate() error {
	if frame.ID == "" || frame.Camera == "" {
		return fmt.Errorf("camera frame identity is incomplete")
	}
	if frame.JobGeneration == 0 || frame.PlateGeneration == 0 {
		return fmt.Errorf("camera frame generation is incomplete")
	}
	if frame.CapturedAt.IsZero() || frame.ReceivedAt.IsZero() {
		return fmt.Errorf("camera frame timestamps are incomplete")
	}
	return nil
}

func (frame CameraFrame) BelongsTo(jobGeneration, plateGeneration uint64, planID string, planRevision uint64) bool {
	return frame.JobGeneration == jobGeneration && frame.PlateGeneration == plateGeneration && frame.PlanID == planID && frame.PlanRevision == planRevision
}

func (frame CameraFrame) DeliveryDelay() time.Duration {
	return frame.ReceivedAt.Sub(frame.CapturedAt)
}
