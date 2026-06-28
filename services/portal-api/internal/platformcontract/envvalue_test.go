package platformcontract

import "testing"

func TestValidateProdEnvValue_BlocksLocalhost(t *testing.T) {
	if err := ValidateProdEnvValue("prod", "VITE_API_BASE", "http://localhost:8080"); err == nil {
		t.Fatal("expected error")
	}
	if err := ValidateProdEnvValue("dev", "VITE_API_BASE", "http://localhost:8080"); err != nil {
		t.Fatalf("dev should allow: %v", err)
	}
	if err := ValidateProdEnvValue("prod", "VITE_API_BASE", "/api"); err != nil {
		t.Fatalf("prod /api ok: %v", err)
	}
}

func TestContainsLocalhostRef(t *testing.T) {
	if !ContainsLocalhostRef("http://127.0.0.1:5050") {
		t.Fatal("127.0.0.1")
	}
}

func TestMergeContracts(t *testing.T) {
	repo, _ := Parse(`version: 1
vars:
  CUSTOM:
    required: true
`)
	def := File{Version: 1, Vars: map[string]VarSpec{
		"VITE_API_BASE": {Required: true},
	}}
	merged := MergeContracts(&repo, def)
	if !merged.Vars["VITE_API_BASE"].Required || !merged.Vars["CUSTOM"].Required {
		t.Fatalf("%+v", merged.Vars)
	}
}
