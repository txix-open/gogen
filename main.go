package main

import (
	"bufio"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"time"

	"github.com/brianvoe/gofakeit/v5"
	"github.com/go-playground/validator/v10"
	io2 "github.com/integration-system/isp-io"
	"github.com/pkg/errors"
)

var (
	configPath = "config.json"
	forceWrite = false
	check      = false
)

const (
	bufSize = 10 << 10
)

func main() {
	gofakeit.Seed(time.Now().UnixNano())

	flag.StringVar(&configPath, "config", "config.json", "config path")
	flag.BoolVar(&forceWrite, "force", false, "overwrite previous generated files")
	flag.BoolVar(&check, "check", false, "validate config")

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

	err = validate.Struct(config)
	if err != nil {
		fmt.Println("Config validation errors:")
		const fieldErrMsg string = "Key: '%s' Error:Field validation for '%s' failed on the '%s' tag: %s\n"
		for _, err := range err.(validator.ValidationErrors) {
			fmt.Printf(fieldErrMsg, err.Namespace(), err.Field(), err.Tag(), err.Param())
		}
		return
	}

	if check {
		err := checkCommand(config)
		if err != nil {
			fmt.Printf("check command: %v\n", err)
		}
		return
	}

	err = generateCommand(config)
	if err != nil {
		fmt.Printf("generate command: %v\n", err)
	}
}

func checkCommand(config *Config) error {
	writers := make([]io.Writer, 0, len(config.Entities))
	for range config.Entities {
		writers = append(writers, ioutil.Discard)
	}

	config.TotalCount = 5
	err := config.GenerateEntities(writers)
	if err != nil {
		return errors.WithMessage(err, "generate entities")
	}

	return nil
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
			csvWriter.Comma = defaultCsvSep
			if err := csvWriter.Write(entity.CsvColumns()); err != nil {
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
	err := config.GenerateEntities(writers)
	if err != nil {
		return errors.WithMessage(err, "generate entities")
	}
	fmt.Printf("Elapsed time: %v\n", time.Since(now))

	return nil
}
