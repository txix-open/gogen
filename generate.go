package main

import (
	"bytes"
	json2 "encoding/json"
	"fmt"
	"io"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"

	"github.com/brianvoe/gofakeit/v5"
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

func (cfg *Config) GenerateSharedFields(extSrcReader *csvReader) map[string]any {
	sharedFields := make(map[string]any, len(cfg.SharedFields))
	for _, field := range cfg.SharedFields {
		if field.Name == "" {
			fmt.Printf("invalid shared field %v: empty name", field)
			continue
		}
		sharedFields[field.Name] = field.Generate(emptySharedFields, extSrcReader)
	}

	return sharedFields
}

func (cfg *Config) GenerateEntities(writers []io.Writer) error {
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

	var (
		extCsvReader *csvReader
		err          error
	)
	if cfg.ExternalCsvSource != nil {
		extCsvReader, err = NewCsvReader(cfg.ExternalCsvSource)
		if err != nil {
			return errors.WithMessage(err, "new external csv source reader")
		}
	}

	writersWg.Add(workersCount)
	for i := 0; i < workersCount; i++ {
		go newWorker(sharedFieldsCh, writersWg, cfg.Entities, readersChs, extCsvReader)
	}

	for count := 0; count < cfg.TotalCount; count++ {
		sharedFields := cfg.GenerateSharedFields(extCsvReader)
		sharedFieldsCh <- sharedFields
	}

	close(sharedFieldsCh)
	writersWg.Wait()
	for i := range readersChs {
		close(readersChs[i])
	}
	readersWg.Wait()

	return nil
}

func newWorker(fieldsCh <-chan map[string]any, wg *sync.WaitGroup, entities []Entity, writers []chan *bytes.Buffer,
	extSrcReader *csvReader) {
	defer wg.Done()

	for fields := range fieldsCh {
		for i := range entities {
			entity := entities[i]
			val, generated := entity.Generate(fields, extSrcReader)
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

func (ent *Entity) Generate(sharedFields map[string]any, extSrcReader *csvReader) (any, bool) {
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
	return ent.Field.Generate(sharedFields, extSrcReader), true
}

func (ent *Entity) CsvColumns() []string {
	switch {
	case ent.csvColumnsCache != nil:
		return ent.csvColumnsCache
	case len(ent.Field.Fields) > 0:
		ent.csvColumnsCache = makeCsvColumnsFromFields(ent.Field.Fields)
	case len(ent.Field.OneOfFields) > 0:
		ent.csvColumnsCache = makeCsvColumnsFromOneOfFields(ent.Field.OneOfFields)
	}
	return ent.csvColumnsCache
}

func (f *Field) Generate(sharedFields map[string]any, extSrcReader *csvReader) any {
	if f.NilChance > 0 && randPercent() <= f.NilChance {
		return nil
	}

	if fields := f.Fields; fields != nil {
		m := make(map[string]any, len(fields))
		for _, f := range fields {
			m[f.Name] = f.Generate(sharedFields, extSrcReader)
		}
		return m
	}

	//nolint:nestif
	if arr := f.Array; arr != nil {
		if arr.Fixed != nil {
			result := make([]any, 0, len(arr.Fixed))
			for i := range arr.Fixed {
				field := arr.Fixed[i]
				val := field.Generate(sharedFields, extSrcReader)
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
			val := arr.Value.Generate(sharedFields, extSrcReader)
			if val == nil {
				continue
			}
			result = append(result, val)
		}
		return result
	}

	if len(f.OneOfFields) > 0 {
		i := rand.Intn(len(f.OneOfFields))
		return f.OneOfFields[i].Generate(sharedFields, extSrcReader)
	}

	if f.Type != nil {
		val, err := f.Type.GenerateByType(sharedFields, extSrcReader)
		if err != nil {
			fmt.Printf("invalid value: %v\n", err)
		}
		return val
	}

	fmt.Println("invalid field: zero path at generating")
	return nil
}

func (t *Type) GenerateByType(sharedFields map[string]any, extSrcReader *csvReader) (val any, err error) {
	switch {
	case t.Reference != "":
		var ok bool
		val, ok = sharedFields[t.Reference]
		if !ok {
			return nil, errors.Errorf("reference %s not found\n", t.Reference)
		}
	case t.FromExternalSource:
		if extSrcReader == nil {
			return nil, errors.Errorf("external data source is not set")
		}
		val = extSrcReader.ReadRandom()
	default:
		val, err = t.generateSelf()
		if err != nil {
			return nil, errors.WithMessage(err, "generate self")
		}
	}

	if t.AsString {
		val = fmt.Sprintf("%v", val)
	}
	if t.Template != "" {
		val = fmt.Sprintf(t.Template, val)
	}
	if t.AsJson {
		val = json2.RawMessage(val.(string))
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

func makeCsvColumnsFromFields(fields []Field) []string {
	columns := make([]string, len(fields))
	for i, field := range fields {
		columns[i] = field.Name
	}
	slices.Sort(columns)
	return columns
}

func makeCsvColumnsFromOneOfFields(oneOfFields []Field) []string {
	dict := make(map[string]bool)
	for _, field := range oneOfFields {
		for _, v := range field.Fields {
			dict[v.Name] = true
		}
	}
	columns := make([]string, 0, len(oneOfFields))
	for field := range dict {
		columns = append(columns, field)
	}
	slices.Sort(columns)
	return columns
}
