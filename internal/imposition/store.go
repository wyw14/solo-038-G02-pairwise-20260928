package imposition

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"sync"
	"time"

	"github.com/wyw14/solo-038/internal/journal"
)

// PlanSummary describes one plan lineage without its layout tables.
type PlanSummary struct {
	PlanID        PlanID    `json:"plan_id"`
	Revisions     uint64    `json:"revisions"`
	LatestInput   Input     `json:"latest_input"`
	LatestCreated time.Time `json:"latest_created_at"`
}

type activePointer struct {
	PlanID   PlanID `json:"plan_id"`
	Revision uint64 `json:"revision"`
}

// Store keeps every revision of every plan. Revisions are append-only:
// Revise adds a new entry and never mutates the old ones, so plate positions
// recorded against an older revision can always be read back unchanged.
type Store struct {
	mu          sync.Mutex
	path        string
	saveVersion uint64
	plans       map[PlanID][]Revision
	active      activePointer
}

func NewStore(path string) *Store {
	return &Store{path: path, plans: map[PlanID][]Revision{}}
}

// Load restores plans from the snapshot file. A missing file means an empty
// store; other read or decode failures are returned.
func (store *Store) Load() error {
	store.mu.Lock()
	defer store.mu.Unlock()
	snapshot, err := journal.LoadSnapshot(store.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	plans, active, err := decodeSnapshot(snapshot.Values)
	if err != nil {
		return err
	}
	store.saveVersion = snapshot.Version
	store.plans = plans
	store.active = active
	return nil
}

// decodeSnapshot moves the map[string]any payload produced by saveLocked
// back into typed values with a JSON round-trip.
func decodeSnapshot(values map[string]any) (map[PlanID][]Revision, activePointer, error) {
	plans := map[PlanID][]Revision{}
	active := activePointer{}
	if raw, ok := values["plans"]; ok {
		data, err := json.Marshal(raw)
		if err != nil {
			return nil, activePointer{}, fmt.Errorf("decode imposition plans: %w", err)
		}
		if err := json.Unmarshal(data, &plans); err != nil {
			return nil, activePointer{}, fmt.Errorf("decode imposition plans: %w", err)
		}
		for id, revisions := range plans {
			for _, revision := range revisions {
				if revision.PlanID != id {
					return nil, activePointer{}, fmt.Errorf("imposition plan id mismatch in snapshot")
				}
				if err := revision.Validate(); err != nil {
					return nil, activePointer{}, err
				}
			}
		}
	}
	if raw, ok := values["active"]; ok {
		data, err := json.Marshal(raw)
		if err != nil {
			return nil, activePointer{}, fmt.Errorf("decode active imposition plan: %w", err)
		}
		if err := json.Unmarshal(data, &active); err != nil {
			return nil, activePointer{}, fmt.Errorf("decode active imposition plan: %w", err)
		}
		if active.PlanID != "" || active.Revision != 0 {
			if _, err := revisionOf(plans, active.PlanID, active.Revision); err != nil {
				return nil, activePointer{}, err
			}
		}
	}
	return plans, active, nil
}

// Create opens a new plan lineage at revision 1.
func (store *Store) Create(input Input, units []string, now time.Time) (Revision, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	id := NewPlanID()
	revision, err := Compute(input, units, now, id, 1)
	if err != nil {
		return Revision{}, err
	}
	store.plans[id] = []Revision{revision}
	if err := store.saveLocked(); err != nil {
		delete(store.plans, id)
		return Revision{}, err
	}
	return revision, nil
}

// Revise appends a new revision to an existing plan. The colour unit set is
// a plan-level property copied from revision 1; the input may change.
func (store *Store) Revise(id PlanID, input Input, now time.Time) (Revision, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	history, ok := store.plans[id]
	if !ok {
		return Revision{}, ErrPlanNotFound
	}
	revision, err := Compute(input, history[0].Units, now, id, uint64(len(history))+1)
	if err != nil {
		return Revision{}, err
	}
	store.plans[id] = append(history, revision)
	if err := store.saveLocked(); err != nil {
		store.plans[id] = history
		return Revision{}, err
	}
	return revision, nil
}

func (store *Store) Current(id PlanID) (Revision, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	history, ok := store.plans[id]
	if !ok {
		return Revision{}, ErrPlanNotFound
	}
	return history[len(history)-1], nil
}

func (store *Store) Get(id PlanID, revision uint64) (Revision, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return revisionOf(store.plans, id, revision)
}

func revisionOf(plans map[PlanID][]Revision, id PlanID, revision uint64) (Revision, error) {
	history, ok := plans[id]
	if !ok {
		return Revision{}, ErrPlanNotFound
	}
	if revision == 0 || int(revision) > len(history) {
		return Revision{}, ErrRevisionNotFound
	}
	return history[revision-1], nil
}

func (store *Store) List() []PlanSummary {
	store.mu.Lock()
	defer store.mu.Unlock()
	summaries := make([]PlanSummary, 0, len(store.plans))
	for id, history := range store.plans {
		latest := history[len(history)-1]
		summaries = append(summaries, PlanSummary{PlanID: id, Revisions: uint64(len(history)), LatestInput: latest.Input, LatestCreated: latest.CreatedAt})
	}
	sort.Slice(summaries, func(left, right int) bool { return summaries[left].PlanID < summaries[right].PlanID })
	return summaries
}

// Activate marks a revision as the one the next plate change binds to.
func (store *Store) Activate(id PlanID, revision uint64) (Revision, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	target, err := revisionOf(store.plans, id, revision)
	if err != nil {
		return Revision{}, err
	}
	previous := store.active
	store.active = activePointer{PlanID: id, Revision: revision}
	if err := store.saveLocked(); err != nil {
		store.active = previous
		return Revision{}, err
	}
	return target, nil
}

func (store *Store) Active() (PlanID, uint64, bool) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.active.PlanID, store.active.Revision, store.active.PlanID != ""
}

func (store *Store) saveLocked() error {
	store.saveVersion++
	snapshot := journal.Snapshot{
		Version: store.saveVersion,
		Values: map[string]any{
			"plans":  store.plans,
			"active": store.active,
		},
	}
	if err := journal.SaveSnapshot(store.path, snapshot); err != nil {
		store.saveVersion--
		return err
	}
	return nil
}
