package resource

import (
	"encoding/json"
	"testing"
)

func TestProviderSpansRejectImpossibleTotalsAndContinuationOverflow(t *testing.T) {
	max := int(^uint(0) >> 1)
	for _, sample := range []struct {
		span  Range
		total int
	}{{Range{Start: 1, End: 1}, -1}, {Range{Start: 2, End: 2}, 1}, {Range{Start: max, End: max}, 0}} {
		if _, err := RenderSpans(t.Context(), "test://source", "test://source", []Range{sample.span}, []string{"source"}, sample.total, true, Budget{Bytes: 4096, Lines: 20}); err == nil {
			t.Fatalf("invalid provider spans accepted: %+v", sample)
		}
	}
}

func TestPageJSONDistinguishesUnknownTotalFromKnownEmpty(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		page, err := RenderText(t.Context(), "test://source", "test://source", nil, Selection{}, Budget{Bytes: 4096, Lines: 20})
		if err != nil {
			t.Fatal(err)
		}
		if !page.ResourceComplete {
			t.Fatal("known empty source was not complete")
		}
		if unknown {
			var err error
			page, err = RenderSpans(t.Context(), "test://source", "test://source", []Range{{Start: 1, End: 1}}, []string{"source"}, 0, true, Budget{Bytes: 4096, Lines: 20})
			if err != nil {
				t.Fatal(err)
			}
		}
		data, err := json.Marshal(page)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]any
		if err := json.Unmarshal(data, &fields); err != nil {
			t.Fatal(err)
		}
		total, present := fields["totalLines"]
		if present == unknown || present && total != float64(0) {
			t.Fatalf("unknown=%v serialized total=%v present=%v: %s", unknown, total, present, data)
		}
	}
}
