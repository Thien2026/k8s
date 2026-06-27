package handler

import "testing"

func TestOptionalEnvScope(t *testing.T) {
	s, ok := optionalEnvScope("")
	if !ok || s != "" {
		t.Fatalf("empty scope should mean all scopes, got %q ok=%v", s, ok)
	}
	s, ok = optionalEnvScope("build")
	if !ok || s != "build" {
		t.Fatalf("build scope: got %q ok=%v", s, ok)
	}
	_, ok = optionalEnvScope("pod")
	if ok {
		t.Fatal("invalid scope should fail")
	}
}

func TestValidEnvScopeDefaultsRuntime(t *testing.T) {
	s, ok := validEnvScope("")
	if !ok || s != "runtime" {
		t.Fatalf("create default scope should be runtime, got %q", s)
	}
}
