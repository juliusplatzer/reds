package asdex

import (
	"fmt"
	stdmath "math"
	"strings"

	redsmath "github.com/juliusplatzer/reds/math"
	"github.com/juliusplatzer/reds/panes"
	"github.com/juliusplatzer/reds/platform"
	"github.com/juliusplatzer/reds/radar"
	"github.com/juliusplatzer/reds/renderer"
)

const (
	closedRunwayXAngleDeg         = 15.0
	closedRunwayBrightnessDefault = 95

	maxTempDataObjects = 100
	maxTempAreaNodes   = 20
	minTempAreaNodes   = 3

	tempMapAreasBrightnessDefault = 95
	tempAreaDrawLineWidth         = 1.0
)

var (
	tempClosedAreaRGB     = renderer.RGB8(255, 0, 0)
	tempRestrictedAreaRGB = renderer.RGB8(255, 255, 0)
	tempAreaDrawRGB       = renderer.RGB8(255, 255, 255)
	tempAreaHighlightRGB  = renderer.RGB8(0, 0, 255)
)

type TempDataType int

const (
	TempDataClosedArea TempDataType = iota
	TempDataRestrictedArea
)

type TempData struct {
	closedRunways map[string]bool

	closedAreas     []TempArea
	restrictedAreas []TempArea

	nextAreaID int
}

type TempArea struct {
	ID       string
	Type     TempDataType
	Points   []redsmath.Vec2
	Hidden   bool
	Selected bool

	meshVertices []renderer.PointVertex
	meshIndices  []uint32
}

type TempAreaDraft struct {
	Type   TempDataType
	Points []redsmath.Vec2
	Mouse  redsmath.Vec2
}

func NewTempData() TempData {
	return TempData{
		closedRunways: make(map[string]bool),
		nextAreaID:    1,
	}
}

func (td *TempData) DcbRunwayClosureStates(sl *SafetyLogic) []DcbRunwayClosureState {
	if td == nil || sl == nil {
		return nil
	}

	out := make([]DcbRunwayClosureState, 0, len(sl.runways))
	for _, rwy := range sl.runways {
		out = append(out, DcbRunwayClosureState{
			ID:       rwy.ID,
			IsClosed: td.closedRunways != nil && td.closedRunways[rwy.ID],
		})
	}
	return out
}

func (td *TempData) ToggleRunwayClosedByDcbIndex(sl *SafetyLogic, index int) bool {
	if td == nil || sl == nil || index < 1 || index > len(sl.runways) {
		return false
	}

	rwy := sl.runways[index-1]
	if rwy.ID == "" {
		return false
	}

	if td.closedRunways == nil {
		td.closedRunways = make(map[string]bool)
	}
	if td.closedRunways[rwy.ID] {
		delete(td.closedRunways, rwy.ID)
	} else {
		td.closedRunways[rwy.ID] = true
	}
	return true
}

func (td *TempData) RunwayClosed(id string) bool {
	if td == nil || td.closedRunways == nil {
		return false
	}
	id = strings.ToUpper(strings.TrimSpace(id))
	return td.closedRunways[id]
}

func (td *TempData) VisibleObjectCount() int {
	if td == nil {
		return 0
	}

	count := 0
	for _, area := range td.closedAreas {
		if !area.Hidden {
			count++
		}
	}
	for _, area := range td.restrictedAreas {
		if !area.Hidden {
			count++
		}
	}
	return count
}

func (td *TempData) AddArea(kind TempDataType, points []redsmath.Vec2) {
	if td == nil || len(points) < minTempAreaNodes+1 {
		return
	}

	vertices, indices := renderer.TessellateRings([][]redsmath.Vec2{points})
	if len(vertices) == 0 || len(indices) == 0 {
		return
	}

	area := TempArea{
		ID:           td.nextTempAreaID(kind),
		Type:         kind,
		Points:       append([]redsmath.Vec2(nil), points...),
		meshVertices: vertices,
		meshIndices:  indices,
	}

	switch kind {
	case TempDataClosedArea:
		td.closedAreas = append(td.closedAreas, area)
	case TempDataRestrictedArea:
		td.restrictedAreas = append(td.restrictedAreas, area)
	}
}

func (td *TempData) nextTempAreaID(kind TempDataType) string {
	if td.nextAreaID <= 0 {
		td.nextAreaID = 1
	}

	prefix := "CLOSED"
	if kind == TempDataRestrictedArea {
		prefix = "RESTR"
	}

	id := fmt.Sprintf("%s:%d", prefix, td.nextAreaID)
	td.nextAreaID++
	return id
}

