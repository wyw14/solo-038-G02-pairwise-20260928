package imposition

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// PlanID identifies one imposition plan lineage. Every revision of a plan
// shares the PlanID; the revision number distinguishes versions inside it.
type PlanID string

func NewPlanID() PlanID {
	return PlanID(uuid.NewString())
}

func (id PlanID) Valid() bool {
	_, err := uuid.Parse(string(id))
	return err == nil
}

var (
	ErrPlanNotFound     = errors.New("imposition plan is unknown")
	ErrRevisionNotFound = errors.New("imposition plan revision is unknown")
	errUnitsRequired    = errors.New("imposition plan requires distinct colour units")
)

// SizeMM is a sheet or plate extent in millimetres.
type SizeMM struct {
	WidthMM  float64 `json:"width_mm"`
	HeightMM float64 `json:"height_mm"`
}

// RectMM is an axis-aligned rectangle in millimetres. The origin is the
// corner nearest the gripper (leading) edge of the sheet.
type RectMM struct {
	XMM      float64 `json:"x_mm"`
	YMM      float64 `json:"y_mm"`
	WidthMM  float64 `json:"width_mm"`
	HeightMM float64 `json:"height_mm"`
}

// GripperEdge names the sheet edge the press grippers hold.
type GripperEdge string

const (
	GripperLeading  GripperEdge = "leading"
	GripperTrailing GripperEdge = "trailing"
	GripperLeft     GripperEdge = "left"
	GripperRight    GripperEdge = "right"
)

// Gripper describes where the press bites the sheet and how deep the
// unprintable bite margin is.
type Gripper struct {
	Edge   GripperEdge `json:"edge"`
	BiteMM float64     `json:"bite_mm"`
}

// FoldAxis is the axis of one physical fold applied to the printed sheet.
// Folds are listed in the order the folder performs them.
type FoldAxis string

const (
	FoldVertical   FoldAxis = "vertical"
	FoldHorizontal FoldAxis = "horizontal"
)

// Side names one face of the printed sheet.
type Side string

const (
	SideFront Side = "front"
	SideBack  Side = "back"
)

// Input carries everything needed to lay finished pages out on a sheet:
// the finished page sequence, the sheet size, the printable area, the
// gripper position and the fold sequence.
type Input struct {
	FinishedPages int       `json:"finished_pages"`
	PageOrder     []int     `json:"page_order,omitempty"`
	Sheet         SizeMM    `json:"sheet"`
	Printable     RectMM    `json:"printable"`
	Gripper       Gripper   `json:"gripper"`
	Folds         []FoldAxis `json:"folds"`
}

// PagesPerSignature derives the finished pages one folded signature holds:
// every fold doubles the leaf count and each leaf carries two pages.
func PagesPerSignature(folds int) int {
	return 1 << (folds + 1)
}

func (input Input) pagesPerSignature() int {
	return PagesPerSignature(len(input.Folds))
}

// orderedPage maps a 1-based position in the finished reading sequence to
// the emitted finished page number, honouring an explicit PageOrder.
func (input Input) orderedPage(position int) int {
	if len(input.PageOrder) == 0 {
		return position
	}
	return input.PageOrder[position-1]
}

