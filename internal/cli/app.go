package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/curtiscovington/ssa-names/internal/namesdata"
	"github.com/curtiscovington/ssa-names/internal/visualize"
)

// Version is the semantic version of the CLI binary and is overridden at build time.
var Version = "dev"

// App wraps the command-line interface logic so it can be reused in tests.
type App struct {
	Dataset fs.FS
	Stdout  io.Writer
	Stderr  io.Writer
}

// NewApp constructs an App with the provided dataset and I/O writers.
func NewApp(dataset fs.FS, stdout, stderr io.Writer) *App {
	return &App{Dataset: dataset, Stdout: stdout, Stderr: stderr}
}

// Run dispatches to the appropriate sub-command based on the provided args.
func (a *App) Run(args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "version", "--version", "-v":
			a.printVersion()
			return nil
		}
	}

	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return a.runTop(args)
	}

	switch args[0] {
	case "generate":
		return a.runGenerate(args[1:])
	case "trend":
		return a.runTrend(args[1:])
	case "help", "-h", "--help":
		a.printUsage()
		return nil
	default:
		fmt.Fprintf(a.Stderr, "unknown command: %s\n\n", args[0])
		a.printUsage()
		return fmt.Errorf("unknown command: %s", args[0])
	}
}

func (a *App) printVersion() {
	version := strings.TrimSpace(Version)
	if version == "" {
		version = "dev"
	}
	fmt.Fprintf(a.Stdout, "names %s\n", version)
}

type yearFilter struct {
	all   bool
	years map[int]struct{}
}

func parseYearFilter(raw string) (yearFilter, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "0" {
		return yearFilter{all: true}, nil
	}

	result := yearFilter{years: make(map[int]struct{})}
	parts := strings.Split(trimmed, ",")
	for _, part := range parts {
		segment := strings.TrimSpace(part)
		if segment == "" {
			return yearFilter{}, errors.New("invalid year value: empty segment")
		}
		if segment == "0" {
			return yearFilter{all: true}, nil
		}

		if strings.Contains(segment, "-") {
			rangeParts := strings.Split(segment, "-")
			if len(rangeParts) != 2 {
				return yearFilter{}, fmt.Errorf("invalid year range: %s", segment)
			}

			start, err := strconv.Atoi(strings.TrimSpace(rangeParts[0]))
			if err != nil {
				return yearFilter{}, fmt.Errorf("invalid year in range %q: %w", rangeParts[0], err)
			}
			end, err := strconv.Atoi(strings.TrimSpace(rangeParts[1]))
			if err != nil {
				return yearFilter{}, fmt.Errorf("invalid year in range %q: %w", rangeParts[1], err)
			}
			if start <= 0 || end <= 0 {
				return yearFilter{}, errors.New("year ranges must be positive")
			}
			if end < start {
				return yearFilter{}, fmt.Errorf("invalid year range: %s", segment)
			}

			for year := start; year <= end; year++ {
				result.years[year] = struct{}{}
			}
			continue
		}

		year, err := strconv.Atoi(segment)
		if err != nil {
			return yearFilter{}, fmt.Errorf("invalid year value %q: %w", segment, err)
		}
		if year <= 0 {
			return yearFilter{}, errors.New("year must be positive")
		}
		result.years[year] = struct{}{}
	}

	if len(result.years) == 0 {
		return yearFilter{}, errors.New("no valid years provided")
	}

	return result, nil
}

func (f yearFilter) All() bool {
	return f.all
}

func (f yearFilter) Contains(year int) bool {
	if f.all {
		return true
	}
	_, ok := f.years[year]
	return ok
}

func (f yearFilter) String() string {
	if f.all {
		return ""
	}
	years := make([]int, 0, len(f.years))
	for year := range f.years {
		years = append(years, year)
	}
	sort.Ints(years)

	segments := make([]string, 0)
	for i := 0; i < len(years); {
		start := years[i]
		end := start
		j := i + 1
		for j < len(years) && years[j] == end+1 {
			end = years[j]
			j++
		}
		segments = append(segments, formatYearSegment(start, end))
		i = j
	}

	return strings.Join(segments, ", ")
}

func formatYearSegment(start, end int) string {
	if start == end {
		return fmt.Sprintf("%d", start)
	}
	return fmt.Sprintf("%d-%d", start, end)
}

func filterRecordsByYear(records []namesdata.Record, filter yearFilter) []namesdata.Record {
	if filter.All() {
		return records
	}
	filtered := make([]namesdata.Record, 0, len(records))
	for _, record := range records {
		if filter.Contains(record.Year) {
			filtered = append(filtered, record)
		}
	}
	return filtered
}

