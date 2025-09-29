package visualize

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/curtiscovington/ssa-names/internal/namesdata"
)

// Sparkline renders an ASCII visualization for the provided data.
func Sparkline(years []int, series []namesdata.TrendSeries, totals map[int]int, metric string, width, height int) (string, error) {
	if width <= 0 {
		return "", errors.New("plot width must be positive")
	}
	if height <= 0 {
		return "", errors.New("plot height must be positive")
	}

	columns := width
	if columns > len(years) {
		columns = len(years)
	}
	if columns < 1 {
		columns = 1
	}

	yearIndices := make([]int, columns)
	if len(years) == 1 || columns == 1 {
		for i := range yearIndices {
			yearIndices[i] = 0
		}
	} else {
		for i := range yearIndices {
			ratio := float64(i) / float64(columns-1)
			idx := int(math.Round(ratio * float64(len(years)-1)))
			if idx >= len(years) {
				idx = len(years) - 1
			}
			yearIndices[i] = idx
		}
	}

	values := make([][]float64, len(series))
	minVal := math.Inf(1)
	maxVal := math.Inf(-1)

	for si, s := range series {
		values[si] = make([]float64, columns)
		for ci, yearIdx := range yearIndices {
			point := s.Points[yearIdx]
			if !point.Present {
				values[si][ci] = math.NaN()
				continue
			}

			var v float64
			switch metric {
			case "rank":
				v = -float64(point.Rank)
			case "count":
				v = float64(point.Count)
			case "share":
				total := totals[point.Year]
				if total == 0 {
					values[si][ci] = math.NaN()
					continue
				}
				v = float64(point.Count) / float64(total)
			}

			values[si][ci] = v
			if v < minVal {
				minVal = v
			}
			if v > maxVal {
				maxVal = v
			}
		}
	}

	if minVal == math.Inf(1) || maxVal == math.Inf(-1) {
		return "", errors.New("plot: no data available for the selected metric")
	}

	if math.Abs(maxVal-minVal) < 1e-9 {
		maxVal = minVal + 1
	}

	grid := make([][]rune, height)
	for r := range grid {
		grid[r] = make([]rune, columns)
		for c := range grid[r] {
			grid[r][c] = ' '
		}
	}

	plotChars := []rune{'█', '▓', '▒', '░', '●', '◆', '▲', '■', '✦', '✚', '✖'}

	for si, seriesValues := range values {
		char := plotChars[si%len(plotChars)]
		for ci, v := range seriesValues {
			if math.IsNaN(v) {
				continue
			}
			normalized := (v - minVal) / (maxVal - minVal)
			row := int(math.Round(normalized * float64(height-1)))
			row = (height - 1) - row
			if row < 0 {
				row = 0
			}
			if row >= height {
				row = height - 1
			}
			if grid[row][ci] == ' ' {
				grid[row][ci] = char
			} else if grid[row][ci] != char {
				grid[row][ci] = '●'
			}
		}
	}

	var builder strings.Builder
	builder.Grow(height*(columns+1) + 64)

	builder.WriteString(fmt.Sprintf("Plot (metric=%s)\n", metric))
	for r := 0; r < height; r++ {
		builder.WriteString(string(grid[r]))
		builder.WriteByte('\n')
	}

	startYear := years[yearIndices[0]]
	endYear := years[yearIndices[len(yearIndices)-1]]
	startLabel := fmt.Sprintf("%d", startYear)
	endLabel := fmt.Sprintf("%d", endYear)

	builder.WriteString(startLabel)
	if columns > len(startLabel)+len(endLabel) {
		padding := columns - len(startLabel) - len(endLabel)
		builder.WriteString(strings.Repeat(" ", padding))
	} else {
		builder.WriteString(" ")
	}
	builder.WriteString(endLabel)
	builder.WriteByte('\n')

	legend := make([]string, len(series))
	for i, s := range series {
		char := plotChars[i%len(plotChars)]
		legend[i] = fmt.Sprintf("%c %s", char, s.Name)
	}
	builder.WriteString("Legend: ")
	builder.WriteString(strings.Join(legend, ", "))

	if metric == "rank" {
		builder.WriteString("\n(higher = better rank)")
	}

	return builder.String(), nil
}

