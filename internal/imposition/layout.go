package imposition

import (
	"sort"
	"time"
)

// leaf is one printed sheet leaf in a folded stack. Each leaf carries two
// pages, one on each face; up/down name the faces in the current folded
// orientation. ux/uy is the content-up unit vector and tracks how the leaf
// has been rotated/reflected by unfolding.
type leaf struct {
	upPage   int
	downPage int
	ux       int8
	uy       int8
}

// grid is the unfolded signature indexed [row][col]; each cell holds a
// stack of leaves top-to-bottom.
type grid [][][]leaf

// newFoldedStack builds the closed signature: a single cell containing
// perSignature/2 leaves in finished reading order (leaf i faces pages
// 2i-1 up and 2i down, content pointing toward the gripper).
func newFoldedStack(perSignature int) grid {
	stack := make([]leaf, 0, perSignature/2)
	for page := 1; page <= perSignature; page += 2 {
		stack = append(stack, leaf{upPage: page, downPage: page + 1, ux: 0, uy: 1})
	}
	return grid{{stack}}
}

// unfold opens one fold: the grid doubles along the fold axis, every cell
// splits at its midpoint, and the bottom half of each stack moves to the
// mirrored cell reversed, face-swapped and reflected.
func unfold(current grid, axis FoldAxis) grid {
	rows := len(current)
	cols := len(current[0])
	nextRows, nextCols := rows, cols*2
	if axis == FoldHorizontal {
		nextRows, nextCols = rows*2, cols
	}
	next := make(grid, nextRows)
	for row := range next {
		next[row] = make([][]leaf, nextCols)
	}
	for row := 0; row < rows; row++ {
		for col := 0; col < cols; col++ {
			stack := current[row][col]
			middle := len(stack) / 2
			top, bottom := stack[:middle], stack[middle:]
			if axis == FoldVertical {
				next[row][col] = top
				next[row][2*cols-1-col] = mirror(bottom, axis)
			} else {
				next[row][col] = top
				next[2*rows-1-row][col] = mirror(bottom, axis)
			}
		}
	}
	return next
}

// mirror turns a moving stack into the mirrored cell: the leaves arrive in
// reverse stack order, faces swap and the content-up vector reflects across
// the fold axis.
func mirror(stack []leaf, axis FoldAxis) []leaf {
	moved := make([]leaf, 0, len(stack))
	for index := len(stack) - 1; index >= 0; index-- {
		value := stack[index]
		leaf := leaf{upPage: value.downPage, downPage: value.upPage, ux: value.ux, uy: value.uy}
		if axis == FoldVertical {
			leaf.ux = -value.ux
		} else {
			leaf.uy = -value.uy
		}
		moved = append(moved, leaf)
	}
	return moved
}

// unfoldSignature opens folds in reverse physical order.
func unfoldSignature(folds []FoldAxis) grid {
	perSignature := PagesPerSignature(len(folds))
	current := newFoldedStack(perSignature)
	for index := len(folds) - 1; index >= 0; index-- {
		current = unfold(current, folds[index])
	}
	return current
}

type placedCell struct {
	row      int
	col      int
	rotation int
	front    int
	back     int
}

func layoutSignature(folds []FoldAxis) []placedCell {
	opened := unfoldSignature(folds)
	rows := len(opened)
	cols := len(opened[0])
	cells := make([]placedCell, 0, rows*cols)
	for row := 0; row < rows; row++ {
		for col := 0; col < cols; col++ {
			value := opened[row][col][0]
			rotation := 0
			if value.uy < 0 {
				rotation = 180
			}
			cells = append(cells, placedCell{row: row, col: col, rotation: rotation, front: value.upPage, back: value.downPage})
		}
	}
	return cells
}

