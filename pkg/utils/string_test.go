// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package utils

import "testing"

func TestMatchString(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		value   string
		want    bool
	}{
		{name: "exact match", pattern: "429", value: "429", want: true},
		{name: "uppercase X wildcard matches one character", pattern: "50X", value: "500", want: true},
		{name: "multiple uppercase X wildcards match", pattern: "5XX", value: "502", want: true},
		{name: "wildcard does not change prefix", pattern: "40X", value: "500", want: false},
		{name: "lowercase x is literal", pattern: "50x", value: "500", want: false},
		{name: "spaces are literal", pattern: " 50X ", value: "500", want: false},
		{name: "length must match", pattern: "50X", value: "5000", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MatchString(tt.pattern, tt.value); got != tt.want {
				t.Fatalf("MatchString(%q, %q) = %v, want %v", tt.pattern, tt.value, got, tt.want)
			}
		})
	}
}

func TestMatchAnyString(t *testing.T) {
	if !MatchAnyString([]string{"408", "429", "50X"}, "503") {
		t.Fatal("expected 50X to match 503")
	}
	if MatchAnyString([]string{"408", "429", "40X"}, "503") {
		t.Fatal("did not expect 40X to match 503")
	}
}