// SVG builds an SVG chart for the provided trend data.
func SVG(years []int, series []namesdata.TrendSeries, totals map[int]int, metric string, width, height int, scope []string) (string, error) {
	if len(years) == 0 {
		return "", errors.New("svg: no data available")
	}
	if width <= 0 {
		return "", errors.New("svg: width must be positive")
	}
	if height <= 0 {
		return "", errors.New("svg: height must be positive")
	}

	values := make([][]float64, len(series))
	minVal := math.Inf(1)
	maxVal := math.Inf(-1)

	for si, s := range series {
		values[si] = make([]float64, len(years))
		for idx, point := range s.Points {
			if !point.Present {
				values[si][idx] = math.NaN()
				continue
			}
			switch metric {
			case "rank":
				values[si][idx] = -float64(point.Rank)
			case "count":
				values[si][idx] = float64(point.Count)
			case "share":
				total := totals[point.Year]
				if total == 0 {
					values[si][idx] = math.NaN()
					continue
				}
				values[si][idx] = float64(point.Count) / float64(total)
			}
			v := values[si][idx]
			if !math.IsNaN(v) {
				if v < minVal {
					minVal = v
				}
				if v > maxVal {
					maxVal = v
				}
			}
		}
	}

	if minVal == math.Inf(1) || maxVal == math.Inf(-1) {
		return "", errors.New("svg: no data available for the selected metric")
	}

	if math.Abs(maxVal-minVal) < 1e-9 {
		maxVal = minVal + 1
	}

	paddingTop := 80.0
	paddingLeft := 80.0
	paddingRight := 80.0
	paddingBottom := 120.0

	plotWidth := float64(width) - paddingLeft - paddingRight
	plotHeight := float64(height) - paddingTop - paddingBottom
	if plotWidth <= 0 || plotHeight <= 0 {
		return "", errors.New("svg: insufficient space for plot")
	}

	xCoords := make([]float64, len(years))
	if len(years) == 1 {
		xCoords[0] = paddingLeft + plotWidth/2
	} else {
		step := plotWidth / float64(len(years)-1)
		for i := range years {
			xCoords[i] = paddingLeft + float64(i)*step
		}
	}

	yForValue := func(v float64) float64 {
		normalized := (v - minVal) / (maxVal - minVal)
		return paddingTop + (1-normalized)*plotHeight
	}

	palette := []string{
		"#1f77b4", "#ff7f0e", "#2ca02c", "#d62728", "#9467bd",
		"#8c564b", "#e377c2", "#7f7f7f", "#bcbd22", "#17becf",
	}

	var builder strings.Builder
	builder.Grow(width*height/2 + 1024)

	builder.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	builder.WriteString(fmt.Sprintf("<svg xmlns=\"http://www.w3.org/2000/svg\" width=\"%d\" height=\"%d\" viewBox=\"0 0 %d %d\">\n", width, height, width, height))
	builder.WriteString("  <defs>\n")
	builder.WriteString("    <linearGradient id=\"backgroundGradient\" x1=\"0\" y1=\"0\" x2=\"0\" y2=\"1\">\n")
	builder.WriteString("      <stop offset=\"0%\" stop-color=\"#fafafa\"/>\n")
	builder.WriteString("      <stop offset=\"100%\" stop-color=\"#ffffff\"/>\n")
	builder.WriteString("    </linearGradient>\n")
	builder.WriteString("  </defs>\n")
	builder.WriteString("  <style>\n")
	builder.WriteString("    text { font-family: 'Helvetica Neue', Helvetica, Arial, sans-serif; fill: #1f2933; font-size: 12px; }\n")
	builder.WriteString("    .axis { stroke: #7b8794; stroke-width: 1; }\n")
	builder.WriteString("    .grid { stroke: #e4e7eb; stroke-width: 1; }\n")
	builder.WriteString("  </style>\n")

	builder.WriteString(fmt.Sprintf("  <rect x=\"0\" y=\"0\" width=\"%d\" height=\"%d\" fill=\"url(#backgroundGradient)\"/>\n", width, height))

	title := fmt.Sprintf("Trend (%s)", metric)
	if len(scope) > 0 {
		title = fmt.Sprintf("Trend (%s, %s)", metric, strings.Join(scope, ", "))
	}
	titleY := paddingTop - 36
	subtitleY := titleY + 18
	builder.WriteString(fmt.Sprintf("  <text x=\"%0.1f\" y=\"%0.1f\" font-size=\"20\" font-weight=\"600\">%s</text>\n", paddingLeft, titleY, title))
	builder.WriteString(fmt.Sprintf("  <text x=\"%0.1f\" y=\"%0.1f\" fill=\"#52606d\">%d–%d</text>\n", paddingLeft, subtitleY, years[0], years[len(years)-1]))
	if metric == "rank" {
		builder.WriteString(fmt.Sprintf("  <text x=\"%0.1f\" y=\"%0.1f\" text-anchor=\"end\" fill=\"#52606d\">Lower rank = higher popularity</text>\n", paddingLeft+plotWidth, subtitleY))
	}

	horizontalLines := 5
	for i := 0; i <= horizontalLines; i++ {
		ratio := float64(i) / float64(horizontalLines)
		y := paddingTop + plotHeight*ratio
		builder.WriteString(fmt.Sprintf("  <line class=\"grid\" x1=\"%0.1f\" y1=\"%0.1f\" x2=\"%0.1f\" y2=\"%0.1f\"/>\n", paddingLeft, y, paddingLeft+plotWidth, y))
		if i != 0 && i != horizontalLines {
			value := maxVal - (maxVal-minVal)*ratio
			builder.WriteString(fmt.Sprintf("  <text x=\"%0.1f\" y=\"%0.1f\" text-anchor=\"end\" fill=\"#6b7280\">%s</text>\n", paddingLeft-10, y+4, formatMetricLabel(value, metric)))
		}
	}

	xAxisY := paddingTop + plotHeight
	builder.WriteString(fmt.Sprintf("  <line class=\"axis\" x1=\"%0.1f\" y1=\"%0.1f\" x2=\"%0.1f\" y2=\"%0.1f\"/>\n", paddingLeft, xAxisY, paddingLeft+plotWidth, xAxisY))
	builder.WriteString(fmt.Sprintf("  <line class=\"axis\" x1=\"%0.1f\" y1=\"%0.1f\" x2=\"%0.1f\" y2=\"%0.1f\"/>\n", paddingLeft, paddingTop, paddingLeft, xAxisY))

	topLabel := formatMetricLabel(maxVal, metric)
	bottomLabel := formatMetricLabel(minVal, metric)
	builder.WriteString(fmt.Sprintf("  <text x=\"%0.1f\" y=\"%0.1f\" text-anchor=\"end\">%s</text>\n", paddingLeft-10, paddingTop+4, topLabel))
	builder.WriteString(fmt.Sprintf("  <text x=\"%0.1f\" y=\"%0.1f\" text-anchor=\"end\">%s</text>\n", paddingLeft-10, xAxisY+16, bottomLabel))

	tickCount := 6
	if tickCount > len(years) {
		tickCount = len(years)
	}
	tickStep := int(math.Max(1, math.Round(float64(len(years))/float64(tickCount))))
	labelIndexes := make(map[int]struct{})
	for i := 0; i < len(years); i += tickStep {
		labelIndexes[i] = struct{}{}
	}
	labelIndexes[0] = struct{}{}
	labelIndexes[len(years)-1] = struct{}{}
	midIndex := len(years) / 2
	labelIndexes[midIndex] = struct{}{}

	sortedIndexes := make([]int, 0, len(labelIndexes))
	for idx := range labelIndexes {
		sortedIndexes = append(sortedIndexes, idx)
	}
	sort.Ints(sortedIndexes)

	midLabelEmitted := false

	for _, idx := range sortedIndexes {
		x := xCoords[idx]
		builder.WriteString(fmt.Sprintf("  <line class=\"grid\" x1=\"%0.1f\" y1=\"%0.1f\" x2=\"%0.1f\" y2=\"%0.1f\"/>\n", x, paddingTop, x, xAxisY))
		builder.WriteString(fmt.Sprintf("  <line class=\"axis\" x1=\"%0.1f\" y1=\"%0.1f\" x2=\"%0.1f\" y2=\"%0.1f\"/>\n", x, xAxisY, x, xAxisY+6))
		builder.WriteString(fmt.Sprintf("  <text x=\"%0.1f\" y=\"%0.1f\" text-anchor=\"middle\">%d</text>\n", x, xAxisY+24, years[idx]))
		if idx == midIndex {
			midLabelEmitted = true
		}
	}

	if !midLabelEmitted && len(years) > 0 {
		x := xCoords[midIndex]
		builder.WriteString(fmt.Sprintf("  <line class=\"grid\" x1=\"%0.1f\" y1=\"%0.1f\" x2=\"%0.1f\" y2=\"%0.1f\"/>\n", x, paddingTop, x, xAxisY))
		builder.WriteString(fmt.Sprintf("  <line class=\"axis\" x1=\"%0.1f\" y1=\"%0.1f\" x2=\"%0.1f\" y2=\"%0.1f\"/>\n", x, xAxisY, x, xAxisY+6))
		builder.WriteString(fmt.Sprintf("  <text x=\"%0.1f\" y=\"%0.1f\" text-anchor=\"middle\">%d</text>\n", x, xAxisY+24, years[midIndex]))
	}

	for si, seriesValues := range values {
		color := palette[si%len(palette)]
		var path strings.Builder
		var circles []string
		pathStarted := false
		for idx, v := range seriesValues {
			if math.IsNaN(v) {
				pathStarted = false
				continue
			}
			x := xCoords[idx]
			y := yForValue(v)
			if !pathStarted {
				path.WriteString(fmt.Sprintf("M %0.2f %0.2f ", x, y))
				pathStarted = true
			} else {
				path.WriteString(fmt.Sprintf("L %0.2f %0.2f ", x, y))
			}
			circles = append(circles, fmt.Sprintf("    <circle cx=\"%0.2f\" cy=\"%0.2f\" r=\"2.5\" fill=\"%s\"/>\n", x, y, color))
		}
		builder.WriteString(fmt.Sprintf("  <path d=\"%s\" fill=\"none\" stroke=\"%s\" stroke-width=\"2\" stroke-linejoin=\"round\" stroke-linecap=\"round\"/>\n", strings.TrimSpace(path.String()), color))
		for _, circle := range circles {
			builder.WriteString(circle)
		}
	}

	legendEntryWidth := 150.0
	entriesPerRow := int(math.Max(1, math.Floor(plotWidth/legendEntryWidth)))
	if entriesPerRow < 1 {
		entriesPerRow = 1
	}
	legendRows := int(math.Ceil(float64(len(series)) / float64(entriesPerRow)))
	legendWidth := math.Min(plotWidth, float64(entriesPerRow)*legendEntryWidth)
	legendHeight := float64(legendRows)*22 + 12
	legendX := paddingLeft + (plotWidth-legendWidth)/2
	legendY := paddingTop + plotHeight + 32

	builder.WriteString(fmt.Sprintf("  <rect x=\"%0.1f\" y=\"%0.1f\" width=\"%0.1f\" height=\"%0.1f\" rx=\"10\" fill=\"#f5f7fa\" stroke=\"#d9dde2\"/>\n", legendX, legendY, legendWidth, legendHeight))

	for si, s := range series {
		color := palette[si%len(palette)]
		row := si / entriesPerRow
		col := si % entriesPerRow
		entryX := legendX + float64(col)*legendEntryWidth + 20
		entryY := legendY + float64(row)*24 + 20
		builder.WriteString(fmt.Sprintf("  <rect x=\"%0.1f\" y=\"%0.1f\" width=\"14\" height=\"14\" fill=\"%s\" rx=\"4\"/>\n", entryX-18, entryY-10, color))
		builder.WriteString(fmt.Sprintf("  <text x=\"%0.1f\" y=\"%0.1f\" text-anchor=\"start\">%s</text>\n", entryX, entryY+1, s.Name))
	}

	builder.WriteString("</svg>\n")

	return builder.String(), nil
}

func formatMetricLabel(v float64, metric string) string {
	switch metric {
	case "rank":
		return fmt.Sprintf("#%d", int(math.Round(-v)))
	case "count":
		return fmt.Sprintf("%.0f", v)
	case "share":
		return fmt.Sprintf("%.2f%%", v*100)
	default:
		return fmt.Sprintf("%.2f", v)
	}
}