func cellRect(input Input, rows, cols, row, col int) RectMM {
	width := input.Printable.WidthMM / float64(cols)
	height := input.Printable.HeightMM / float64(rows)
	return RectMM{
		XMM:      input.Printable.XMM + float64(col)*width,
		YMM:      input.Printable.YMM + float64(row)*height,
		WidthMM:  width,
		HeightMM: height,
	}
}

// Compute builds an immutable imposition revision from the validated input:
// each colour unit gets a front/back table of finished-page positions, and
// trim order maps post-trim pages back to their sheet cells.
func Compute(input Input, units []string, now time.Time, planID PlanID, revision uint64) (Revision, error) {
	if err := input.Validate(); err != nil {
		return Revision{}, err
	}
	orderedUnits := append([]string(nil), units...)
	if len(orderedUnits) == 0 {
		return Revision{}, errUnitsRequired
	}
	sort.Strings(orderedUnits)
	perSignature := input.pagesPerSignature()
	signatures := input.FinishedPages / perSignature
	folds := append([]FoldAxis(nil), input.Folds...)
	template := layoutSignature(folds)
	rows, cols := gridShape(folds)

	// one table per unit per side, cells ordered signature,row,col
	positions := make([]PlatePosition, 0, len(orderedUnits)*2)
	frontCells := make([][]Cell, len(orderedUnits))
	backCells := make([][]Cell, len(orderedUnits))
	for index, unit := range orderedUnits {
		if unit == "" || (index > 0 && unit == orderedUnits[index-1]) {
			return Revision{}, errUnitsRequired
		}
		frontCells[index] = make([]Cell, 0, rows*cols*signatures)
		backCells[index] = make([]Cell, 0, rows*cols*signatures)
	}

	type pageLocation struct {
		signature int
		side      Side
		row       int
		col       int
	}
	locations := make([]pageLocation, input.FinishedPages+1)

	for signature := 0; signature < signatures; signature++ {
		for _, placed := range template {
			rect := cellRect(input, rows, cols, placed.row, placed.col)
			frontPage := input.orderedPage(signature*perSignature + placed.front)
			backPage := input.orderedPage(signature*perSignature + placed.back)
			for index := range orderedUnits {
				frontCells[index] = append(frontCells[index], Cell{Signature: signature, Row: placed.row, Col: placed.col, Rect: rect, Page: frontPage, Rotation: placed.rotation})
				backCells[index] = append(backCells[index], Cell{Signature: signature, Row: placed.row, Col: placed.col, Rect: rect, Page: backPage, Rotation: placed.rotation})
			}
			locations[frontPage] = pageLocation{signature: signature, side: SideFront, row: placed.row, col: placed.col}
			locations[backPage] = pageLocation{signature: signature, side: SideBack, row: placed.row, col: placed.col}
		}
	}
	for index, unit := range orderedUnits {
		positions = append(positions, PlatePosition{Unit: unit, Side: SideFront, Cells: frontCells[index]})
		positions = append(positions, PlatePosition{Unit: unit, Side: SideBack, Cells: backCells[index]})
	}

	trimOrder := make([]TrimEntry, 0, input.FinishedPages)
	for page := 1; page <= input.FinishedPages; page++ {
		location := locations[page]
		trimOrder = append(trimOrder, TrimEntry{Page: page, Signature: location.signature, Side: location.side, Row: location.row, Col: location.col})
	}

	result := Revision{
		PlanID:            planID,
		Revision:          revision,
		Input:             input,
		Units:             orderedUnits,
		PagesPerSignature: perSignature,
		Signatures:        signatures,
		Positions:         positions,
		TrimOrder:         trimOrder,
		CreatedAt:         now.UTC(),
	}
	if err := result.Validate(); err != nil {
		return Revision{}, err
	}
	return result, nil
}

func gridShape(folds []FoldAxis) (rows, cols int) {
	rows, cols = 1, 1
	for _, fold := range folds {
		if fold == FoldVertical {
			cols *= 2
		} else {
			rows *= 2
		}
	}
	return rows, cols
}
