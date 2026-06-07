package asdex

import (
	redsmath "github.com/juliusplatzer/reds/math"
	"github.com/juliusplatzer/reds/renderer"
)

type DcbPosition int

const (
	DcbTop DcbPosition = iota
	DcbBottom
	DcbLeft
	DcbRight
	DcbOff
)

type DcbMenu int

const (
	DcbMenuMain DcbMenu = iota
	DcbMenuOff
)

type DcbButtonKind int

const (
	DcbButtonNormal DcbButtonKind = iota
	DcbButtonMenu
	DcbButtonValue
	DcbButtonToggle
	DcbButtonError
	DcbButtonVacant
)

type DcbFunction int

const (
	DcbFunctionVacant DcbFunction = iota
	DcbFunctionRange
	DcbFunctionMapReposition
	DcbFunctionRotate
	DcbFunctionUndo
	DcbFunctionDefault
	DcbFunctionPrefs
	DcbFunctionDayNite
	DcbFunctionBrightness
	DcbFunctionCharSize
	DcbFunctionSafetyLogic
	DcbFunctionTools
	DcbFunctionVectorOnOff
	DcbFunctionVectorLength
	DcbFunctionTempData
	DcbFunctionLeaderLength
	DcbFunctionLocal1
	DcbFunctionLocal2
	DcbFunctionDataBlockArea
	DcbFunctionDataBlockEdit
	DcbFunctionDataBlocksOnOff
	DcbFunctionInitControl
	DcbFunctionTrackSuspend
	DcbFunctionTermControl
	DcbFunctionDcbOnOff
	DcbFunctionOperationalMode
	DcbFunctionMlatOff
	DcbFunctionAsrOff
)

const (
	dcbButtonSpacing = 3
	dcbColumnCount   = 14
	dcbMinBrightness = 20
)

var (
	dcbBackgroundRGB  = renderer.RGB8(56, 56, 56)
	dcbMenuSlabRGB    = renderer.RGB8(100, 100, 100)
	dcbButtonRGB      = renderer.RGB8(56, 56, 56)
	dcbMenuButtonRGB  = renderer.RGB8(80, 80, 80)
	dcbDepressedRGB   = renderer.RGB8(45, 45, 45)
	dcbErrorButtonRGB = renderer.RGB8(255, 0, 0)
)

type Dcb struct {
	visible    bool
	position   DcbPosition
	menu       DcbMenu
	brightness int
	charSize   int
}

type DcbButtonSpec struct {
	Function  DcbFunction
	Kind      DcbButtonKind
	Large     bool
	Visible   bool
	Depressed bool
	Active    bool
}

type DcbButtonLayout struct {
	Spec   DcbButtonSpec
	Bounds redsmath.Rect
	Index  int
}

type DcbLayout struct {
	Bounds     redsmath.Rect
	MenuBounds redsmath.Rect

	ButtonSize redsmath.Vec2
	MenuSize   redsmath.Vec2

	AutoSize       int
	RenderFontSize int

	Buttons []DcbButtonLayout
}

func NewDcb() Dcb {
	return Dcb{
		visible:    true,
		position:   DcbTop,
		menu:       DcbMenuMain,
		brightness: brightnessDefault,
		charSize:   2,
	}
}

// TODO(DCB): Keep all layout code position-aware. CRC supports TOP, BOTTOM,
// LEFT, and RIGHT DCB positions. Button text and behavior are not implemented
// yet, but Layout already returns correct bar/slab/button bounds for all
// positions.
func (p DcbPosition) IsHorizontal() bool {
	return p == DcbTop || p == DcbBottom
}

func (d *Dcb) Visible() bool {
	return d != nil && d.visible && d.position != DcbOff
}

func (d *Dcb) SetPosition(position DcbPosition) {
	if d == nil {
		return
	}
	d.position = position
}

func (d *Dcb) Position() DcbPosition {
	if d == nil {
		return DcbOff
	}
	return d.position
}

func (d *Dcb) SetCharSize(size int) {
	if d == nil {
		return
	}
	d.charSize = clampInt(size, 1, 3)
}

func (d *Dcb) buttonSizeForFont(font *renderer.BitmapFont, autoSize int) redsmath.Vec2 {
	if font == nil {
		return redsmath.Vec2{}
	}

	_, charHeight := font.CharSize(autoSize)
	if charHeight <= 0 {
		return redsmath.Vec2{}
	}

	buttonHeight := float32(charHeight*2 + 9)
	return redsmath.Vec2{
		X: buttonHeight * 3,
		Y: buttonHeight,
	}
}

