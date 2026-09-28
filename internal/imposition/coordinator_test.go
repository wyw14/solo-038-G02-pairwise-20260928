package imposition

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/wyw14/solo-038/internal/journal"
	"github.com/wyw14/solo-038/internal/model"
)

func newCoordinator(t *testing.T) (*Coordinator, *journal.Store) {
	t.Helper()
	store := journal.NewStore(filepath.Join(t.TempDir(), "events.jsonl"))
	return NewCoordinator(NewState(), store), store
}

func TestRevisionCreatesNewVersionAndIsolatesPositions(t *testing.T) {
	coordinator, _ := newCoordinator(t)
	job := model.NewJob(time.Now())

	first, err := coordinator.Plan(job, sampleInput(), time.Now())
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if first.Generation != 1 {
		t.Fatalf("first generation = %d, want 1", first.Generation)
	}

	// A second plan call on the same job must be a revision, not an overwrite.
	if _, err := coordinator.Plan(job, sampleInput(), time.Now()); err == nil {
		t.Fatal("expected error re-planning a job that already has a plan")
	}

	// Change the finished page order and the stock: this is a plan change.
	revisedInput := sampleInput()
	revisedInput.PageSequence = []int{1, 2, 3, 4, 5, 6, 7, 8}
	revisedInput.Sheet = model.SheetSize{WidthMM: 1060, HeightMM: 740}
	revisedInput.Area = model.PrintableArea{
		Bounds:      model.Rect{XMM: 15, YMM: 60, WidthMM: 1030, HeightMM: 665},
		GripEdge:    model.GripLeading,
		GripAllowMM: 50,
	}
	revised, err := coordinator.Revise(job, revisedInput, time.Now())
	if err != nil {
		t.Fatalf("revise: %v", err)
	}
	if revised.ID != first.ID {
		t.Fatalf("revision changed plan id: %q != %q", revised.ID, first.ID)
	}
	if revised.Generation != 2 || revised.Supersedes != 1 {
		t.Fatalf("revision generation = %d/%d, want 2/1", revised.Generation, revised.Supersedes)
	}

	// Current plan is the new generation.
	current, ok := coordinator.Current(job.ID)
	if !ok || current.Generation != 2 {
		t.Fatalf("current generation = %d, want 2", current.Generation)
	}

	// Old version stays readable and its positions are still bound to gen 1.
	old, ok := coordinator.Get(first.ID, 1)
	if !ok {
		t.Fatal("superseded generation 1 was lost")
	}
	if old.Sheet.WidthMM != 1020 {
		t.Fatalf("old plan sheet width mutated: %v", old.Sheet)
	}
	for _, position := range old.Positions {
		if position.BelongsTo(current) {
			t.Fatal("an old job's plate position is being interpreted as the new plan")
		}
	}
	for _, position := range current.Positions {
		if position.PlanGeneration != 2 || !position.BelongsTo(current) {
			t.Fatal("current positions are not bound to generation 2")
		}
	}

	// Acting on a stale reference must be rejected.
	if err := coordinator.RequireCurrent(job.ID, first.ID, 1); err == nil {
		t.Fatal("stale generation reference was accepted as current")
	}
	if err := coordinator.RequireCurrent(job.ID, current.ID, 2); err != nil {
		t.Fatalf("current reference rejected: %v", err)
	}
}

func TestCommitRejectsOverwriteAndBackwardsRevision(t *testing.T) {
	state := NewState()
	job := model.NewJob(time.Now())
	plan, err := BuildPlan(job, sampleInput(), time.Now())
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	if err := state.Commit(plan); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if err := state.Commit(plan); err == nil {
		t.Fatal("expected error overwriting an existing generation")
	}

	// A plan claiming to supersede generation 1 but staying at generation 1
	// must never replace the committed layout.
	clone := plan
	clone.Supersedes = 1
	if err := state.Commit(clone); err == nil {
		t.Fatal("expected error committing a non-advancing revision")
	}
}

func TestRebuildFromJournalRestoresEveryGeneration(t *testing.T) {
	coordinator, store := newCoordinator(t)
	job := model.NewJob(time.Now())
	first, err := coordinator.Plan(job, sampleInput(), time.Now())
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	revisedInput := sampleInput()
	revisedInput.PageSequence = []int{8, 7, 6, 5, 4, 3, 2, 1}
	revised, err := coordinator.Revise(job, revisedInput, time.Now())
	if err != nil {
		t.Fatalf("revise: %v", err)
	}

	rebuilt, err := RebuildFromJournal(store)
	if err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	gotFirst, ok := rebuilt.Get(first.ID, 1)
	if !ok {
		t.Fatal("replay lost generation 1")
	}
	gotRevised, ok := rebuilt.Get(revised.ID, 2)
	if !ok {
		t.Fatal("replay lost generation 2")
	}
	if gotFirst.PageSequence[0] != 1 || gotRevised.PageSequence[0] != 8 {
		t.Fatalf("replay mixed generations: %v / %v", gotFirst.PageSequence, gotRevised.PageSequence)
	}
	current, ok := rebuilt.Current(job.ID)
	if !ok || current.Generation != 2 {
		t.Fatalf("replayed current generation = %d, want 2", current.Generation)
	}
	generations := rebuilt.Generations(first.ID)
	if len(generations) != 2 || generations[0] != 1 || generations[1] != 2 {
		t.Fatalf("replayed generations = %v, want [1 2]", generations)
	}
}
