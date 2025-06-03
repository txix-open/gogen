// nolint:forbidigo
package main

import (
	"bytes"
	json2 "encoding/json"
	"fmt"
	"io"
	"runtime"
	"slices"
	"strings"
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
	ExternalType = "external"
)

const (
	chanBuffer = 100
)

var emptySharedFields = make(map[string]any)

func (cfg *Config) GenerateSharedFields(alphabets map[string][]rune) map[string]any {
	sharedFields := make(map[string]any, len(cfg.SharedFields))
	for _, field := range cfg.SharedFields {
		if field.Name == "" {
			fmt.Printf("invalid shared field %v: empty name", field)
			continue
		}
		sharedFields[field.Name] = field.Generate(emptySharedFields, alphabets)
	}

	return sharedFields
}

func (cfg *Config) generateAlphabets() map[string][]rune {
	alphabets := make(map[string][]rune, len(cfg.Alphabets))
	for _, alphabet := range cfg.Alphabets {
		alphabets[alphabet.Name] = []rune(alphabet.Values)
	}
	return alphabets
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

	alphabets := cfg.generateAlphabets()
	writersWg.Add(workersCount)
	for range workersCount {
		go newWorker(sharedFieldsCh, writersWg, cfg.Entities, alphabets, readersChs)
	}

	for range cfg.TotalCount {
		sharedFields := cfg.GenerateSharedFields(alphabets)
		sharedFieldsCh <- sharedFields
	}

	close(sharedFieldsCh)
	writersWg.Wait()
	for i := range readersChs {
		close(readersChs[i])
	}
	readersWg.Wait()
}

func newWorker(fieldsCh <-chan map[string]any, wg *sync.WaitGroup, entities []Entity, alphabets map[string][]rune,
	writers []chan *bytes.Buffer) {
	defer wg.Done()

	for fields := range fieldsCh {
		for i := range entities {
			entity := entities[i]
			val, generated := entity.Generate(fields, alphabets)
			if generated {
				var (
					buf *bytes.Buffer
					err error
				)
				switch entity.Config.OutputFormat {
				case CsvFormat:
					buf, err = writeCsv(val, entity)
				default:
					buf, err = writeJson(val)
				}
				if err != nil {
					fmt.Println(errors.WithMessage(err, "write error"))
					continue
				}
				ch := writers[i]
				ch <- buf
			}
		}
	}
}

