package model

import (
	"errors"
	"fmt"
	"time"
)

// GripEdge identifies the sheet edge held by the press gripper (咬口). The
// printable area must always begin on the opposite edge so the grip strip is
// never covered by a plate position.
type GripEdge string

const (
	GripLeading  GripEdge = "leading"
	GripTrailing GripEdge = "trailing"
	GripLeft     GripEdge = "left"
	GripRight    GripEdge = "right"
)

func validGripEdge(edge GripEdge) bool {
	switch edge {
	case GripLeading, GripTrailing, GripLeft, GripRight:
		return true
	default:
		return false
	}
}

// SheetSize is the paper stock (纸张幅面) measured in millimetres.
type SheetSize struct {
	WidthMM  float64 `json:"width_mm"`
	HeightMM float64 `json:"height_mm"`
}

func (size SheetSize) Validate() error {
	if size.WidthMM <= 0 || size.HeightMM <= 0 {
		return errors.New("sheet size dimensions must be positive")
	}
	return nil
}

// Rect is an axis-aligned rectangle in millimetres with its origin at the
// sheet's leading-left corner.
type Rect struct {
	XMM      float64 `json:"x_mm"`
	YMM      float64 `json:"y_mm"`
	WidthMM  float64 `json:"width_mm"`
	HeightMM float64 `json:"height_mm"`
}

func (rect Rect) Validate() error {
	if rect.XMM < 0 || rect.YMM < 0 || rect.WidthMM <= 0 || rect.HeightMM <= 0 {
		return errors.New("printable rectangle is invalid")
	}
	return nil
}

// FitsInside reports whether the rectangle lies wholly within bounds.
func (rect Rect) FitsInside(bounds Rect) bool {
	return rect.XMM >= bounds.XMM && rect.YMM >= bounds.YMM &&
		rect.XMM+rect.WidthMM <= bounds.XMM+bounds.WidthMM+1e-6 &&
		rect.YMM+rect.HeightMM <= bounds.YMM+bounds.HeightMM+1e-6
}

// PrintableArea is the region of the sheet that may carry ink (可印区域),
// expressed as a rectangle on the sheet plus the grip allowance reserved on
// the grip edge.
type PrintableArea struct {
	Bounds      Rect     `json:"bounds"`
	GripEdge    GripEdge `json:"grip_edge"`
	GripAllowMM float64  `json:"grip_allow_mm"`
}

func (area PrintableArea) Validate(sheet SheetSize) error {
	if err := sheet.Validate(); err != nil {
		return err
	}
	if err := area.Bounds.Validate(); err != nil {
		return err
	}
	if !validGripEdge(area.GripEdge) {
		return errors.New("grip edge is invalid")
	}
	if area.GripAllowMM < 0 {
		return errors.New("grip allowance cannot be negative")
	}
	sheetBounds := Rect{XMM: 0, YMM: 0, WidthMM: sheet.WidthMM, HeightMM: sheet.HeightMM}
	if !area.Bounds.FitsInside(sheetBounds) {
		return errors.New("printable area lies outside the sheet")
	}
	switch area.GripEdge {
	case GripLeading:
		if area.Bounds.YMM < area.GripAllowMM-1e-6 {
			return errors.New("printable area overlaps the leading grip strip")
		}
	case GripTrailing:
		if area.Bounds.YMM+area.Bounds.HeightMM > sheet.HeightMM-area.GripAllowMM+1e-6 {
			return errors.New("printable area overlaps the trailing grip strip")
		}
	case GripLeft:
		if area.Bounds.XMM < area.GripAllowMM-1e-6 {
			return errors.New("printable area overlaps the left grip strip")
		}
	case GripRight:
		if area.Bounds.XMM+area.Bounds.WidthMM > sheet.WidthMM-area.GripAllowMM+1e-6 {
			return errors.New("printable area overlaps the right grip strip")
		}
	}
	return nil
}

// PlateSide selects which side of the press sheet a plate position belongs to.
type PlateSide string

const (
	SideFront PlateSide = "front"
	SideBack  PlateSide = "back"
)

