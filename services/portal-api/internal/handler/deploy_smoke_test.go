package handler

import "testing"

func TestImageTagFromImageRefMultiContainer(t *testing.T) {
	img := "harbor.example/test-hanbor/worker:bd34a6a0d38d00e27e66121389db38a9942556be (+1)"
	got := imageTagFromImageRef(img)
	want := "bd34a6a0d38d00e27e66121389db38a9942556be"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if !imageTagsMatch(want, got) {
		t.Fatal("should match after normalize")
	}
}

func TestIsTLSCertPendingErr(t *testing.T) {
	msg := `tls: failed to verify certificate: x509: certificate is valid for ingress.local, not test-hanbor-prod.platform.7mlabs.com`
	if !isTLSCertPendingErr(msg) {
		t.Fatal("expected tls pending")
	}
	if isTLSCertPendingErr("connection refused") {
		t.Fatal("connection refused is not tls pending")
	}
}