func (a *App) runTop(args []string) error {
	fs := flag.NewFlagSet("names", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)

	state := fs.String("state", "", "optional two-letter state abbreviation (e.g. CA)")
	year := fs.String("year", "", "specific year or range to filter on (comma-separated or range, 0 for all years)")
	gender := fs.String("gender", "", "filter by gender (M, F, or leave empty for both)")
	topN := fs.Int("top", 10, "number of names to display")
	name := fs.String("name", "", "specific name to report rank for (requires -year)")
	formatFlag := fs.String("format", "table", "output format: table, json, or csv")

	if err := fs.Parse(args); err != nil {
		return err
	}

	yearFilter, err := parseYearFilter(*year)
	if err != nil {
		return err
	}

	if *topN < 1 {
		return errors.New("-top must be 1 or greater")
	}

	if strings.TrimSpace(*name) != "" && yearFilter.All() {
		return errors.New("-year must be set when using -name")
	}

	trimmedState := strings.TrimSpace(*state)

	var records []namesdata.Record
	if trimmedState == "" {
		records, err = namesdata.LoadAllRecords(a.Dataset)
	} else {
		records, err = namesdata.LoadStateRecords(a.Dataset, trimmedState)
	}
	if err != nil {
		return err
	}

	filteredRecords := filterRecordsByYear(records, yearFilter)

	aggregated, ranks := namesdata.AggregateNames(filteredRecords, 0, *gender)

	format, err := parseOutputFormat(*formatFlag)
	if err != nil {
		return err
	}

	metadata := map[string]string{}

	metadataState := strings.ToUpper(trimmedState)
	displayLocation := metadataState
	if trimmedState == "" {
		metadataState = "NATIONAL"
		displayLocation = "the United States"
	}
	metadata["state"] = metadataState

	if desc := yearFilter.String(); desc != "" {
		metadata["year"] = desc
	}
	if trimmed := strings.TrimSpace(*gender); trimmed != "" {
		metadata["gender"] = strings.ToUpper(trimmed)
	}

	if len(aggregated) == 0 {
		rpt := report{
			Lines:    []string{"No matching names found."},
			Metadata: metadata,
			Headers:  []string{"Rank", "Name", "Count"},
			Rows:     nil,
		}
		return renderReport(a.Stdout, format, rpt)
	}

	lines := make([]string, 0, 3)

	if trimmed := strings.TrimSpace(*name); trimmed != "" {
		rank, entry, err := namesdata.RankFromAggregate(aggregated, ranks, trimmed)
		if err != nil {
			return err
		}
		rankLine := fmt.Sprintf("%s ranks #%d in %s", entry.Name, rank, displayLocation)
		if desc := yearFilter.String(); desc != "" {
			rankLine += fmt.Sprintf(" for %s", desc)
		}
		if strings.TrimSpace(*gender) != "" {
			rankLine += fmt.Sprintf(" (%s)", strings.ToUpper(*gender))
		}
		rankLine += fmt.Sprintf(" with %d occurrences", entry.Count)
		lines = append(lines, rankLine, "")

		metadata["queried_name"] = entry.Name
		metadata["queried_rank"] = fmt.Sprintf("%d", rank)
		metadata["queried_count"] = fmt.Sprintf("%d", entry.Count)
	}

	topNames := aggregated
	if *topN > 0 && len(topNames) > *topN {
		topNames = topNames[:*topN]
	}

	title := fmt.Sprintf("Top %d names in %s", len(topNames), displayLocation)
	if desc := yearFilter.String(); desc != "" {
		title += fmt.Sprintf(" for %s", desc)
	}
	if strings.TrimSpace(*gender) != "" {
		title += fmt.Sprintf(" (%s)", strings.ToUpper(*gender))
	}
	title += ":"
	lines = append(lines, title)

	rows := make([][]string, len(topNames))
	for i, entry := range topNames {
		rows[i] = []string{
			fmt.Sprintf("%d", i+1),
			entry.Name,
			fmt.Sprintf("%d", entry.Count),
		}
	}

	rpt := report{
		Lines:    lines,
		Metadata: metadata,
		Headers:  []string{"Rank", "Name", "Count"},
		Rows:     rows,
	}

	return renderReport(a.Stdout, format, rpt)
}