func (input Input) Validate() error {
	if input.FinishedPages <= 0 {
		return fmt.Errorf("finished page count is required")
	}
	if len(input.Folds) == 0 {
		return fmt.Errorf("fold sequence is required")
	}
	for _, fold := range input.Folds {
		if fold != FoldVertical && fold != FoldHorizontal {
			return fmt.Errorf("fold axis %q is unsupported", fold)
		}
	}
	perSignature := input.pagesPerSignature()
	if input.FinishedPages%perSignature != 0 {
		return fmt.Errorf("finished pages %d do not fill whole signatures of %d pages", input.FinishedPages, perSignature)
	}
	if input.Sheet.WidthMM <= 0 || input.Sheet.HeightMM <= 0 {
		return fmt.Errorf("sheet size is invalid")
	}
	printable := input.Printable
	if printable.WidthMM <= 0 || printable.HeightMM <= 0 {
		return fmt.Errorf("printable area is invalid")
	}
	if printable.XMM < 0 || printable.YMM < 0 || printable.XMM+printable.WidthMM > input.Sheet.WidthMM || printable.YMM+printable.HeightMM > input.Sheet.HeightMM {
		return fmt.Errorf("printable area lies outside the sheet")
	}
	if input.Gripper.BiteMM < 0 {
		return fmt.Errorf("gripper bite is invalid")
	}
	var inset float64
	switch input.Gripper.Edge {
	case GripperLeading:
		inset = printable.YMM
	case GripperTrailing:
		inset = input.Sheet.HeightMM - (printable.YMM + printable.HeightMM)
	case GripperLeft:
		inset = printable.XMM
	case GripperRight:
		inset = input.Sheet.WidthMM - (printable.XMM + printable.WidthMM)
	default:
		return fmt.Errorf("gripper edge %q is unsupported", input.Gripper.Edge)
	}
	if inset < input.Gripper.BiteMM {
		return fmt.Errorf("printable area intrudes into the gripper bite")
	}
	if len(input.PageOrder) > 0 {
		if len(input.PageOrder) != input.FinishedPages {
			return fmt.Errorf("page order must list every finished page")
		}
		seen := make(map[int]bool, len(input.PageOrder))
		for _, page := range input.PageOrder {
			if page < 1 || page > input.FinishedPages || seen[page] {
				return fmt.Errorf("page order is not a permutation of the finished pages")
			}
			seen[page] = true
		}
	}
	return nil
}

// Cell is one finished page's placement on one side of one signature sheet.
type Cell struct {
	Signature int    `json:"signature"`
	Row       int    `json:"row"`
	Col       int    `json:"col"`
	Rect      RectMM `json:"rect"`
	Page      int    `json:"page"`
	Rotation  int    `json:"rotation"`
}

// PlatePosition is the layout table for one colour unit on one sheet side:
// which finished page sits in which grid cell.
type PlatePosition struct {
	Unit  string `json:"unit"`
	Side  Side   `json:"side"`
	Cells []Cell `json:"cells"`
}

// TrimEntry maps one finished page (post-trim reading order) back to the
// sheet position it was printed from.
type TrimEntry struct {
	Page      int  `json:"page"`
	Signature int  `json:"signature"`
	Side      Side `json:"side"`
	Row       int  `json:"row"`
	Col       int  `json:"col"`
}

// Revision is one immutable version of an imposition plan. Revisions are
// never edited in place: changing the plan creates a new Revision so plate
// positions recorded against an older revision keep their meaning.
type Revision struct {
	PlanID            PlanID          `json:"plan_id"`
	Revision          uint64          `json:"revision"`
	Input             Input           `json:"input"`
	Units             []string        `json:"units"`
	PagesPerSignature int             `json:"pages_per_signature"`
	Signatures        int             `json:"signatures"`
	Positions         []PlatePosition `json:"positions"`
	TrimOrder         []TrimEntry     `json:"trim_order"`
	CreatedAt         time.Time       `json:"created_at"`
}

func (revision Revision) Validate() error {
	if !revision.PlanID.Valid() {
		return fmt.Errorf("imposition plan id is invalid")
	}
	if revision.Revision == 0 {
		return fmt.Errorf("imposition plan revision is required")
	}
	if err := revision.Input.Validate(); err != nil {
		return err
	}
	if len(revision.Units) == 0 {
		return fmt.Errorf("imposition plan requires at least one colour unit")
	}
	seen := make(map[string]bool, len(revision.Units))
	for _, unit := range revision.Units {
		if unit == "" || seen[unit] {
			return fmt.Errorf("imposition plan colour units are invalid")
		}
		seen[unit] = true
	}
	if revision.PagesPerSignature != revision.Input.pagesPerSignature() {
		return fmt.Errorf("imposition plan signature size is inconsistent")
	}
	if revision.Signatures <= 0 || revision.Signatures*revision.PagesPerSignature != revision.Input.FinishedPages {
		return fmt.Errorf("imposition plan signature count is inconsistent")
	}
	if len(revision.Positions) != len(revision.Units)*2 {
		return fmt.Errorf("imposition plan positions do not cover every unit and side")
	}
	if len(revision.TrimOrder) != revision.Input.FinishedPages {
		return fmt.Errorf("imposition plan trim order is incomplete")
	}
	if revision.CreatedAt.IsZero() {
		return fmt.Errorf("imposition plan timestamp is required")
	}
	return nil
}
