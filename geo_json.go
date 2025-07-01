package main

import (
	"github.com/pkg/errors"
	"math/rand"

	"github.com/brianvoe/gofakeit/v7"
)

// nolint:tagliatelle
type GeoJson struct {
	FeatureType      string            `json:",omitempty"` // GeoJSON feature type, usually "Feature" by default
	GeoSrs           string            `json:",omitempty"` // Spatial Reference System identifier, e.g. "EPSG:4326"
	GeoContainerType string            `json:",omitempty"` // Geometry container type, e.g. "GeometryCollection" or empty string
	GeoGeometries    []GeoGeometrySpec `json:",omitempty"` // List of geometries contained in this container
}

// nolint:tagliatelle
type GeoGeometrySpec struct {
	Type        string `json:",omitempty"` // Supported geometry types: "Point", "MultiPoint", "Polygon", "MultiPolygon"
	Coordinates any    `json:",omitempty"` // Coordinates of the geometry (structure depends on geometry type)
	MinPoints   int    `json:",omitempty"` // Minimum number of points expected for this geometry (if specified)
	MaxPoints   int    `json:",omitempty"` // Maximum number of points allowed for this geometry (if specified)
}

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

	switch spec.Type {
	case "Point":
		return []float64{gofakeit.Longitude(), gofakeit.Latitude()}
	case "MultiPoint":
		points := make([][]float64, count)
		for i := range count {
			points[i] = []float64{gofakeit.Longitude(), gofakeit.Latitude()}
		}
		return points
	case "Polygon":
		ring := make([][]float64, count)
		for i := range count {
			ring[i] = []float64{gofakeit.Longitude(), gofakeit.Latitude()}
		}
		// nolint:makezero
		if count > 0 {
			ring = append(ring, ring[0]) // closing the contour
		}
		return [][]([]float64){ring}
	case "MultiPolygon":
		ring := make([][]float64, count)
		for i := range count {
			ring[i] = []float64{gofakeit.Longitude(), gofakeit.Latitude()}
		}
		// nolint:makezero
		if count > 0 {
			ring = append(ring, ring[0]) // closing the contour
		}
		return [][][]([]float64){{ring}}
	default:
		return nil
	}
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
