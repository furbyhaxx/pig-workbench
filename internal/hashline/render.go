package hashline

import (
	"strconv"
	"strings"
)

// Row is one numbered line in a rendered hashline view.
type Row struct {
	Line int
	Text string
}

// Render formats a tagged header and its numbered source rows.
func Render(path, tag string, rows []Row) string {
	var output strings.Builder
	output.WriteByte('[')
	output.WriteString(path)
	output.WriteByte('#')
	output.WriteString(tag)
	output.WriteString("]\n")
	for _, row := range rows {
		output.WriteString(strconv.Itoa(row.Line))
		output.WriteByte(':')
		output.WriteString(row.Text)
		output.WriteByte('\n')
	}
	return output.String()
}
