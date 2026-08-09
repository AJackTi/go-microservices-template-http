package main

import (
	"reflect"
	"testing"
)

func TestCorsAllowedOriginsUsesConfiguredList(t *testing.T) {
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://app.example, https://admin.example ,,")

	got := corsAllowedOrigins()
	want := []string{"https://app.example", "https://admin.example"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected origins: got %#v want %#v", got, want)
	}
}

func TestCorsAllowedOriginsFallsBackToLocalhost(t *testing.T) {
	t.Setenv("CORS_ALLOWED_ORIGINS", "  ")

	got := corsAllowedOrigins()
	want := []string{"http://localhost:8081", "http://127.0.0.1:8081"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected origins: got %#v want %#v", got, want)
	}
}