func (td *TempData) DrawClosedRunways(
	cb *renderer.CmdBuffer,
	sl *SafetyLogic,
	brightness int,
) {
	if td == nil || sl == nil || cb == nil || len(td.closedRunways) == 0 {
		return
	}

	builder := renderer.GetLinesBuilder()
	defer renderer.ReturnLinesBuilder(builder)

	for _, rwy := range sl.runways {
		if !td.closedRunways[rwy.ID] {
			continue
		}
		buildClosedRunwayXLines(builder, rwy)
	}

	cb.SetRGB(applyBrightness(renderer.RGB8(255, 255, 255), brightness, brightnessFloorDefault))
	cb.LineWidth(1)
	builder.GenerateCommands(cb)
}

func (td *TempData) DrawClosedAreas(
	cb *renderer.CmdBuffer,
	transforms radar.ScopeTransformations,
	brightness int,
) {
	if td == nil {
		return
	}

	td.drawAreas(cb, transforms, td.closedAreas, tempClosedAreaRGB, brightness)
}

func (td *TempData) DrawRestrictedAreas(
	cb *renderer.CmdBuffer,
	transforms radar.ScopeTransformations,
	brightness int,
) {
	if td == nil {
		return
	}

	td.drawAreas(cb, transforms, td.restrictedAreas, tempRestrictedAreaRGB, brightness)
}

func (td *TempData) drawAreas(
	cb *renderer.CmdBuffer,
	transforms radar.ScopeTransformations,
	areas []TempArea,
	rgb renderer.RGB,
	brightness int,
) {
	if td == nil || cb == nil || len(areas) == 0 {
		return
	}

	color := applyBrightness(rgb, brightness, brightnessFloorDefault)
	lineBuilder := renderer.GetLinesBuilder()
	for _, area := range areas {
		if area.Hidden || len(area.Points) < 2 {
			continue
		}

		points := make([]renderer.PointVertex, 0, len(area.Points))
		for _, pt := range area.Points {
			points = append(points, renderer.PointVertex{X: pt.X, Y: pt.Y})
		}
		lineBuilder.AddLineStrip(points)
	}
	cb.SetRGB(color)
	cb.LineWidth(1)
	lineBuilder.GenerateCommands(cb)
	renderer.ReturnLinesBuilder(lineBuilder)

	for _, area := range areas {
		if area.Hidden || len(area.meshVertices) == 0 || len(area.meshIndices) == 0 {
			continue
		}

		triBuilder := renderer.GetTrianglesBuilder()
		triBuilder.AddIndexed(area.meshVertices, area.meshIndices)
		cb.SetRGB(color)
		triBuilder.GenerateCommands(cb, renderer.DrawHatched, tempAreaHatchOffset(area, transforms))
		renderer.ReturnTrianglesBuilder(triBuilder)
	}
}

func tempAreaHatchOffset(area TempArea, transforms radar.ScopeTransformations) float32 {
	if len(area.Points) == 0 {
		return 0
	}

	p := transforms.WindowFromWorldP(area.Points[0])
	return -float32(stdmath.Mod(float64(4*p.Y+p.X), 50))
}

func buildClosedRunwayXLines(
	builder *renderer.LinesBuilder,
	rwy surfaceRunway,
) {
	if builder == nil || rwy.LengthFeet <= 0 {
		return
	}

	c0 := runwayCorner(rwy, rwy.MinAlongFeet, rwy.MinAcrossFeet)
	c1 := runwayCorner(rwy, rwy.MaxAlongFeet, rwy.MinAcrossFeet)
	c2 := runwayCorner(rwy, rwy.MaxAlongFeet, rwy.MaxAcrossFeet)
	c3 := runwayCorner(rwy, rwy.MinAlongFeet, rwy.MaxAcrossFeet)

	angle := float32(closedRunwayXAngleDeg * stdmath.Pi / 180)
	addClosedXLine(builder, c0, closedXDirection(rwy, 1, 1, angle), c3, c2)
	addClosedXLine(builder, c1, closedXDirection(rwy, -1, 1, angle), c3, c2)
	addClosedXLine(builder, c2, closedXDirection(rwy, -1, -1, angle), c0, c1)
	addClosedXLine(builder, c3, closedXDirection(rwy, 1, -1, angle), c0, c1)
}