func validPlateSide(side PlateSide) bool {
	return side == SideFront || side == SideBack
}

// PlanInput is the operator request: the finished page sequence (成品页序),
// the stock size and the printable area.
type PlanInput struct {
	PageSequence []int         `json:"page_sequence"`
	Sheet        SheetSize     `json:"sheet"`
	Area         PrintableArea `json:"area"`
	Colors       []string      `json:"colors"`
	Columns      int           `json:"columns"`
	Rows         int           `json:"rows"`
}

func (input PlanInput) Validate() error {
	if len(input.PageSequence) == 0 {
		return errors.New("finished page sequence is required")
	}
	seen := map[int]bool{}
	for _, page := range input.PageSequence {
		if page <= 0 {
			return errors.New("finished page numbers must be positive")
		}
		if seen[page] {
			return fmt.Errorf("finished page %d appears more than once", page)
		}
		seen[page] = true
	}
	if err := input.Area.Validate(input.Sheet); err != nil {
		return err
	}
	if len(input.Colors) == 0 {
		return errors.New("at least one ink color is required")
	}
	for _, color := range input.Colors {
		if color == "" {
			return errors.New("ink color names cannot be blank")
		}
	}
	if input.Columns <= 0 || input.Rows <= 0 {
		return errors.New("plate grid columns and rows must be positive")
	}
	capacity := input.Columns * input.Rows * 2
	if capacity < len(input.PageSequence) {
		return fmt.Errorf("plate grid holds %d pages but %d were supplied", capacity, len(input.PageSequence))
	}
	return nil
}

// PlatePosition is one cell of one color plate: where the cell sits on the
// press sheet and which finished page it carries after folding and trimming.
type PlatePosition struct {
	PlanID         PlanID    `json:"plan_id"`
	PlanGeneration uint64    `json:"plan_generation"`
	Color          string    `json:"color"`
	Side           PlateSide `json:"side"`
	Column         int       `json:"column"`
	Row            int       `json:"row"`
	Slot           int       `json:"slot"`
	Rect           Rect      `json:"rect"`
	FinishedPage   int       `json:"finished_page"`
	Orientation    string    `json:"orientation"`
}

func (position PlatePosition) validate(sheet SheetSize) error {
	if !position.PlanID.Valid() || position.PlanGeneration == 0 {
		return errors.New("plate position is not bound to a plan version")
	}
	if position.Color == "" || !validPlateSide(position.Side) {
		return errors.New("plate position identity is incomplete")
	}
	if position.Column < 0 || position.Row < 0 || position.Slot < 0 {
		return errors.New("plate position grid references are invalid")
	}
	if err := position.Rect.Validate(); err != nil {
		return err
	}
	if !position.Rect.FitsInside(Rect{WidthMM: sheet.WidthMM, HeightMM: sheet.HeightMM}) {
		return errors.New("plate position lies outside the sheet")
	}
	if position.FinishedPage <= 0 {
		return errors.New("plate position must reference a finished page")
	}
	return nil
}

// BelongsTo reports whether the position was produced by the given plan
// version. Positions from a superseded generation never belong to the current
// plan and must not be interpreted as its layout.
func (position PlatePosition) BelongsTo(plan ImpositionPlan) bool {
	return position.PlanID == plan.ID && position.PlanGeneration == plan.Generation
}

// TrimmedEntry maps a cut-and-trim output slot back to the finished page it
// yields, in the order signatures leave the folder (裁后页序).
type TrimmedEntry struct {
	Order        int `json:"order"`
	Slot         int `json:"slot"`
	FinishedPage int `json:"finished_page"`
}

// ImpositionPlan is an immutable, versioned imposition plan. Changing the
// inputs always produces a new PlanGeneration under the same PlanID; the
// positions recorded for an old generation are never reinterpreted as the new
// layout.
type ImpositionPlan struct {
	ID           PlanID          `json:"id"`
	JobID        JobID           `json:"job_id"`
	Generation   uint64          `json:"generation"`
	Supersedes   uint64          `json:"supersedes"`
	PageSequence []int           `json:"page_sequence"`
	Sheet        SheetSize       `json:"sheet"`
	Area         PrintableArea   `json:"area"`
	Colors       []string        `json:"colors"`
	Columns      int             `json:"columns"`
	Rows         int             `json:"rows"`
	FoldScheme   string          `json:"fold_scheme"`
	Positions    []PlatePosition `json:"positions"`
	TrimmedOrder []TrimmedEntry  `json:"trimmed_order"`
	CreatedAt    time.Time       `json:"created_at"`
}

