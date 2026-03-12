package web

import "testing"

func TestParseServerPaidUntil(t *testing.T) {
	parsed := ParseServerPaidUntil("A=01-01-2027; B = 02-02-2028 ;invalid;=03-03-2029;C=")

	if parsed["A"] != "01-01-2027" {
		t.Fatalf("expected A to be parsed")
	}
	if parsed["B"] != "02-02-2028" {
		t.Fatalf("expected B to be parsed")
	}
	if _, ok := parsed["invalid"]; ok {
		t.Fatalf("unexpected invalid entry")
	}
	if _, ok := parsed[""]; ok {
		t.Fatalf("unexpected empty key")
	}
	if _, ok := parsed["C"]; ok {
		t.Fatalf("unexpected empty value")
	}
}
