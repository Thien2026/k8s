package deploy

import (
	"encoding/json"
	"testing"
)

func TestGHCRPullSecret(t *testing.T) {
	raw, err := GHCRPullSecret("demo-dev", "octocat", "pat_secret")
	if err != nil {
		t.Fatal(err)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatal(err)
	}
	meta := obj["metadata"].(map[string]any)
	if meta["name"] != GHCRPullSecretName {
		t.Fatalf("name=%v", meta["name"])
	}
	if meta["namespace"] != "demo-dev" {
		t.Fatalf("namespace=%v", meta["namespace"])
	}
}
