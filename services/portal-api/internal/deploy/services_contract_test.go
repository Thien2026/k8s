package deploy

import (
	"testing"

	"github.com/Thien2026/k8s/services/portal-api/internal/platformcontract"
)

const sampleNServiceYAML = `
version: 1
layout: multi
services:
  - name: api
    path: backend
    ingress: /api
  - name: web
    path: frontend
    ingress: /
  - name: worker
    path: worker
    expose: false
`

func TestServiceDefsFromContract(t *testing.T) {
	f, err := platformcontract.ParseServices(sampleNServiceYAML)
	if err != nil {
		t.Fatal(err)
	}
	svcs := ServiceDefsFromContract(f)
	if len(svcs) != 3 {
		t.Fatalf("want 3, got %d", len(svcs))
	}
	if svcs[2].ExposeIngress {
		t.Fatal("worker should be internal")
	}
}

func TestServicesContractSameAsDB(t *testing.T) {
	f, _ := platformcontract.ParseServices(sampleNServiceYAML)
	db := ServiceDefsFromContract(f)
	if !ServicesContractSameAsDB(f, db) {
		t.Fatal("expected in sync")
	}
}
