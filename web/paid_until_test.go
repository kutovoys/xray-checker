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

func TestGetPaidUntilForProxyName_NormalizesHiddenCharactersAndSpacing(t *testing.T) {
	parsed := ParseServerPaidUntil("🇪🇪 Эстония - 2=31-12-2026")

	proxyNameWithHiddenChars := " 🇪🇪 \u200b Эстония   -   2 "
	if got := GetPaidUntilForProxyName(parsed, proxyNameWithHiddenChars); got != "31-12-2026" {
		t.Fatalf("expected paid-until to match normalized proxy name, got %q", got)
	}
}
