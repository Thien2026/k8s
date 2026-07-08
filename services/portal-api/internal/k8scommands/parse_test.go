package k8scommands

import "testing"

func TestParseGetPods(t *testing.T) {
	p, err := ParseReadOnlyKubectl("kubectl get pods -n my-ns")
	if err != nil {
		t.Fatal(err)
	}
	if p.CommandKey != "pods_list" || p.Namespace != "my-ns" {
		t.Fatalf("unexpected: %+v", p)
	}
}

func TestParseLogs(t *testing.T) {
	p, err := ParseReadOnlyKubectl("kubectl logs api-abc -n my-ns --tail=500")
	if err != nil {
		t.Fatal(err)
	}
	if p.CommandKey != "pods_logs" || p.Name != "api-abc" || p.Tail != 500 {
		t.Fatalf("unexpected: %+v", p)
	}
}

func TestParseBlocksDelete(t *testing.T) {
	_, err := ParseReadOnlyKubectl("kubectl delete pod x -n ns")
	if err == nil {
		t.Fatal("expected block delete")
	}
}

func TestParseDescribeDeployment(t *testing.T) {
	p, err := ParseReadOnlyKubectl("kubectl describe deployment web -n prod")
	if err != nil {
		t.Fatal(err)
	}
	if p.CommandKey != "deployments_describe" || p.Name != "web" {
		t.Fatalf("unexpected: %+v", p)
	}
}

func TestParseStatefulSetList(t *testing.T) {
	p, err := ParseReadOnlyKubectl("kubectl get sts -n app-dev")
	if err != nil {
		t.Fatal(err)
	}
	if p.CommandKey != "statefulsets_list" {
		t.Fatalf("unexpected: %+v", p)
	}
}

func TestParseBlocksTop(t *testing.T) {
	_, err := ParseReadOnlyKubectl("kubectl top pods -n ns")
	if err == nil {
		t.Fatal("expected block top")
	}
}
