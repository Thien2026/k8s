package handler

// Deploy HTTP handlers split across:
//   deploy_apply.go    — plan / apply / finish runtime
//   deploy_promote.go  — promote + readiness checklist
//   deploy_rollback.go — rollback + auto-deploy patch