func closedXDirection(
	rwy surfaceRunway,
	alongSign float32,
	acrossSign float32,
	angleRad float32,
) redsmath.Vec2 {
	direction := rwy.AxisFeet.Mul(alongSign * float32(stdmath.Cos(float64(angleRad)))).
		Add(rwy.NormalFeet.Mul(acrossSign * float32(stdmath.Sin(float64(angleRad)))))
	normalized, ok := safetyNormalize(direction)
	if !ok {
		return redsmath.Vec2{}
	}
	return normalized
}

func runwayCorner(rwy surfaceRunway, along float32, across float32) redsmath.Vec2 {
	return rwy.CenterFeet.
		Add(rwy.AxisFeet.Mul(along)).
		Add(rwy.NormalFeet.Mul(across))
}

func addClosedXLine(
	builder *renderer.LinesBuilder,
	start redsmath.Vec2,
	dir redsmath.Vec2,
	edgeA redsmath.Vec2,
	edgeB redsmath.Vec2,
) {
	end, ok := intersectRaySegment(start, dir, edgeA, edgeB)
	if !ok {
		return
	}

	builder.AddLine(
		renderer.PointVertex{X: start.X, Y: start.Y},
		renderer.PointVertex{X: end.X, Y: end.Y},
	)
}

func intersectRaySegment(
	p redsmath.Vec2,
	r redsmath.Vec2,
	q redsmath.Vec2,
	sEnd redsmath.Vec2,
) (redsmath.Vec2, bool) {
	s := sEnd.Sub(q)
	denominator := cross2(r, s)
	if abs32(denominator) < degenerateRunwayAxisLength2 {
		return redsmath.Vec2{}, false
	}

	qmp := q.Sub(p)
	t := cross2(qmp, s) / denominator
	u := cross2(qmp, r) / denominator
	if t < 0 || u < 0 || u > 1 {
		return redsmath.Vec2{}, false
	}

	return p.Add(r.Mul(t)), true
}

func cross2(a, b redsmath.Vec2) float32 {
	return a.X*b.Y - a.Y*b.X
}

func (p *ASDEXPane) activateTempDataDcbHit(hit DcbHit) bool {
	if p == nil {
		return false
	}

	switch hit.Function {
	case DcbFunctionTempData:
		p.openTempDataDcbMenu()
		return true
	case DcbFunctionDone:
		p.closeDcbCurrentSubmenu()
		return true
	case DcbFunctionClosedRunway:
		p.openTempDataClosedRunwayDcbMenu()
		return true
	case DcbFunctionDefineClosedArea:
		p.startDefineClosedArea()
		return true
	case DcbFunctionDefineRestrictedArea:
		p.startDefineRestrictedArea()
		return true
	case DcbFunctionCloseRunway:
		if strings.TrimSpace(hit.Label) == "" {
			return true
		}
		p.tempData.ToggleRunwayClosedByDcbIndex(&p.safetyLogic, hit.ConfigID)
		p.previewArea.SetSystemResponse("")
		p.clearHighlightedTarget()
		return true
	case DcbFunctionStoredGlobalTempData,
		DcbFunctionDefineTempText,
		DcbFunctionShowHiddenTempData,
		DcbFunctionHideTempData,
		DcbFunctionDeleteGlobalTempData:
		return true
	default:
		return false
	}
}

func (p *ASDEXPane) startDefineClosedArea() {
	p.startDefineTempArea(TempDataClosedArea, "DEFINE CLOSED AREA")
}

func (p *ASDEXPane) startDefineRestrictedArea() {
	p.startDefineTempArea(TempDataRestrictedArea, "DEFINE RESTR AREA")
}

func (p *ASDEXPane) startDefineTempArea(kind TempDataType, commandLine string) {
	if p == nil {
		return
	}

	if p.tempData.VisibleObjectCount() >= maxTempDataObjects {
		p.previewArea.SetSystemResponse("ERROR: MAX LIMIT")
		return
	}

	p.clearTempDataCommandConflicts()
	p.dcbMenuCommand = NewDcbMenuCommand("TEMP DATA", commandLine)
	p.tempAreaDraft = &TempAreaDraft{
		Type:   kind,
		Points: make([]redsmath.Vec2, 0, maxTempAreaNodes+1),
	}
	p.previewArea.SetSystemResponse("")
	p.clearHighlightedTarget()
}

func (p *ASDEXPane) updateTempAreaDraftMouse(
	ctx *panes.Context,
	transforms radar.ScopeTransformations,
) {
	if p == nil || p.tempAreaDraft == nil || ctx == nil || ctx.Mouse == nil {
		return
	}

	p.tempAreaDraft.Mouse = transforms.WorldFromWindowP(ctx.Mouse.Pos)
}

