package domain

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseBypassPolicyNormalizesAndDeduplicatesDomains(t *testing.T) {
	policy, err := ParseBypassPolicy(" Example.COM,\napi.example.com\r\nexample.com., api.example.com ")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := policy.Domains(), []string{"example.com", "api.example.com"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("normalized bypass domains = %#v, want %#v", got, want)
	}
	if policy.Empty() {
		t.Fatal("non-empty bypass policy reported empty")
	}
}

func TestBypassPolicyRejectsUnsupportedSyntax(t *testing.T) {
	for _, input := range []string{
		"https://example.com",
		"example.com:443",
		"*.example.com",
		"192.0.2.1",
		"2001:db8::1",
		"192.0.2.0/24",
		"-example.com",
		"example-.com",
	} {
		t.Run(strings.ReplaceAll(input, ":", "_"), func(t *testing.T) {
			if _, err := ParseBypassPolicy(input); err == nil {
				t.Fatalf("ParseBypassPolicy(%q) accepted unsupported syntax", input)
			}
		})
	}
}

func TestBypassPolicyDoesNotAliasInputOrOutputSlices(t *testing.T) {
	input := []string{"Example.com", "api.example.com"}
	policy, err := NewBypassPolicy(input)
	if err != nil {
		t.Fatal(err)
	}
	input[0] = "changed.example"
	if got := policy.Domains(); got[0] != "example.com" {
		t.Fatalf("policy changed when caller mutated input: %#v", got)
	}

	first := policy.Domains()
	first[0] = "changed.example"
	if got := policy.Domains(); got[0] != "example.com" {
		t.Fatalf("policy changed when caller mutated output: %#v", got)
	}
}

func TestBypassPolicyEmptyZeroValue(t *testing.T) {
	var policy BypassPolicy
	if !policy.Empty() || len(policy.Domains()) != 0 {
		t.Fatalf("zero-value policy is not empty: %#v", policy.Domains())
	}
}
