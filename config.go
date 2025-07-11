// nolint:tagliatelle
package main

import (
	"sync"
	"sync/atomic"

	"github.com/go-playground/validator/v10"
)

const (
	CsvFormat = "csv"
)

type Config struct {
	TotalCount   int        `validate:"gt=0"`
	Alphabets    []alphabet `validate:"dive"`
	SharedFields []Field    `validate:"dive"`
	Entities     []Entity   `validate:"required,gt=0,dive"`
}

type alphabet struct {
	Name   string `validate:"required"`
	Values string `validate:"required"`
}

type csvDataSource struct {
	Filepath              string `validate:"required"`
	TargetField           string `validate:"required"`
	CsvSeparator          string
	DisableReadRandomMode bool

	lock   sync.Mutex
	reader atomic.Pointer[csvReader]
}

func (s *csvDataSource) ensureReader() (*csvReader, error) {
	reader := s.reader.Load()
	if reader != nil {
		return reader, nil
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	reader = s.reader.Load()
	if reader != nil {
		return reader, nil
	}

	reader, err := NewCsvReader(s)
	if err != nil {
		return nil, err
	}
	s.reader.Store(reader)
	return reader, nil
}

type Entity struct {
	Field           Field
	Config          EntityConfig
	csvColumnsCache []string
	alphabets       map[string]string
}

func (ent *Entity) WithAlphabets(alphabets map[string]string) {
	ent.alphabets = alphabets
}

type EntityConfig struct {
	// Count and Rate are optional
	Count int64 `validate:"gte=0"`
	// 1..100; if == 0, default is 100
	Rate         int    `validate:"gte=0,lte=100"`
	Filepath     string `validate:"required"`
	OutputFormat string
	CsvSeparator string
	currentCount int64
}

type Field struct {
	Name        string  `json:",omitempty"`
	NilChance   int     `json:",omitempty" validate:"gte=0,lte=100"`
	Weight      float64 `json:",omitempty" validate:"gte=0,lte=1"`
	Type        *Type   `json:",omitempty"`
	Fields      []Field `json:",omitempty" validate:"dive"`
	Array       *Array  `json:",omitempty"`
	OneOfFields []Field `json:",omitempty" validate:"dive"`
}

type Type struct {
	Type string `json:",omitempty"`
	// TODO:
	//  - MaskedString by gofakeit.Generate() or gofakeit.Numerify()
	//  - Date interval by DateRange()
	Const             any            `json:",omitempty"`
	OneOf             []any          `json:",omitempty"`
	DateFormat        string         `json:",omitempty"`
	Min               any            `json:",omitempty"`
	Max               any            `json:",omitempty" validate:"omitempty"`
	AsString          bool           `json:",omitempty"`
	AsJson            bool           `json:",omitempty"`
	ExternalCsvSource *csvDataSource `json:",omitempty"`
	Template          string         `json:",omitempty"`
	Reference         string         `json:",omitempty"`
	Alphabet          string         `json:",omitempty"`
	GeoJson           *GeoJson       `json:",omitempty"`

	seq int64
}

type Array struct {
	Value  *Field
	Fixed  []Field `json:",omitempty" validate:"dive"`
	MinLen int
	MaxLen int `validate:"omitempty,gtefield=MinLen"`
}

func FieldStructLevelValidation(sl validator.StructLevel) {
	field, _ := sl.Current().Interface().(Field)

	setCount := 0

	if field.Type != nil {
		setCount++
	}
	if field.Fields != nil {
		setCount++
	}
	if field.Array != nil {
		setCount++
	}
	if field.OneOfFields != nil {
		setCount++
	}

	switch {
	case setCount == 0:
		sl.ReportError(field.Name, "Struct", "", "missing_one_of_optionals", "Zero optional params set")
	case setCount > 1:
		sl.ReportError(field.Name, "Struct", "", "many_optional_params", "More than 1 optional params set")
	}
}

func ArrayStructLevelValidation(sl validator.StructLevel) {
	arr, _ := sl.Current().Interface().(Array)

	if arr.Value == nil && arr.Fixed == nil {
		sl.ReportError(arr.Value, "Value", "", "missing_one_of_optionals", "Value or Fixed must be set")
	}
}

func TypeStructLevelValidation(sl validator.StructLevel) {
	t, _ := sl.Current().Interface().(Type)

	if t.Type == OneOfType && len(t.OneOf) == 0 {
		sl.ReportError(t.OneOf, "OneOf", "", "missing_param", "'OneOf' param not set")
	}
}