func (p *ASDEXPane) consumeTempAreaDraftInput(
	ctx *panes.Context,
	transforms radar.ScopeTransformations,
) bool {
	if p == nil || p.tempAreaDraft == nil || ctx == nil || ctx.Mouse == nil {
		return false
	}

	mouse := ctx.Mouse
	world := transforms.WorldFromWindowP(mouse.Pos)
	p.tempAreaDraft.Mouse = world

	switch {
	case mouse.WasReleased(platform.MouseButtonLeft):
		p.addTempAreaDraftPoint(world)
		return true
	case mouse.WasReleased(platform.MouseButtonMiddle):
		p.finishTempAreaDraft()
		return true
	default:
		return false
	}
}

func (p *ASDEXPane) addTempAreaDraftPoint(point redsmath.Vec2) {
	if p == nil || p.tempAreaDraft == nil {
		return
	}

	draft := p.tempAreaDraft
	if len(draft.Points) > 0 {
		last := draft.Points[len(draft.Points)-1]
		if tempSegmentWouldSelfIntersect(last, point, draft.Points) {
			return
		}
	}

	draft.Points = append(draft.Points, point)
	p.previewArea.SetSystemResponse("")

	if len(draft.Points) >= maxTempAreaNodes {
		p.finishTempAreaDraft()
	}
}

func (p *ASDEXPane) finishTempAreaDraft() {
	if p == nil || p.tempAreaDraft == nil {
		return
	}

	draft := p.tempAreaDraft
	if len(draft.Points) < minTempAreaNodes {
		draft.Points = draft.Points[:0]
		p.previewArea.SetSystemResponse("BAD POLYGON,REDRAW POINT")
		return
	}

	last := draft.Points[len(draft.Points)-1]
	first := draft.Points[0]
	if tempClosingSegmentWouldSelfIntersect(last, first, draft.Points) {
		p.previewArea.SetSystemResponse("BAD POLYGON,REDRAW POINT")
		return
	}

	polygon := append([]redsmath.Vec2(nil), draft.Points...)
	polygon = append(polygon, first)
	p.tempData.AddArea(draft.Type, polygon)

	p.tempAreaDraft = nil
	p.dcb.SetMenu(DcbMenuTempData)
	p.dcbMenuCommand = NewDcbMenuCommand("TEMP DATA")
	p.previewArea.SetSystemResponse("")
	p.clearHighlightedTarget()
}

func (p *ASDEXPane) cancelTempAreaDraft() {
	if p == nil || p.tempAreaDraft == nil {
		return
	}

	p.tempAreaDraft = nil
	p.dcb.SetMenu(DcbMenuTempData)
	p.dcbMenuCommand = NewDcbMenuCommand("TEMP DATA")
	p.previewArea.SetSystemResponse("")
	p.clearHighlightedTarget()
}

func (p *ASDEXPane) DrawTempAreaDraft(cb *renderer.CmdBuffer) {
	if p == nil || p.tempAreaDraft == nil || cb == nil {
		return
	}

	draft := p.tempAreaDraft
	if len(draft.Points) == 0 {
		return
	}

	points := make([]renderer.PointVertex, 0, len(draft.Points)+1)
	for _, pt := range draft.Points {
		points = append(points, renderer.PointVertex{X: pt.X, Y: pt.Y})
	}
	points = append(points, renderer.PointVertex{X: draft.Mouse.X, Y: draft.Mouse.Y})

	builder := renderer.GetLinesBuilder()
	defer renderer.ReturnLinesBuilder(builder)

	builder.AddLineStrip(points)
	cb.SetRGB(applyBrightness(tempAreaDrawRGB, brightnessDefault, brightnessFloorDefault))
	cb.LineWidth(tempAreaDrawLineWidth)
	builder.GenerateCommands(cb)
}

func tempSegmentWouldSelfIntersect(
	a redsmath.Vec2,
	b redsmath.Vec2,
	points []redsmath.Vec2,
) bool {
	if len(points) < 2 {
		return false
	}

	for i := 0; i+1 < len(points); i++ {
		if segmentsIntersectStrict(a, b, points[i], points[i+1]) {
			return true
		}
	}
	return false
}

func tempClosingSegmentWouldSelfIntersect(
	last redsmath.Vec2,
	first redsmath.Vec2,
	points []redsmath.Vec2,
) bool {
	if len(points) < minTempAreaNodes {
		return false
	}

	for i := 0; i+1 < len(points)-1; i++ {
		if segmentsIntersectStrict(last, first, points[i], points[i+1]) {
			return true
		}
	}
	return false
}