func (plan ImpositionPlan) Validate() error {
	if !plan.ID.Valid() || !plan.JobID.Valid() {
		return errors.New("imposition plan identity is incomplete")
	}
	if plan.Generation == 0 {
		return errors.New("imposition plan generation is required")
	}
	if len(plan.PageSequence) == 0 {
		return errors.New("imposition plan has no page sequence")
	}
	if err := plan.Area.Validate(plan.Sheet); err != nil {
		return err
	}
	if len(plan.Colors) == 0 || plan.Columns <= 0 || plan.Rows <= 0 {
		return errors.New("imposition plan layout is incomplete")
	}
	if plan.FoldScheme == "" {
		return errors.New("imposition plan fold scheme is required")
	}
	if len(plan.Positions) == 0 {
		return errors.New("imposition plan has no plate positions")
	}
	pageSlots := map[int]int{}
	colorSets := map[string]map[int]bool{}
	for _, position := range plan.Positions {
		if err := position.validate(plan.Sheet); err != nil {
			return err
		}
		if position.PlanID != plan.ID || position.PlanGeneration != plan.Generation {
			return fmt.Errorf("plate position belongs to plan %s generation %d, not generation %d", position.PlanID, position.PlanGeneration, plan.Generation)
		}
		set, ok := colorSets[position.Color]
		if !ok {
			set = map[int]bool{}
			colorSets[position.Color] = set
		}
		// A leaf slot carries one page per side, so key slot and side together.
		sideOrdinal := 0
		if position.Side == SideBack {
			sideOrdinal = 1
		}
		key := position.Slot*2 + sideOrdinal
		if set[key] {
			return fmt.Errorf("color %s places more than one page on slot %d side %s", position.Color, position.Slot, position.Side)
		}
		set[key] = true
		pageSlots[position.FinishedPage]++
	}
	if len(colorSets) != len(plan.Colors) {
		return errors.New("plate positions do not cover every ink color")
	}
	for _, page := range plan.PageSequence {
		if pageSlots[page] != len(plan.Colors) {
			return fmt.Errorf("finished page %d is not placed on every color plate", page)
		}
	}
	if len(plan.TrimmedOrder) != len(plan.PageSequence) {
		return errors.New("trimmed order does not cover the page sequence")
	}
	trimmedPages := map[int]bool{}
	for index, entry := range plan.TrimmedOrder {
		if entry.Order != index+1 || entry.Slot < 0 || entry.FinishedPage <= 0 {
			return errors.New("trimmed order entries are invalid")
		}
		if trimmedPages[entry.FinishedPage] {
			return fmt.Errorf("trimmed order repeats finished page %d", entry.FinishedPage)
		}
		trimmedPages[entry.FinishedPage] = true
	}
	for _, page := range plan.PageSequence {
		if !trimmedPages[page] {
			return fmt.Errorf("trimmed order is missing finished page %d", page)
		}
	}
	if plan.CreatedAt.IsZero() {
		return errors.New("imposition plan creation time is required")
	}
	return nil
}

// Matches reports whether the plan still reflects the supplied inputs. A
// revision is required whenever this returns false.
func (plan ImpositionPlan) Matches(input PlanInput) bool {
	if len(plan.PageSequence) != len(input.PageSequence) ||
		plan.Columns != input.Columns || plan.Rows != input.Rows ||
		len(plan.Colors) != len(input.Colors) ||
		plan.Sheet != input.Sheet || plan.Area != input.Area {
		return false
	}
	for index, page := range input.PageSequence {
		if plan.PageSequence[index] != page {
			return false
		}
	}
	for index, color := range input.Colors {
		if plan.Colors[index] != color {
			return false
		}
	}
	return true
}