func (ent *Entity) Generate(sharedFields map[string]any, alphabets map[string][]rune) (any, bool) {
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
	return ent.Field.Generate(sharedFields, alphabets), true
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

// nolint:cyclop
func (f *Field) Generate(sharedFields map[string]any, alphabets map[string][]rune) any {
	if f.NilChance > 0 && randPercent() <= f.NilChance {
		return nil
	}

	if fields := f.Fields; fields != nil {
		m := make(map[string]any, len(fields))
		for _, f := range fields {
			m[f.Name] = f.Generate(sharedFields, alphabets)
		}
		return m
	}

	//nolint:nestif
	if arr := f.Array; arr != nil {
		if arr.Fixed != nil {
			result := make([]any, 0, len(arr.Fixed))
			for i := range arr.Fixed {
				field := arr.Fixed[i]
				val := field.Generate(sharedFields, alphabets)
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
		for range size {
			val := arr.Value.Generate(sharedFields, alphabets)
			if val == nil {
				continue
			}
			result = append(result, val)
		}
		return result
	}

	if len(f.OneOfFields) > 0 {
		val, err := generateRandomOneOfField(f.OneOfFields, sharedFields, alphabets)
		if err != nil {
			fmt.Printf("failed to generate random one of field: %v\n", err)
		}
		return val
	}

	if f.Type != nil {
		val, err := f.Type.GenerateByType(sharedFields, alphabets)
		if err != nil {
			fmt.Printf("invalid value: %v\n", err)
		}
		return val
	}

	fmt.Println("invalid field: zero path at generating")
	return nil
}

// nolint:nonamedreturns
func (t *Type) GenerateByType(sharedFields map[string]any, alphabets map[string][]rune) (val any, err error) {
	switch {
	case t.Reference != "":
		var ok bool
		val, ok = sharedFields[t.Reference]
		if !ok {
			return nil, errors.Errorf("reference %s not found\n", t.Reference)
		}
	default:
		val, err = t.generateSelf(alphabets)
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
		v, _ := val.(string)
		val = json2.RawMessage(v)
	}

	return val, err
}

func (t *Type) generateByAlphabet(alphabets map[string][]rune) (any, error) {
	alphabet, ok := alphabets[t.Alphabet]
	if !ok {
		return nil, errors.Errorf("not found alphabet '%s'", t.Alphabet)
	}

	minLength, maxLength, err := t.getMinMaxIntegers()
	if err != nil {
		return nil, errors.WithMessage(err, "get min max integers")
	}
	length := randRange(minLength, maxLength)
	if length == 0 {
		length = int(gofakeit.Uint8()) + 1
	}

	var b strings.Builder
	b.Grow(length)
	for range length {
		i := rand.Intn(len(alphabet))
		b.WriteRune(alphabet[i])
	}

	return b.String(), nil
}

// nolint:cyclop,nonamedreturns,funlen
func (t *Type) generateSelf(alphabets map[string][]rune) (val any, err error) {
	switch t.Type {
	case StringType:
		val, err = t.generateString(alphabets)
	case IntType:
		minValue, maxValue, err := t.getMinMaxIntegers()
		if err != nil {
			return nil, errors.WithMessage(err, "get min max integers")
		}
		return randRange(minValue, maxValue), nil
	case DateType:
		val, err = t.generateDate()
	case BoolType:
		return gofakeit.Bool(), nil
	case EmailType:
		return gofakeit.Email(), nil
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
	case SequenceType:
		val, err = t.generateSequence()
	case ExternalType:
		if t.ExternalCsvSource == nil {
			return nil, errors.New("nil ExternalCsvSource in config for 'external' type")
		}
		reader, err := t.ExternalCsvSource.ensureReader()
		if err != nil {
			return nil, err
		}
		return reader.Read(), nil
	default:
		return nil, errors.Errorf("unknown type %q", t.Type)
	}
	if err != nil {
		return nil, errors.WithMessagef(err, "generate by type: %s", t.Type)
	}
	return val, nil
}

func (t *Type) generateString(alphabets map[string][]rune) (any, error) {
	if t.Alphabet != "" {
		return t.generateByAlphabet(alphabets)
	}

	mn, mx, err := t.getMinMaxIntegers()
	if err != nil {
		return nil, errors.WithMessage(err, "get min max integers")
	}
	length := randRange(mn, mx)
	if length != 0 {
		// TODO: adjust the length of the generated string, otherwise big string is generated every time
		str := gofakeit.HipsterSentence(4)
		if len(str) > length {
			return str[:length], nil
		}
		return str, nil
	}

	return gofakeit.Word(), nil
}

func (t *Type) generateDate() (any, error) {
	var result time.Time
	if t.Min != nil && t.Max != nil {
		minDate, maxDate, err := t.getMinMaxDates()
		if err != nil {
			return nil, errors.WithMessage(err, "get min max dates")
		}
		result = gofakeit.DateRange(minDate, maxDate)
	} else {
		result = randDate()
	}

	if t.DateFormat != "" {
		return result.Format(t.DateFormat), nil
	}

	return result, nil
}

// nolint:predeclared
func (t *Type) generateSequence() (any, error) {
	min, max, err := t.getMinMaxIntegers()
	if err != nil {
		return nil, errors.WithMessage(err, "get min max integers")
	}
	if atomic.CompareAndSwapInt64(&t.seq, 0, int64(min)) {
		return min, nil
	}
	v := atomic.AddInt64(&t.seq, 1)
	if max > 0 && v > int64(max) {
		return max, nil
	}
	return v, nil
}

// nolint:nonamedreturns,predeclared
func (t *Type) getMinMaxIntegers() (min int, max int, err error) {
	if t.Min == nil || t.Max == nil {
		return 0, 0, nil
	}

	mn, ok := t.Min.(float64)
	if !ok {
		return 0, 0, errors.Errorf("expect min as float64; got %T", t.Min)
	}
	mx, ok := t.Max.(float64)
	if !ok {
		return 0, 0, errors.Errorf("expect max as float64; got %T", t.Min)
	}

	return int(mn), int(mx), nil
}

// nolint:nonamedreturns,predeclared
func (t *Type) getMinMaxDates() (min time.Time, max time.Time, err error) {
	mn, ok := t.Min.(string)
	if !ok {
		return time.Time{}, time.Time{}, errors.Errorf("expect min as string; got %T", t.Min)
	}
	min, err = time.Parse(time.DateOnly, mn)
	if err != nil {
		return time.Time{}, time.Time{}, errors.WithMessage(err, "parse min date string")
	}

	mx, ok := t.Max.(string)
	if !ok {
		return time.Time{}, time.Time{}, errors.Errorf("expect max as string; got %T", t.Min)
	}
	max, err = time.Parse(time.DateOnly, mx)
	if err != nil {
		return time.Time{}, time.Time{}, errors.WithMessage(err, "parse max date string")
	}

	return min, max, nil
}

func generateRandomOneOfField(oneOf []Field, sharedFields map[string]any, alphabets map[string][]rune) (any, error) {
	if oneOf[0].Weight > 0 {
		return generateRandomWeightedOneOfField(oneOf, sharedFields, alphabets)
	}
	i := rand.Intn(len(oneOf))
	return oneOf[i].Generate(sharedFields, alphabets), nil
}

func generateRandomWeightedOneOfField(oneOf []Field, sharedFields map[string]any, alphabets map[string][]rune) (any, error) {
	var (
		r   = rand.Float64()
		sum float64
	)
	for _, v := range oneOf {
		if v.Weight == 0 {
			return nil, errors.Errorf("expect non-zero weight")
		}
		sum += v.Weight
		if sum > r {
			return v.Generate(sharedFields, alphabets), nil
		}
	}
	return nil, errors.New("failed to rand & generate weighted oneOf field")
}

// nolint:predeclared
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
