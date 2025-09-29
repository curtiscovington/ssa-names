package namesdata_test

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"testing"
	"testing/fstest"

	"gonum.org/v1/gonum/stat/distuv"

	"github.com/curtiscovington/ssa-names/data/namesbystate"
	"github.com/curtiscovington/ssa-names/internal/namesdata"
)

func sampleFS() fstest.MapFS {
	return fstest.MapFS{
		"CA.TXT": {Data: []byte(
			"CA,F,2019,Olivia,100\n" +
				"CA,F,2019,Olivia,40\n" +
				"CA,F,2019,Emma,90\n" +
				"CA,M,2019,Liam,95\n" +
				"CA,M,2019,Noah,70\n" +
				"CA,F,2018,Olivia,80\n" +
				"CA,M,2018,Liam,85\n" +
				"CA,F,2018,Emma,50\n"),
		},
		"NY.TXT": {Data: []byte(
			"NY,F,2019,Olivia,60\n" +
				"NY,M,2019,Liam,65\n" +
				"NY,F,2018,Emma,45\n"),
		},
	}
}

func TestLoadStateRecords(t *testing.T) {
	fs := sampleFS()

	records, err := namesdata.LoadStateRecords(fs, "CA")
	if err != nil {
		t.Fatalf("LoadStateRecords: %v", err)
	}

	if len(records) != 8 {
		t.Fatalf("expected 8 records, got %d", len(records))
	}

	if records[0].Name != "Olivia" || records[0].Count != 100 {
		t.Fatalf("unexpected first record: %+v", records[0])
	}
}

func TestLoadAllRecords(t *testing.T) {
	fs := sampleFS()

	records, err := namesdata.LoadAllRecords(fs)
	if err != nil {
		t.Fatalf("LoadAllRecords: %v", err)
	}

	if len(records) != 11 {
		t.Fatalf("expected 11 records, got %d", len(records))
	}
}

func TestAggregateNamesAndRank(t *testing.T) {
	fs := sampleFS()
	records, err := namesdata.LoadStateRecords(fs, "CA")
	if err != nil {
		t.Fatalf("LoadStateRecords: %v", err)
	}

	aggregated, ranks := namesdata.AggregateNames(records, 2019, "F")
	if len(aggregated) != 2 {
		t.Fatalf("expected 2 aggregated names, got %d", len(aggregated))
	}

	first := aggregated[0]
	if first.Name != "Olivia" || first.Count != 140 {
		t.Fatalf("unexpected first aggregate: %+v", first)
	}

	top := namesdata.TopNames(records, 2019, "F", 1)
	if len(top) != 1 || top[0].Name != "Olivia" {
		t.Fatalf("TopNames mismatch: %+v", top)
	}

	rank, entry, err := namesdata.RankFromAggregate(aggregated, ranks, "emma")
	if err != nil {
		t.Fatalf("RankFromAggregate: %v", err)
	}
	if rank != 2 || entry.Name != "Emma" {
		t.Fatalf("unexpected rank result: rank=%d entry=%+v", rank, entry)
	}

	overallRank, overallEntry, err := namesdata.Rank(records, 0, "", "Liam")
	if err != nil {
		t.Fatalf("Rank: %v", err)
	}
	if overallEntry.Count != 180 {
		t.Fatalf("expected Liam total 180, got %d", overallEntry.Count)
	}
	if overallRank != 2 {
		t.Fatalf("expected Liam rank 2 overall, got %d", overallRank)
	}
}

func TestTrendAggregates(t *testing.T) {
	fs := sampleFS()
	records, err := namesdata.LoadStateRecords(fs, "CA")
	if err != nil {
		t.Fatalf("LoadStateRecords: %v", err)
	}

	years, series, totals, err := namesdata.Trend(records, "", []string{"Olivia", "Liam"})
	if err != nil {
		t.Fatalf("Trend: %v", err)
	}

	if len(years) != 2 || years[0] != 2018 || years[1] != 2019 {
		t.Fatalf("unexpected years: %v", years)
	}

	if len(series) != 2 {
		t.Fatalf("expected 2 series, got %d", len(series))
	}

	// Check duplicate aggregation for Olivia in 2019.
	olivia := series[0]
	point2019 := olivia.Points[1]
	if !point2019.Present || point2019.Count != 140 {
		t.Fatalf("expected Olivia count 140 in 2019, got %+v", point2019)
	}
	if totals[2019] != 395 {
		t.Fatalf("unexpected 2019 total: %d", totals[2019])
	}

	// Share metric validation: compare computed share with known ratio from fixture.
	expectedShare := float64(point2019.Count) / float64(totals[2019])
	knownShare := 140.0 / 395.0
	if math.Abs(expectedShare-knownShare) > 1e-9 {
		t.Fatalf("unexpected share value: got %0.6f want %0.6f", expectedShare, knownShare)
	}
}

