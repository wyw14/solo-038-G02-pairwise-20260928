package imposition

import (
	"testing"
	"time"

	"github.com/wyw14/solo-038/internal/model"
)

func sampleInput() model.PlanInput {
	return model.PlanInput{
		PageSequence: []int{1, 2, 3, 4, 5, 6, 7, 8},
		Sheet:        model.SheetSize{WidthMM: 1020, HeightMM: 720},
		Area: model.PrintableArea{
			Bounds:      model.Rect{XMM: 15, YMM: 60, WidthMM: 990, HeightMM: 645},
			GripEdge:    model.GripLeading,
			GripAllowMM: 50,
		},
		Colors:  []string{"cyan", "magenta", "yellow", "black"},
		Columns: 2,
		Rows:    2,
	}
}

// TestSheetwiseFoldOrder verifies the classic 8-page sheetwise signature:
// outer forme carries 8/1, nesting inward to 2/7, 6/3 and 4/5.
func TestSheetwiseFoldOrder(t *testing.T) {
	job := model.NewJob(time.Now())
	plan, err := BuildPlan(job, sampleInput(), time.Now())
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	if plan.Generation != 1 || plan.Supersedes != 0 {
		t.Fatalf("new plan generation = %d/%d, want 1/0", plan.Generation, plan.Supersedes)
	}
	if len(plan.Positions) != 4*2*4 {
		t.Fatalf("positions = %d, want %d (colors x sides x leaves)", len(plan.Positions), 4*2*4)
	}

	frontByLeaf := map[int]int{}
	backByLeaf := map[int]int{}
	for _, position := range plan.Positions {
		if position.Color != "black" {
			continue
		}
		if !position.Rect.FitsInside(model.Rect{WidthMM: 1020, HeightMM: 720}) {
			t.Fatalf("position falls outside the sheet: %+v", position.Rect)
		}
		if position.PlanID != plan.ID || position.PlanGeneration != 1 {
			t.Fatalf("position not bound to plan v1")
		}
		switch position.Side {
		case model.SideFront:
			frontByLeaf[position.Slot] = position.FinishedPage
		case model.SideBack:
			backByLeaf[position.Slot] = position.FinishedPage
		}
	}
	wantFront := map[int]int{0: 8, 1: 2, 2: 6, 3: 4}
	wantBack := map[int]int{0: 1, 1: 7, 2: 3, 3: 5}
	for leaf, page := range wantFront {
		if frontByLeaf[leaf] != page {
			t.Errorf("leaf %d front = %d, want %d", leaf, frontByLeaf[leaf], page)
		}
	}
	for leaf, page := range wantBack {
		if backByLeaf[leaf] != page {
			t.Errorf("leaf %d back = %d, want %d", leaf, backByLeaf[leaf], page)
		}
	}

	// Every color plate must carry the identical front/back page per leaf.
	for _, color := range plan.Colors {
		for _, position := range plan.Positions {
			if position.Color != color {
				continue
			}
			want := wantFront[position.Slot]
			if position.Side == model.SideBack {
				want = wantBack[position.Slot]
			}
			if position.FinishedPage != want {
				t.Errorf("color %s leaf %d side %s page = %d, want %d", color, position.Slot, position.Side, position.FinishedPage, want)
			}
		}
	}

	// Trimmed order leaves the folder in finished sequence 1..8 and points
	// each page back at the leaf that produced it.
	wantSlot := map[int]int{1: 0, 2: 1, 3: 2, 4: 3, 5: 3, 6: 2, 7: 1, 8: 0}
	if len(plan.TrimmedOrder) != 8 {
		t.Fatalf("trimmed order length = %d, want 8", len(plan.TrimmedOrder))
	}
	for _, entry := range plan.TrimmedOrder {
		if wantSlot[entry.FinishedPage] != entry.Slot {
			t.Errorf("page %d slot = %d, want %d", entry.FinishedPage, entry.Slot, wantSlot[entry.FinishedPage])
		}
		if entry.Order != entry.FinishedPage {
			t.Errorf("page %d trim order = %d", entry.FinishedPage, entry.Order)
		}
	}
}

func TestGripAndAreaValidation(t *testing.T) {
	job := model.NewJob(time.Now())
	input := sampleInput()

	overlap := input
	overlap.Area = model.PrintableArea{
		Bounds:      model.Rect{XMM: 15, YMM: 20, WidthMM: 990, HeightMM: 645},
		GripEdge:    model.GripLeading,
		GripAllowMM: 50,
	}
	if _, err := BuildPlan(job, overlap, time.Now()); err == nil {
		t.Fatal("expected error when printable area overlaps the grip strip")
	}

	outside := input
	outside.Area = model.PrintableArea{
		Bounds:      model.Rect{XMM: 15, YMM: 60, WidthMM: 990, HeightMM: 700},
		GripEdge:    model.GripLeading,
		GripAllowMM: 50,
	}
	if _, err := BuildPlan(job, outside, time.Now()); err == nil {
		t.Fatal("expected error when printable area exceeds the sheet")
	}

	badPages := input
	badPages.PageSequence = []int{1, 2, 3}
	if _, err := BuildPlan(job, badPages, time.Now()); err == nil {
		t.Fatal("expected error for page count not divisible by 4")
	}

	badGrid := input
	badGrid.Columns, badGrid.Rows = 1, 1
	if _, err := BuildPlan(job, badGrid, time.Now()); err == nil {
		t.Fatal("expected error when the plate grid cannot hold the leaves")
	}
}

func TestPositionsStayInsidePrintableArea(t *testing.T) {
	job := model.NewJob(time.Now())
	plan, err := BuildPlan(job, sampleInput(), time.Now())
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	area := plan.Area.Bounds
	for _, position := range plan.Positions {
		if !position.Rect.FitsInside(area) {
			t.Fatalf("position rect %+v lies outside printable area %+v", position.Rect, area)
		}
	}
}
