package math

import stdmath "math"

const FeetPerNM = 6076.12

// LonLatToFeet returns a local equirectangular projection from lon/lat degrees
// to feet, anchored at the given lon/lat point.
func LonLatToFeet(anchor Vec2) func(Vec2) Vec2 {
	cosLat := float32(stdmath.Cos(float64(anchor.Y) * stdmath.Pi / 180.0))
	return func(p Vec2) Vec2 {
		return Vec2{
			X: (p.X - anchor.X) * 60.0 * cosLat * FeetPerNM,
			Y: (p.Y - anchor.Y) * 60.0 * FeetPerNM,
		}
	}
}