func TestTrendGenderFilter(t *testing.T) {
	fs := sampleFS()
	records, err := namesdata.LoadStateRecords(fs, "CA")
	if err != nil {
		t.Fatalf("LoadStateRecords: %v", err)
	}

	_, series, totals, err := namesdata.Trend(records, "F", []string{"Olivia", "Emma"})
	if err != nil {
		t.Fatalf("Trend: %v", err)
	}

	if len(series) != 2 {
		t.Fatalf("expected 2 female series, got %d", len(series))
	}

	if totals[2019] != 230 {
		// Female totals for 2019: Olivia 140 + Emma 90 = 230.
		t.Fatalf("unexpected female total for 2019: %d", totals[2019])
	}

	// Ensure ranks reflect female-only ordering.
	olivia2019 := series[0].Points[1]
	if olivia2019.Rank != 1 {
		t.Fatalf("expected Olivia rank 1 among females, got %d", olivia2019.Rank)
	}
}

func TestRandomNameFromAggregateDeterministic(t *testing.T) {
	aggregated := []namesdata.NameCount{{Name: "Olivia", Count: 140}, {Name: "Emma", Count: 90}}
	trials := 5000
	share := float64(140) / float64(230)
	lower, upper := binomialConfidenceBounds(trials, share, 0.99)
	for _, seed := range []int64{123, 4567, 98765} {
		rng := rand.New(rand.NewSource(seed))
		counts := map[string]int{}
		for i := 0; i < trials; i++ {
			entry, err := namesdata.RandomNameFromAggregate(aggregated, rng)
			if err != nil {
				t.Fatalf("RandomNameFromAggregate: %v", err)
			}
			counts[entry.Name]++
		}

		olivia := counts["Olivia"]
		if olivia < lower || olivia > upper {
			t.Fatalf("seed %d: Olivia draws %d out of %d outside 99%% interval [%d, %d]", seed, olivia, trials, lower, upper)
		}
		if counts["Emma"] == 0 {
			t.Fatalf("seed %d: expected to sample Emma at least once", seed)
		}
	}
}

func TestRandomNameFromAggregateErrors(t *testing.T) {
	if _, err := namesdata.RandomNameFromAggregate(nil, rand.New(rand.NewSource(1))); err == nil {
		t.Fatalf("expected error for empty aggregate")
	}

	aggregated := []namesdata.NameCount{{Name: "Test", Count: 0}}
	if _, err := namesdata.RandomNameFromAggregate(aggregated, rand.New(rand.NewSource(1))); err == nil {
		t.Fatalf("expected error for zero probability mass")
	}
}

func TestAggregateFromFSMatchesRecords(t *testing.T) {
	fs := sampleFS()
	records, err := namesdata.LoadStateRecords(fs, "CA")
	if err != nil {
		t.Fatalf("LoadStateRecords: %v", err)
	}

	aggregatedRecords, _ := namesdata.AggregateNames(records, 2019, "F")
	totalRecords := 0
	for _, entry := range aggregatedRecords {
		totalRecords += entry.Count
	}

	aggregatedFS, totalFS, err := namesdata.AggregateFromFS(fs, "CA", 2019, "F")
	if err != nil {
		t.Fatalf("AggregateFromFS: %v", err)
	}

	if totalFS != totalRecords {
		t.Fatalf("total mismatch: fs=%d records=%d", totalFS, totalRecords)
	}

	if len(aggregatedFS) != len(aggregatedRecords) {
		t.Fatalf("length mismatch: fs=%d records=%d", len(aggregatedFS), len(aggregatedRecords))
	}

	for i := range aggregatedFS {
		if aggregatedFS[i] != aggregatedRecords[i] {
			t.Fatalf("entry %d mismatch: %+v vs %+v", i, aggregatedFS[i], aggregatedRecords[i])
		}
	}
}

