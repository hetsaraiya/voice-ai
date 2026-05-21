// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

// Package validator contains small reusable validation helpers.
package validator

import "net/mail"

// OneOf returns true when value matches one of the provided options.
func OneOf[T comparable](value T, options ...T) bool {
	for _, option := range options {
		if value == option {
			return true
		}
	}
	return false
}

// NotEmpty returns true when the provided slice has at least one value.
func NotEmpty[T any](values []T) bool {
	return len(values) > 0
}

// Email returns true when value is a valid mailbox exactly as provided.
func Email(value string) bool {
	parsedEmail, err := mail.ParseAddress(value)
	return err == nil && parsedEmail.Address == value && parsedEmail.Name == ""
}

// AllNonZero returns true when every provided value is not its zero value.
func AllNonZero[T comparable](values ...T) bool {
	var zero T
	for _, value := range values {
		if value == zero {
			return false
		}
	}
	return true
}
