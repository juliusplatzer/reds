package renderer

import (
	redsmath "github.com/juliusplatzer/reds/math"
	earcut "github.com/mmp/earcut-go"
)

// TessellateRings triangulates a polygon with optional holes. Rings may use
// any winding order; the underlying mapbox earcut tessellator normalizes them.
func TessellateRings(rings [][]redsmath.Vec2) ([]PointVertex, []uint32) {
	polygon := earcut.Polygon{Rings: make([][]earcut.Vertex, 0, len(rings))}
	for _, ring := range rings {
		if len(ring) < 3 {
			continue
		}
		vertices := make([]earcut.Vertex, 0, len(ring))
		for _, p := range ring {
			vertices = append(vertices, earcut.Vertex{
				P: [2]float64{float64(p.X), float64(p.Y)},
			})
		}
		polygon.Rings = append(polygon.Rings, vertices)
	}
	if len(polygon.Rings) == 0 {
		return nil, nil
	}

	triangles := earcut.Triangulate(polygon)
	if len(triangles) == 0 {
		return nil, nil
	}

	points := make([]PointVertex, 0, len(triangles)*3)
	indices := make([]uint32, 0, len(triangles)*3)
	for _, triangle := range triangles {
		for _, vertex := range triangle.Vertices {
			indices = append(indices, uint32(len(points)))
			points = append(points, PointVertex{
				X: float32(vertex.P[0]),
				Y: float32(vertex.P[1]),
			})
		}
	}
	return points, indices
}
