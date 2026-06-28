package deploy

import "testing"

func TestPublicHealthPath(t *testing.T) {
	if got := PublicHealthPath("/api", "/health"); got != "/api/health" {
		t.Fatalf("got %q", got)
	}
	if got := PublicHealthPath("/", "/health"); got != "/health" {
		t.Fatalf("got %q", got)
	}
}

func TestSmokeCheckPaths_Multi(t *testing.T) {
	paths := SmokeCheckPaths(LayoutMulti, DefaultMultiServices)
	if len(paths) < 2 {
		t.Fatalf("paths=%v", paths)
	}
	if paths[0] != "/api/health" {
		t.Fatalf("expected /api/health first, got %v", paths)
	}
}

func TestSmokeCheckPaths_Single(t *testing.T) {
	paths := SmokeCheckPaths(LayoutSingle, nil)
	if paths[0] != "/health" {
		t.Fatalf("got %v", paths)
	}
}
