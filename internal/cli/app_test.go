package cli_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/curtiscovington/ssa-names/internal/cli"
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

type jsonOutput struct {
	Metadata map[string]string   `json:"metadata"`
	Headers  []string            `json:"headers"`
	Lines    []string            `json:"lines"`
	Rows     []map[string]string `json:"rows"`
	Footer   []string            `json:"footer"`
}

func TestAppTopJSON(t *testing.T) {
	fs := sampleFS()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := cli.NewApp(fs, stdout, stderr)

	err := app.Run([]string{"--state", "CA", "--year", "2019", "--gender", "F", "--format", "json", "--name", "Emma", "--top", "2"})
	if err != nil {
		t.Fatalf("Run top json: %v", err)
	}

	var payload jsonOutput
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal json: %v\n%s", err, stdout.String())
	}

	if got := payload.Metadata["queried_rank"]; got != "2" {
		t.Fatalf("expected queried_rank=2, got %s", got)
	}

	if len(payload.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(payload.Rows))
	}

	if payload.Rows[0]["Name"] != "Olivia" || payload.Rows[0]["Rank"] != "1" {
		t.Fatalf("unexpected first row: %+v", payload.Rows[0])
	}

	if stderr.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", stderr.String())
	}
}

func TestAppTopNationalYearRangeJSON(t *testing.T) {
	fs := sampleFS()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := cli.NewApp(fs, stdout, stderr)

	if err := app.Run([]string{"--year", "2018-2019", "--gender", "F", "--format", "json", "--top", "2"}); err != nil {
		t.Fatalf("Run top national range json: %v", err)
	}

	var payload jsonOutput
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal json: %v\n%s", err, stdout.String())
	}

	if payload.Metadata["state"] != "NATIONAL" {
		t.Fatalf("expected state metadata NATIONAL, got %q", payload.Metadata["state"])
	}

	if payload.Metadata["year"] != "2018-2019" {
		t.Fatalf("expected year metadata 2018-2019, got %q", payload.Metadata["year"])
	}

	if len(payload.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(payload.Rows))
	}

	if payload.Rows[0]["Name"] != "Olivia" || payload.Rows[0]["Count"] != "280" {
		t.Fatalf("unexpected first row: %+v", payload.Rows[0])
	}

	if payload.Rows[1]["Name"] != "Emma" || payload.Rows[1]["Count"] != "185" {
		t.Fatalf("unexpected second row: %+v", payload.Rows[1])
	}

	if stderr.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", stderr.String())
	}
}

func TestAppTrendJSONSharePlot(t *testing.T) {
	fs := sampleFS()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := cli.NewApp(fs, stdout, stderr)

	err := app.Run([]string{"trend", "--name", "Olivia", "--state", "CA", "--format", "json", "--metric", "share", "--plot", "--width", "10"})
	if err != nil {
		t.Fatalf("Run trend json: %v", err)
	}

	var payload jsonOutput
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal json: %v\n%s", err, stdout.String())
	}

	if payload.Metadata["metric"] != "share" {
		t.Fatalf("expected metric=share, got %s", payload.Metadata["metric"])
	}

	if len(payload.Rows) != 2 {
		t.Fatalf("expected 2 yearly rows, got %d", len(payload.Rows))
	}

	if len(payload.Footer) == 0 || !strings.HasPrefix(payload.Footer[0], "Plot (metric=share)") {
		t.Fatalf("expected sparkline footer, got %v", payload.Footer)
	}

	if stderr.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", stderr.String())
	}
}

func TestAppTopNoResultsJSON(t *testing.T) {
	fs := sampleFS()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := cli.NewApp(fs, stdout, stderr)

	if err := app.Run([]string{"--state", "CA", "--year", "2001", "--format", "json"}); err != nil {
		t.Fatalf("Run top no results: %v", err)
	}

	var payload jsonOutput
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal json: %v\n%s", err, stdout.String())
	}

	if len(payload.Rows) != 0 {
		t.Fatalf("expected zero rows, got %d", len(payload.Rows))
	}

	if len(payload.Lines) == 0 || payload.Lines[0] != "No matching names found." {
		t.Fatalf("expected message about no results, got %v", payload.Lines)
	}

	if stderr.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", stderr.String())
	}
}

func TestAppGenerateJSONSeeded(t *testing.T) {
	fs := sampleFS()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := cli.NewApp(fs, stdout, stderr)

	args := []string{"generate", "--state", "CA", "--year", "2019", "--gender", "F", "--format", "json", "--seed", "99"}
	if err := app.Run(args); err != nil {
		t.Fatalf("Run generate json: %v", err)
	}

	var payload jsonOutput
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal json: %v\n%s", err, stdout.String())
	}

	aggregated, _, err := namesdata.AggregateFromFS(fs, "CA", 2019, "F")
	if err != nil {
		t.Fatalf("AggregateFromFS: %v", err)
	}
	samplr, err := namesdata.NewNameSampler(aggregated)
	if err != nil {
		t.Fatalf("NewNameSampler: %v", err)
	}
	expected, err := samplr.Pick(rand.New(rand.NewSource(99)))
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}

	if payload.Metadata["generated_name"] != expected.Name {
		t.Fatalf("expected generated_name=%s, got %s", expected.Name, payload.Metadata["generated_name"])
	}

	if payload.Metadata["sample_count"] != "1" {
		t.Fatalf("expected sample_count=1, got %s", payload.Metadata["sample_count"])
	}

	if len(payload.Rows) != 1 {
		t.Fatalf("expected single row, got %d", len(payload.Rows))
	}

	row := payload.Rows[0]
	if row["Pick"] != "1" {
		t.Fatalf("expected first pick index 1, got %s", row["Pick"])
	}
	if row["Name"] != expected.Name {
		t.Fatalf("row name mismatch: %+v", row)
	}
	if row["DatasetCount"] != fmt.Sprintf("%d", expected.Count) {
		t.Fatalf("expected dataset count %d, got %s", expected.Count, row["DatasetCount"])
	}

	if stderr.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", stderr.String())
	}
}

func TestAppGenerateMultiple(t *testing.T) {
	fs := sampleFS()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := cli.NewApp(fs, stdout, stderr)

	args := []string{"generate", "--state", "CA", "--year", "2019", "--gender", "F", "--format", "json", "--seed", "123", "--count", "5"}
	if err := app.Run(args); err != nil {
		t.Fatalf("Run generate json count: %v", err)
	}

	var payload jsonOutput
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal json: %v\n%s", err, stdout.String())
	}

	if payload.Metadata["sample_count"] != "5" {
		t.Fatalf("expected sample_count=5, got %s", payload.Metadata["sample_count"])
	}

	if len(payload.Rows) != 5 {
		t.Fatalf("expected 5 rows, got %d", len(payload.Rows))
	}

	for i, row := range payload.Rows {
		expectedPick := fmt.Sprintf("%d", i+1)
		if row["Pick"] != expectedPick {
			t.Fatalf("row %d pick mismatch: %+v", i, row)
		}
	}

	if stderr.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", stderr.String())
	}
}

func TestAppVersionCommand(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	oldVersion := cli.Version
	t.Cleanup(func() {
		cli.Version = oldVersion
	})
	cli.Version = "v1.2.3"
	app := cli.NewApp(sampleFS(), stdout, stderr)

	if err := app.Run([]string{"--version"}); err != nil {
		t.Fatalf("Run version: %v", err)
	}

	if got := strings.TrimSpace(stdout.String()); got != "names v1.2.3" {
		t.Fatalf("unexpected version output: %q", got)
	}

	if stderr.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", stderr.String())
	}
}