func (a *App) runGenerate(args []string) error {
	fs := flag.NewFlagSet("generate", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)

	state := fs.String("state", "", "optional two-letter state abbreviation")
	year := fs.Int("year", 0, "specific year to filter on (0 for all years)")
	gender := fs.String("gender", "", "filter by gender (M, F, or leave empty for both)")
	count := fs.Int("count", 1, "number of names to generate")
	formatFlag := fs.String("format", "table", "output format: table, json, or csv")
	seed := fs.Int64("seed", 0, "optional RNG seed for reproducible suggestions")

	if err := fs.Parse(args); err != nil {
		return err
	}

	trimmedState := strings.TrimSpace(*state)

	if *count < 1 {
		return errors.New("--count must be at least 1")
	}

	format, err := parseOutputFormat(*formatFlag)
	if err != nil {
		return err
	}

	metadata := map[string]string{}
	if trimmedState != "" {
		metadata["state"] = strings.ToUpper(trimmedState)
	} else {
		metadata["state"] = "NATIONAL"
	}
	if *year != 0 {
		metadata["year"] = fmt.Sprintf("%d", *year)
	}
	if trimmedGender := strings.TrimSpace(*gender); trimmedGender != "" {
		metadata["gender"] = strings.ToUpper(trimmedGender)
	}
	metadata["sample_count"] = fmt.Sprintf("%d", *count)

	var rng *rand.Rand
	if *seed != 0 {
		rng = rand.New(rand.NewSource(*seed))
		metadata["seed"] = fmt.Sprintf("%d", *seed)
	}

	aggregated, total, err := namesdata.AggregateFromFS(a.Dataset, trimmedState, *year, *gender)
	if err != nil {
		if strings.Contains(err.Error(), "no matching records") {
			metadata["total_occurrences"] = "0"
			lines := []string{"No matching names found."}
			rpt := report{
				Lines:    lines,
				Metadata: metadata,
				Headers:  []string{"Pick", "Name", "DatasetCount", "Chance"},
			}
			return renderReport(a.Stdout, format, rpt)
		}
		return err
	}
	metadata["total_occurrences"] = fmt.Sprintf("%d", total)

	sampler, err := namesdata.NewNameSampler(aggregated)
	if err != nil {
		return err
	}

	if rng == nil {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}

	scope := metadata["state"]
	if strings.EqualFold(scope, "NATIONAL") {
		scope = "National"
	}
	title := fmt.Sprintf("Generated %d name", *count)
	if *count != 1 {
		title += "s"
	}
	title += fmt.Sprintf(" for %s", scope)
	if *year != 0 {
		title += fmt.Sprintf(" in %d", *year)
	}
	if trimmed := strings.TrimSpace(*gender); trimmed != "" {
		title += fmt.Sprintf(" (%s)", strings.ToUpper(trimmed))
	}

	lines := []string{title, ""}
	rows := make([][]string, *count)

	for i := 0; i < *count; i++ {
		entry, err := sampler.Pick(rng)
		if err != nil {
			return err
		}
		probability := float64(entry.Count) / float64(total)
		rows[i] = []string{
			fmt.Sprintf("%d", i+1),
			entry.Name,
			fmt.Sprintf("%d", entry.Count),
			fmt.Sprintf("%.2f%%", probability*100),
		}

		if i == 0 {
			metadata["generated_name"] = entry.Name
			metadata["generated_count"] = fmt.Sprintf("%d", entry.Count)
			metadata["chance"] = fmt.Sprintf("%.6f", probability)
		}
	}

	rpt := report{
		Lines:    lines,
		Metadata: metadata,
		Headers:  []string{"Pick", "Name", "DatasetCount", "Chance"},
		Rows:     rows,
	}

	return renderReport(a.Stdout, format, rpt)
}

