# Checklist triển khai Platform

Tick `[x]` khi xong. Làm **theo thứ tự**, không nhảy phase.

---

## Phase 0 — Chuẩn bị (làm trước khi đụng K8s)

- [ ] Chọn profile K8s: **`rke2`** (mặc định) / `kubeadm` / `eks` | `gke` | `aks`
- [ ] Liệt kê server: IP, RAM, disk (prod ≥ 3 node nếu HA)
- [ ] Mua/chuẩn bị domain + quyền DNS
- [ ] Tạo GitHub org/repo:
  - [ ] `k8s-platform` (platform components)
  - [ ] `company-gitops` (manifest app — repo khách sở hữu)
- [ ] Copy `config/env.example.yaml` → `config/env.yaml`, điền hết
- [ ] Cài local: `kubectl`, `helm`, `k9s` (optional)

**Xong phase 0 khi:** có server, domain, repo, file `env.yaml`.

---

## Phase 1 — Cluster cơ bản ⭐ BẮT ĐẦU TỪ ĐÂY

- [ ] Provision node (Terraform/Ansible/manual SSH)
- [ ] Cài Kubernetes chuẩn (RKE2 hoặc kubeadm — không K3s)
- [ ] Verify: `kubectl version`, API server + etcd + containerd chạy ổn
- [ ] `kubectl get nodes` → tất cả Ready
- [ ] Cài StorageClass (Longhorn hoặc cloud CSI)
- [ ] Cài NGINX Ingress Controller
- [ ] Cài cert-manager + ClusterIssuer (Let's Encrypt)
- [ ] Deploy app test `hello-world`:
  - [ ] Deployment + Service
  - [ ] Ingress + TLS
  - [ ] Truy cập HTTPS được qua browser
- [ ] Ghi kubeconfig + runbook backup kubeconfig

**Xong phase 1 khi:** 1 URL HTTPS public chạy ổn 24h.

---

## Phase 1b — PostgreSQL (Platform Console)

- [ ] Đặt `POSTGRES_PASSWORD` trong `config/env.sh`
- [ ] `./bootstrap/run.sh 07` — cài Postgres namespace `platform`
- [ ] Kiểm tra `config/postgres.env` sinh ra (connection cho portal-api)
- [ ] Document backup PVC (Velero hoặc pg_dump định kỳ)

**Lưu ý:** DB này chỉ lưu user/project của Console — **không** thay DB app khách.

**Xong phase 1b khi:** `platform-postgresql` pod Running, portal-api connect được.

---

## Phase 2 — Rancher

- [ ] Cài Rancher (Helm) trên cluster
- [ ] Truy cập Rancher UI qua domain riêng (vd: `rancher.{domain}`)
- [ ] Tạo admin user, tắt password mặc định
- [ ] Import cluster vào Rancher
- [ ] Tạo namespace mẫu: `platform`, `demo-dev`, `demo-prod`
- [ ] Cấu hình RBAC: role Dev vs DevOps
- [ ] Test xem pod log từ Rancher UI

**Xong phase 2 khi:** DevOps vào Rancher quản lý namespace/pod được.

---

## Phase 3 — Harbor

- [ ] Cài Harbor (Helm) + PVC
- [ ] Domain: `harbor.{domain}` + TLS
- [ ] Tạo project `demo`, robot account cho CI
- [ ] Test push image thủ công: `docker push harbor.{domain}/demo/hello:v1`
- [ ] Bật scan image (Trivy)
- [ ] Document retention policy (giữ bao nhiêu tag)

**Xong phase 3 khi:** push/pull image từ máy dev được.

---

## Phase 4 — Argo CD + app mẫu GitOps

- [ ] Cài Argo CD
- [ ] Domain: `argocd.{domain}` + TLS
- [ ] Connect repo `company-gitops`
- [ ] Tạo cấu trúc app mẫu:
  ```
  apps/demo/
    base/
    overlays/dev/
    overlays/prod/
  ```
- [ ] Argo CD Application: `demo-dev` (auto sync)
- [ ] Argo CD Application: `demo-prod` (manual sync)
- [ ] GitHub Actions workflow mẫu:
  - [ ] build → push Harbor
  - [ ] update image tag trong repo gitops (dev)
- [ ] Chạy thử full pipeline: push code → dev deploy tự động
- [ ] Promote prod: đổi tag → sync thủ công ArgoCD

**Xong phase 4 khi:** 1 app chạy full CI/CD dev, prod deploy tay qua ArgoCD.

---

## Phase 5 — Monitoring

- [ ] Cài kube-prometheus-stack (Prometheus + Grafana)
- [ ] Domain: `grafana.{domain}` + TLS
- [ ] Import dashboard: node, pod, ingress
- [ ] Cấu hình Alertmanager (Slack/email tối thiểu):
  - [ ] Node NotReady
  - [ ] Pod CrashLoopBackOff
  - [ ] Cert sắp hết hạn
- [ ] Test alert (scale deployment sai → xem có báo không)

**Xong phase 5 khi:** Grafana có metric, nhận được 1 alert test.

---

## Phase 6 — Logging

- [ ] Cài Loki + Promtail (hoặc Fluent Bit)
- [ ] Grafana datasource → Loki
- [ ] Test query log theo namespace/pod
- [ ] Set retention (vd: 7–14 ngày dev, 30 ngày prod)

**Xong phase 6 khi:** search log app demo trên Grafana được.

---

## Phase 7 — Hardening & bàn giao

- [ ] Sealed Secrets hoặc External Secrets Operator
- [ ] Kyverno policy cơ bản (bắt buộc label `env`, limit resource)
- [ ] NetworkPolicy giữa namespace dev/prod (nếu cần)
- [ ] Velero backup (etcd + PVC quan trọng)
- [ ] Pin version tất cả Helm chart vào git
- [ ] Viết/viết xong:
  - [ ] `docs/onboarding-new-app.md` (thêm project mới 5 bước)
  - [ ] `docs/runbook-incident.md`
  - [ ] `docs/handover-checklist.md`
- [ ] Workshop handover 2–4h với DevOps khách
- [ ] Hypercare 2 tuần sau bàn giao

**Xong phase 7 khi:** DevOps khách tự thêm app mới không cần bạn.

---

## Sau bàn giao — thêm project mới (5 bước)

1. Copy `templates/new-project/` → `apps/{tên-app}/`
2. Sửa tên, image, domain, resource trong overlay dev/prod
3. Thêm ArgoCD Application (hoặc ApplicationSet tự pick)
4. Copy GitHub Actions workflow, đổi secret Harbor
5. Push → kiểm tra dev → promote prod

---

## Ghi chú / blocker

| Ngày | Ghi chú |
|------|---------|
|      |         |
