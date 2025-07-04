// nolint:tagliatelle
package main

import (
	"math/rand"
	"sort"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/pkg/errors"
)

type GeoJson struct {
	FeatureType      string            `json:",omitempty"` // GeoJSON feature type, usually "Feature" by default
	GeoSrs           string            `json:",omitempty"` // Spatial Reference System identifier, e.g. "EPSG:4326"
	GeoContainerType string            `json:",omitempty"` // Geometry container type, e.g. "GeometryCollection" or empty string
	GeoGeometries    []GeoGeometrySpec `json:",omitempty"` // List of geometries contained in this container
}

type GeoGeometrySpec struct {
	// Supported geometry types: "Point", "MultiPoint", "LineString", "MultiLineString", "Polygon", "MultiPolygon"
	Type          string `json:",omitempty"`
	Coordinates   any    `json:",omitempty"` // Coordinates of the geometry (structure depends on geometry type)
	MinPoints     int    `json:",omitempty"` // Minimum number of points expected for this geometry (if specified)
	MaxPoints     int    `json:",omitempty"` // Maximum number of points allowed for this geometry (if specified)
	PolygonsCount int    `json:",omitempty"` // For MultiPolygon

	MinLon float64 `json:",omitempty"`
	MaxLon float64 `json:",omitempty"`
	MinLat float64 `json:",omitempty"`
	MaxLat float64 `json:",omitempty"`
}

// nolint:cyclop,funlen
func (spec *GeoGeometrySpec) GenerateCoordinates() any {
	if spec.Coordinates != nil {
		return spec.Coordinates
	}

	count := spec.MinPoints
	if spec.MaxPoints > spec.MinPoints {
		count = spec.MinPoints + rand.Intn(spec.MaxPoints-spec.MinPoints+1)
	}
	if count == 0 {
		count = 1
	}

	minLon, maxLon := spec.MinLon, spec.MaxLon
	if minLon == 0 && maxLon == 0 {
		minLon, maxLon = -180, 180
	}
	minLat, maxLat := spec.MinLat, spec.MaxLat
	if minLat == 0 && maxLat == 0 {
		minLat, maxLat = -90, 90
	}

	generatePoint := func() []float64 {
		lon := longitudeInRange(minLon, maxLon)
		lat := latitudeInRange(minLat, maxLat)
		return []float64{lon, lat}
	}

	switch spec.Type {
	case "Point":
		return generatePoint()

	case "MultiPoint":
		points := make([][]float64, count)
		for i := range count {
			points[i] = generatePoint()
		}
		return points

	case "LineString":
		line := make([][]float64, count)
		for i := range count {
			line[i] = generatePoint()
		}
		return line

	case "MultiLineString":
		multiLine := make([][][]float64, count)
		for i := range count {
			nPoints := spec.MinPoints
			if spec.MaxPoints > spec.MinPoints {
				nPoints = spec.MinPoints + rand.Intn(spec.MaxPoints-spec.MinPoints+1)
			}
			line := make([][]float64, nPoints)
			for j := range nPoints {
				line[j] = generatePoint()
			}
			multiLine[i] = line
		}
		return multiLine

	case "Polygon":
		points := randomConvexPolygon(count, minLon, maxLon, minLat, maxLat)
		points = append(points, points[0])
		return [][][]float64{points}

	case "MultiPolygon":
		nPolygons := spec.PolygonsCount
		if nPolygons == 0 {
			nPolygons = 1
		}

		multiPolygons := make([][][][]float64, nPolygons)
		for i := range nPolygons {
			polygon := randomConvexPolygon(count, minLon, maxLon, minLat, maxLat)
			polygon = append(polygon, polygon[0])
			multiPolygons[i] = [][][]float64{polygon}
		}
		return multiPolygons
	default:
		return nil
	}
}

func randomConvexPolygon(n int, minLon, maxLon, minLat, maxLat float64) [][]float64 {
	points := make([][]float64, n)
	for i := range n {
		points[i] = []float64{
			longitudeInRange(minLon, maxLon),
			latitudeInRange(minLat, maxLat),
		}
	}

	sort.Slice(points, func(i, j int) bool {
		if points[i][0] == points[j][0] {
			return points[i][1] < points[j][1]
		}
		return points[i][0] < points[j][0]
	})

	return convexHull(points)
}

func cross(o, a, b []float64) float64 {
	return (a[0]-o[0])*(b[1]-o[1]) - (a[1]-o[1])*(b[0]-o[0])
}

func convexHull(points [][]float64) [][]float64 {
	n := len(points)
	if n <= 3 {
		return points
	}

	lower := [][]float64{}
	for _, p := range points {
		for len(lower) >= 2 && cross(lower[len(lower)-2], lower[len(lower)-1], p) <= 0 {
			lower = lower[:len(lower)-1]
		}
		lower = append(lower, p)
	}

	upper := [][]float64{}
	for i := n - 1; i >= 0; i-- {
		p := points[i]
		for len(upper) >= 2 && cross(upper[len(upper)-2], upper[len(upper)-1], p) <= 0 {
			upper = upper[:len(upper)-1]
		}
		upper = append(upper, p)
	}

	// remove duplicated point
	return append(lower[:len(lower)-1], upper[:len(upper)-1]...)
}

func (t *Type) generateGeoJSON() (any, error) {
	if t.GeoJson == nil {
		return nil, errors.New("Geo spec is nil")
	}

	geometries := make([]map[string]any, 0, len(t.GeoJson.GeoGeometries))
	for _, geomSpec := range t.GeoJson.GeoGeometries {
		coords := geomSpec.GenerateCoordinates()
		if coords == nil {
			continue
		}

		geometries = append(geometries, map[string]any{
			"type":        geomSpec.Type,
			"coordinates": coords,
		})
	}

	featureType := t.GeoJson.FeatureType
	if featureType == "" {
		featureType = "Feature"
	}

	var geometry any
	switch {
	case t.GeoJson.GeoContainerType == "GeometryCollection" || len(geometries) > 1:
		geometry = map[string]any{
			"type":       "GeometryCollection",
			"geometries": geometries,
		}
	case len(geometries) == 1:
		geometry = geometries[0]
	default:
		return nil, errors.New("no geometries generated")
	}

	geoJSON := map[string]any{
		"type":       featureType,
		"properties": map[string]any{"srs": t.GeoJson.GeoSrs},
		"geometry":   geometry,
	}

	b, err := json.Marshal(geoJSON)
	if err != nil {
		return nil, errors.WithMessage(err, "marshal geo json")
	}
	return string(b), nil
}

func longitudeInRange(minLon float64, maxLon float64) float64 {
	lon, _ := gofakeit.LongitudeInRange(minLon, maxLon)
	return lon
}

func latitudeInRange(minLat float64, maxLat float64) float64 {
	lat, _ := gofakeit.LatitudeInRange(minLat, maxLat)
	return lat
}
