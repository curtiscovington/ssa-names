package namesdata

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Record represents a single row from the names by state dataset.
type Record struct {
	State  string
	Gender string
	Year   int
	Name   string
	Count  int
}

// NameCount represents an aggregated total for a name.
type NameCount struct {
	Name  string
	Count int
}

// TrendPoint captures the rank and count for a name in a specific year.
type TrendPoint struct {
	Year    int
	Rank    int
	Count   int
	Present bool
}

// TrendSeries contains a chronologically ordered slice of TrendPoints for a name.
type TrendSeries struct {
	Name   string
	Points []TrendPoint
}

// LoadStateRecords loads all records for the given state abbreviation (e.g. "CA")
// from the provided filesystem.
func LoadStateRecords(fsys fs.FS, state string) ([]Record, error) {
	if state == "" {
		return nil, errors.New("state is required")
	}

	fileName := strings.ToUpper(state) + ".TXT"
	return loadRecordsFromFile(fsys, fileName)
}

// LoadAllRecords loads every state's records from the filesystem.
func LoadAllRecords(fsys fs.FS) ([]Record, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("read dataset directory: %w", err)
	}

	records := make([]Record, 0, 1024)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(strings.ToUpper(name), ".TXT") {
			continue
		}

		if err := readRecordsFromFile(fsys, name, func(r Record) error {
			records = append(records, r)
			return nil
		}); err != nil {
			return nil, err
		}
	}

	if len(records) == 0 {
		return nil, errors.New("no records found in dataset")
	}

	return records, nil
}

func loadRecordsFromFile(fsys fs.FS, fileName string) ([]Record, error) {
	records := make([]Record, 0, 1024)
	if err := readRecordsFromFile(fsys, fileName, func(r Record) error {
		records = append(records, r)
		return nil
	}); err != nil {
		return nil, err
	}

	return records, nil
}