func horizontalDcbMenuSize(button redsmath.Vec2) redsmath.Vec2 {
	return redsmath.Vec2{
		X: (button.X+float32(dcbButtonSpacing))*float32(dcbColumnCount) + float32(dcbButtonSpacing),
		Y: button.Y*2 + 9,
	}
}

func verticalDcbMenuSize(button redsmath.Vec2) redsmath.Vec2 {
	return redsmath.Vec2{
		X: button.X + 6,
		Y: button.Y*float32(dcbColumnCount)*2 + 87,
	}
}

func (d *Dcb) buttonColor(spec DcbButtonSpec) renderer.RGB {
	if d == nil {
		return dcbButtonRGB
	}
	if spec.Depressed {
		return applyBrightness(dcbDepressedRGB, d.brightness, dcbMinBrightness)
	}

	switch spec.Kind {
	case DcbButtonMenu:
		return applyBrightness(dcbMenuButtonRGB, d.brightness, dcbMinBrightness)
	case DcbButtonError:
		return applyBrightness(dcbErrorButtonRGB, d.brightness, dcbMinBrightness)
	default:
		return applyBrightness(dcbButtonRGB, d.brightness, dcbMinBrightness)
	}
}

func isLargeDcbFunction(function DcbFunction) bool {
	switch function {
	case DcbFunctionRange,
		DcbFunctionSafetyLogic,
		DcbFunctionTools,
		DcbFunctionVacant:
		return true
	default:
		return false
	}
}

func (d *Dcb) mainButtonSpecs() []DcbButtonSpec {
	normal := func(function DcbFunction) DcbButtonSpec {
		return DcbButtonSpec{
			Function: function,
			Kind:     DcbButtonNormal,
			Large:    isLargeDcbFunction(function),
			Visible:  true,
		}
	}
	menu := func(function DcbFunction) DcbButtonSpec {
		return DcbButtonSpec{
			Function: function,
			Kind:     DcbButtonMenu,
			Large:    isLargeDcbFunction(function),
			Visible:  true,
		}
	}
	value := func(function DcbFunction) DcbButtonSpec {
		return DcbButtonSpec{
			Function: function,
			Kind:     DcbButtonValue,
			Large:    isLargeDcbFunction(function),
			Visible:  true,
		}
	}
	toggle := func(function DcbFunction) DcbButtonSpec {
		return DcbButtonSpec{
			Function: function,
			Kind:     DcbButtonToggle,
			Large:    isLargeDcbFunction(function),
			Visible:  true,
		}
	}

	return []DcbButtonSpec{
		value(DcbFunctionRange),
		normal(DcbFunctionMapReposition),
		value(DcbFunctionRotate),
		normal(DcbFunctionUndo),
		normal(DcbFunctionDefault),
		menu(DcbFunctionPrefs),
		toggle(DcbFunctionDayNite),
		menu(DcbFunctionBrightness),
		menu(DcbFunctionCharSize),
		menu(DcbFunctionSafetyLogic),
		menu(DcbFunctionTools),
		toggle(DcbFunctionVectorOnOff),
		value(DcbFunctionVectorLength),
		menu(DcbFunctionTempData),
		value(DcbFunctionLeaderLength),
		menu(DcbFunctionLocal1),
		menu(DcbFunctionLocal2),
		menu(DcbFunctionDataBlockArea),
		menu(DcbFunctionDataBlockEdit),
		toggle(DcbFunctionDataBlocksOnOff),
		normal(DcbFunctionInitControl),
		normal(DcbFunctionTrackSuspend),
		normal(DcbFunctionTermControl),
		toggle(DcbFunctionDcbOnOff),
		menu(DcbFunctionOperationalMode),
	}
}

func (d *Dcb) buttonSpecs() []DcbButtonSpec {
	if d == nil || d.menu == DcbMenuOff {
		return nil
	}
	return d.mainButtonSpecs()
}

