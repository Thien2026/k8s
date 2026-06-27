package deploy

import (
	"encoding/json"
	"fmt"
)

const AppEnvSecretName = "app-env"

// AppEnvSecret manifest Secret Opaque chứa biến môi trường runtime cho app.
func AppEnvSecret(namespace string, vars map[string]string) ([]byte, error) {
	if namespace == "" {
		return nil, fmt.Errorf("namespace bắt buộc")
	}
	if len(vars) == 0 {
		return nil, nil
	}
	secret := map[string]any{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata": map[string]any{
			"name":      AppEnvSecretName,
			"namespace": namespace,
			"labels": map[string]string{
				"app.kubernetes.io/managed-by": "platform-console",
			},
		},
		"type":       "Opaque",
		"stringData": vars,
	}
	return json.Marshal(secret)
}