func readRecordsFromFile(fsys fs.FS, fileName string, fn func(Record) error) error {
	file, err := fsys.Open(fileName)
	if err != nil {
		return fmt.Errorf("open %s: %w", fileName, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	sawRecord := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		parts := strings.Split(line, ",")
		if len(parts) != 5 {
			return fmt.Errorf("malformed line in %s: %q", fileName, line)
		}

		year, err := strconv.Atoi(parts[2])
		if err != nil {
			return fmt.Errorf("parse year %q in %s: %w", parts[2], fileName, err)
		}

		count, err := strconv.Atoi(parts[4])
		if err != nil {
			return fmt.Errorf("parse count %q in %s: %w", parts[4], fileName, err)
		}

		record := Record{
			State:  parts[0],
			Gender: parts[1],
			Year:   year,
			Name:   parts[3],
			Count:  count,
		}

		if err := fn(record); err != nil {
			return err
		}
		sawRecord = true
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan %s: %w", fileName, err)
	}

	if !sawRecord {
		return fmt.Errorf("no records found in %s", fileName)
	}

	return nil
}

// AggregateNames filters the provided records and returns a sorted list of
// totals along with a lookup map for 1-based rank by name (case-insensitive).
// year == 0 means all years. gender can be "M", "F", or empty for all.
func AggregateNames(records []Record, year int, gender string) ([]NameCount, map[string]int) {
	gender = strings.ToUpper(strings.TrimSpace(gender))

	totals := make(map[string]*NameCount)
	for _, r := range records {
		if year != 0 && r.Year != year {
			continue
		}
		if gender != "" && strings.ToUpper(r.Gender) != gender {
			continue
		}

		key := strings.ToUpper(r.Name)
		entry, ok := totals[key]
		if !ok {
			entry = &NameCount{Name: r.Name}
			totals[key] = entry
		}
		entry.Count += r.Count
	}

	aggregated := make([]NameCount, 0, len(totals))
	for _, entry := range totals {
		aggregated = append(aggregated, *entry)
	}

	sort.Slice(aggregated, func(i, j int) bool {
		if aggregated[i].Count == aggregated[j].Count {
			return aggregated[i].Name < aggregated[j].Name
		}
		return aggregated[i].Count > aggregated[j].Count
	})

	ranks := make(map[string]int, len(aggregated))
	for idx, entry := range aggregated {
		ranks[strings.ToUpper(entry.Name)] = idx + 1
	}

	return aggregated, ranks
}

// TopNames filters the provided records and returns the most frequent names.
// year == 0 means all years. gender can be "M", "F", or empty for all.
func TopNames(records []Record, year int, gender string, limit int) []NameCount {
	aggregated, _ := AggregateNames(records, year, gender)

	if limit > 0 && len(aggregated) > limit {
		aggregated = aggregated[:limit]
	}

	return aggregated
}

// Rank computes the 1-based rank of a name within the provided filters.
// When limit is 0, TopNames returns the full list, so we reuse it here.
func Rank(records []Record, year int, gender, name string) (int, NameCount, error) {
	aggregated, ranks := AggregateNames(records, year, gender)
	return RankFromAggregate(aggregated, ranks, name)
}

// RankFromAggregate is a helper that returns rank information from precomputed
// aggregates. aggregated should be in descending order of popularity, and
// ranks must contain 1-based positions keyed by upper-cased name.
func RankFromAggregate(aggregated []NameCount, ranks map[string]int, name string) (int, NameCount, error) {
	if strings.TrimSpace(name) == "" {
		return 0, NameCount{}, errors.New("name is required")
	}

	if len(aggregated) == 0 {
		return 0, NameCount{}, errors.New("no matching records for the provided filters")
	}

	target := strings.ToUpper(name)
	rank, ok := ranks[target]
	if !ok {
		return 0, NameCount{}, fmt.Errorf("name %q not found for the provided filters", name)
	}

	return rank, aggregated[rank-1], nil
}

// RandomName selects a name from the filtered records using the aggregated
// counts as weights for a probability distribution. When r is nil a new
// time-seeded RNG is used.
func RandomName(records []Record, year int, gender string, r *rand.Rand) (NameCount, error) {
	aggregated, _ := AggregateNames(records, year, gender)
	return RandomNameFromAggregate(aggregated, r)
}

// RandomNameFromAggregate returns a weighted random name from the aggregated
// list. The probability of each name is proportional to its Count value.
func RandomNameFromAggregate(aggregated []NameCount, r *rand.Rand) (NameCount, error) {
	if len(aggregated) == 0 {
		return NameCount{}, errors.New("no matching records for the provided filters")
	}

	total := 0
	for _, entry := range aggregated {
		if entry.Count < 0 {
			return NameCount{}, fmt.Errorf("negative count for %q", entry.Name)
		}
		total += entry.Count
	}

	return RandomNameFromAggregateWithTotal(aggregated, total, r)
}

// NameSampler precomputes probability tables for repeated random selections.
type NameSampler struct {
	entries []NameCount
	prob    []float64
	alias   []int
	total   int
}

// NewNameSampler builds a sampler from aggregated name counts.
func NewNameSampler(aggregated []NameCount) (*NameSampler, error) {
	if len(aggregated) == 0 {
		return nil, errors.New("no matching records for the provided filters")
	}

	entries := make([]NameCount, len(aggregated))
	copy(entries, aggregated)

	total := 0
	for _, entry := range aggregated {
		if entry.Count < 0 {
			return nil, fmt.Errorf("negative count for %q", entry.Name)
		}
		total += entry.Count
	}

	if total == 0 {
		return nil, errors.New("no probability mass available")
	}

	n := len(aggregated)
	prob := make([]float64, n)
	alias := make([]int, n)
	scaled := make([]float64, n)

	small := make([]int, 0, n)
	large := make([]int, 0, n)

	for i, entry := range aggregated {
		alias[i] = i
		scaled[i] = float64(entry.Count) * float64(n) / float64(total)
		if scaled[i] < 1.0 {
			small = append(small, i)
		} else {
			large = append(large, i)
		}
	}

	for len(small) > 0 && len(large) > 0 {
		smallIdx := small[len(small)-1]
		small = small[:len(small)-1]
		largeIdx := large[len(large)-1]
		large = large[:len(large)-1]

		prob[smallIdx] = scaled[smallIdx]
		alias[smallIdx] = largeIdx

		scaled[largeIdx] = scaled[largeIdx] + scaled[smallIdx] - 1
		if scaled[largeIdx] < 1.0 {
			small = append(small, largeIdx)
		} else {
			large = append(large, largeIdx)
		}
	}

	for _, idx := range large {
		prob[idx] = 1
	}
	for _, idx := range small {
		prob[idx] = 1
	}

	return &NameSampler{entries: entries, prob: prob, alias: alias, total: total}, nil
}

// Pick returns a random NameCount using the sampler's precomputed weights.
func (s *NameSampler) Pick(r *rand.Rand) (NameCount, error) {
	if s == nil || len(s.entries) == 0 {
		return NameCount{}, errors.New("no matching records for the provided filters")
	}

	rng := r
	if rng == nil {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}

	idx := rng.Intn(len(s.entries))
	if len(s.prob) == 0 || len(s.alias) == 0 {
		return s.entries[idx], nil
	}

	if rng.Float64() < s.prob[idx] {
		return s.entries[idx], nil
	}

	return s.entries[s.alias[idx]], nil
}

// RandomNameFromAggregateWithTotal selects a random name using the provided
// total count, avoiding recomputing the sum when it is already known.
func RandomNameFromAggregateWithTotal(aggregated []NameCount, total int, r *rand.Rand) (NameCount, error) {
	if len(aggregated) == 0 {
		return NameCount{}, errors.New("no matching records for the provided filters")
	}

	if total <= 0 {
		return NameCount{}, errors.New("no probability mass available")
	}

	rng := r
	if rng == nil {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}

	pick := rng.Intn(total)
	running := 0
	for _, entry := range aggregated {
		running += entry.Count
		if pick < running {
			return entry, nil
		}
	}

	return aggregated[len(aggregated)-1], nil
}

func walkRecords(fsys fs.FS, state string, fn func(Record) error) error {
	state = strings.TrimSpace(state)
	if state != "" {
		fileName := strings.ToUpper(state) + ".TXT"
		return readRecordsFromFile(fsys, fileName, fn)
	}

	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return fmt.Errorf("read dataset directory: %w", err)
	}

	processed := false
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(strings.ToUpper(name), ".TXT") {
			continue
		}

		if err := readRecordsFromFile(fsys, name, fn); err != nil {
			return err
		}
		processed = true
	}

	if !processed {
		return errors.New("no records found in dataset")
	}

	return nil
}

