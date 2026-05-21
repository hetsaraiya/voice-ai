// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package validator

import "testing"

func TestOneOf(t *testing.T) {
	if !OneOf("admin", "owner", "admin") {
		t.Fatal("expected value to match one option")
	}
	if OneOf("member", "owner", "admin") {
		t.Fatal("expected value not to match any option")
	}
}

func TestNotEmpty(t *testing.T) {
	if !NotEmpty([]uint64{1}) {
		t.Fatal("expected non-empty slice to pass validation")
	}
	if NotEmpty([]uint64{}) {
		t.Fatal("expected empty slice to fail validation")
	}
}

func TestEmail(t *testing.T) {
	if !Email("user@example.com") {
		t.Fatal("expected valid email to pass validation")
	}
	if Email(" user@example.com") {
		t.Fatal("expected email with leading space to fail validation")
	}
	if Email("User <user@example.com>") {
		t.Fatal("expected named address to fail exact email validation")
	}
	if Email("not-email") {
		t.Fatal("expected invalid email to fail validation")
	}
}

func TestAllNonZero(t *testing.T) {
	if !AllNonZero(uint64(1), uint64(2)) {
		t.Fatal("expected all values to be non-zero")
	}
	if AllNonZero(uint64(1), uint64(0)) {
		t.Fatal("expected zero value to fail validation")
	}
}
