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
	"strings"
)

func MergeMaps(maps ...map[string]interface{}) Option {
	result := make(map[string]interface{})
	for _, m := range maps {
		for k, v := range m {
			if nestedMap, ok := v.(map[string]interface{}); ok {
				if existingNestedMap, exists := result[k].(map[string]interface{}); exists {
					result[k] = map[string]interface{}(MergeMaps(existingNestedMap, nestedMap))
				} else {
					result[k] = map[string]interface{}(MergeMaps(nestedMap))
				}
			} else {
				result[k] = v
			}
		}
	}
	return result
}

// CloneMap returns a shallow copy of the input map.
// Nil or empty input returns an empty writable map.
func CloneMap[K comparable, V any](input map[K]V) map[K]V {
	if len(input) == 0 {
		return map[K]V{}
	}
	out := make(map[K]V, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}

// Unique returns the first occurrence of each value while preserving input order.
func Unique[T comparable](input []T) []T {
	if len(input) == 0 {
		return []T{}
	}
	out := make([]T, 0, len(input))
	seen := map[T]bool{}
	for _, value := range input {
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func GetCaseInsensitiveKeyValue(cfg map[string]string, key string) (string, bool) {
	if value, ok := cfg[key]; ok {
		return value, true
	}
	if value, ok := cfg[strings.ToUpper(key)]; ok {
		return value, true
	}
	return "", false
}

func EmbeddingToFloat64[T float32 | float64](embedding []T) []float64 {
	float64Embedding := make([]float64, len(embedding))
	for i, val := range embedding {
		float64Embedding[i] = float64(val)
	}
	return float64Embedding
}

func EmbeddingToFloat32[T float32 | float64](embedding []T) []float32 {
	float32Embedding := make([]float32, len(embedding))
	for i, val := range embedding {
		float32Embedding[i] = float32(val)
	}
	return float32Embedding
}

// Convert a slice of float32 to a byte array
func Float64SliceToByteArray(data []float64) ([]byte, error) {
	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.LittleEndian, data)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func EmbeddingToBase64(embedding []float64) string {
	byteArray, err := Float64SliceToByteArray(embedding)
	if err != nil {
		return ""
	}
	base64Str := base64.StdEncoding.EncodeToString(byteArray)
	return base64Str
}
