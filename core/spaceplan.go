package core

import "fmt"

// SpacePlanType is the outcome of measuring an element against a given amount
// of available space.
type SpacePlanType int

const (
	// SpaceWrap means the element cannot render anything useful in the space
	// offered and wants to be retried on a fresh page.
	SpaceWrap SpacePlanType = iota

	// SpacePartialRender means the element rendered as much as fit and has
	// content left over for the next page. The document loop will draw it,
	// start a new page, and measure the same element again without resetting
	// its state.
	SpacePartialRender

	// SpaceFullRender means the element fully fits and will be finished after
	// one draw.
	SpaceFullRender
)

func (t SpacePlanType) String() string {
	switch t {
	case SpaceWrap:
		return "Wrap"
	case SpacePartialRender:
		return "PartialRender"
	case SpaceFullRender:
		return "FullRender"
	}
	return "Unknown"
}

// SpacePlan is the result of Element.Measure: how much space the element wants
// and whether that covers all of its content.
//
// This three-state result is the heart of the layout engine. An element that
// only ever answers "I need N points" cannot participate in pagination; by
// distinguishing PartialRender from FullRender, every element can split itself
// across pages, and containers can split because their children can.
type SpacePlan struct {
	Type SpacePlanType
	Size Size

	// WrapReason explains why an element wrapped. It is surfaced in layout
	// error messages, where "why did this not fit" is the only question worth
	// answering.
	WrapReason string
}

// Wrap builds a SpaceWrap plan carrying a human-readable reason.
func Wrap(reason string, args ...any) SpacePlan {
	if len(args) > 0 {
		reason = fmt.Sprintf(reason, args...)
	}
	return SpacePlan{Type: SpaceWrap, WrapReason: reason}
}

// PartialRender builds a plan for an element that rendered part of its content.
func PartialRender(size Size) SpacePlan {
	return SpacePlan{Type: SpacePartialRender, Size: size}
}

// FullRender builds a plan for an element that fully fits.
func FullRender(size Size) SpacePlan {
	return SpacePlan{Type: SpaceFullRender, Size: size}
}

// EmptyRender is a FullRender of zero size, the plan for an element with
// nothing to draw.
func EmptyRender() SpacePlan {
	return SpacePlan{Type: SpaceFullRender}
}

// Wrapped reports whether the plan is a wrap.
func (p SpacePlan) Wrapped() bool { return p.Type == SpaceWrap }

// Partial reports whether the plan has content remaining.
func (p SpacePlan) Partial() bool { return p.Type == SpacePartialRender }

// Full reports whether the plan covers all remaining content.
func (p SpacePlan) Full() bool { return p.Type == SpaceFullRender }

func (p SpacePlan) String() string {
	if p.Wrapped() {
		return fmt.Sprintf("Wrap(%s)", p.WrapReason)
	}
	return fmt.Sprintf("%s(%.2f x %.2f)", p.Type, p.Size.Width, p.Size.Height)
}