func TestRandomNameMatchesAggregate(t *testing.T) {
	fs := sampleFS()
	records, err := namesdata.LoadStateRecords(fs, "CA")
	if err != nil {
		t.Fatalf("LoadStateRecords: %v", err)
	}

	aggregated, _ := namesdata.AggregateNames(records, 2019, "F")
	rng1 := rand.New(rand.NewSource(77))
	rng2 := rand.New(rand.NewSource(77))

	expected, err := namesdata.RandomNameFromAggregate(aggregated, rng1)
	if err != nil {
		t.Fatalf("RandomNameFromAggregate: %v", err)
	}

	got, err := namesdata.RandomName(records, 2019, "F", rng2)
	if err != nil {
		t.Fatalf("RandomName: %v", err)
	}

	if got != expected {
		t.Fatalf("random selection mismatch: expected %+v got %+v", expected, got)
	}
}

func TestRandomNameFromAggregateWithTotal(t *testing.T) {
	aggregated := []namesdata.NameCount{{Name: "A", Count: 2}, {Name: "B", Count: 3}}
	rng := rand.New(rand.NewSource(1))

	entry, err := namesdata.RandomNameFromAggregateWithTotal(aggregated, 5, rng)
	if err != nil {
		t.Fatalf("RandomNameFromAggregateWithTotal: %v", err)
	}

	if entry.Name == "" {
		t.Fatalf("expected a name, got empty entry")
	}

	if _, err := namesdata.RandomNameFromAggregateWithTotal(nil, 5, rng); err == nil {
		t.Fatalf("expected error for empty aggregated input")
	}

	if _, err := namesdata.RandomNameFromAggregateWithTotal(aggregated, 0, rng); err == nil {
		t.Fatalf("expected error for zero total")
	}
}

func TestNameSamplerPick(t *testing.T) {
	fs := sampleFS()
	records, err := namesdata.LoadStateRecords(fs, "CA")
	if err != nil {
		t.Fatalf("LoadStateRecords: %v", err)
	}

	aggregated, _ := namesdata.AggregateNames(records, 2019, "F")
	sampler, err := namesdata.NewNameSampler(aggregated)
	if err != nil {
		t.Fatalf("NewNameSampler: %v", err)
	}

	trials := 5000
	share := float64(140) / float64(230)
	lower, upper := binomialConfidenceBounds(trials, share, 0.99)
	for _, seed := range []int64{555, 2024, 8080} {
		rng := rand.New(rand.NewSource(seed))
		counts := map[string]int{}
		for i := 0; i < trials; i++ {
			entry, err := sampler.Pick(rng)
			if err != nil {
				t.Fatalf("Pick: %v", err)
			}
			counts[entry.Name]++
		}

		if len(counts) != len(aggregated) {
			t.Fatalf("seed %d: expected %d names, got %v", seed, len(aggregated), counts)
		}
		olivia := counts["Olivia"]
		if olivia < lower || olivia > upper {
			t.Fatalf("seed %d: Olivia draws %d out of %d outside 99%% interval [%d, %d]", seed, olivia, trials, lower, upper)
		}
	}
}

func TestNameSamplerErrors(t *testing.T) {
	if _, err := namesdata.NewNameSampler(nil); err == nil {
		t.Fatalf("expected error for nil aggregate")
	}

	sampler := &namesdata.NameSampler{}
	if _, err := sampler.Pick(rand.New(rand.NewSource(1))); err == nil {
		t.Fatalf("expected error for empty sampler")
	}
}

func TestNameSamplerRealDataset(t *testing.T) {
	aggregated, total, err := namesdata.AggregateFromFS(namesbystate.Files, "CA", 2019, "F")
	if err != nil {
		t.Fatalf("AggregateFromFS real data: %v", err)
	}

	if total != 184494 {
		t.Fatalf("unexpected total females in CA 2019: got %d", total)
	}

	oliviaCount := 0
	for _, entry := range aggregated {
		if strings.EqualFold(entry.Name, "Olivia") {
			oliviaCount = entry.Count
			break
		}
	}
	if oliviaCount == 0 {
		t.Fatalf("Olivia missing from aggregated real dataset")
	}
	if oliviaCount != 2610 {
		t.Fatalf("unexpected Olivia count in CA 2019 F: got %d", oliviaCount)
	}

	sampler, err := namesdata.NewNameSampler(aggregated)
	if err != nil {
		t.Fatalf("NewNameSampler real data: %v", err)
	}

	trials := 6000
	share := float64(oliviaCount) / float64(total)
	lower, upper := binomialConfidenceBounds(trials, share, 0.99)
	for _, seed := range []int64{2025, 9090} {
		rng := rand.New(rand.NewSource(seed))
		oliviaDraws := 0
		for i := 0; i < trials; i++ {
			entry, err := sampler.Pick(rng)
			if err != nil {
				t.Fatalf("Pick real data: %v", err)
			}
			if strings.EqualFold(entry.Name, "Olivia") {
				oliviaDraws++
			}
		}

		if oliviaDraws < lower || oliviaDraws > upper {
			t.Fatalf("seed %d: Olivia draws %d out of %d outside 99%% interval [%d, %d]", seed, oliviaDraws, trials, lower, upper)
		}
	}
}