// RandomNameFromFS selects a random name directly from the dataset without
// materializing all records. It returns the aggregated count for the selected
// name and the total matches for the provided filters.
func RandomNameFromFS(fsys fs.FS, state string, year int, gender string, r *rand.Rand) (NameCount, int, error) {
	genderFilter := strings.ToUpper(strings.TrimSpace(gender))
	rng := r
	if rng == nil {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}

	total := 0
	var candidate string
	chosen := false

	err := walkRecords(fsys, state, func(rec Record) error {
		if year != 0 && rec.Year != year {
			return nil
		}
		if genderFilter != "" && strings.ToUpper(rec.Gender) != genderFilter {
			return nil
		}
		if rec.Count <= 0 {
			return nil
		}

		total += rec.Count
		if total <= 0 {
			return nil
		}

		if !chosen {
			candidate = rec.Name
			chosen = true
			return nil
		}

		if rng.Float64() < float64(rec.Count)/float64(total) {
			candidate = rec.Name
		}
		return nil
	})
	if err != nil {
		return NameCount{}, 0, err
	}

	if !chosen || total == 0 {
		return NameCount{}, 0, errors.New("no matching records for the provided filters")
	}

	aggCount := 0
	upperCandidate := strings.ToUpper(candidate)
	err = walkRecords(fsys, state, func(rec Record) error {
		if year != 0 && rec.Year != year {
			return nil
		}
		if genderFilter != "" && strings.ToUpper(rec.Gender) != genderFilter {
			return nil
		}
		if strings.ToUpper(rec.Name) != upperCandidate {
			return nil
		}
		aggCount += rec.Count
		return nil
	})
	if err != nil {
		return NameCount{}, 0, err
	}

	return NameCount{Name: candidate, Count: aggCount}, total, nil
}

