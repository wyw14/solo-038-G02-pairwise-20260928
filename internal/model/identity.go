package model

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

type OperationID string
type JobID string
type LeaseID string
type EventID string
type PlanID string

func NewOperationID() OperationID {
	return OperationID(uuid.NewString())
}

func NewJobID() JobID {
	return JobID(uuid.NewString())
}

func NewLeaseID() LeaseID {
	return LeaseID(uuid.NewString())
}

func NewEventID() EventID {
	return EventID(uuid.NewString())
}

func NewPlanID() PlanID {
	return PlanID(uuid.NewString())
}

func ParseJobID(value string) (JobID, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("job id is required")
	}
	if _, err := uuid.Parse(value); err != nil {
		return "", fmt.Errorf("invalid job id: %w", err)
	}
	return JobID(value), nil
}

func ParsePlanID(value string) (PlanID, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("plan id is required")
	}
	if _, err := uuid.Parse(value); err != nil {
		return "", fmt.Errorf("invalid plan id: %w", err)
	}
	return PlanID(value), nil
}

func (id OperationID) Valid() bool {
	_, err := uuid.Parse(string(id))
	return err == nil
}

func (id JobID) Valid() bool {
	_, err := uuid.Parse(string(id))
	return err == nil
}

func (id LeaseID) Valid() bool {
	_, err := uuid.Parse(string(id))
	return err == nil
}

func (id PlanID) Valid() bool {
	_, err := uuid.Parse(string(id))
	return err == nil
}
