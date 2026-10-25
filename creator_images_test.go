package main

import "testing"

func TestTargetWidth(t *testing.T) {
	for _, tc := range []struct {
		name        string
		input, want int
		invalid     bool
	}{
		{"thumbnail", 640, 640, false}, {"large creator upload", 3200, 1600, false}, {"empty", 0, 0, true}, {"oversize", 5000, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := targetWidth(tc.input)
			if (err != nil) != tc.invalid || got != tc.want {
				t.Fatalf("targetWidth(%d) = %d, %v; want %d, invalid=%v", tc.input, got, err, tc.want, tc.invalid)
			}
		})
	}
}
