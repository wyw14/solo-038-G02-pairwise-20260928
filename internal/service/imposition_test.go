package service

import (
	"context"
	"testing"
	"time"

	"github.com/wyw14/solo-038/internal/imposition"
	"github.com/wyw14/solo-038/internal/model"
	"github.com/wyw14/solo-038/internal/sensor"
)

func testRuntime(t *testing.T) *Runtime {
	t.Helper()
	runtime, err := NewRuntime(Config{
		DataDir:           t.TempDir(),
		FestoonCapacity:   120,
		RegisterGap:       500 * time.Millisecond,
		RegisterTolerance: 0.25,
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	return runtime
}

func planInput() imposition.Input {
	return imposition.Input{
		FinishedPages: 8,
		Sheet:         imposition.SizeMM{WidthMM: 880, HeightMM: 630},
		Printable:     imposition.RectMM{XMM: 20, YMM: 40, WidthMM: 840, HeightMM: 570},
		Gripper:       imposition.Gripper{Edge: imposition.GripperLeading, BiteMM: 30},
		Folds:         []imposition.FoldAxis{imposition.FoldVertical, imposition.FoldHorizontal},
	}
}

func frameFor(jobGeneration, plateGeneration uint64, planID string, planRevision uint64) sensor.CameraFrame {
	now := time.Now()
	return sensor.CameraFrame{
		ID:              string(model.NewOperationID()),
		Camera:          "test-camera",
		JobGeneration:   jobGeneration,
		PlateGeneration: plateGeneration,
		PlanID:          planID,
		PlanRevision:    planRevision,
		WebMeter:        2,
		Offset:          0.01,
		CapturedAt:      now,
		ReceivedAt:      now.Add(time.Millisecond),
	}
}

func TestPlateChangeBindsActivePlan(t *testing.T) {
	runtime := testRuntime(t)
	revision, err := runtime.CreateImpositionPlan(planInput())
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if _, err := runtime.ActivateImpositionPlan(revision.PlanID, revision.Revision); err != nil {
		t.Fatalf("activate plan: %v", err)
	}
	if _, err := runtime.CreateOperation(context.Background(), "adjust_phase"); err != nil {
		t.Fatalf("adjust_phase: %v", err)
	}

	_, plateGeneration, planID, planRevision, _, _ := runtime.press.Snapshot()
	if planID != string(revision.PlanID) || planRevision != revision.Revision {
		t.Fatalf("press bound to %s/%d, want %s/%d", planID, planRevision, revision.PlanID, revision.Revision)
	}

	current, ok := runtime.jobs.Current()
	if !ok || current.PlanID != planID || current.PlanRevision != planRevision {
		t.Fatalf("job not stamped with plan binding: %+v", current)
	}

	window := runtime.register.Snapshot()
	if !window.Valid || window.PlateGeneration != plateGeneration || window.PlanID != planID || window.PlanRevision != planRevision {
		t.Fatalf("register window not bound to plan: %+v", window)
	}

	// A frame carrying a different plan revision must be rejected by both
	// the state guard and the coordinator guard.
	stale := frameFor(current.Generation, plateGeneration, planID, planRevision+1)
	if err := runtime.register.Add(stale); err == nil {
		t.Fatalf("register state accepted a frame from another plan revision")
	}
	if err := runtime.registerControl.AcceptFrame(stale, runtime.config.RegisterGap); err == nil {
		t.Fatalf("register coordinator accepted a frame from another plan revision")
	}
	otherPlan := frameFor(current.Generation, plateGeneration, string(imposition.NewPlanID()), planRevision)
	if err := runtime.registerControl.AcceptFrame(otherPlan, runtime.config.RegisterGap); err == nil {
		t.Fatalf("register coordinator accepted a frame from another plan")
	}

	// A matching frame is accepted.
	matching := frameFor(current.Generation, plateGeneration, planID, planRevision)
	if err := runtime.registerControl.AcceptFrame(matching, runtime.config.RegisterGap); err != nil {
		t.Fatalf("matching frame rejected: %v", err)
	}
}

func TestPlateChangeWithoutPlanStaysUnbound(t *testing.T) {
	runtime := testRuntime(t)
	if _, _, ok := runtime.ActivePlan(); ok {
		t.Fatalf("fresh runtime should have no active plan")
	}
	if _, err := runtime.CreateOperation(context.Background(), "adjust_phase"); err != nil {
		t.Fatalf("adjust_phase without plan: %v", err)
	}
	_, plateGeneration, planID, planRevision, _, _ := runtime.press.Snapshot()
	if planID != "" || planRevision != 0 {
		t.Fatalf("unbound plate carries plan %s/%d", planID, planRevision)
	}
	current, _ := runtime.jobs.Current()
	frame := frameFor(current.Generation, plateGeneration, "", 0)
	if err := runtime.registerControl.AcceptFrame(frame, runtime.config.RegisterGap); err != nil {
		t.Fatalf("unbound frame rejected: %v", err)
	}
}

func TestPlanRevisionKeptAcrossReload(t *testing.T) {
	runtime := testRuntime(t)
	revision, err := runtime.CreateImpositionPlan(planInput())
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if _, err := runtime.ActivateImpositionPlan(revision.PlanID, revision.Revision); err != nil {
		t.Fatalf("activate: %v", err)
	}
	reloaded, err := NewRuntime(Config{
		DataDir:           runtime.config.DataDir,
		FestoonCapacity:   120,
		RegisterGap:       500 * time.Millisecond,
		RegisterTolerance: 0.25,
	})
	if err != nil {
		t.Fatalf("reload runtime: %v", err)
	}
	id, rev, ok := reloaded.ActivePlan()
	if !ok || id != string(revision.PlanID) || rev != 1 {
		t.Fatalf("active plan after reload = %s/%d ok=%v", id, rev, ok)
	}
	old, err := reloaded.GetImpositionRevision(revision.PlanID, 1)
	if err != nil || old.Input.FinishedPages != 8 {
		t.Fatalf("revision 1 after reload: %v %+v", err, old.Input)
	}
}
