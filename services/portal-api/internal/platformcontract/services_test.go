package platformcontract

import "testing"

func TestParseServicesRequiresPublic(t *testing.T) {
	_, err := ParseServices(`version: 1
layout: multi
services:
  - name: a
    path: a
    expose: false
  - name: b
    path: b
    expose: false`)
	if err == nil {
		t.Fatal("expected error for no public service")
	}
}

func TestResolveSubmodulesMode(t *testing.T) {
	if got := ResolveSubmodulesMode("", "recursive"); got != "recursive" {
		t.Fatalf("recursive: %q", got)
	}
	if got := ResolveSubmodulesMode("true"); got != "true" {
		t.Fatalf("true: %q", got)
	}
	if got := ResolveSubmodulesMode("false", "off"); got != "" {
		t.Fatalf("empty: %q", got)
	}
}

func TestParseServicesGitSubmodules(t *testing.T) {
	f, err := ParseServices(`version: 1
layout: multi
git:
  submodules: recursive
services:
  - name: api
    path: backend
    ingress: /api
  - name: web
    path: frontend
    ingress: /`)
	if err != nil {
		t.Fatal(err)
	}
	if ResolveSubmodulesMode(f.Git.Submodules, f.Submodules) != "recursive" {
		t.Fatal("expected recursive from git.submodules")
	}
}

func TestParseServicesLayoutDefault(t *testing.T) {
	f, err := ParseServices(`version: 1
services:
  - name: api
    path: backend
    ingress: /api
  - name: web
    path: frontend
    ingress: /`)
	if err != nil {
		t.Fatal(err)
	}
	if f.Layout != LayoutMulti {
		t.Fatalf("want multi default, got %s", f.Layout)
	}
}