// AggregateFromFS builds name totals directly from the dataset without
// materializing every record. It returns the aggregated slice sorted by
// descending count along with the total occurrences that matched the filters.
func AggregateFromFS(fsys fs.FS, state string, year int, gender string) ([]NameCount, int, error) {
	genderFilter := strings.ToUpper(strings.TrimSpace(gender))

	counts := make(map[string]int)
	display := make(map[string]string)
	total := 0

	err := walkRecords(fsys, state, func(rec Record) error {
		if year != 0 && rec.Year != year {
			return nil
		}
		if genderFilter != "" && strings.ToUpper(rec.Gender) != genderFilter {
			return nil
		}
		if rec.Count <= 0 {
			return nil
		}

		key := strings.ToUpper(rec.Name)
		counts[key] += rec.Count
		if _, ok := display[key]; !ok {
			display[key] = rec.Name
		}
		total += rec.Count
		return nil
	})
	if err != nil {
		return nil, 0, err
	}

	if total == 0 {
		return nil, 0, errors.New("no matching records for the provided filters")
	}

	aggregated := make([]NameCount, 0, len(counts))
	for key, count := range counts {
		aggregated = append(aggregated, NameCount{Name: display[key], Count: count})
	}

	sort.Slice(aggregated, func(i, j int) bool {
		if aggregated[i].Count == aggregated[j].Count {
			return aggregated[i].Name < aggregated[j].Name
		}
		return aggregated[i].Count > aggregated[j].Count
	})

	return aggregated, total, nil
}

// Trend aggregates yearly rank and count information for the provided names.
// If gender is empty, all genders are included.
func Trend(records []Record, gender string, names []string) ([]int, []TrendSeries, map[int]int, error) {
	gender = strings.ToUpper(strings.TrimSpace(gender))

	requested := make([]struct {
		Key   string
		Input string
	}, 0, len(names))
	seen := make(map[string]bool)
	for _, raw := range names {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		key := strings.ToUpper(trimmed)
		if seen[key] {
			continue
		}
		seen[key] = true
		requested = append(requested, struct {
			Key   string
			Input string
		}{Key: key, Input: trimmed})
	}

	if len(requested) == 0 {
		return nil, nil, nil, errors.New("at least one name is required")
	}

	type aggregate struct {
		Name  string
		Count int
	}

	yearly := make(map[int]map[string]*aggregate)
	totals := make(map[int]int)
	for _, r := range records {
		if gender != "" && strings.ToUpper(r.Gender) != gender {
			continue
		}
		yearMap, ok := yearly[r.Year]
		if !ok {
			yearMap = make(map[string]*aggregate)
			yearly[r.Year] = yearMap
		}
		key := strings.ToUpper(r.Name)
		agg, ok := yearMap[key]
		if !ok {
			agg = &aggregate{Name: r.Name}
			yearMap[key] = agg
		}
		agg.Count += r.Count
		totals[r.Year] += r.Count
	}

	if len(yearly) == 0 {
		return nil, nil, nil, errors.New("no matching records for the provided filters")
	}

	years := make([]int, 0, len(yearly))
	for year := range yearly {
		years = append(years, year)
	}
	sort.Ints(years)

	// Build rank lookups per year.
	yearRanks := make(map[int]map[string]int, len(years))
	for _, year := range years {
		yearMap := yearly[year]
		entries := make([]NameCount, 0, len(yearMap))
		for _, agg := range yearMap {
			entries = append(entries, NameCount{Name: agg.Name, Count: agg.Count})
		}
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].Count == entries[j].Count {
				return entries[i].Name < entries[j].Name
			}
			return entries[i].Count > entries[j].Count
		})
		ranks := make(map[string]int, len(entries))
		for idx, entry := range entries {
			ranks[strings.ToUpper(entry.Name)] = idx + 1
		}
		yearRanks[year] = ranks
	}

	displayNames := make(map[string]string)
	for _, yearMap := range yearly {
		for key, agg := range yearMap {
			if _, ok := displayNames[key]; !ok {
				displayNames[key] = agg.Name
			}
		}
	}

	series := make([]TrendSeries, 0, len(requested))
	for _, req := range requested {
		display := displayNames[req.Key]
		if display == "" {
			display = req.Input
		}
		points := make([]TrendPoint, 0, len(years))
		for _, year := range years {
			stats := yearly[year]
			rankLookup := yearRanks[year]
			point := TrendPoint{Year: year}
			if agg, ok := stats[req.Key]; ok {
				point.Present = true
				point.Count = agg.Count
				point.Rank = rankLookup[req.Key]
			}
			points = append(points, point)
		}
		series = append(series, TrendSeries{Name: display, Points: points})
	}

	return years, series, totals, nil
}
