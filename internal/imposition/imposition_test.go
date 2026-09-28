package imposition

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func testInput(pages int, folds []FoldAxis) Input {
	return Input{
		FinishedPages: pages,
		Sheet:         SizeMM{WidthMM: 880, HeightMM: 630},
		Printable:     RectMM{XMM: 20, YMM: 40, WidthMM: 840, HeightMM: 570},
		Gripper:       Gripper{Edge: GripperLeading, BiteMM: 30},
		Folds:         folds,
	}
}

func testCompute(t *testing.T, input Input) Revision {
	t.Helper()
	revision, err := Compute(input, []string{"cyan", "magenta", "yellow", "black"}, time.Unix(1700000000, 0), NewPlanID(), 1)
	if err != nil {
		t.Fatalf("compute: %v", err)
	}
	return revision
}

func findPosition(revision Revision, unit string, side Side) PlatePosition {
	for _, position := range revision.Positions {
		if position.Unit == unit && position.Side == side {
			return position
		}
	}
	return PlatePosition{}
}

func cellGrid(t *testing.T, position PlatePosition, signature int) (pages, rotations [][]int) {
	t.Helper()
	rows, cols := 0, 0
	for _, cell := range position.Cells {
		if cell.Signature != signature {
			continue
		}
		if cell.Row+1 > rows {
			rows = cell.Row + 1
		}
		if cell.Col+1 > cols {
			cols = cell.Col + 1
		}
	}
	pages = make([][]int, rows)
	rotations = make([][]int, rows)
	for index := range pages {
		pages[index] = make([]int, cols)
		rotations[index] = make([]int, cols)
	}
	for _, cell := range position.Cells {
		if cell.Signature != signature {
			continue
		}
		pages[cell.Row][cell.Col] = cell.Page
		rotations[cell.Row][cell.Col] = cell.Rotation
	}
	return pages, rotations
}

func gridsEqual(got, want [][]int) bool {
	if len(got) != len(want) {
		return false
	}
	for row := range got {
		if len(got[row]) != len(want[row]) {
			return false
		}
		for col := range got[row] {
			if got[row][col] != want[row][col] {
				return false
			}
		}
	}
	return true
}

func TestLayout4pp(t *testing.T) {
	revision := testCompute(t, testInput(4, []FoldAxis{FoldVertical}))
	if revision.PagesPerSignature != 4 || revision.Signatures != 1 || len(revision.Positions) != 8 {
		t.Fatalf("unexpected revision shape: n=%d sigs=%d positions=%d", revision.PagesPerSignature, revision.Signatures, len(revision.Positions))
	}
	front, frontRot := cellGrid(t, findPosition(revision, "black", SideFront), 0)
	back, backRot := cellGrid(t, findPosition(revision, "black", SideBack), 0)
	if !gridsEqual(front, [][]int{{1, 4}}) {
		t.Fatalf("front layout = %v, want [1 4]", front)
	}
	if !gridsEqual(back, [][]int{{2, 3}}) {
		t.Fatalf("back layout = %v, want [2 3]", back)
	}
	for _, grid := range [][][]int{frontRot, backRot} {
		for _, row := range grid {
			for _, rotation := range row {
				if rotation != 0 {
					t.Fatalf("unexpected rotation %d in 4pp layout", rotation)
				}
			}
		}
	}
	cells := findPosition(revision, "black", SideFront).Cells
	if cells[0].Rect.XMM != 20 || cells[0].Rect.YMM != 40 || cells[0].Rect.WidthMM != 420 || cells[0].Rect.HeightMM != 570 {
		t.Fatalf("first cell rect = %+v", cells[0].Rect)
	}
	if cells[1].Rect.XMM != 440 {
		t.Fatalf("second cell x = %v, want 440", cells[1].Rect.XMM)
	}
}

func TestLayout8ppVerticalThenHorizontal(t *testing.T) {
	revision := testCompute(t, testInput(8, []FoldAxis{FoldVertical, FoldHorizontal}))
	front, frontRot := cellGrid(t, findPosition(revision, "black", SideFront), 0)
	back, backRot := cellGrid(t, findPosition(revision, "black", SideBack), 0)
	if !gridsEqual(front, [][]int{{1, 4}, {8, 5}}) {
		t.Fatalf("front layout = %v", front)
	}
	if !gridsEqual(back, [][]int{{2, 3}, {7, 6}}) {
		t.Fatalf("back layout = %v", back)
	}
	if !gridsEqual(frontRot, [][]int{{0, 0}, {180, 180}}) || !gridsEqual(backRot, [][]int{{0, 0}, {180, 180}}) {
		t.Fatalf("unexpected rotations front=%v back=%v", frontRot, backRot)
	}
}

