package main

import (
	"bufio"
	"encoding/csv"
	"io"
	random "math/rand"
	"os"
	"slices"

	"github.com/pkg/errors"
)

const (
	buffSize = 64 * 1024 * 1024
)

type csvReader struct {
	values []string
}

func NewCsvReader(cfg *csvDataSource) (*csvReader, error) {
	fd, err := os.Open(cfg.Filepath)
	if err != nil {
		return nil, errors.WithMessagef(err, "open file '%s'", cfg.Filepath)
	}
	defer func() { _ = fd.Close() }()

	reader := csv.NewReader(bufio.NewReaderSize(fd, buffSize))
	if cfg.CsvSeparator != "" {
		reader.Comma = rune(cfg.CsvSeparator[0])
	}
	header, err := reader.Read()
	if err != nil {
		return nil, errors.WithMessagef(err, "read columnInde")
	}
	idx := slices.Index(header, cfg.TargetField)
	if idx == -1 {
		return nil, errors.Errorf("not found target field '%s' in csv file '%s'", cfg.TargetField, cfg.Filepath)
	}

	values := make([]string, 0)
	for {
		line, err := reader.Read()
		switch {
		case err == io.EOF:
			return &csvReader{values: values}, nil
		case err != nil:
			return nil, errors.WithMessage(err, "read line")
		default:
			values = append(values, line[idx])
		}
	}
}

func (r csvReader) ReadRandom() string {
	idx := random.Intn(len(r.values)) // nolint:gosec
	return r.values[idx]
}
