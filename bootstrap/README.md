# Bootstrap — Core vs Addon

## Hai lớp

| Lớp | Thư mục | Runner | Mục đích |
|-----|---------|--------|----------|
| **Core** | `bootstrap/core/steps/` | `./bootstrap/run.sh` | RKE2, Ingress, Console (00–08) |
| **Addon** | `bootstrap/addons/` | `./bootstrap/addons/run.sh` | Rancher, Harbor, Argo CD (tùy chọn) |

State file dùng chung: `bootstrap/state/` (giữ tương thích VPS cũ).

## Core — chạy đầu tiên

```bash
# Cách nhanh (lần đầu trên VPS)
sudo ./bootstrap/install.sh

# Hoặc từng bước
tmux new -s k8s
cd ~/k8s
chmod +x bootstrap/run.sh bootstrap/core/steps/*.sh bootstrap/addons/*.sh

./bootstrap/run.sh list
./bootstrap/run.sh next    # lặp đến hết 08-portal
```

| Bước | Việc |
|------|------|
| 00–06 | RKE2, Helm, Ingress, cert-manager, hello-world |
| 07 | PostgreSQL (metadata Console) |
| 08 | Build + deploy portal-api & portal-web |

**Deploy app khách (micro thường api+web):** [docs/MICRO_DEPLOY.md](../docs/MICRO_DEPLOY.md)

## Addon — sau khi Console chạy

Bật addon trong Console → `#/addons`, rồi trên VPS:

```bash
./bootstrap/addons/run.sh list
./bootstrap/addons/run.sh rancher
./bootstrap/addons/run.sh harbor
./bootstrap/addons/run.sh argocd
```

Hoặc:

```bash
bash bootstrap/addons/install-rancher.sh
bash bootstrap/addons/install-harbor.sh
```

## Backward compatible

Script cũ `bootstrap/steps/09-rancher.sh` và `bootstrap/steps/11-argocd.sh` vẫn chạy được (wrapper → addon).

State cũ `09-rancher.done` / `10-harbor.done` / `11-argocd.done` vẫn được nhận.

## SSH disconnect

```bash
tmux attach -t k8s
./bootstrap/run.sh next          # core
./bootstrap/addons/run.sh rancher  # addon
```

Log: `bootstrap/logs/`
