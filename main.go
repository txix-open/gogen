// nolint:forbidigo
package main

import (
	"bufio"
	"encoding/csv"
	"flag"
	"fmt"
	"github.com/txix-open/isp-kit/infra"
	"github.com/txix-open/isp-kit/infra/pprof"
	"io"
	"os"
	"time"

	"github.com/go-playground/validator/v10"
	io2 "github.com/integration-system/isp-io"
	"github.com/pkg/errors"
)

var (
	configPath = "config.json"
	forceWrite = false
	check      = false
	pprofPort  = 0
)

const (
	bufSize = 32 * 1024
)

func main() {
	flag.StringVar(&configPath, "config", "config.json", "config path")
	flag.BoolVar(&forceWrite, "force", false, "overwrite previous generated files")
	flag.BoolVar(&check, "check", false, "validate config")
	flag.IntVar(&pprofPort, "pprofPort", 0, "pprof port, default = 0 - disabled")

	flag.CommandLine.SetOutput(os.Stdout)
	flag.Parse()

	validate := validator.New()
	validate.RegisterStructValidation(FieldStructLevelValidation, Field{})
	validate.RegisterStructValidation(TypeStructLevelValidation, Type{})
	validate.RegisterStructValidation(ArrayStructLevelValidation, Array{})

	configBytes, err := os.ReadFile(configPath)
	if err != nil {
		fmt.Printf("error opening config file: %v\n", err)
		return
	}
	config := new(Config)
	err = json.Unmarshal(configBytes, config)
	if err != nil {
		fmt.Printf("error unmarshaling config: %v\n", err)
		return
	}

	var errList validator.ValidationErrors
	err = validate.Struct(config)
	switch {
	case errors.As(err, &errList):
		fmt.Println("Config validation errors:")
		const fieldErrMsg string = "Key: '%s' Error:Field validation for '%s' failed on the '%s' tag: %s\n"
		for _, err := range errList {
			fmt.Printf(fieldErrMsg, err.Namespace(), err.Field(), err.Tag(), err.Param())
		}
		return
	case err != nil:
		fmt.Println(errors.WithMessage(err, "unexpected error"))
		return
	}

	if pprofPort != 0 {
		infraServer := infra.NewServer()
		pprof.RegisterHandlers("/internal", infraServer)
		go infraServer.ListenAndServe(fmt.Sprintf(":%d", pprofPort))
		fmt.Printf("pprof server listening on http://127.0.0.1:%d/internal/debug/pprof\n", pprofPort)
	}

	if check {
		checkCommand(config)
		return
	}

	err = generateCommand(config)
	if err != nil {
		fmt.Printf("generate command: %v\n", err)
	}
}

func checkCommand(config *Config) {
	writers := make([]io.Writer, 0, len(config.Entities))
	for range config.Entities {
		writers = append(writers, io.Discard)
	}

	config.TotalCount = 5
	config.GenerateEntities(writers)
}

func generateCommand(config *Config) error {
	pipes := make([]io2.WritePipe, len(config.Entities))
	defer func() {
		for _, pipe := range pipes {
			if pipe != nil {
				_ = pipe.Close()
			}
		}
	}()
	var filePerm int
	if forceWrite {
		filePerm = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	} else {
		filePerm = os.O_WRONLY | os.O_CREATE | os.O_EXCL
	}

	writers := make([]io.Writer, len(config.Entities))
	for i := range config.Entities {
		entity := config.Entities[i]
		conf := entity.Config
		f, err := os.OpenFile(conf.Filepath, filePerm, 0755)
		if err != nil {
			fmt.Printf("opening %s file: %v\n", conf.Filepath, err)
			return nil
		}

		if conf.OutputFormat == CsvFormat {
			csvWriter := csv.NewWriter(f)
			if conf.CsvSeparator != "" {
				csvWriter.Comma = []rune(conf.CsvSeparator)[0]
			}

			err := csvWriter.Write(entity.CsvColumns())
			if err != nil {
				fmt.Printf("csv write: %v\n", err)
				return nil
			}
			csvWriter.Flush()
		}

		bufWriter := bufio.NewWriterSize(f, bufSize)
		pipes[i] = io2.NewWritePipe(bufWriter, f)
		writers[i] = bufWriter
	}

	now := time.Now()
	config.GenerateEntities(writers)
	fmt.Printf("Elapsed time: %v\n", time.Since(now))

	return nil
}
