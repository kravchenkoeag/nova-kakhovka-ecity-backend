// internal/models/roles.go

package models

// UserRole представляє роль користувача в системі
type UserRole string

// Константи для ролей
const (
	RoleUser       UserRole = "USER"
	RoleModerator  UserRole = "MODERATOR"
	RoleAdmin      UserRole = "ADMIN"
	RoleSuperAdmin UserRole = "SUPER_ADMIN"
)

// IsValid перевіряє чи роль валідна
func (r UserRole) IsValid() bool {
	switch r {
	case RoleUser, RoleModerator, RoleAdmin, RoleSuperAdmin:
		return true
	}
	return false
}

// IsHigherOrEqual перевіряє чи поточна роль вища або рівна цільовій
func (r UserRole) IsHigherOrEqual(target UserRole) bool {
	roleHierarchy := map[UserRole]int{
		RoleUser:       0,
		RoleModerator:  1,
		RoleAdmin:      2,
		RoleSuperAdmin: 3,
	}

	currentLevel, exists1 := roleHierarchy[r]
	targetLevel, exists2 := roleHierarchy[target]

	if !exists1 || !exists2 {
		return false
	}

	return currentLevel >= targetLevel
}

// CanManageUser перевіряє чи користувач може керувати іншим користувачем
func (r UserRole) CanManageUser(targetRole UserRole) bool {
	// Super Admin може керувати всіма
	if r == RoleSuperAdmin {
		return true
	}

	// Admin може керувати User та Moderator
	if r == RoleAdmin {
		return targetRole == RoleUser || targetRole == RoleModerator
	}

	// Moderator та User не можуть керувати іншими
	return false
}

// String повертає строкове представлення ролі
func (r UserRole) String() string {
	return string(r)
}

// AllRoles повертає список всіх доступних ролей
func AllRoles() []UserRole {
	return []UserRole{
		RoleUser,
		RoleModerator,
		RoleAdmin,
		RoleSuperAdmin,
	}
}

// FromString конвертує string в UserRole
func FromString(role string) (UserRole, bool) {
	r := UserRole(role)
	if r.IsValid() {
		return r, true
	}
	return "", false
}