func TestLayout8ppHorizontalThenVertical(t *testing.T) {
	revision := testCompute(t, testInput(8, []FoldAxis{FoldHorizontal, FoldVertical}))
	front, _ := cellGrid(t, findPosition(revision, "black", SideFront), 0)
	// Last physical fold is vertical, so pages summing to 9 share a row.
	if !gridsEqual(front, [][]int{{1, 8}, {4, 5}}) {
		t.Fatalf("front layout = %v", front)
	}
}

func TestLayout16ppCoverage(t *testing.T) {
	revision := testCompute(t, testInput(16, []FoldAxis{FoldVertical, FoldHorizontal, FoldVertical}))
	seen := map[int]bool{}
	for _, side := range []Side{SideFront, SideBack} {
		for _, cell := range findPosition(revision, "black", side).Cells {
			if cell.Rotation != 0 && cell.Rotation != 180 {
				t.Fatalf("rotation = %d, want 0 or 180", cell.Rotation)
			}
			if seen[cell.Page] {
				t.Fatalf("page %d appears more than once", cell.Page)
			}
			seen[cell.Page] = true
		}
	}
	if len(seen) != 16 {
		t.Fatalf("coverage = %d pages, want 16", len(seen))
	}
	for page := 1; page <= 16; page++ {
		if !seen[page] {
			t.Fatalf("page %d missing", page)
		}
	}
}

func TestPairSumsAcrossLastFold(t *testing.T) {
	cases := []struct {
		name  string
		folds []FoldAxis
	}{
		{"v", []FoldAxis{FoldVertical}},
		{"vh", []FoldAxis{FoldVertical, FoldHorizontal}},
		{"hv", []FoldAxis{FoldHorizontal, FoldVertical}},
		{"vhv", []FoldAxis{FoldVertical, FoldHorizontal, FoldVertical}},
		{"hvh", []FoldAxis{FoldHorizontal, FoldVertical, FoldHorizontal}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n := PagesPerSignature(len(tc.folds))
			revision := testCompute(t, testInput(n, tc.folds))
			front := map[[2]int]int{}
			back := map[[2]int]int{}
			for _, cell := range findPosition(revision, "black", SideFront).Cells {
				front[[2]int{cell.Row, cell.Col}] = cell.Page
			}
			for _, cell := range findPosition(revision, "black", SideBack).Cells {
				back[[2]int{cell.Row, cell.Col}] = cell.Page
			}
			if len(front) != len(back) {
				t.Fatalf("front/back cell counts differ: %d vs %d", len(front), len(back))
			}
			sideOf := map[int]Side{}
			for key, page := range front {
				// The two faces of a cell are one folded leaf: an
				// unordered consecutive pair {2k-1, 2k}.
				lo, hi := page, back[key]
				if lo > hi {
					lo, hi = hi, lo
				}
				if lo%2 != 1 || hi != lo+1 {
					t.Fatalf("cell %v pages %d/%d are not one leaf pair", key, page, back[key])
				}
				sideOf[page] = SideFront
				sideOf[back[key]] = SideBack
			}
			// Complementary booklet pages p and n+1-p print on the same
			// sheet side.
			for page := 1; page <= n; page++ {
				if sideOf[page] != sideOf[n+1-page] {
					t.Fatalf("pages %d and %d land on different sides", page, n+1-page)
				}
			}
		})
	}
}

