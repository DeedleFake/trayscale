package ui

import "testing"

func TestUSStateName(t *testing.T) {
	tests := []struct {
		city string
		want string
	}{
		{"Ashburn, VA", "Virginia"},
		{"New York, NY", "New York"},
		{"Washington, DC", "District of Columbia"},
		{"Stockholm", ""},
		{"Toronto", ""},
		{"Adelaide", ""},
		{"Somewhere, XX", ""},
	}
	for _, tt := range tests {
		if got := usStateName(tt.city); got != tt.want {
			t.Errorf("usStateName(%q) = %q, want %q", tt.city, got, tt.want)
		}
	}
}
