package main

import (
	"github.com/go-playground/validator/v10"
)

const (
	JsonFormat = "json"
	CsvFormat  = "csv"
)

type Config struct {
	TotalCount   int      `validate:"gt=0"`
	SharedFields []Field  `validate:"dive"`
	Entities     []Entity `validate:"required,gt=0,dive"`
}

type Entity struct {
	Name            string // TODO: remove? not used
	Field           Field
	Config          EntityConfig
	csvColumnsCache []string
}

type EntityConfig struct {
	// Count and Rate are optional
	Count int64 `validate:"gte=0"`
	// 1..100; if == 0, default is 100
	Rate         int    `validate:"gte=0,lte=100"`
	Filepath     string `validate:"required"`
	OutputFormat string
	currentCount int64
}

type Field struct {
	Name      string  `json:",omitempty"`
	Reference string  `json:",omitempty"`
	NilChance int     `json:",omitempty" validate:"gte=0,lte=100"`
	Type      *Type   `json:",omitempty"`
	Fields    []Field `json:",omitempty" validate:"dive"`
	Array     *Array  `json:",omitempty"`
}

type Type struct {
	Type string `validate:"required" json:",omitempty"`
	// TODO:
	//  - MaskedString by gofakeit.Generate() or gofakeit.Numerify()
	//  - Date interval by DateRange()
	Const      interface{}   `json:",omitempty"`
	OneOf      []interface{} `json:",omitempty"`
	DateFormat string        `json:",omitempty"`
	Min        int           `json:",omitempty"`
	Max        int           `json:",omitempty" validate:"omitempty,gtefield=Min"`
}

type Array struct {
	Value  *Field
	Fixed  []Field `json:",omitempty" validate:"dive"`
	MinLen int
	MaxLen int `validate:"omitempty,gtefield=MinLen"`
}

func FieldStructLevelValidation(sl validator.StructLevel) {
	field := sl.Current().Interface().(Field)

	setCount := 0
	if field.Reference != "" {
		setCount++
	}
	if field.Type != nil {
		setCount++
	}
	if field.Fields != nil {
		setCount++
	}
	if field.Array != nil {
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
	arr := sl.Current().Interface().(Array)

	if arr.Value == nil && arr.Fixed == nil {
		sl.ReportError(arr.Value, "Value", "", "missing_one_of_optionals", "Value or Fixed must be set")
	}
}

func TypeStructLevelValidation(sl validator.StructLevel) {
	t := sl.Current().Interface().(Type)

	if t.Type == OneOfType && len(t.OneOf) == 0 {
		sl.ReportError(t.OneOf, "OneOf", "", "missing_param", "'OneOf' param not set")
	}
}
