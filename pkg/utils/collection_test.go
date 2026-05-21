// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package utils

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"testing"
)

func TestMergeMaps(t *testing.T) {
	tests := []struct {
		name     string
		maps     []map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name: "merge two simple maps",
			maps: []map[string]interface{}{
				{"a": 1, "b": 2},
				{"c": 3, "d": 4},
			},
			expected: map[string]interface{}{
				"a": 1, "b": 2, "c": 3, "d": 4,
			},
		},
		{
			name: "merge with overlapping keys",
			maps: []map[string]interface{}{
				{"a": 1, "b": 2},
				{"a": 10, "c": 3},
			},
			expected: map[string]interface{}{
				"a": 10, "b": 2, "c": 3,
			},
		},
		{
			name: "merge nested maps",
			maps: []map[string]interface{}{
				{"nested": map[string]interface{}{"x": 1}},
				{"nested": map[string]interface{}{"y": 2}},
			},
			expected: map[string]interface{}{
				"nested": map[string]interface{}{"x": 1, "y": 2},
			},
		},
		{
			name:     "empty maps",
			maps:     []map[string]interface{}{},
			expected: map[string]interface{}{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MergeMaps(tt.maps...)
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d keys, got %d", len(tt.expected), len(result))
			}
			for k, v := range tt.expected {
				if nestedExpected, ok := v.(map[string]interface{}); ok {
					if nestedResult, ok := result[k].(map[string]interface{}); ok {
						for nk, nv := range nestedExpected {
							if nestedResult[nk] != nv {
								t.Errorf("nested key %s.%s: expected %v, got %v", k, nk, nv, nestedResult[nk])
							}
						}
					} else {
						t.Errorf("key %s: expected map, got %v", k, result[k])
					}
				} else if result[k] != v {
					t.Errorf("key %s: expected %v, got %v", k, v, result[k])
				}
			}
		})
	}
}

func TestCloneMap(t *testing.T) {
	t.Run("nil map returns empty writable map", func(t *testing.T) {
		var src map[string]interface{}
		cloned := CloneMap(src)
		if cloned == nil {
			t.Fatalf("expected non-nil map")
		}
		if len(cloned) != 0 {
			t.Fatalf("expected empty map, got %d", len(cloned))
		}
		cloned["k"] = "v"
		if _, ok := src["k"]; ok {
			t.Fatalf("source map should not be mutated")
		}
	})

	t.Run("clone copies entries and is independent", func(t *testing.T) {
		src := map[string]int{"a": 1, "b": 2}
		cloned := CloneMap(src)
		if len(cloned) != len(src) {
			t.Fatalf("expected len %d, got %d", len(src), len(cloned))
		}
		if cloned["a"] != 1 || cloned["b"] != 2 {
			t.Fatalf("unexpected cloned values: %#v", cloned)
		}
		cloned["a"] = 10
		if src["a"] != 1 {
			t.Fatalf("source map mutated: %#v", src)
		}
	})
}

func TestUnique(t *testing.T) {
	t.Run("preserves first occurrence order", func(t *testing.T) {
		got := Unique([]uint64{3, 1, 3, 2, 1})
		expected := []uint64{3, 1, 2}
		if len(got) != len(expected) {
			t.Fatalf("expected len %d, got %d", len(expected), len(got))
		}
		for i := range expected {
			if got[i] != expected[i] {
				t.Fatalf("expected %v, got %v", expected, got)
			}
		}
	})

	t.Run("empty input returns empty writable slice", func(t *testing.T) {
		got := Unique([]string{})
		if got == nil {
			t.Fatal("expected non-nil slice")
		}
		if len(got) != 0 {
			t.Fatalf("expected empty slice, got %d", len(got))
		}
	})
}

func TestGetCaseInsensitiveKeyValue(t *testing.T) {
	cfg := map[string]string{
		"KEY1": "value1",
		"KEY2": "value2",
		"KEY3": "value3",
	}

	tests := []struct {
		name     string
		key      string
		expected string
		found    bool
	}{
		{"exact match", "KEY1", "value1", true},
		{"lowercase", "key1", "value1", true},
		{"uppercase", "KEY2", "value2", true},
		{"mixed case", "Key3", "value3", true},
		{"not found", "key4", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, found := GetCaseInsensitiveKeyValue(cfg, tt.key)
			if result != tt.expected || found != tt.found {
				t.Errorf("expected %s, %t; got %s, %t", tt.expected, tt.found, result, found)
			}
		})
	}
}

func TestEmbeddingToFloat64(t *testing.T) {
	tests := []struct {
		name     string
		input    []float32
		expected []float64
	}{
		{"float32 to float64", []float32{1.0, 2.0, 3.0}, []float64{1.0, 2.0, 3.0}},
		{"empty slice", []float32{}, []float64{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EmbeddingToFloat64(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("expected length %d, got %d", len(tt.expected), len(result))
			}
			for i, v := range result {
				if v != tt.expected[i] {
					t.Errorf("at index %d: expected %f, got %f", i, tt.expected[i], v)
				}
			}
		})
	}
}

func TestEmbeddingToFloat32(t *testing.T) {
	tests := []struct {
		name     string
		input    []float64
		expected []float32
	}{
		{"float64 to float32", []float64{1.0, 2.5, 3.7}, []float32{1.0, 2.5, 3.7}},
		{"empty slice", []float64{}, []float32{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EmbeddingToFloat32(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("expected length %d, got %d", len(tt.expected), len(result))
			}
			for i, v := range result {
				if v != tt.expected[i] {
					t.Errorf("at index %d: expected %f, got %f", i, tt.expected[i], v)
				}
			}
		})
	}
}

func TestFloat64SliceToByteArray(t *testing.T) {
	data := []float64{1.0, 2.0, 3.0}
	result, err := Float64SliceToByteArray(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify by decoding back
	buf := bytes.NewReader(result)
	var decoded []float64
	for i := 0; i < len(data); i++ {
		var val float64
		err := binary.Read(buf, binary.LittleEndian, &val)
		if err != nil {
			t.Fatalf("error decoding: %v", err)
		}
		decoded = append(decoded, val)
	}

	if len(decoded) != len(data) {
		t.Errorf("expected length %d, got %d", len(data), len(decoded))
	}
	for i, v := range decoded {
		if v != data[i] {
			t.Errorf("at index %d: expected %f, got %f", i, data[i], v)
		}
	}
}

func TestEmbeddingToBase64(t *testing.T) {
	embedding := []float64{1.0, 2.0, 3.0}
	result := EmbeddingToBase64(embedding)
	if result == "" {
		t.Error("expected non-empty base64 string")
	}

	// Decode and verify
	data, err := base64.StdEncoding.DecodeString(result)
	if err != nil {
		t.Fatalf("error decoding base64: %v", err)
	}

	buf := bytes.NewReader(data)
	var decoded []float64
	for i := 0; i < len(embedding); i++ {
		var val float64
		err := binary.Read(buf, binary.LittleEndian, &val)
		if err != nil {
			t.Fatalf("error decoding: %v", err)
		}
		decoded = append(decoded, val)
	}

	if len(decoded) != len(embedding) {
		t.Errorf("expected length %d, got %d", len(embedding), len(decoded))
	}
	for i, v := range decoded {
		if v != embedding[i] {
			t.Errorf("at index %d: expected %f, got %f", i, embedding[i], v)
		}
	}
}
