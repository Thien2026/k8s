# Bootstrap — chạy từng file, SSH rớt không sao

## Chuẩn bị 1 lần

```bash
# Trên máy local: copy config
cp config/env.sh.example config/env.sh
# Sửa DOMAIN, NODE_PUBLIC_IP, LETSENCRYPT_EMAIL

# Đẩy repo lên VPS (git clone hoặc rsync)
rsync -avz --exclude kubeconfig ./ user@VPS_IP:~/k8s/

# SSH vào VPS — LUÔN dùng tmux
ssh user@VPS_IP
tmux new -s k8s
cd ~/k8s
chmod +x bootstrap/run.sh bootstrap/steps/*.sh
```

## Chạy từng bước

```bash
./bootstrap/run.sh list      # xem bước nào xong [x]
./bootstrap/run.sh next      # chạy bước tiếp theo
./bootstrap/run.sh 04        # chạy riêng bước 04
./bootstrap/run.sh 04 --force  # chạy lại dù đã xong
```

## Thứ tự file

| File | Việc làm | Chạy ở đâu |
|------|----------|------------|
| `00-preflight.sh` | Kiểm tra OS, tắt swap | VPS (root) |
| `01-rke2-server.sh` | Cài RKE2 | VPS (root) |
| `02-kubeconfig.sh` | Lấy kubeconfig | VPS |
| `03-helm.sh` | Cài Helm + repo | VPS |
| `04-ingress-nginx.sh` | Ingress | VPS (kubectl) |
| `05-cert-manager.sh` | TLS tự động | VPS |
| `06-hello-world.sh` | App test HTTPS | VPS |

Bước 07+ (Rancher, Harbor, ArgoCD...) thêm dần vào `bootstrap/steps/` theo TASKS.md.

## SSH disconnect — làm sao?

1. **tmux** — rớt SSH session vẫn chạy:
   ```bash
   tmux attach -t k8s    # vào lại
   ```
2. **State file** — mỗi bước xong ghi `bootstrap/state/XX-*.done`, chạy `next` tiếp tục đúng chỗ.
3. **Log** — mỗi lần chạy lưu `bootstrap/logs/XX-*.log`, lỗi xem ở đây.
4. **Chạy lại an toàn** — hầu hết script idempotent (đã cài thì skip/upgrade).

## Chạy từ máy Mac (không SSH lâu)

Chỉ bước 04–06 nếu đã có `kubeconfig/rke2.yaml` copy về Mac:

```bash
export KUBECONFIG=./kubeconfig/rke2.yaml
./bootstrap/run.sh 04
```

Bước 00–01 **bắt buộc chạy trên VPS** (cài RKE2).

## Thêm bước mới

1. Tạo `bootstrap/steps/07-rancher.sh`
2. `source ../lib/common.sh` + logic Helm
3. `mark_step_done "$0"` cuối file
4. `./bootstrap/run.sh next`
