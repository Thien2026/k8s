# GitOps (Phase 4) — admin

Dev **không bắt buộc** đọc doc này. Deploy Phase 1 / 2 qua Console + repo code là đủ.

---

## Hai repo khác nhau

| | Repo code | Repo GitOps |
|---|-----------|-------------|
| Ví dụ | `huuthienit97/test-k8s` | `Thien2026/gitopt` |
| Chứa | source app | manifest K8s + image tag |
| Kết nối Console | ✅ Deploy / Git | ❌ Không |
| Ai setup | Dev / khách | Admin platform |

---

## Không có GitOps

```text
Push repo code → CI build → hook Platform → Rancher deploy
```

Smoke test pilot vẫn pass — không cần repo `gitopt`.

---

## Bật GitOps (một lần / platform)

1. Tạo repo GitHub (vd. `https://github.com/Thien2026/gitopt`)
2. Copy scaffold từ `templates/gitops/` → `apps/<slug>/overlays/dev|prod/`
3. VPS `config/env.sh`:
   ```bash
   GITOPS_REPO_URL="https://github.com/Thien2026/gitopt"
   GITOPS_REPO_BRANCH="main"
   GITOPS_BASE_PATH="apps"
   GITOPS_PUSH_TOKEN="ghp_..."   # PAT ghi repo GitOps
   ```
4. Redeploy portal: `./bootstrap/core/steps/08-portal.sh`
5. Argo CD: connect repo GitOps (read)
6. Mỗi project: sync workflow trên Console (inject `PLATFORM_GITOPS_TOKEN`)

---

## Thêm project mới

Không tạo repo GitOps mới — chỉ thêm:

```text
apps/<slug>/overlays/dev/kustomization.yaml
apps/<slug>/overlays/prod/kustomization.yaml
```

Dev vẫn làm 4 bước trên Console với **repo code**.

---

## Flow khi bật

```text
Push repo code
  → CI build image
  → (tuỳ chọn) hook Platform
  → CI cập nhật newTag trong repo GitOps
  → Argo CD sync
```

---

## PAT GitHub

Fine-grained token trên repo GitOps:

- **Contents**: Read and write

Hoặc classic token scope `repo` (private).

---

## UI

Console → Deploy / Git → `?` → tab **GitOps**.
