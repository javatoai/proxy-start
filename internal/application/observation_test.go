package application

import (
	"proxy-start/internal/domain"
	"testing"
)

func TestProxyObservationPreservesUncertainty(t *testing.T) {
	cases := []struct {
		name          string
		info          domain.ProcessInfo
		kind, address string
	}{
		{"unreadable", domain.ProcessInfo{}, "unknown", ""},
		{"ordinary", domain.ProcessInfo{Readable: true}, "unspecified", ""},
		{"explicit", domain.ProcessInfo{Readable: true, Arguments: []string{"--proxy-server=http://127.0.0.1:7897"}}, "explicit", "http://127.0.0.1:7897"},
		{"split", domain.ProcessInfo{Readable: true, Arguments: []string{"--proxy-server", "http://localhost:8"}}, "explicit", "http://localhost:8"},
		{"redacted", domain.ProcessInfo{Readable: true, Arguments: []string{"--proxy-server=http://user:secret@localhost:8/?token=secret"}}, "explicit", "http://localhost:8"},
		{"disabled", domain.ProcessInfo{Readable: true, Arguments: []string{"--no-proxy-server"}}, "disabled", ""},
		{"pac", domain.ProcessInfo{Readable: true, Arguments: []string{"--proxy-pac-url=secret"}}, "automatic", ""},
		{"conflict", domain.ProcessInfo{Readable: true, Arguments: []string{"--no-proxy-server", "--proxy-server=http://localhost:8"}}, "unknown", ""},
		{"missing", domain.ProcessInfo{Readable: true, Arguments: []string{"--proxy-server"}}, "unknown", ""},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got := ObserveProxy(test.info)
			if got.Kind != test.kind || got.Address != test.address {
				t.Fatalf("%+v", got)
			}
		})
	}
	if got := CombineProxyObservations([]domain.ProxyObservation{{Kind: "explicit", Address: "a"}, {Kind: "explicit", Address: "b"}}); got.Kind != "unknown" {
		t.Fatal("inconsistent roots reported as configured")
	}
}
