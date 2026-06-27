# Cài Platform từ source

Giống các dự án lớn (Rancher, Harbor…): **clone → config → một script cài** — không cần AI “bấm hộ” từng lệnh helm/rsync.

## Yêu cầu

- VPS Ubuntu 22.04+ (hoặc tương đương), ≥ 4GB RAM
- Domain hoặc `sslip.io` trỏ về IP
- Git, quyền `sudo`

## Cài lần đầu

```bash
git clone https://github.com/Thien2026/k8s.git
cd k8s
cp config/env.sh.example config/env.sh
# Sửa: DOMAIN, NODE_PUBLIC_IP, POSTGRES_PASSWORD, LETSENCRYPT_EMAIL…

chmod +x bootstrap/install.sh
sudo ./bootstrap/install.sh
```

Script chạy lần lượt **00 → 08** (RKE2, Ingress, cert-manager, Postgres, Console).  
SSH rớt → `tmux attach` rồi chạy lại `./bootstrap/install.sh` (bước đã xong được bỏ qua).

## Addon (sau Console)

```bash
./bootstrap/addons/run.sh rancher
./bootstrap/addons/run.sh harbor
```

Bật plugin trong Console → `#/addons` nếu cần.

## Cập nhật sau khi đổi code

Trên **cùng VPS** (đã `git pull`):

```bash
cd ~/k8s
git pull
FORCE_BUILD=1 ./bootstrap/run.sh 08 --force
```

Chỉ đổi portal-api/web — không cài lại cluster.

## Không nên

| Cách | Vì sao |
|------|--------|
| `rsync` tay từ laptop | Dễ lệch file, quên bước |
| `kubectl apply` lẻ tẻ | Không có state `bootstrap/state/*.done` |
| Sửa image trên node | Mất sau rebuild |

## Handover khách

Khách nhận: **repo + `config/env.sh` mẫu + `bootstrap/install.sh`** — chạy trên VPS của họ là ra cùng stack.
