package platformcontract

// Đường dẫn contract trong repo (dev khai báo key, không chứa secret).
const (
	BuildContractPath    = ".platform/build.yaml"
	RuntimeContractPath  = ".platform/runtime.yaml"
	ContractVersion      = 1
)

// Platform tự inject — không yêu cầu trên Console.
var PlatformBuildArgs = map[string]bool{
	"GIT_SHA": true,
	"GIT_REF": true,
}

// VarSpec một biến trong contract.
type VarSpec struct {
	Required    bool   `yaml:"required"`
	Description string `yaml:"description"`
}

// File contract YAML trong repo.
type File struct {
	Version int                `yaml:"version"`
	Vars    map[string]VarSpec `yaml:"vars"`
}

// ConsoleVar giá trị đã khai báo trên Platform Console.
type ConsoleVar struct {
	Key      string
	Value    string
	IsSecret bool
}

// Issue một vấn đề cấu hình.
type Issue struct {
	Code        string `json:"code"`
	Key         string `json:"key,omitempty"`
	Message     string `json:"message"`
	Severity    string `json:"severity"` // error | warning
	Description string `json:"description,omitempty"`
}

// CheckResult kết quả kiểm tra sẵn sàng cấu hình.
type CheckResult struct {
	Ready           bool     `json:"ready"`
	Scope           string   `json:"scope"`
	ContractFound   bool     `json:"contract_found"`
	ContractPath    string   `json:"contract_path,omitempty"`
	ContractVersion int      `json:"contract_version,omitempty"`
	Issues          []Issue  `json:"issues"`
	MissingRequired []string `json:"missing_required,omitempty"`
	EmptyRequired   []string `json:"empty_required,omitempty"`
	Warnings        []string `json:"warnings,omitempty"`
}