func (a *App) runTrend(args []string) error {
	fs := flag.NewFlagSet("trend", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)

	name := fs.String("name", "", "name to track")
	namesCSV := fs.String("names", "", "comma-separated list of names to track")
	state := fs.String("state", "", "optional two-letter state abbreviation")
	gender := fs.String("gender", "", "filter by gender (M, F, or leave empty for both)")
	plot := fs.Bool("plot", false, "render ASCII sparkline for the selected metric")
	metric := fs.String("metric", "rank", "metric for plotting: rank, count, or share")
	width := fs.Int("width", 80, "plot width when --plot is enabled")
	height := fs.Int("height", 10, "plot height when --plot is enabled")
	svgPath := fs.String("svg", "", "optional file path to write an SVG chart")
	svgWidth := fs.Int("svg-width", 800, "SVG width in pixels")
	svgHeight := fs.Int("svg-height", 400, "SVG height in pixels")
	formatFlag := fs.String("format", "table", "output format: table, json, or csv")

	if err := fs.Parse(args); err != nil {
		return err
	}

	namesList := make([]string, 0, 4)
	if trimmed := strings.TrimSpace(*name); trimmed != "" {
		namesList = append(namesList, trimmed)
	}
	if trimmed := strings.TrimSpace(*namesCSV); trimmed != "" {
		parts := strings.Split(trimmed, ",")
		for _, part := range parts {
			if t := strings.TrimSpace(part); t != "" {
				namesList = append(namesList, t)
			}
		}
	}

	if len(namesList) == 0 {
		return errors.New("trend: at least one -name or -names value is required")
	}

	metricValue := strings.ToLower(strings.TrimSpace(*metric))
	switch metricValue {
	case "rank", "count", "share":
	default:
		return fmt.Errorf("trend: unsupported metric %q", metricValue)
	}

	var (
		records []namesdata.Record
		err     error
	)

	if trimmed := strings.TrimSpace(*state); trimmed != "" {
		records, err = namesdata.LoadStateRecords(a.Dataset, trimmed)
	} else {
		records, err = namesdata.LoadAllRecords(a.Dataset)
	}
	if err != nil {
		return err
	}

	years, series, totals, err := namesdata.Trend(records, *gender, namesList)
	if err != nil {
		return err
	}

	nameLabels := make([]string, len(series))
	for i, s := range series {
		nameLabels[i] = s.Name
	}

	scopeParts := make([]string, 0, 2)
	if g := strings.TrimSpace(*gender); g != "" {
		scopeParts = append(scopeParts, strings.ToUpper(g))
	}
	if trimmed := strings.TrimSpace(*state); trimmed != "" {
		scopeParts = append(scopeParts, strings.ToUpper(trimmed))
	} else {
		scopeParts = append(scopeParts, "National")
	}

	format, err := parseOutputFormat(*formatFlag)
	if err != nil {
		return err
	}

	metadata := map[string]string{
		"metric": metricValue,
		"names":  strings.Join(nameLabels, ", "),
	}
	if trimmed := strings.TrimSpace(*state); trimmed != "" {
		metadata["state"] = strings.ToUpper(trimmed)
	} else {
		metadata["state"] = "National"
	}
	if trimmed := strings.TrimSpace(*gender); trimmed != "" {
		metadata["gender"] = strings.ToUpper(trimmed)
	}
	if len(scopeParts) > 0 {
		metadata["scope"] = strings.Join(scopeParts, ", ")
	}

	title := fmt.Sprintf("Trend for %s", strings.Join(nameLabels, ", "))
	if len(scopeParts) > 0 {
		title += fmt.Sprintf(" (%s)", strings.Join(scopeParts, ", "))
	}
	title += ":"

	lines := []string{title, ""}

	headers := []string{"Year"}
	for _, s := range series {
		headers = append(headers, fmt.Sprintf("%s Rank", s.Name))
		headers = append(headers, fmt.Sprintf("%s Count", s.Name))
	}

	rows := make([][]string, len(years))
	for rowIdx, year := range years {
		row := make([]string, len(headers))
		row[0] = fmt.Sprintf("%d", year)

		col := 1
		for _, seriesEntry := range series {
			point := seriesEntry.Points[rowIdx]
			rank := "-"
			count := "-"
			if point.Present {
				rank = fmt.Sprintf("%d", point.Rank)
				count = fmt.Sprintf("%d", point.Count)
			}
			row[col] = rank
			col++
			row[col] = count
			col++
		}
		rows[rowIdx] = row
	}

	footer := make([]string, 0)
	if *plot {
		plotOutput, err := visualize.Sparkline(years, series, totals, metricValue, *width, *height)
		if err != nil {
			return err
		}
		plotLines := strings.Split(strings.TrimRight(plotOutput, "\n"), "\n")
		footer = append(footer, plotLines...)
	}

	if trimmed := strings.TrimSpace(*svgPath); trimmed != "" {
		svgOutput, err := visualize.SVG(years, series, totals, metricValue, *svgWidth, *svgHeight, scopeParts)
		if err != nil {
			return err
		}
		if err := os.WriteFile(trimmed, []byte(svgOutput), 0o644); err != nil {
			return fmt.Errorf("write svg: %w", err)
		}
		if len(footer) > 0 {
			footer = append(footer, "")
		}
		footer = append(footer, fmt.Sprintf("SVG chart written to %s", trimmed))
	}

	rpt := report{
		Lines:    lines,
		Footer:   footer,
		Metadata: metadata,
		Headers:  headers,
		Rows:     rows,
	}

	return renderReport(a.Stdout, format, rpt)
}

func (a *App) printUsage() {
	fmt.Fprintln(a.Stdout, "Usage:")
	fmt.Fprintln(a.Stdout, "  names [flags]           # Show top names for a state (default command)")
	fmt.Fprintln(a.Stdout, "  names generate [flags]  # Generate a random name using popularity weights")
	fmt.Fprintln(a.Stdout, "  names trend [flags]     # Show popularity trend over time")
	fmt.Fprintln(a.Stdout)
	fmt.Fprintln(a.Stdout, "Run 'names -h' or 'names trend -h' for detailed flag information.")
}
