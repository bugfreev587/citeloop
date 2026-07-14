package articleassets

import (
	"errors"
	"fmt"
	"html"
	"strings"
)

func RenderBenchmarkChart(points []BenchmarkPoint) (GenerateResult, error) {
	if len(points) == 0 || len(points) > 12 {
		return GenerateResult{}, errors.New("benchmark chart requires 1 to 12 cited data points")
	}
	maxValue := float64(0)
	for _, point := range points {
		if strings.TrimSpace(point.Label) == "" || strings.TrimSpace(point.SourceID) == "" || point.Value < 0 {
			return GenerateResult{}, errors.New("benchmark chart data requires label, non-negative value, and source id")
		}
		if point.Value > maxValue {
			maxValue = point.Value
		}
	}
	if maxValue == 0 {
		maxValue = 1
	}
	var bars strings.Builder
	for index, point := range points {
		y := 70 + index*52
		width := int(point.Value / maxValue * 760)
		fmt.Fprintf(&bars, `<text x="40" y="%d" font-family="sans-serif" font-size="18" fill="#172033">%s</text><rect x="300" y="%d" width="%d" height="24" rx="4" fill="#d93820"/><text x="%d" y="%d" font-family="sans-serif" font-size="16" fill="#172033">%.2f</text><text x="300" y="%d" font-family="sans-serif" font-size="11" fill="#667085">source: %s</text>`, y, html.EscapeString(point.Label), y-19, width, 310+width, y, point.Value, y+17, html.EscapeString(point.SourceID))
	}
	height := 110 + len(points)*52
	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="%d" viewBox="0 0 1200 %d"><rect width="100%%" height="100%%" fill="#fff"/><text x="40" y="34" font-family="sans-serif" font-size="22" font-weight="700" fill="#172033">Evidence benchmark</text>%s</svg>`, height, height, bars.String())
	return GenerateResult{Bytes: []byte(svg), MimeType: "image/svg+xml", Provider: "deterministic", Model: "benchmark-chart-v1", Width: 1200, Height: int32(height)}, nil
}
