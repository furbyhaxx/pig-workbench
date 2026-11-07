package resource

import "encoding/json"

// MarshalJSON omits unknown totals while preserving a known empty source's zero.
func (p Page) MarshalJSON() ([]byte, error) {
	type fields Page
	var total *int
	if !p.UnknownTotal {
		total = &p.TotalLines
	}
	return json.Marshal(struct {
		fields
		TotalLines *int `json:"totalLines,omitempty"`
	}{fields: fields(p), TotalLines: total})
}
