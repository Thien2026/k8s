package handler

// Deploy activity / deployment rows are split across:
//   deployment_model.go    — row types, stages
//   deployment_pick.go     — pick current / dedupe / seal
//   deployment_store.go    — upsert / mark phases
//   deployment_runtime.go  — runtime enrich / logs
//   deployment_list.go     — listProjectDeployments + GH run attach
//   deployment_github.go   — merge GH rows, build live, Harbor scan
//   deployment_activity.go — GetProjectDeployActivity
//   deployment_hooks.go    — DeployEventHook
//   deployment_reconcile.go — status reconcile helpers
