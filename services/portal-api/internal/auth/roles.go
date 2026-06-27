package auth

const (
	RoleAdmin    = "admin"
	RoleTechLead = "tech_lead"
	RoleDev      = "dev"
	RoleReadonly = "readonly"
)

func ValidRole(role string) bool {
	switch role {
	case RoleAdmin, RoleTechLead, RoleDev, RoleReadonly:
		return true
	default:
		return false
	}
}

// CanManageUsers — tạo/xóa user platform.
func CanManageUsers(role string) bool {
	return role == RoleAdmin
}

// CanDeleteProject — xóa project (cả cụm).
func CanDeleteProject(role string) bool {
	return role == RoleAdmin || role == RoleTechLead
}

// CanCreateProject — tạo project mới.
func CanCreateProject(role string) bool {
	return role == RoleAdmin || role == RoleTechLead || role == RoleDev
}

// CanJoinWorker — thêm node vào cluster.
func CanJoinWorker(role string) bool {
	return role == RoleAdmin || role == RoleTechLead
}

// CanViewAllProjects — xem mọi project (lead overview).
func CanViewAllProjects(role string) bool {
	return role == RoleAdmin || role == RoleTechLead
}

// CanWriteProd — thao tác namespace prod (sau này).
func CanWriteProd(role string) bool {
	return role == RoleAdmin || role == RoleTechLead
}

// CanViewInfra — menu Hạ tầng + cluster dashboard.
func CanViewInfra(role string) bool {
	return role == RoleAdmin || role == RoleTechLead
}

// CanWriteK8s — scale, restart, delete (readonly = xem only).
func CanWriteK8s(role string) bool {
	return role == RoleAdmin || role == RoleTechLead || role == RoleDev
}

// CanViewAudit — xem audit log toàn hệ.
func CanViewAudit(role string) bool {
	return role == RoleAdmin || role == RoleTechLead
}
