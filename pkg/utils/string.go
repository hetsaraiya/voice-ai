// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package utils

// MatchString returns true when pattern equals value, or when pattern uses
// uppercase X as a single-character wildcard. Inputs are matched as provided.
func MatchString(pattern string, value string) bool {
	if pattern == value {
		return true
	}

	patternRunes := []rune(pattern)
	valueRunes := []rune(value)
	if len(patternRunes) != len(valueRunes) {
		return false
	}

	for i, patternRune := range patternRunes {
		if patternRune == 'X' {
			continue
		}
		if patternRune != valueRunes[i] {
			return false
		}
	}
	return true
}

func MatchAnyString(patterns []string, value string) bool {
	for _, pattern := range patterns {
		if MatchString(pattern, value) {
			return true
		}
	}
	return false
}
