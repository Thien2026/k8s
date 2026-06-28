package platformcontract

import (
	"fmt"
	"strings"
)

func isEmptyValue(v string) bool {
	return strings.TrimSpace(v) == ""
}

func consoleMap(vars []ConsoleVar) map[string]ConsoleVar {
	out := map[string]ConsoleVar{}
	for _, v := range vars {
		key := strings.TrimSpace(v.Key)
		if key == "" {
			continue
		}
		out[key] = v
	}
	return out
}

// CheckBuild kiểm tra cấu hình build (Console + contract tùy chọn + Dockerfile tùy chọn).
func CheckBuild(contract *File, console []ConsoleVar, dockerfileArgs []string) CheckResult {
	return check("build", contract, BuildContractPath, console, dockerfileArgs, PlatformBuildArgs)
}

// CheckRuntime kiểm tra cấu hình runtime (Console + contract tùy chọn).
func CheckRuntime(contract *File, console []ConsoleVar) CheckResult {
	return check("runtime", contract, RuntimeContractPath, console, nil, nil)
}

func check(scope string, contract *File, contractPath string, console []ConsoleVar, dockerfileArgs []string, skipKeys map[string]bool) CheckResult {
	res := CheckResult{Ready: true, Scope: scope, Issues: []Issue{}}
	cm := consoleMap(console)

	if contract != nil {
		res.ContractFound = true
		res.ContractPath = contractPath
		res.ContractVersion = contract.Version

		for key, spec := range contract.Vars {
			if skipKeys != nil && skipKeys[key] {
				continue
			}
			cv, ok := cm[key]
			if !spec.Required {
				continue
			}
			if !ok {
				res.Ready = false
				res.MissingRequired = append(res.MissingRequired, key)
				res.Issues = append(res.Issues, Issue{
					Code:        "missing_required",
					Key:         key,
					Severity:    "error",
					Description: spec.Description,
					Message:     fmtMsg(scope, key, "chưa khai báo trên Console"),
				})
				continue
			}
			if isEmptyValue(cv.Value) {
				res.Ready = false
				res.EmptyRequired = append(res.EmptyRequired, key)
				res.Issues = append(res.Issues, Issue{
					Code:        "empty_required",
					Key:         key,
					Severity:    "error",
					Description: spec.Description,
					Message:     fmtMsg(scope, key, "đã khai báo nhưng giá trị rỗng — điền giá trị trên Console"),
				})
			}
		}
	}

	// Mọi biến build/runtime trên Console không được rỗng (phủ định rỗng).
	for key, cv := range cm {
		if skipKeys != nil && skipKeys[key] {
			continue
		}
		if !isEmptyValue(cv.Value) {
			continue
		}
		if contract != nil {
			if spec, ok := contract.Vars[key]; ok && !spec.Required {
				res.Warnings = append(res.Warnings, key+": biến tùy chọn đang rỗng — xóa hoặc điền giá trị")
				res.Issues = append(res.Issues, Issue{
					Code:     "empty_optional",
					Key:      key,
					Severity: "warning",
					Message:  fmtMsg(scope, key, "tùy chọn nhưng đang rỗng"),
				})
				continue
			}
			if _, inContract := contract.Vars[key]; inContract {
				continue // đã báo empty_required
			}
		}
		res.Ready = false
		res.EmptyRequired = append(res.EmptyRequired, key)
		res.Issues = append(res.Issues, Issue{
			Code:     "empty_value",
			Key:      key,
			Severity: "error",
			Message:  fmtMsg(scope, key, "giá trị rỗng không được chấp nhận"),
		})
	}

	if contract != nil && len(dockerfileArgs) > 0 {
		drift := driftDockerfile(contract, dockerfileArgs, skipKeys)
		res.Warnings = append(res.Warnings, drift.Warnings...)
		for _, iss := range drift.Issues {
			res.Issues = append(res.Issues, iss)
			if iss.Severity == "error" {
				res.Ready = false
			}
		}
	}

	if res.MissingRequired == nil {
		res.MissingRequired = []string{}
	}
	if res.EmptyRequired == nil {
		res.EmptyRequired = []string{}
	}
	if res.Warnings == nil {
		res.Warnings = []string{}
	}
	return res
}

type driftResult struct {
	Warnings []string
	Issues   []Issue
}

func driftDockerfile(contract *File, dockerfileArgs []string, skipKeys map[string]bool) driftResult {
	argSet := map[string]bool{}
	for _, a := range dockerfileArgs {
		a = strings.TrimSpace(a)
		if a == "" || (skipKeys != nil && skipKeys[a]) {
			continue
		}
		argSet[a] = true
	}
	out := driftResult{}
	for key, spec := range contract.Vars {
		if skipKeys != nil && skipKeys[key] {
			continue
		}
		if !argSet[key] {
			msg := "contract yêu cầu " + key + " nhưng Dockerfile không có ARG " + key
			sev := "warning"
			if spec.Required {
				sev = "error"
			}
			out.Warnings = append(out.Warnings, msg)
			out.Issues = append(out.Issues, Issue{
				Code:     "dockerfile_missing_arg",
				Key:      key,
				Severity: sev,
				Message:  msg,
			})
		}
	}
	for key := range argSet {
		if skipKeys != nil && skipKeys[key] {
			continue
		}
		if _, ok := contract.Vars[key]; !ok {
			msg := "Dockerfile có ARG " + key + " nhưng chưa khai báo trong " + BuildContractPath
			out.Warnings = append(out.Warnings, msg)
			out.Issues = append(out.Issues, Issue{
				Code:     "contract_missing_key",
				Key:      key,
				Severity: "warning",
				Message:  msg,
			})
		}
	}
	return out
}

func fmtMsg(scope, key, detail string) string {
	return key + ": " + detail
}

// ValidateSaveValue kiểm tra trước khi lưu biến trên Console.
func ValidateSaveValue(scope, env, key, value string, contract *File) error {
	key = strings.TrimSpace(key)
	if scope == "build" && PlatformBuildArgs[key] {
		return fmt.Errorf("%s do platform tự inject — không thêm thủ công", key)
	}
	if err := ValidateProdEnvValue(env, key, value); err != nil {
		return err
	}
	if isEmptyValue(value) {
		if contract != nil {
			if spec, ok := contract.Vars[key]; ok && !spec.Required {
				return fmt.Errorf("biến tùy chọn %s: không lưu giá trị rỗng — bỏ qua hoặc điền giá trị", key)
			}
		}
		if scope == "build" {
			return fmt.Errorf("biến build %s không được rỗng — điền giá trị trên Console", key)
		}
		return fmt.Errorf("biến %s không được rỗng", key)
	}
	return nil
}

// FormatCheckError gom lỗi blocking để trả API.
func FormatCheckError(res CheckResult) string {
	if res.Ready {
		return ""
	}
	var parts []string
	for _, iss := range res.Issues {
		if iss.Severity != "error" {
			continue
		}
		parts = append(parts, iss.Message)
	}
	return strings.Join(parts, "; ")
}
