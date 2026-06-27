package platformcontract

import (
	"strings"
	"testing"
)

func TestParseBuildContract(t *testing.T) {
	raw := `version: 1
vars:
  BUILD_LABEL:
    required: true
    description: Nhãn app
  BANNER:
    required: false
`
	f, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !f.Vars["BUILD_LABEL"].Required {
		t.Fatal("BUILD_LABEL should be required")
	}
	if f.Vars["BANNER"].Required {
		t.Fatal("BANNER should be optional")
	}
}

func TestCheckBuild_MissingRequired(t *testing.T) {
	f, _ := Parse(`version: 1
vars:
  BUILD_LABEL:
    required: true
`)
	res := CheckBuild(&f, nil, nil)
	if res.Ready {
		t.Fatal("expected not ready")
	}
	if len(res.MissingRequired) != 1 || res.MissingRequired[0] != "BUILD_LABEL" {
		t.Fatalf("missing: %v", res.MissingRequired)
	}
}

func TestCheckBuild_EmptyConsoleValue(t *testing.T) {
	f, _ := Parse(`version: 1
vars:
  BUILD_LABEL:
    required: true
`)
	res := CheckBuild(&f, []ConsoleVar{{Key: "BUILD_LABEL", Value: "  "}}, nil)
	if res.Ready {
		t.Fatal("expected not ready for empty value")
	}
}

func TestCheckBuild_OK(t *testing.T) {
	f, _ := Parse(`version: 1
vars:
  BUILD_LABEL:
    required: true
`)
	res := CheckBuild(&f, []ConsoleVar{{Key: "BUILD_LABEL", Value: "hello"}}, []string{"BUILD_LABEL"})
	if !res.Ready {
		t.Fatalf("expected ready: %+v", res)
	}
}

func TestParseDockerfileARGs(t *testing.T) {
	df := `FROM golang:1.23 AS build
ARG GIT_SHA=""
ARG BUILD_LABEL=""
ARG BUILD_LABEL=""
`
	args := ParseDockerfileARGs(df)
	if len(args) != 1 || args[0] != "BUILD_LABEL" {
		t.Fatalf("args=%v", args)
	}
}

func TestDriftWarning(t *testing.T) {
	f, _ := Parse(`version: 1
vars:
  BUILD_LABEL:
    required: false
`)
	res := CheckBuild(&f, []ConsoleVar{{Key: "BUILD_LABEL", Value: "x"}}, []string{"OTHER"})
	if !res.Ready {
		t.Fatal("optional drift should stay ready")
	}
	found := false
	for _, w := range res.Warnings {
		if strings.Contains(w, "OTHER") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected drift warning: %v", res.Warnings)
	}
}

func TestDriftRequiredMissingARGBlocks(t *testing.T) {
	f, _ := Parse(`version: 1
vars:
  BUILD_LABEL:
    required: true
`)
	res := CheckBuild(&f, []ConsoleVar{{Key: "BUILD_LABEL", Value: "x"}}, []string{"GIT_SHA"})
	if res.Ready {
		t.Fatal("missing required ARG should block ready")
	}
}
