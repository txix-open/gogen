package main

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"sync"

	jsoniter "github.com/json-iterator/go"
	"github.com/pkg/errors"
)

const (
	defaultCsvSep = ';'
)

var (
	bpool = sync.Pool{
		New: func() interface{} {
			return new(bytes.Buffer)
		},
	}
	json                 = jsoniter.ConfigFastest
	ErrIsNotObjectForCsv = errors.New("unexpected type for csv")
)

func writeJson(val interface{}) (*bytes.Buffer, error) {
	buf, ok := bpool.Get().(*bytes.Buffer)
	if !ok {
		return nil, errors.Errorf("failed type assertion to *bytes.Buffer")
	}

	err := json.NewEncoder(buf).Encode(val)
	if err != nil {
		buf.Reset()
		bpool.Put(buf)
		return nil, errors.WithMessage(err, "encode json value")
	}

	return buf, nil
}

func writeCsv(val interface{}, columnsOrder []string) (*bytes.Buffer, error) {
	buf, ok := bpool.Get().(*bytes.Buffer)
	if !ok {
		return nil, errors.Errorf("failed type assertion to *bytes.Buffer")
	}

	switch m := val.(type) {
	case map[string]interface{}:
		csvWriter := csv.NewWriter(buf)
		csvWriter.Comma = defaultCsvSep
		columns := make([]string, 0, len(m))
		for _, column := range columnsOrder {
			if v, ok := m[column]; ok {
				columns = append(columns, fmt.Sprintf("%v", v))
			} else {
				columns = append(columns, "")
			}
		}
		if err := csvWriter.Write(columns); err != nil {
			buf.Reset()
			bpool.Put(buf)
			return nil, errors.WithMessage(err, "write csv columns")
		}
		csvWriter.Flush()
	default:
		return nil, ErrIsNotObjectForCsv
	}

	return buf, nil
}

func newWriterWorker(bytesCh <-chan *bytes.Buffer, wg *sync.WaitGroup, writer io.Writer) {
	defer wg.Done()

	for buf := range bytesCh {
		_, err := buf.WriteTo(writer)
		if err != nil {
			fmt.Println(errors.WithMessage(err, "unexpected write error")) // nolint:forbidigo
		}
		bpool.Put(buf)
	}
}
