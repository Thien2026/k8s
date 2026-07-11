package domains

import (
	"context"
	"fmt"

	"github.com/Thien2026/k8s/services/portal-api/internal/deploy"
	"github.com/Thien2026/k8s/services/portal-api/internal/rancher"
)

type Syncer struct {
	Rancher  *rancher.Client
	Platform Platform
}

type DomainInput struct {
	ID             int64
	Hostname       string
	Environment    string
	TLSEnabled     bool
	Namespace      string
	Routes         []deploy.IngressRoute
	ProxyBodySize  string
}

func (s *Syncer) Ready() bool {
	return s.Rancher != nil && s.Rancher.Enabled()
}

func (s *Syncer) SyncIngress(ctx context.Context, clusterID string, d DomainInput) error {
	if !s.Ready() {
		return fmt.Errorf("Rancher chưa sẵn sàng")
	}
	if err := s.Rancher.EnsureNamespace(ctx, clusterID, d.Namespace); err != nil {
		return err
	}
	payload, err := IngressManifest(d.Hostname, d.Namespace, d.ID, d.TLSEnabled, d.Routes, d.ProxyBodySize)
	if err != nil {
		return err
	}
	if err := s.Rancher.ApplyNamespacedObject(ctx, clusterID, "/apis/networking.k8s.io/v1/ingresses", d.Namespace, payload); err != nil {
		return err
	}
	patch, err := IngressPathsPatch(d.Routes)
	if err != nil {
		return err
	}
	return s.Rancher.PatchNamespacedObject(ctx, clusterID, "/apis/networking.k8s.io/v1/ingresses", d.Namespace, IngressName(d.ID), patch)
}

func (s *Syncer) DeleteIngress(ctx context.Context, clusterID, namespace string, domainID int64) error {
	if !s.Ready() {
		return nil
	}
	return s.Rancher.DeleteNamespacedObject(ctx, clusterID, "/apis/networking.k8s.io/v1/ingresses", namespace, IngressName(domainID))
}

func (s *Syncer) CertStatus(ctx context.Context, clusterID, namespace string, domainID int64, tlsEnabled bool) string {
	if !tlsEnabled || !s.Ready() {
		return "n/a"
	}
	st, err := s.Rancher.CertificateReady(ctx, clusterID, namespace, TLSSecretName(domainID))
	if err != nil || st == "" {
		return "pending"
	}
	return st
}
