package asdex

import (
	"encoding/json"
	"fmt"
	stdmath "math"
	"path/filepath"
	"strings"

	redsmath "github.com/juliusplatzer/reds/math"
	"github.com/juliusplatzer/reds/util"
)

type TowerReference struct {
	ID   string
	Feet redsmath.Vec2
}

type surfaceTowerJSON struct {
	ID       string    `json:"id"`
	Position []float64 `json:"position"`
}

type towerSurfaceJSON struct {
	Towers []surfaceTowerJSON `json:"towers"`
}

func LoadTowerReference(airport string, vm *VideoMap) (TowerReference, bool, error) {
	airport = strings.ToUpper(strings.TrimSpace(airport))
	if airport == "" || vm == nil {
		return TowerReference{}, false, nil
	}

	path := filepath.ToSlash(filepath.Join("asdex", "surface", airport+".json"))
	if !util.ResourceExists(path) {
		return TowerReference{}, false, nil
	}

	var surface towerSurfaceJSON
	if err := json.Unmarshal(util.LoadResourceBytes(path), &surface); err != nil {
		return TowerReference{}, false, err
	}

	for _, tower := range surface.Towers {
		if len(tower.Position) < 2 {
			continue
		}

		return TowerReference{
			ID: tower.ID,
			Feet: vm.LonLatToFeet(
				tower.Position[0],
				tower.Position[1],
			),
		}, true, nil
	}

	return TowerReference{}, false, nil
}

type TowerReadoutCommand struct {
	Tower TowerReference
	x     int
	y     int
}

func NewTowerReadoutCommand(tower TowerReference) *TowerReadoutCommand {
	return &TowerReadoutCommand{Tower: tower}
}

func (cmd *TowerReadoutCommand) SetValues(x int, y int) {
	if cmd == nil {
		return
	}
	cmd.x = x
	cmd.y = y
}

func (cmd *TowerReadoutCommand) DisplayLines() []string {
	if cmd == nil {
		return nil
	}

	return []string{
		fmt.Sprintf("X: %d", cmd.x),
		fmt.Sprintf("Y: %d", cmd.y),
	}
}

func towerReadoutValues(
	cursorFeet redsmath.Vec2,
	towerFeet redsmath.Vec2,
	rotationDeg float32,
) (int, int) {
	delta := cursorFeet.Sub(towerFeet)

	rot := float32(stdmath.Pi) * rotationDeg / 180
	c := float32(stdmath.Cos(float64(rot)))
	s := float32(stdmath.Sin(float64(rot)))

	x := delta.X*c - delta.Y*s
	y := delta.Y*c + delta.X*s

	return int(stdmath.Round(float64(x))),
		int(stdmath.Round(float64(y)))
}
