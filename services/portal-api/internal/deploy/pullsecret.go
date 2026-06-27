package deploy

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

const (
	HarborPullSecretName = "platform-harbor"
	GHCRPullSecretName   = "platform-ghcr"
	GHCRRegistryHost     = "ghcr.io"
)

// DockerRegistryPullSecret manifest Secret docker-registry cho pull image private.
func DockerRegistryPullSecret(secretName, namespace, registryHost, username, password string) ([]byte, error) {
	if secretName == "" || namespace == "" || registryHost == "" || username == "" || password == "" {
		return nil, fmt.Errorf("thiếu thông tin docker pull secret")
	}
	auth := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
	cfg, err := json.Marshal(map[string]any{
		"auths": map[string]any{
			registryHost: map[string]string{
				"username": username,
				"password": password,
				"auth":     auth,
			},
		},
	})
	if err != nil {
		return nil, err
	}
	secret := map[string]any{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata": map[string]any{
			"name":      secretName,
			"namespace": namespace,
		},
		"type": "kubernetes.io/dockerconfigjson",
		"data": map[string]string{
			".dockerconfigjson": base64.StdEncoding.EncodeToString(cfg),
		},
	}
	return json.Marshal(secret)
}

// HarborPullSecret manifest Secret docker-registry cho pull image private từ Harbor.
func HarborPullSecret(namespace, registryHost, username, password string) ([]byte, error) {
	return DockerRegistryPullSecret(HarborPullSecretName, namespace, registryHost, username, password)
}

// GHCRPullSecret manifest Secret docker-registry cho pull image private từ ghcr.io.
func GHCRPullSecret(namespace, username, password string) ([]byte, error) {
	return DockerRegistryPullSecret(GHCRPullSecretName, namespace, GHCRRegistryHost, username, password)
}
