package cli

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
)

type outputFormat string

const (
	formatTable outputFormat = "table"
	formatJSON  outputFormat = "json"
	formatCSV   outputFormat = "csv"
)

func parseOutputFormat(raw string) (outputFormat, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch outputFormat(value) {
	case formatTable, formatJSON, formatCSV:
		return outputFormat(value), nil
	default:
		return "", fmt.Errorf("unsupported format %q (expected table, json, or csv)", raw)
	}
}

// report holds reusable rendering data for all output formats.
type report struct {
	Lines    []string
	Footer   []string
	Metadata map[string]string
	Headers  []string
	Rows     [][]string
}

func renderReport(w io.Writer, format outputFormat, rpt report) error {
	switch format {
	case formatTable:
		for _, line := range rpt.Lines {
			fmt.Fprintln(w, line)
		}
		if len(rpt.Lines) > 0 && len(rpt.Headers) > 0 {
			fmt.Fprintln(w)
		}

		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		if len(rpt.Headers) > 0 {
			fmt.Fprintln(tw, strings.Join(rpt.Headers, "\t"))
		}
		for _, row := range rpt.Rows {
			fmt.Fprintln(tw, strings.Join(row, "\t"))
		}
		if err := tw.Flush(); err != nil {
			return err
		}

		if len(rpt.Footer) > 0 {
			if len(rpt.Headers) > 0 {
				fmt.Fprintln(w)
			}
			for _, line := range rpt.Footer {
				fmt.Fprintln(w, line)
			}
		}
		return nil

	case formatJSON:
		rows := make([]map[string]string, len(rpt.Rows))
		for i, row := range rpt.Rows {
			entry := make(map[string]string, len(rpt.Headers))
			for j, header := range rpt.Headers {
				if j < len(row) {
					entry[header] = row[j]
				} else {
					entry[header] = ""
				}
			}
			rows[i] = entry
		}

		payload := map[string]any{
			"metadata": rpt.Metadata,
			"headers":  rpt.Headers,
			"lines":    rpt.Lines,
			"rows":     rows,
			"footer":   rpt.Footer,
		}

		data, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w, string(data))
		return err

	case formatCSV:
		for _, line := range rpt.Lines {
			if strings.TrimSpace(line) == "" {
				continue
			}
			if _, err := fmt.Fprintf(w, "# %s\n", line); err != nil {
				return err
			}
		}

		for _, line := range rpt.Footer {
			if strings.TrimSpace(line) == "" {
				continue
			}
			if _, err := fmt.Fprintf(w, "# %s\n", line); err != nil {
				return err
			}
		}

		if len(rpt.Metadata) > 0 {
			keys := make([]string, 0, len(rpt.Metadata))
			for k := range rpt.Metadata {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, key := range keys {
				if _, err := fmt.Fprintf(w, "# %s: %s\n", key, rpt.Metadata[key]); err != nil {
					return err
				}
			}
		}

		writer := csv.NewWriter(w)
		if len(rpt.Headers) > 0 {
			if err := writer.Write(rpt.Headers); err != nil {
				return err
			}
		}
		for _, row := range rpt.Rows {
			if err := writer.Write(row); err != nil {
				return err
			}
		}
		writer.Flush()
		return writer.Error()
	}

	return fmt.Errorf("unknown format %q", format)
}