func TestRandomNameFromFS(t *testing.T) {
	fs := sampleFS()
	rng := rand.New(rand.NewSource(99))

	entry, total, err := namesdata.RandomNameFromFS(fs, "CA", 2019, "F", rng)
	if err != nil {
		t.Fatalf("RandomNameFromFS: %v", err)
	}

	if total != 230 {
		t.Fatalf("expected total 230 for CA 2019 F, got %d", total)
	}

	if entry.Name == "" || entry.Count <= 0 {
		t.Fatalf("unexpected entry: %+v", entry)
	}

	trials := 4000
	share := float64(140) / float64(230)
	lower, upper := binomialConfidenceBounds(trials, share, 0.99)
	for _, seed := range []int64{1234, 7070, 424242} {
		rng = rand.New(rand.NewSource(seed))
		counts := map[string]int{}
		for i := 0; i < trials; i++ {
			pick, _, err := namesdata.RandomNameFromFS(fs, "CA", 2019, "F", rng)
			if err != nil {
				t.Fatalf("RandomNameFromFS: %v", err)
			}
			counts[pick.Name]++
		}

		olivia := counts["Olivia"]
		if olivia < lower || olivia > upper {
			t.Fatalf("seed %d: Olivia draws %d out of %d outside 99%% interval [%d, %d]", seed, olivia, trials, lower, upper)
		}
		if counts["Emma"] == 0 {
			t.Fatalf("seed %d: expected to sample Emma at least once", seed)
		}
	}
}

func binomialConfidenceBounds(trials int, successProb, confidence float64) (int, int) {
	if trials <= 0 {
		return 0, 0
	}
	if confidence <= 0 || confidence >= 1 {
		return 0, trials
	}
	dist := distuv.Binomial{N: float64(trials), P: successProb}
	lowerQ := (1 - confidence) / 2
	upperQ := 1 - lowerQ
	lower := binomialQuantile(dist, lowerQ)
	upper := binomialQuantile(dist, upperQ)
	if lower < 0 {
		lower = 0
	}
	if upper > trials {
		upper = trials
	}
	return lower, upper
}

func binomialQuantile(dist distuv.Binomial, q float64) int {
	if q <= 0 {
		return 0
	}
	target := math.Min(q, 1)
	cumulative := 0.0
	max := int(dist.N)
	for k := 0; k <= max; k++ {
		cumulative += dist.Prob(float64(k))
		if cumulative >= target {
			return k
		}
	}
	return max
}

func TestSamplerDistributionChiSquare(t *testing.T) {
	fs := sampleFS()
	aggregated, total, err := namesdata.AggregateFromFS(fs, "CA", 2019, "F")
	if err != nil {
		t.Fatalf("AggregateFromFS: %v", err)
	}
	sampler, err := namesdata.NewNameSampler(aggregated)
	if err != nil {
		t.Fatalf("NewNameSampler: %v", err)
	}

	trials := 20000
	rng := rand.New(rand.NewSource(4242))
	observed := make(map[string]int, len(aggregated))
	for i := 0; i < trials; i++ {
		entry, err := sampler.Pick(rng)
		if err != nil {
			t.Fatalf("Pick: %v", err)
		}
		observed[entry.Name]++
	}

	type bucket struct {
		expected float64
		observed float64
	}
	const minExpected = 5.0
	buckets := make([]bucket, 0, len(aggregated))
	smallExpected := 0.0
	smallObserved := 0.0

	for _, entry := range aggregated {
		expectedProb := float64(entry.Count) / float64(total)
		expected := expectedProb * float64(trials)
		obs := float64(observed[entry.Name])
		if expected < minExpected {
			smallExpected += expected
			smallObserved += obs
			if smallExpected >= minExpected {
				buckets = append(buckets, bucket{expected: smallExpected, observed: smallObserved})
				smallExpected = 0
				smallObserved = 0
			}
			continue
		}
		buckets = append(buckets, bucket{expected: expected, observed: obs})
	}

	if smallExpected > 0 {
		if len(buckets) == 0 {
			buckets = append(buckets, bucket{expected: smallExpected, observed: smallObserved})
		} else {
			last := &buckets[len(buckets)-1]
			last.expected += smallExpected
			last.observed += smallObserved
		}
	}

	if len(buckets) < 2 {
		t.Fatalf("not enough categories after bucketing: %d", len(buckets))
	}

	chiSquare := 0.0
	for _, b := range buckets {
		diff := b.observed - b.expected
		chiSquare += (diff * diff) / b.expected
	}

	dof := len(buckets) - 1
	if dof <= 0 {
		t.Fatalf("not enough categories for chi-square test")
	}

	const confidence = 0.99
	threshold := distuv.ChiSquared{K: float64(dof)}.Quantile(confidence)
	if chiSquare > threshold {
		t.Fatalf("chi-square too large: %.2f > %.2f (dof=%d)", chiSquare, threshold, dof)
	}
}

