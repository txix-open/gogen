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
	values        []string
	curIdx        int
	isShuffleMode bool
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
	values, err := loadCsvValuesInMem(reader, cfg.TargetField)
	if err != nil {
		return nil, errors.WithMessage(err, "load csv values in memory")
	}

	return &csvReader{
		curIdx:        -1,
		isShuffleMode: !cfg.DisableRowShuffle,
		values:        values,
	}, nil
}

func (r *csvReader) Read() string {
	if r.isShuffleMode {
		return r.readRandom()
	}
	return r.readCircular()
}

func (r *csvReader) readRandom() string {
	r.curIdx = random.Intn(len(r.values)) // nolint:gosec
	return r.values[r.curIdx]
}

func (r *csvReader) readCircular() string {
	r.curIdx = (r.curIdx + 1) % len(r.values)
	return r.values[r.curIdx]
}

func loadCsvValuesInMem(reader *csv.Reader, targetField string) ([]string, error) {
	header, err := reader.Read()
	if err != nil {
		return nil, errors.WithMessagef(err, "read column header")
	}
	idx := slices.Index(header, targetField)
	if idx == -1 {
		return nil, errors.Errorf("not found target field '%s'", targetField)
	}

	values := make([]string, 0)
	for {
		line, err := reader.Read()
		switch {
		case err == io.EOF:
			return values, nil
		case err != nil:
			return nil, errors.WithMessage(err, "read line")
		default:
			values = append(values, line[idx])
		}
	}
}
