package main

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"sync"

	jsoniter "github.com/json-iterator/go"
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
	buf := bpool.Get().(*bytes.Buffer)

	err := json.NewEncoder(buf).Encode(val)
	if err != nil {
		buf.Reset()
		bpool.Put(buf)
		return nil, err
	}

	return buf, nil
}

func writeCsv(val interface{}, columnsOrder []string) (*bytes.Buffer, error) {
	buf := bpool.Get().(*bytes.Buffer)

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
			return nil, err
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
			fmt.Println(fmt.Errorf("unexpected write error: %v\n", err))
		}
		bpool.Put(buf)
	}
}
