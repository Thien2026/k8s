# Kubernetes Platform Foundation

Source tái sử dụng: K8s + Rancher + ArgoCD + Harbor + Monitoring + Logging + GitOps.

> Chi tiết từng bước: [TASKS.md](./TASKS.md) · Công nghệ & vì sao chọn: [docs/STACK.md](./docs/STACK.md)

## Bắt đầu từ đâu?

**Tuần 1 — chỉ làm 3 việc:**

1. Chọn profile hạ tầng (mục 1 bên dưới)
2. Dựng cluster + kubectl OK
3. Cài Ingress + cert-manager → test 1 app hello-world qua HTTPS

Đừng cài Rancher/Harbor/ArgoCD cùng lúc. Làm xong Layer 1 ổn định rồi mới lên Layer 2.

## Kubernetes — cài gì?

Yêu cầu là **Kubernetes chuẩn** (upstream-compatible), không dùng K3s cho bàn giao.

| Profile | Node tối thiểu | Dùng khi |
|---------|----------------|----------|
| **`rke2`** ⭐ | 3+ (HA) | On-prem / VPS — **mặc định**, hợp Rancher (cùng SUSE/Rancher) |
| `kubeadm` | 3+ | Muốn K8s vanilla, tự cài từng thành phần |
| `eks` / `gke` / `aks` | managed | Khách chạy trên cloud |

> K3s vẫn là K8s (CNCF certified) nhưng khách enterprise/DevOps thường không chấp nhận. Chỉ dùng K3s nếu **tự lab** trên laptop, không đưa vào handover.

Ghi lựa chọn vào `config/env.yaml` (tạo ở Phase 0).

## Thứ tự triển khai (tóm tắt)

```
Phase 0  Chuẩn bị (domain, GitHub, server)
Phase 1  Cluster + Ingress + cert-manager + Storage
Phase 1b PostgreSQL (Console metadata)
Phase 2  Rancher
Phase 3  Harbor
Phase 4  Argo CD + 1 app mẫu GitOps
Phase 5  Prometheus + Grafana
Phase 6  Loki (logs)
Phase 7  Hardening + backup + handover
```

## Cấu trúc repo (dần dần tạo)

```
k8s/
├── README.md
├── TASKS.md              ← checklist, tick dần
├── docs/
│   └── STACK.md          ← công nghệ & quyết định kiến trúc
├── config/
│   └── env.yaml          ← biến môi trường khách
├── bootstrap/            ← script cài cluster
├── platform/             ← Helm/GitOps cho platform components
│   ├── ingress/
│   ├── cert-manager/
│   ├── rancher/
│   ├── harbor/
│   ├── argocd/
│   ├── monitoring/
│   └── logging/
└── templates/
    └── new-project/      ← copy để thêm app mới
```

## Workflow sau khi xong

```
Developer:  git push → GitHub Actions → Harbor → update GitOps repo → ArgoCD → K8s
DevOps:     Rancher (namespace, pod, log, RBAC) + Grafana (metric/log)
```

## Dev / Prod

Mặc định: **1 cluster, namespace tách env**

```
project-a-dev
project-a-prod
project-b-dev
project-b-prod
```

Convention domain: `{app}-{env}.{domain}` — ví dụ `api-dev.company.com`

## Chạy setup trên VPS (từng file, SSH rớt OK)

```bash
cp config/env.sh.example config/env.sh   # sửa IP, domain
tmux new -s k8s                          # bắt buộc trước khi chạy dài
./bootstrap/run.sh list
./bootstrap/run.sh next                  # chạy lần lượt 00 → 06
```

Chi tiết: [bootstrap/README.md](./bootstrap/README.md)

## Liên kết nhanh

- [TASKS.md](./TASKS.md) — checklist đầy đủ, tick khi xong
- [docs/STACK.md](./docs/STACK.md) — công nghệ đã chọn & lý do
- [bootstrap/README.md](./bootstrap/README.md) — script từng bước + tmux
- `config/env.sh.example` — biến cho script (copy → `config/env.sh`)