func (d *Dcb) Layout(displaySize redsmath.Vec2, font *renderer.BitmapFont) DcbLayout {
	var out DcbLayout
	if d == nil || !d.Visible() || font == nil || displaySize.X <= 0 || displaySize.Y <= 0 {
		return out
	}

	autoSize := 3
	var buttonSize redsmath.Vec2
	var menuSize redsmath.Vec2
	for autoSize >= 1 {
		buttonSize = d.buttonSizeForFont(font, autoSize)
		if buttonSize.X <= 0 || buttonSize.Y <= 0 {
			return DcbLayout{}
		}

		if d.position.IsHorizontal() {
			menuSize = horizontalDcbMenuSize(buttonSize)
			if autoSize == 1 || displaySize.X >= menuSize.X {
				break
			}
		} else {
			menuSize = verticalDcbMenuSize(buttonSize)
			if autoSize == 1 || displaySize.Y >= menuSize.Y {
				break
			}
		}
		autoSize--
	}

	out.AutoSize = autoSize
	out.RenderFontSize = autoSize
	charSize := clampInt(d.charSize, 1, 3)
	if charSize < out.RenderFontSize {
		out.RenderFontSize = charSize
	}
	out.ButtonSize = buttonSize
	out.MenuSize = menuSize

	switch d.position {
	case DcbTop:
		out.Bounds = redsmath.NewRect(0, 0, displaySize.X, menuSize.Y)
		menuX := float32(0)
		if displaySize.X > menuSize.X {
			menuX = (displaySize.X - menuSize.X) * 0.5
		}
		out.MenuBounds = redsmath.NewRect(menuX, 0, menuX+menuSize.X, menuSize.Y)

	case DcbBottom:
		y := displaySize.Y - menuSize.Y
		if y < 0 {
			y = 0
		}
		out.Bounds = redsmath.NewRect(0, y, displaySize.X, y+menuSize.Y)
		menuX := float32(0)
		if displaySize.X > menuSize.X {
			menuX = (displaySize.X - menuSize.X) * 0.5
		}
		out.MenuBounds = redsmath.NewRect(menuX, y, menuX+menuSize.X, y+menuSize.Y)

	case DcbLeft:
		out.Bounds = redsmath.NewRect(0, 0, menuSize.X, displaySize.Y)
		menuY := float32(0)
		if displaySize.Y > menuSize.Y {
			menuY = (displaySize.Y - menuSize.Y) * 0.5
		}
		out.MenuBounds = redsmath.NewRect(0, menuY, menuSize.X, menuY+menuSize.Y)

	case DcbRight:
		x := displaySize.X - menuSize.X
		if x < 0 {
			x = 0
		}
		out.Bounds = redsmath.NewRect(x, 0, x+menuSize.X, displaySize.Y)
		menuY := float32(0)
		if displaySize.Y > menuSize.Y {
			menuY = (displaySize.Y - menuSize.Y) * 0.5
		}
		out.MenuBounds = redsmath.NewRect(x, menuY, x+menuSize.X, menuY+menuSize.Y)
	}

	out.Buttons = d.layoutButtons(out.MenuBounds, out.ButtonSize, d.buttonSpecs())
	return out
}

func (d *Dcb) layoutButtons(
	menuBounds redsmath.Rect,
	buttonSize redsmath.Vec2,
	specs []DcbButtonSpec,
) []DcbButtonLayout {
	if d == nil || menuBounds.Empty() || buttonSize.X <= 0 || buttonSize.Y <= 0 {
		return nil
	}
	if d.position.IsHorizontal() {
		return layoutHorizontalDcbButtons(menuBounds, buttonSize, specs)
	}
	return layoutVerticalDcbButtons(menuBounds, buttonSize, specs)
}

func layoutHorizontalDcbButtons(
	menuBounds redsmath.Rect,
	buttonSize redsmath.Vec2,
	specs []DcbButtonSpec,
) []DcbButtonLayout {
	buttons := make([]DcbButtonLayout, 0, len(specs))
	row := 1
	column := 1

	for _, spec := range specs {
		if !spec.Visible {
			continue
		}
		if column > dcbColumnCount {
			break
		}

		x := menuBounds.Min.X +
			float32(column*dcbButtonSpacing) +
			float32(column-1)*buttonSize.X

		y := menuBounds.Min.Y + float32(dcbButtonSpacing)
		if row == 2 {
			y = menuBounds.Min.Y + float32(2*dcbButtonSpacing) + buttonSize.Y
		}

		height := buttonSize.Y
		if spec.Large {
			height = buttonSize.Y*2 + float32(dcbButtonSpacing)
		}

		buttons = append(buttons, DcbButtonLayout{
			Spec: spec,
			Bounds: redsmath.NewRect(
				x,
				y,
				x+buttonSize.X,
				y+height,
			),
			Index: len(buttons),
		})

		if row == 2 || (row == 1 && spec.Large) {
			column++
			row = 1
		} else {
			row = 2
		}
	}

	return buttons
}

