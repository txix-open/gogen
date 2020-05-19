package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/brianvoe/gofakeit/v4"
)

const (
	StringType    = "string"
	IntType       = "int"
	IntStringType = "intstring"
	DateType      = "date"
	BoolType      = "bool"
	UuidType      = "uuid"
	ConstType     = "const"
	OneOfType     = "oneof"
)

const (
	chanBuffer = 100
)

var emptySharedFields = make(map[string]interface{})

func (cfg *Config) GenerateSharedFields() map[string]interface{} {
	sharedFields := make(map[string]interface{}, len(cfg.SharedFields))
	for _, field := range cfg.SharedFields {
		if field.Name == "" {
			fmt.Printf("invalid shared field %v: empty name", field)
			continue
		}
		sharedFields[field.Name] = field.Generate(emptySharedFields)
	}

	return sharedFields
}

func (cfg *Config) GenerateEntities(writers []io.Writer) {
	workersCount := runtime.NumCPU() * 2

	sharedFieldsCh := make(chan map[string]interface{}, chanBuffer)
	writersWg := new(sync.WaitGroup)
	readersWg := new(sync.WaitGroup)

	readersChs := make([]chan *bytes.Buffer, len(writers))
	readersWg.Add(len(writers))
	for i := range writers {
		ch := make(chan *bytes.Buffer, chanBuffer)
		readersChs[i] = ch

		go newWriterWorker(ch, readersWg, writers[i])
	}

	writersWg.Add(workersCount)
	for i := 0; i < workersCount; i++ {
		go newWorker(sharedFieldsCh, writersWg, cfg, readersChs)
	}

	for count := 0; count < cfg.TotalCount; count++ {
		sharedFields := cfg.GenerateSharedFields()
		sharedFieldsCh <- sharedFields
	}

	close(sharedFieldsCh)
	writersWg.Wait()
	for i := range readersChs {
		close(readersChs[i])
	}
	readersWg.Wait()
}

func newWorker(fieldsCh <-chan map[string]interface{}, wg *sync.WaitGroup, cfg *Config, writers []chan *bytes.Buffer) {
	defer wg.Done()

	for fields := range fieldsCh {
		for i := range cfg.Entities {
			entity := cfg.Entities[i]
			val, generated := entity.Generate(fields)
			if generated {
				var (
					buf *bytes.Buffer
					err error
				)
				switch entity.Config.OutputFormat {
				case CsvFormat:
					buf, err = writeCsv(val, entity.CsvColumns())
				default:
					buf, err = writeJson(val)
				}
				if err != nil {
					fmt.Println(fmt.Errorf("write error: %v", err))
					continue
				}
				ch := writers[i]
				ch <- buf
			}
		}
	}
}

func (ent *Entity) Generate(sharedFields map[string]interface{}) (interface{}, bool) {
	cfg := &ent.Config
	if cfg.Count == 0 && cfg.Rate == 0 {
		cfg.Rate = 100
	}

	switch {
	case cfg.Count > 0 && cfg.Count > atomic.LoadInt64(&cfg.currentCount):
	case cfg.Rate > 0 && randPercent() <= cfg.Rate:
	default:
		return nil, false
	}

	atomic.AddInt64(&cfg.currentCount, 1)
	return ent.Field.Generate(sharedFields), true
}

func (ent *Entity) CsvColumns() []string {
	if ent.csvColumnsCache == nil {
		cache := make([]string, len(ent.Field.Fields))
		for i, field := range ent.Field.Fields {
			cache[i] = field.Name
		}
		ent.csvColumnsCache = cache
	}
	return ent.csvColumnsCache
}

func (f *Field) Generate(sharedFields map[string]interface{}) interface{} {
	if f.NilChance > 0 && randPercent() <= f.NilChance {
		return nil
	}

	if f.Reference != "" {
		val, exists := sharedFields[f.Reference]
		if !exists {
			fmt.Printf("invalid field %s, reference %s not found\n", f.Name, f.Reference)
			return nil
		}
		return val
	}

	if fields := f.Fields; fields != nil {
		m := make(map[string]interface{}, len(fields))
		for _, f := range fields {
			m[f.Name] = f.Generate(sharedFields)
		}
		return m
	}

	if arr := f.Array; arr != nil {
		if arr.Fixed != nil {
			result := make([]interface{}, 0, len(arr.Fixed))
			for i := range arr.Fixed {
				field := arr.Fixed[i]
				val := field.Generate(sharedFields)
				if val == nil {
					continue
				}
				result = append(result, val)
			}
			return result
		}

		size := randRange(arr.MinLen, arr.MaxLen)
		if size == 0 && arr.MaxLen == 0 {
			fmt.Printf("zero max array length, probably mistake")
		}
		result := make([]interface{}, 0, size)
		for i := 0; i < size; i++ {
			val := arr.Value.Generate(sharedFields)
			if val == nil {
				continue
			}
			result = append(result, val)
		}
		return result
	}

	if f.Type != nil {
		val, err := f.Type.GenerateByType()
		if err != nil {
			fmt.Printf("invalid value: %v\n", err)
		}
		return val
	}

	fmt.Println("invalid field: zero path at generating")
	return nil
}

func (t *Type) GenerateByType() (interface{}, error) {
	switch t.Type {
	case StringType:
		val := randRange(t.Min, t.Max)
		if val != 0 {
			// TODO: adjust the length of the generated string, otherwise big string is generated every time
			str := gofakeit.HipsterSentence(4)
			if len(str) > val {
				return str[:val], nil
			}
			return str, nil
		}
		return gofakeit.Word(), nil
	case IntType:
		return randRange(t.Min, t.Max), nil
	case IntStringType:
		return strconv.Itoa(randRange(t.Min, t.Max)), nil
	case DateType:
		date := randDate()
		if t.DateFormat != "" {
			return date.Format(t.DateFormat), nil
		}
		return date, nil
	case BoolType:
		return gofakeit.Bool(), nil
	case UuidType:
		return gofakeit.UUID(), nil
	case ConstType:
		if t.Const == nil {
			return nil, errors.New("nil const type")
		}
		return t.Const, nil
	case OneOfType:
		if len(t.OneOf) == 0 {
			return nil, errors.New("zero oneOf values")
		}
		i := rand.Intn(len(t.OneOf))
		return t.OneOf[i], nil
	default:
		return nil, fmt.Errorf("unknown type %q", t.Type)
	}
}

func randRange(min, max int) int {
	if max == min {
		return max
	}
	val := rand.Intn(max-min) + min
	return val
}

func randPercent() int {
	return rand.Intn(101)
}

func randDate() time.Time {
	now := time.Now().Unix()
	randOffset := rand.Int31() / 2

	date := time.Unix(now-int64(randOffset), 0)
	return date
}
