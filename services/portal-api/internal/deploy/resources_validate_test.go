package deploy

import "testing"

func TestValidateServiceResources_CustomOK(t *testing.T) {
	if err := ValidateServiceResources(ResourcesCustom, "100m", "128Mi", "500m", "512Mi"); err != nil {
		t.Fatal(err)
	}
}

func TestValidateServiceResources_ReqGtLim(t *testing.T) {
	err := ValidateServiceResources(ResourcesCustom, "1000m", "128Mi", "500m", "512Mi")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateServiceResources_BadMem(t *testing.T) {
	err := ValidateServiceResources(ResourcesCustom, "100m", "128MB", "500m", "512Mi")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateServiceResources_EmptyCustom(t *testing.T) {
	err := ValidateServiceResources(ResourcesCustom, "", "", "", "")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateServiceResources_PlatformSkip(t *testing.T) {
	if err := ValidateServiceResources(ResourcesPlatform, "bad", "", "", ""); err != nil {
		t.Fatal(err)
	}
}
