package main

import "testing"

func TestShouldFailRequestMatchesProtocolPrefixes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		protocol string
		payload  string
		want     bool
	}{
		{name: "http", protocol: "http", payload: "fail:http", want: true},
		{name: "grpc", protocol: "grpc", payload: "fail:grpc", want: true},
		{name: "rpc", protocol: "rpc", payload: "fail:rpc", want: true},
		{name: "all", protocol: "rpc", payload: "fail:all", want: true},
		{name: "mismatch", protocol: "http", payload: "fail:grpc", want: false},
		{name: "normal", protocol: "http", payload: "event", want: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := shouldFailRequest(tt.protocol, tt.payload); got != tt.want {
				t.Fatalf("shouldFailRequest(%q, %q) = %v, want %v", tt.protocol, tt.payload, got, tt.want)
			}
		})
	}
}