func TestMultipleSignatures(t *testing.T) {
	revision := testCompute(t, testInput(16, []FoldAxis{FoldVertical}))
	if revision.Signatures != 4 {
		t.Fatalf("signatures = %d, want 4", revision.Signatures)
	}
	front := findPosition(revision, "black", SideFront)
	if len(front.Cells) != 8 {
		t.Fatalf("front cells = %d, want 8", len(front.Cells))
	}
	signatureOne, _ := cellGrid(t, front, 1)
	if !gridsEqual(signatureOne, [][]int{{5, 8}}) {
		t.Fatalf("signature 1 front = %v, want [5 8]", signatureOne)
	}
	if len(revision.TrimOrder) != 16 {
		t.Fatalf("trim order length = %d, want 16", len(revision.TrimOrder))
	}
	for index, entry := range revision.TrimOrder {
		if entry.Page != index+1 {
			t.Fatalf("trim order not page-sorted at %d: %+v", index, entry)
		}
	}
	units := map[string]map[Side]bool{}
	for _, position := range revision.Positions {
		if units[position.Unit] == nil {
			units[position.Unit] = map[Side]bool{}
		}
		units[position.Unit][position.Side] = true
	}
	if len(units) != 4 {
		t.Fatalf("units covered = %d, want 4", len(units))
	}
	for unit, sides := range units {
		if !sides[SideFront] || !sides[SideBack] {
			t.Fatalf("unit %s missing a side", unit)
		}
	}
}

func TestPageOrderPermutation(t *testing.T) {
	plain := testInput(8, []FoldAxis{FoldVertical, FoldHorizontal})
	permuted := plain
	permuted.PageOrder = []int{8, 7, 6, 5, 4, 3, 2, 1}
	revision := testCompute(t, permuted)
	front, _ := cellGrid(t, findPosition(revision, "black", SideFront), 0)
	if !gridsEqual(front, [][]int{{8, 5}, {1, 4}}) {
		t.Fatalf("permuted front = %v, want [8 5]/[1 4]", front)
	}
	first := testCompute(t, plain)
	plainFront, _ := cellGrid(t, findPosition(first, "black", SideFront), 0)
	if plainFront[0][0] != 1 {
		t.Fatalf("identity ordering changed: %v", plainFront)
	}
}

func TestInputValidation(t *testing.T) {
	valid := testInput(8, []FoldAxis{FoldVertical, FoldHorizontal})
	cases := []struct {
		name  string
		alter func(*Input)
	}{
		{"no pages", func(in *Input) { in.FinishedPages = 0 }},
		{"no folds", func(in *Input) { in.Folds = nil }},
		{"bad fold", func(in *Input) { in.Folds = []FoldAxis{"diagonal"} }},
		{"not divisible", func(in *Input) { in.FinishedPages = 6 }},
		{"zero sheet", func(in *Input) { in.Sheet.WidthMM = 0 }},
		{"zero printable", func(in *Input) { in.Printable.WidthMM = 0 }},
		{"printable outside right", func(in *Input) { in.Printable.XMM = 800 }},
		{"printable outside top", func(in *Input) { in.Printable.YMM = 600 }},
		{"printable negative origin", func(in *Input) { in.Printable.XMM = -1 }},
		{"bite too deep leading", func(in *Input) { in.Gripper.BiteMM = 41 }},
		{"bite too deep trailing", func(in *Input) {
			in.Gripper.Edge = GripperTrailing
			in.Gripper.BiteMM = 21
		}},
		{"bite too deep left", func(in *Input) {
			in.Gripper.Edge = GripperLeft
			in.Gripper.BiteMM = 21
		}},
		{"bite too deep right", func(in *Input) {
			in.Gripper.Edge = GripperRight
			in.Gripper.BiteMM = 21
		}},
		{"bad gripper edge", func(in *Input) { in.Gripper.Edge = "middle" }},
		{"negative bite", func(in *Input) { in.Gripper.BiteMM = -1 }},
		{"page order length", func(in *Input) { in.PageOrder = []int{1, 2, 3} }},
		{"page order duplicate", func(in *Input) {
			in.PageOrder = []int{1, 1, 3, 4, 5, 6, 7, 8}
		}},
		{"page order out of range", func(in *Input) {
			in.PageOrder = []int{0, 2, 3, 4, 5, 6, 7, 8}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := valid
			tc.alter(&input)
			if err := input.Validate(); err == nil {
				t.Fatalf("expected validation error for %s", tc.name)
			}
		})
	}
}

func TestGripperBiteAtBoundary(t *testing.T) {
	input := testInput(4, []FoldAxis{FoldVertical})
	input.Gripper.BiteMM = 40 // exactly the printable Y inset
	if err := input.Validate(); err != nil {
		t.Fatalf("bite equal to inset should pass: %v", err)
	}
}