func segmentsIntersectStrict(a, b, c, d redsmath.Vec2) bool {
	intersection, ok := segmentIntersectionPoint(a, b, c, d)
	if !ok {
		return false
	}

	const tolerance = 1e-3
	if almostEqualVec2(intersection, a, tolerance) ||
		almostEqualVec2(intersection, b, tolerance) {
		return false
	}
	return true
}

func segmentIntersectionPoint(
	a redsmath.Vec2,
	b redsmath.Vec2,
	c redsmath.Vec2,
	d redsmath.Vec2,
) (redsmath.Vec2, bool) {
	r := b.Sub(a)
	s := d.Sub(c)
	denominator := cross2(r, s)
	if abs32(denominator) < degenerateRunwayAxisLength2 {
		return redsmath.Vec2{}, false
	}

	cma := c.Sub(a)
	t := cross2(cma, s) / denominator
	u := cross2(cma, r) / denominator
	if t < 0 || t > 1 || u < 0 || u > 1 {
		return redsmath.Vec2{}, false
	}

	return a.Add(r.Mul(t)), true
}

func almostEqualVec2(a, b redsmath.Vec2, tolerance float32) bool {
	d := a.Sub(b)
	return d.X*d.X+d.Y*d.Y <= tolerance*tolerance
}

func (p *ASDEXPane) clearTempDataCommandConflicts() {
	if p == nil {
		return
	}

	p.tempAreaDraft = nil
	p.dcbSpinner = nil
	p.commandEntry.Clear()
	p.datablockEdit = nil
	p.editingTargetID = ""
	p.initControlEntry = nil
	p.termControlEntry = nil
	p.multiFunction = nil
	p.previewReposition = nil
	p.coastListReposition = nil
	p.mapReposition = nil
	p.mapRotate = nil
}

func (p *ASDEXPane) openTempDataDcbMenu() {
	if p == nil {
		return
	}

	p.dcb.SetMenu(DcbMenuTempData)
	p.clearTempDataCommandConflicts()
	p.dcbMenuCommand = NewDcbMenuCommand("TEMP DATA")
	p.previewArea.SetSystemResponse("")
	p.clearHighlightedTarget()
}

func (p *ASDEXPane) openTempDataClosedRunwayDcbMenu() {
	if p == nil {
		return
	}

	p.dcb.SetMenu(DcbMenuClosedRunway)
	p.clearTempDataCommandConflicts()
	p.dcbMenuCommand = NewDcbMenuCommand("TEMP DATA", "CLOSED RUNWAY")
	p.previewArea.SetSystemResponse("")
	p.clearHighlightedTarget()
}

func (p *ASDEXPane) closeDcbCurrentSubmenu() {
	if p == nil {
		return
	}

	switch p.dcb.Menu() {
	case DcbMenuClosedRunway:
		p.dcb.SetMenu(DcbMenuTempData)
		p.dcbMenuCommand = NewDcbMenuCommand("TEMP DATA")
	case DcbMenuTempData:
		p.dcb.SetMenu(DcbMenuMain)
		p.dcbMenuCommand = nil
	default:
		p.closeDcbSubmenu()
		return
	}

	p.tempAreaDraft = nil
	p.dcbSpinner = nil
	p.previewArea.SetSystemResponse("")
	p.clearHighlightedTarget()
}

func (p *ASDEXPane) closeDcbSubmenu() {
	if p == nil {
		return
	}

	if p.dcb.Menu() != DcbMenuOff {
		p.dcb.SetMenu(DcbMenuMain)
	}
	p.dcbMenuCommand = nil
	p.dcbSpinner = nil
	p.tempAreaDraft = nil
	p.previewArea.SetSystemResponse("")
	p.clearHighlightedTarget()
}

func (p *ASDEXPane) handleDcbMenuKeyboard(ctx *panes.Context) bool {
	if p == nil || p.dcbMenuCommand == nil || ctx == nil || ctx.Keyboard == nil {
		return false
	}

	keyboard := ctx.Keyboard
	if keyboard.WasPressed(platform.KeyEscape) ||
		keyboard.WasPressed(platform.KeyBackspace) ||
		keyboard.WasPressed(platform.KeyDelete) {
		if p.tempAreaDraft != nil {
			p.cancelTempAreaDraft()
			return true
		}
		p.closeDcbCurrentSubmenu()
		return true
	}

	return false
}
