package main

import (
	"bytes"
	"fmt"
	"io"
	"sync"

	jsoniter "github.com/json-iterator/go"
)

var bpool = sync.Pool{
	New: func() interface{} {
		return new(bytes.Buffer)
	},
}

var json = jsoniter.ConfigFastest

func writeBuffer(val interface{}) (*bytes.Buffer, error) {
	buf := bpool.Get().(*bytes.Buffer)

	err := json.NewEncoder(buf).Encode(val)
	if err != nil {
		buf.Reset()
		bpool.Put(buf)
		return nil, err
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