func BenchmarkAggregateNames(b *testing.B) {
	fs := sampleFS()
	records, err := namesdata.LoadStateRecords(fs, "CA")
	if err != nil {
		b.Fatalf("LoadStateRecords: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _ = namesdata.AggregateNames(records, 0, ""); false {
			b.Fatal()
		}
	}
}

func BenchmarkAggregateFromFS(b *testing.B) {
	fs := sampleFS()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := namesdata.AggregateFromFS(fs, "", 0, ""); err != nil {
			b.Fatalf("AggregateFromFS: %v", err)
		}
	}
}

func BenchmarkRandomNameFromFS(b *testing.B) {
	fs := sampleFS()
	rng := rand.New(rand.NewSource(55))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := namesdata.RandomNameFromFS(fs, "", 0, "", rng); err != nil {
			b.Fatalf("RandomNameFromFS: %v", err)
		}
	}
}

func BenchmarkRandomNameFromAggregate(b *testing.B) {
	fs := sampleFS()
	records, err := namesdata.LoadStateRecords(fs, "CA")
	if err != nil {
		b.Fatalf("LoadStateRecords: %v", err)
	}

	aggregated, _ := namesdata.AggregateNames(records, 0, "")
	rng := rand.New(rand.NewSource(123))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := namesdata.RandomNameFromAggregate(aggregated, rng); err != nil {
			b.Fatalf("RandomNameFromAggregate: %v", err)
		}
	}
}

func BenchmarkRandomName(b *testing.B) {
	fs := sampleFS()
	records, err := namesdata.LoadStateRecords(fs, "CA")
	if err != nil {
		b.Fatalf("LoadStateRecords: %v", err)
	}

	rng := rand.New(rand.NewSource(456))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := namesdata.RandomName(records, 0, "", rng); err != nil {
			b.Fatalf("RandomName: %v", err)
		}
	}
}

func BenchmarkRandomNameFromAggregateWithTotal(b *testing.B) {
	fs := sampleFS()
	records, err := namesdata.LoadStateRecords(fs, "CA")
	if err != nil {
		b.Fatalf("LoadStateRecords: %v", err)
	}

	aggregated, _ := namesdata.AggregateNames(records, 0, "")
	total := 0
	for _, entry := range aggregated {
		total += entry.Count
	}

	rng := rand.New(rand.NewSource(222))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := namesdata.RandomNameFromAggregateWithTotal(aggregated, total, rng); err != nil {
			b.Fatalf("RandomNameFromAggregateWithTotal: %v", err)
		}
	}
}

func BenchmarkNameSamplerPick(b *testing.B) {
	fs := sampleFS()
	records, err := namesdata.LoadStateRecords(fs, "CA")
	if err != nil {
		b.Fatalf("LoadStateRecords: %v", err)
	}

	aggregated, _ := namesdata.AggregateNames(records, 0, "")
	sampler, err := namesdata.NewNameSampler(aggregated)
	if err != nil {
		b.Fatalf("NewNameSampler: %v", err)
	}

	rng := rand.New(rand.NewSource(789))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := sampler.Pick(rng); err != nil {
			b.Fatalf("Pick: %v", err)
		}
	}
}

func ExampleAggregateNames() {
	fs := sampleFS()
	records, _ := namesdata.LoadStateRecords(fs, "CA")
	aggregated, _ := namesdata.AggregateNames(records, 2019, "F")
	data, _ := json.Marshal(aggregated)
	fmt.Println(string(data))
	// Output: [{"Name":"Olivia","Count":140},{"Name":"Emma","Count":90}]
}