func layoutVerticalDcbButtons(
	menuBounds redsmath.Rect,
	buttonSize redsmath.Vec2,
	specs []DcbButtonSpec,
) []DcbButtonLayout {
	buttons := make([]DcbButtonLayout, 0, len(specs))

	x := menuBounds.Min.X + float32(dcbButtonSpacing)
	y := menuBounds.Min.Y + float32(dcbButtonSpacing)

	for _, spec := range specs {
		if !spec.Visible {
			continue
		}

		height := buttonSize.Y
		if spec.Large {
			height = buttonSize.Y*2 + float32(dcbButtonSpacing)
		}
		if y+height > menuBounds.Max.Y {
			break
		}

		buttons = append(buttons, DcbButtonLayout{
			Spec: spec,
			Bounds: redsmath.NewRect(
				x,
				y,
				x+buttonSize.X,
				y+height,
			),
			Index: len(buttons),
		})

		y += height + float32(dcbButtonSpacing)
	}

	return buttons
}

func (d *Dcb) DrawBackground(cb *renderer.CmdBuffer, layout DcbLayout) {
	if d == nil || cb == nil || layout.Bounds.Empty() {
		return
	}

	builder := renderer.GetColoredTrianglesBuilder()
	defer renderer.ReturnColoredTrianglesBuilder(builder)

	background := applyBrightness(dcbBackgroundRGB, d.brightness, dcbMinBrightness)
	menuSlab := applyBrightness(dcbMenuSlabRGB, d.brightness, dcbMinBrightness)

	addDcbRect(builder, layout.Bounds, background)
	if !layout.MenuBounds.Empty() {
		addDcbRect(builder, layout.MenuBounds, menuSlab)
	}

	builder.GenerateCommands(cb)
}

func (d *Dcb) DrawButtons(cb *renderer.CmdBuffer, layout DcbLayout) {
	if d == nil || cb == nil || layout.Bounds.Empty() || len(layout.Buttons) == 0 {
		return
	}

	builder := renderer.GetColoredTrianglesBuilder()
	defer renderer.ReturnColoredTrianglesBuilder(builder)

	for _, button := range layout.Buttons {
		if button.Bounds.Empty() {
			continue
		}
		addDcbRect(builder, button.Bounds, d.buttonColor(button.Spec))
	}

	builder.GenerateCommands(cb)
}

func addDcbRect(builder *renderer.ColoredTrianglesBuilder, rect redsmath.Rect, color renderer.RGB) {
	if builder == nil || rect.Empty() {
		return
	}

	builder.AddQuad(
		renderer.PointVertex{X: rect.Min.X, Y: rect.Min.Y},
		renderer.PointVertex{X: rect.Max.X, Y: rect.Min.Y},
		renderer.PointVertex{X: rect.Max.X, Y: rect.Max.Y},
		renderer.PointVertex{X: rect.Min.X, Y: rect.Max.Y},
		color,
	)
}

type DcbHit struct {
	OverDcb     bool
	ButtonIndex int
	Function    DcbFunction
	HasFunction bool
}

func (d *Dcb) HitTest(
	point redsmath.Vec2,
	displaySize redsmath.Vec2,
	font *renderer.BitmapFont,
) DcbHit {
	hit := DcbHit{ButtonIndex: -1}
	layout := d.Layout(displaySize, font)
	if layout.Bounds.Empty() || !layout.Bounds.Contains(point) {
		return hit
	}

	hit.OverDcb = true
	for i, button := range layout.Buttons {
		if !button.Bounds.Contains(point) {
			continue
		}

		hit.ButtonIndex = i
		if button.Spec.Function != DcbFunctionVacant {
			hit.Function = button.Spec.Function
			hit.HasFunction = true
		}
		break
	}
	return hit
}

func (d *Dcb) Contains(point redsmath.Vec2, displaySize redsmath.Vec2, font *renderer.BitmapFont) bool {
	return d.HitTest(point, displaySize, font).OverDcb
}