func TestStoreRevisionImmutability(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "plans.json"))
	now := time.Unix(1700000000, 0)
	first, err := store.Create(testInput(8, []FoldAxis{FoldVertical, FoldHorizontal}), []string{"cyan", "magenta", "yellow", "black"}, now)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if first.Revision != 1 {
		t.Fatalf("first revision = %d, want 1", first.Revision)
	}
	second, err := store.Revise(first.PlanID, testInput(16, []FoldAxis{FoldVertical, FoldHorizontal, FoldVertical}), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("revise: %v", err)
	}
	if second.Revision != 2 || second.Signatures != 1 {
		t.Fatalf("second revision = %d sigs=%d", second.Revision, second.Signatures)
	}
	old, err := store.Get(first.PlanID, 1)
	if err != nil {
		t.Fatalf("get old: %v", err)
	}
	if old.Input.FinishedPages != 8 || len(old.Positions) != len(first.Positions) {
		t.Fatalf("old revision was mutated: pages=%d positions=%d", old.Input.FinishedPages, len(old.Positions))
	}
	current, err := store.Current(first.PlanID)
	if err != nil || current.Revision != 2 {
		t.Fatalf("current = rev %d, err %v", current.Revision, err)
	}
	if _, err := store.Get(first.PlanID, 99); !errors.Is(err, ErrRevisionNotFound) {
		t.Fatalf("missing revision err = %v, want ErrRevisionNotFound", err)
	}
	if _, err := store.Current(PlanID("00000000-0000-0000-0000-000000000000")); !errors.Is(err, ErrPlanNotFound) {
		t.Fatalf("missing plan err = %v, want ErrPlanNotFound", err)
	}
	if _, err := store.Revise(PlanID("00000000-0000-0000-0000-000000000000"), first.Input, now); !errors.Is(err, ErrPlanNotFound) {
		t.Fatalf("revise missing plan err = %v", err)
	}
}

func TestStoreActivate(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "plans.json"))
	now := time.Unix(1700000000, 0)
	first, err := store.Create(testInput(8, []FoldAxis{FoldVertical, FoldHorizontal}), []string{"cyan", "magenta", "yellow", "black"}, now)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, _, ok := store.Active(); ok {
		t.Fatalf("active before activation")
	}
	if _, err := store.Activate(first.PlanID, 1); err != nil {
		t.Fatalf("activate: %v", err)
	}
	id, revision, ok := store.Active()
	if !ok || id != first.PlanID || revision != 1 {
		t.Fatalf("active = %s/%d ok=%v", id, revision, ok)
	}
	if _, err := store.Activate(first.PlanID, 5); !errors.Is(err, ErrRevisionNotFound) {
		t.Fatalf("activate missing revision err = %v", err)
	}
}

func TestStorePersistenceRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plans.json")
	store := NewStore(path)
	now := time.Unix(1700000000, 0)
	first, err := store.Create(testInput(8, []FoldAxis{FoldVertical, FoldHorizontal}), []string{"cyan", "magenta", "yellow", "black"}, now)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	second, err := store.Revise(first.PlanID, testInput(16, []FoldAxis{FoldVertical, FoldHorizontal, FoldVertical}), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("revise: %v", err)
	}
	if _, err := store.Activate(second.PlanID, second.Revision); err != nil {
		t.Fatalf("activate: %v", err)
	}

	reloaded := NewStore(path)
	if err := reloaded.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	gotFirst, err := reloaded.Get(first.PlanID, 1)
	if err != nil {
		t.Fatalf("get first after reload: %v", err)
	}
	if gotFirst.Input.FinishedPages != 8 || gotFirst.PlanID != first.PlanID {
		t.Fatalf("first revision changed after reload: %+v", gotFirst.Input)
	}
	gotSecond, err := reloaded.Get(second.PlanID, 2)
	if err != nil || gotSecond.Input.FinishedPages != 16 {
		t.Fatalf("second revision after reload: %v %+v", err, gotSecond.Input)
	}
	id, revision, ok := reloaded.Active()
	if !ok || id != second.PlanID || revision != 2 {
		t.Fatalf("active after reload = %s/%d ok=%v", id, revision, ok)
	}
	if summaries := reloaded.List(); len(summaries) != 1 || summaries[0].Revisions != 2 {
		t.Fatalf("list after reload = %+v", summaries)
	}
}

func TestStoreLoadMissingFile(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "absent.json"))
	if err := store.Load(); err != nil {
		t.Fatalf("missing file should be empty store: %v", err)
	}
	if summaries := store.List(); len(summaries) != 0 {
		t.Fatalf("expected empty store, got %+v", summaries)
	}
}
