// Package imposition turns the finished page sequence, stock size and
// printable area into a versioned plate layout: where every color plate sits
// on the press sheet and which finished page each cell yields after folding
// and trimming.
package imposition

import (
	"fmt"
	"time"

	"github.com/wyw14/solo-038/internal/model"
)

// SheetwiseBooklet is the supported fold scheme (折页方式): one signature
// folded sheetwise, where each grid cell is a leaf carrying one page on the
// front (outer) forme and one on the back (inner) forme.
const SheetwiseBooklet = "sheetwise_booklet"

// foldOrder builds the sheetwise booklet pairing. For leaf i (zero based) it
// returns the finished-page index printed on the front and back of that leaf.
// Leaf 0 is the outermost leaf, so its front carries the last page and its
// back carries the first; successive leaves nest in the signature.
func leafPages(leaves int) (front []int, back []int) {
	front = make([]int, leaves)
	back = make([]int, leaves)
	for leaf := 0; leaf < leaves; leaf++ {
		if leaf%2 == 0 {
			front[leaf] = 2*leaves - 1 - leaf
			back[leaf] = leaf
		} else {
			front[leaf] = leaf
			back[leaf] = 2*leaves - 1 - leaf
		}
	}
	return front, back
}

// cellRect divides the printable area into the plate grid and returns the
// rectangle of one cell. Rows are filled snake-wise (boustrophedon) so panels
// that meet at a fold keep the correct relative orientation.
func cellRect(area model.PrintableArea, columns, rows, column, row int) model.Rect {
	cellWidth := area.Bounds.WidthMM / float64(columns)
	cellHeight := area.Bounds.HeightMM / float64(rows)
	layoutColumn := column
	if row%2 == 1 {
		layoutColumn = columns - 1 - column
	}
	return model.Rect{
		XMM:      area.Bounds.XMM + float64(layoutColumn)*cellWidth,
		YMM:      area.Bounds.YMM + float64(row)*cellHeight,
		WidthMM:  cellWidth,
		HeightMM: cellHeight,
	}
}

// BuildPlan validates the inputs and lays out a new plan at generation 1.
// The page count must be a multiple of four and the plate grid must hold one
// cell per folded leaf (columns*rows == pages/2).
func BuildPlan(job model.Job, input model.PlanInput, now time.Time) (model.ImpositionPlan, error) {
	if err := job.Validate(); err != nil {
		return model.ImpositionPlan{}, err
	}
	if err := input.Validate(); err != nil {
		return model.ImpositionPlan{}, err
	}
	pageCount := len(input.PageSequence)
	if pageCount%4 != 0 {
		return model.ImpositionPlan{}, fmt.Errorf("sheetwise booklet requires a page count divisible by 4, got %d", pageCount)
	}
	leaves := pageCount / 2
	if input.Columns*input.Rows != leaves {
		return model.ImpositionPlan{}, fmt.Errorf("plate grid holds %d leaves but the signature needs %d", input.Columns*input.Rows, leaves)
	}
	plan := model.ImpositionPlan{
		ID:           model.NewPlanID(),
		JobID:        job.ID,
		Generation:   1,
		Supersedes:   0,
		PageSequence: append([]int(nil), input.PageSequence...),
		Sheet:        input.Sheet,
		Area:         input.Area,
		Colors:       append([]string(nil), input.Colors...),
		Columns:      input.Columns,
		Rows:         input.Rows,
		FoldScheme:   SheetwiseBooklet,
		CreatedAt:    now.UTC(),
	}
	if err := populatePositions(&plan); err != nil {
		return model.ImpositionPlan{}, err
	}
	return plan, plan.Validate()
}

// RevisePlan lays out a new generation of an existing plan from changed
// inputs. The returned plan keeps the same PlanID and records the generation
// it supersedes; positions are rebuilt from scratch so an old generation's
// plate positions can never be read as the new layout.
func RevisePlan(previous model.ImpositionPlan, input model.PlanInput, now time.Time) (model.ImpositionPlan, error) {
	if err := previous.Validate(); err != nil {
		return model.ImpositionPlan{}, err
	}
	if err := input.Validate(); err != nil {
		return model.ImpositionPlan{}, err
	}
	pageCount := len(input.PageSequence)
	if pageCount%4 != 0 {
		return model.ImpositionPlan{}, fmt.Errorf("sheetwise booklet requires a page count divisible by 4, got %d", pageCount)
	}
	leaves := pageCount / 2
	if input.Columns*input.Rows != leaves {
		return model.ImpositionPlan{}, fmt.Errorf("plate grid holds %d leaves but the signature needs %d", input.Columns*input.Rows, leaves)
	}
	revised := model.ImpositionPlan{
		ID:           previous.ID,
		JobID:        previous.JobID,
		Generation:   previous.Generation + 1,
		Supersedes:   previous.Generation,
		PageSequence: append([]int(nil), input.PageSequence...),
		Sheet:        input.Sheet,
		Area:         input.Area,
		Colors:       append([]string(nil), input.Colors...),
		Columns:      input.Columns,
		Rows:         input.Rows,
		FoldScheme:   SheetwiseBooklet,
		CreatedAt:    now.UTC(),
	}
	if err := populatePositions(&revised); err != nil {
		return model.ImpositionPlan{}, err
	}
	return revised, revised.Validate()
}

func populatePositions(plan *model.ImpositionPlan) error {
	leaves := len(plan.PageSequence) / 2
	front, back := leafPages(leaves)
	plan.Positions = make([]model.PlatePosition, 0, len(plan.Colors)*2*leaves)
	// slotForPage records the folded leaf that yields each finished-page
	// index, so the trimmed order can point back at the producing cell.
	slotForPage := make([]int, len(plan.PageSequence))
	for leaf := 0; leaf < leaves; leaf++ {
		column := leaf % plan.Columns
		row := leaf / plan.Columns
		rect := cellRect(plan.Area, plan.Columns, plan.Rows, column, row)
		slotForPage[front[leaf]] = leaf
		slotForPage[back[leaf]] = leaf
		for _, color := range plan.Colors {
			plan.Positions = append(plan.Positions,
				model.PlatePosition{
					PlanID: plan.ID, PlanGeneration: plan.Generation,
					Color: color, Side: model.SideFront, Column: column, Row: row, Slot: leaf,
					Rect: rect, FinishedPage: plan.PageSequence[front[leaf]], Orientation: orientation(column, row, model.SideFront),
				},
				model.PlatePosition{
					PlanID: plan.ID, PlanGeneration: plan.Generation,
					Color: color, Side: model.SideBack, Column: column, Row: row, Slot: leaf,
					Rect: rect, FinishedPage: plan.PageSequence[back[leaf]], Orientation: orientation(column, row, model.SideBack),
				},
			)
		}
	}
	plan.TrimmedOrder = make([]model.TrimmedEntry, len(plan.PageSequence))
	for pageIndex := range plan.PageSequence {
		plan.TrimmedOrder[pageIndex] = model.TrimmedEntry{Order: pageIndex + 1, Slot: slotForPage[pageIndex], FinishedPage: plan.PageSequence[pageIndex]}
	}
	return nil
}

// orientation reports the panel rotation for a cell. Snake-wise rows alternate
// direction and the back forme is turned over at the fold, so neighbouring
// panels alternate head direction.
func orientation(column, row int, side model.PlateSide) string {
	rotated := (column+row)%2 == 0
	if side == model.SideBack {
		rotated = !rotated
	}
	if rotated {
		return "180"
	}
	return "0"
}
