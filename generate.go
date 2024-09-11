package main

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/brianvoe/gofakeit/v5"
	"io"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

const (
	StringType   = "string"
	IntType      = "int"
	DateType     = "date"
	BoolType     = "bool"
	UuidType     = "uuid"
	ConstType    = "const"
	OneOfType    = "oneof"
	SequenceType = "sequence"
	EmailType    = "email"
)

const (
	chanBuffer = 100
)

var emptySharedFields = make(map[string]any)

func (cfg *Config) GenerateSharedFields() map[string]any {
	sharedFields := make(map[string]any, len(cfg.SharedFields))
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

	sharedFieldsCh := make(chan map[string]any, chanBuffer)
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

func newWorker(fieldsCh <-chan map[string]any, wg *sync.WaitGroup, cfg *Config, writers []chan *bytes.Buffer) {
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

func (ent *Entity) Generate(sharedFields map[string]any) (any, bool) {
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

func (f *Field) Generate(sharedFields map[string]any) any {
	if f.NilChance > 0 && randPercent() <= f.NilChance {
		return nil
	}

	if fields := f.Fields; fields != nil {
		m := make(map[string]any, len(fields))
		for _, f := range fields {
			m[f.Name] = f.Generate(sharedFields)
		}
		return m
	}

	//nolint:nestif
	if arr := f.Array; arr != nil {
		if arr.Fixed != nil {
			result := make([]any, 0, len(arr.Fixed))
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
		result := make([]any, 0, size)
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
		val, err := f.Type.GenerateByType(sharedFields)
		if err != nil {
			fmt.Printf("invalid value: %v\n", err)
		}
		return val
	}

	fmt.Println("invalid field: zero path at generating")
	return nil
}

func (t *Type) GenerateByType(sharedFields map[string]any) (val any, err error) {
	if t.Reference != "" {
		v, exists := sharedFields[t.Reference]
		if !exists {
			return nil, fmt.Errorf("reference %s not found\n", t.Reference)
		}
		val = v
	} else {
		val, err = t.generateSelf()
	}
	if err != nil {
		return nil, err
	}

	if t.AsString {
		val = fmt.Sprintf("%v", val)
	}
	if t.Template != "" {
		val = fmt.Sprintf(t.Template, val)
	}

	return val, err
}

func (t *Type) generateSelf() (val any, err error) {
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
		val, err = randRange(t.Min, t.Max), nil
	case DateType:
		date := randDate()
		if t.DateFormat != "" {
			return date.Format(t.DateFormat), nil
		}
		val, err = date, nil
	case BoolType:
		val, err = gofakeit.Bool(), nil
	case EmailType:
		return gofakeit.Email(), nil
	case UuidType:
		return gofakeit.UUID(), nil
	case ConstType:
		if t.Const == nil {
			return nil, errors.New("nil const type")
		}
		val, err = t.Const, nil
	case OneOfType:
		if len(t.OneOf) == 0 {
			return nil, errors.New("zero oneOf values")
		}
		i := rand.Intn(len(t.OneOf))
		val, err = t.OneOf[i], nil
	case SequenceType:
		if atomic.CompareAndSwapInt64(&t.seq, 0, int64(t.Min)) {
			val, err = t.Min, nil
		} else if v := atomic.AddInt64(&t.seq, 1); t.Max > 0 && v > int64(t.Max) {
			val, err = t.Max, nil
		} else {
			val, err = v, nil
		}
	default:
		return nil, fmt.Errorf("unknown type %q", t.Type)
	}
	return val, err
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
