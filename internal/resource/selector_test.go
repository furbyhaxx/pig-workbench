package resource

import (
	"reflect"
	"testing"
)

func TestSelectors(t *testing.T) {
	tests := []struct {
		input string
		want  []Range
	}{
		{":17", []Range{{16, 30}}},
		{":17:raw", []Range{{17, 17}}},
		{":raw:17", []Range{{17, 17}}},
		{":17-:raw", []Range{{17, 30}}},
		{":19,29", []Range{{19, 19}, {29, 29}}},
		{":2-4:raw", []Range{{2, 4}}},
		{":4+3", []Range{{3, 9}}},
		{":1-12,-15", []Range{{1, 12}, {16, 30}}},
		{":-60", []Range{{1, 30}}},
		{":2-4,4-6", []Range{{2, 6}}},
		{":raw", []Range{{1, 30}}},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			s, err := ParseSelector(tt.input)
			if err != nil {
				t.Fatal(err)
			}
			got, err := s.Resolve(30, true)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
	for _, input := range []string{":", ":0", ":-0", ":4-2", ":1+0", ":1,,2", ":raw:raw", ":9223372036854775808", ":2+9223372036854775807", ":1-2:garbage"} {
		if _, err := ParseSelector(input); err == nil {
			t.Errorf("accepted %q", input)
		}
	}
}

func TestEmptyAndPastEOF(t *testing.T) {
	s, err := ParseSelector(":1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Resolve(0, true); err == nil {
		t.Fatal("selected nonexistent line")
	}
	if _, err := s.Resolve(2, true); err != nil {
		t.Fatal(err)
	}
	s, _ = ParseSelector(":3")
	if _, err := s.Resolve(2, true); err == nil {
		t.Fatal("selected past EOF")
	}
}
