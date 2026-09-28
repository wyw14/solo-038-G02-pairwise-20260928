package model

import (
	"errors"
	"fmt"
	"time"
)

type JobState string

const (
	JobLoading     JobState = "loading"
	JobThreading   JobState = "threading"
	JobRegistering JobState = "registering"
	JobPrinting    JobState = "printing"
	JobSplicing    JobState = "splicing"
	JobTailing     JobState = "tailing"
	JobComplete    JobState = "complete"
	JobCancelled   JobState = "cancelled"
)

var jobTransitions = map[JobState]map[JobState]bool{
	JobLoading:     {JobThreading: true, JobCancelled: true},
	JobThreading:   {JobRegistering: true, JobCancelled: true},
	JobRegistering: {JobPrinting: true, JobCancelled: true},
	JobPrinting:    {JobSplicing: true, JobTailing: true, JobCancelled: true},
	JobSplicing:    {JobPrinting: true, JobTailing: true, JobCancelled: true},
	JobTailing:     {JobComplete: true, JobCancelled: true},
	JobComplete:    {},
	JobCancelled:   {},
}

type Job struct {
	ID              JobID     `json:"id"`
	State           JobState  `json:"state"`
	Generation      uint64    `json:"generation"`
	RollGeneration  uint64    `json:"roll_generation"`
	PlateGeneration uint64    `json:"plate_generation"`
	PlanID          string    `json:"plan_id"`
	PlanRevision    uint64    `json:"plan_revision"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func NewJob(now time.Time) Job {
	return Job{ID: NewJobID(), State: JobLoading, Generation: 1, CreatedAt: now.UTC(), UpdatedAt: now.UTC()}
}

func (job Job) Transition(next JobState, now time.Time) (Job, error) {
	if !jobTransitions[job.State][next] {
		return job, fmt.Errorf("cannot transition job from %s to %s", job.State, next)
	}
	job.State = next
	job.UpdatedAt = now.UTC()
	return job, nil
}

func (job Job) ChangeRoll(now time.Time) Job {
	job.RollGeneration++
	job.UpdatedAt = now.UTC()
	return job
}

func (job Job) ChangePlate(now time.Time, planID string, planRevision uint64) Job {
	job.PlateGeneration++
	job.PlanID = planID
	job.PlanRevision = planRevision
	job.UpdatedAt = now.UTC()
	return job
}

func (job Job) Validate() error {
	if !job.ID.Valid() {
		return errors.New("job id is invalid")
	}
	if _, ok := jobTransitions[job.State]; !ok {
		return errors.New("job state is invalid")
	}
	if job.Generation == 0 {
		return errors.New("job generation is required")
	}
	return nil
}
