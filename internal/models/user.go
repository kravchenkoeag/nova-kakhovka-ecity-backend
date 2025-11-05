// internal/models/user.go
package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type UserRole string

// Константи для ролей
const (
	RoleUser       UserRole = "USER"
	RoleModerator  UserRole = "MODERATOR"
	RoleAdmin      UserRole = "ADMIN"
	RoleSuperAdmin UserRole = "SUPER_ADMIN"
)

// ========================================
// PERMISSION SYSTEM
// ========================================

// Permission представляє конкретний дозвіл в системі
// ✅ ВІДПОВІДАЄ Frontend: packages/types/src/models/roles.ts -> enum Permission
type Permission string

// Визначення всіх дозволів (має відповідати Frontend enum)
const (
	// User permissions
	PermissionViewProfile    Permission = "view:profile"
	PermissionEditOwnProfile Permission = "edit:own_profile"

	// Content permissions
	PermissionCreateAnnouncement    Permission = "create:announcement"
	PermissionEditOwnAnnouncement   Permission = "edit:own_announcement"
	PermissionDeleteOwnAnnouncement Permission = "delete:own_announcement"
	PermissionCreateEvent           Permission = "create:event"
	PermissionEditOwnEvent          Permission = "edit:own_event"
	PermissionDeleteOwnEvent        Permission = "delete:own_event"
	PermissionCreatePetition        Permission = "create:petition"
	PermissionSignPetition          Permission = "sign:petition"
	PermissionCreatePoll            Permission = "create:poll"
	PermissionVotePoll              Permission = "vote:poll"
	PermissionReportCityIssue       Permission = "report:city_issue"

	// Group permissions
	PermissionCreateGroup Permission = "create:group"
	PermissionJoinGroup   Permission = "join:group"
	PermissionSendMessage Permission = "send:message"

	// Moderator permissions
	PermissionModerateAnnouncement Permission = "moderate:announcement"
	PermissionModerateEvent        Permission = "moderate:event"
	PermissionModerateGroup        Permission = "moderate:group"
	PermissionModerateCityIssue    Permission = "moderate:city_issue"
	PermissionViewReports          Permission = "view:reports"

	// Admin permissions
	PermissionManageUsers       Permission = "manage:users"
	PermissionUsersManage       Permission = "users:manage" // ✅ ДОДАНО для відповідності Frontend
	PermissionBlockUser         Permission = "block:user"
	PermissionVerifyUser        Permission = "verify:user"
	PermissionPromoteModerator  Permission = "promote:moderator"
	PermissionViewAnalytics     Permission = "view:analytics"
	PermissionManageTransport   Permission = "manage:transport"
	PermissionSendNotifications Permission = "send:notifications"

	// Super Admin permissions
	PermissionManageAdmins         Permission = "manage:admins"
	PermissionManageSystemSettings Permission = "manage:system_settings"
	PermissionViewAuditLogs        Permission = "view:audit_logs"
	PermissionManageRoles          Permission = "manage:roles"
)

// ========================================
// USER RELATED TYPES
// ========================================

// Location представляє географічні координати
// ✅ ВІДПОВІДАЄ Frontend: UserLocation
type Location struct {
	Type        string    `bson:"type" json:"type"`                           // "Point"
	Coordinates []float64 `bson:"coordinates" json:"coordinates"`             // [longitude, latitude]
	Address     string    `bson:"address,omitempty" json:"address,omitempty"` // Адреса
	City        string    `bson:"city,omitempty" json:"city,omitempty"`       // Місто
}

// UserStatus представляє статус користувача
// ✅ ВІДПОВІДАЄ Frontend: UserStatus
type UserStatus struct {
	Message   string    `bson:"message" json:"message"`
	IsVisible bool      `bson:"is_visible" json:"is_visible"`
	UpdatedAt time.Time `bson:"updated_at" json:"updated_at"`
}

// BusinessInfo представляє інформацію про бізнес користувача
type BusinessInfo struct {
	Name        string   `bson:"name" json:"name"`
	Description string   `bson:"description" json:"description"`
	Services    []string `bson:"services" json:"services"`
	Category    string   `bson:"category" json:"category"`
}

// ========================================
// USER MODEL
// ========================================

// User представляє користувача системи
// ✅ ВІДПОВІДАЄ Frontend: packages/types/src/models/user.ts -> interface User
type User struct {
	ID           primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Email        string             `bson:"email" json:"email"`
	Phone        string             `bson:"phone,omitempty" json:"phone,omitempty"`
	PasswordHash string             `bson:"password_hash" json:"-"` // Не повертається в JSON
	FirstName    string             `bson:"first_name" json:"first_name"`
	LastName     string             `bson:"last_name" json:"last_name"`
	Avatar       string             `bson:"avatar,omitempty" json:"avatar,omitempty"` // ✅ ДОДАНО для відповідності Frontend

	// Додаткова інформація
	Profession        string   `bson:"profession,omitempty" json:"profession,omitempty"`
	RegisteredAddress string   `bson:"registered_address,omitempty" json:"registered_address,omitempty"`
	IsAddressVisible  bool     `bson:"is_address_visible" json:"is_address_visible"`
	Interests         []string `bson:"interests" json:"interests"`

	// Локація та статус
	CurrentLocation *Location  `bson:"current_location,omitempty" json:"location,omitempty"` // ✅ Відповідає Frontend: location
	Status          UserStatus `bson:"status" json:"status"`

	// Бізнес інформація
	BusinessInfo *BusinessInfo `bson:"business_info,omitempty" json:"business_info,omitempty"`

	// Групи користувача
	Groups []primitive.ObjectID `bson:"groups" json:"groups"`

	// ========================================
	// СИСТЕМА РОЛЕЙ ТА ПРАВ
	// ========================================
	// ✅ ВІДПОВІДАЄ Frontend: role: UserRole
	Role        string `bson:"role" json:"role"`                 // USER, MODERATOR, ADMIN, SUPER_ADMIN
	IsModerator bool   `bson:"is_moderator" json:"is_moderator"` // LEGACY: Для зворотної сумісності

	// Статус акаунту
	IsVerified bool `bson:"is_verified" json:"is_verified"`
	IsBlocked  bool `bson:"is_blocked" json:"is_blocked"`

	// ✅ ДОДАНО: Поля блокування (відповідають Frontend)
	BlockReason *string    `bson:"block_reason,omitempty" json:"block_reason,omitempty"` // Причина блокування
	BlockedAt   *time.Time `bson:"blocked_at,omitempty" json:"blocked_at,omitempty"`     // Час блокування

	// Часові мітки
	CreatedAt       time.Time  `bson:"created_at" json:"created_at"`
	UpdatedAt       time.Time  `bson:"updated_at" json:"updated_at"`
	LastLoginAt     *time.Time `bson:"last_login_at,omitempty" json:"last_login_at,omitempty"`
	EmailVerifiedAt *time.Time `bson:"email_verified_at,omitempty" json:"email_verified_at,omitempty"`
	PhoneVerifiedAt *time.Time `bson:"phone_verified_at,omitempty" json:"phone_verified_at,omitempty"`
}

// ========================================
// USER METHODS
// ========================================

// GetFullName повертає повне ім'я користувача
// ✅ ВІДПОВІДАЄ Frontend: UserHelpers.getFullName()
func (u *User) GetFullName() string {
	fullName := u.FirstName + " " + u.LastName
	if fullName == " " {
		return u.Email
	}
	return fullName
}

// GetInitials повертає ініціали користувача
// ✅ ВІДПОВІДАЄ Frontend: UserHelpers.getInitials()
func (u *User) GetInitials() string {
	firstInitial := ""
	lastInitial := ""

	if len(u.FirstName) > 0 {
		firstInitial = string(u.FirstName[0])
	}
	if len(u.LastName) > 0 {
		lastInitial = string(u.LastName[0])
	}

	if firstInitial == "" && lastInitial == "" {
		if len(u.Email) > 0 {
			return string(u.Email[0])
		}
		return "?"
	}

	return firstInitial + lastInitial
}

// GetRole повертає роль користувача як UserRole
func (u *User) GetRole() UserRole {
	// Якщо роль не встановлена (legacy users), визначаємо за is_moderator
	if u.Role == "" {
		if u.IsModerator {
			return RoleModerator
		}
		return RoleUser
	}
	return UserRole(u.Role)
}

// SetRole встановлює роль користувача
func (u *User) SetRole(role UserRole) {
	u.Role = string(role)

	// Оновлюємо legacy поле для зворотної сумісності
	u.IsModerator = role == RoleModerator ||
		role == RoleAdmin ||
		role == RoleSuperAdmin
}

// HasRole перевіряє чи користувач має певну роль
func (u *User) HasRole(role UserRole) bool {
	return u.GetRole() == role
}

// IsAtLeast перевіряє чи роль користувача не нижча за вказану
func (u *User) IsAtLeast(role UserRole) bool {
	return u.GetRole().IsHigherOrEqual(role)
}

// CanManage перевіряє чи користувач може керувати іншим користувачем
func (u *User) CanManage(target *User) bool {
	return u.GetRole().CanManageUser(target.GetRole())
}

// HasPermission перевіряє чи користувач має конкретний дозвіл
func (u *User) HasPermission(permission Permission) bool {
	return u.GetRole().HasPermission(permission)
}

// ========================================
// ROLE METHODS
// ========================================

// IsValid перевіряє чи роль валідна
func (r UserRole) IsValid() bool {
	switch r {
	case RoleUser, RoleModerator, RoleAdmin, RoleSuperAdmin:
		return true
	default:
		return false
	}
}

// GetRoleLevel повертає числовий рівень ролі для порівняння
// ✅ ВІДПОВІДАЄ Frontend: getRoleLevel()
func (r UserRole) GetRoleLevel() int {
	switch r {
	case RoleUser:
		return 0
	case RoleModerator:
		return 1
	case RoleAdmin:
		return 2
	case RoleSuperAdmin:
		return 3
	default:
		return -1
	}
}

// IsHigherOrEqual перевіряє чи роль вища або рівна за іншу
// ✅ ВІДПОВІДАЄ Frontend: isRoleHigherOrEqual()
func (r UserRole) IsHigherOrEqual(other UserRole) bool {
	return r.GetRoleLevel() >= other.GetRoleLevel()
}

// CanManageUser перевіряє чи може роль керувати іншою роллю
// ✅ ВІДПОВІДАЄ Frontend: canElevateTo()
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

// CanElevateTo перевіряє чи може роль підвищити іншого користувача до певної ролі
// ✅ ВІДПОВІДАЄ Frontend: canElevateTo()
func (r UserRole) CanElevateTo(targetRole UserRole) bool {
	// Тільки адміни та супер-адміни можуть підвищувати ролі
	if r != RoleAdmin && r != RoleSuperAdmin {
		return false
	}

	// Супер-адмін може підвищити до будь-якої ролі
	if r == RoleSuperAdmin {
		return true
	}

	// Звичайний адмін може підвищити тільки до модератора
	return targetRole == RoleModerator
}

// String повертає строкове представлення ролі
func (r UserRole) String() string {
	return string(r)
}

// HasPermission перевіряє чи має роль конкретний дозвіл
func (r UserRole) HasPermission(permission Permission) bool {
	rolePermissions := GetRolePermissions(r)
	for _, p := range rolePermissions {
		if p == permission {
			return true
		}
	}
	return false
}

// ========================================
// PERMISSION SYSTEM FUNCTIONS
// ========================================

// GetRolePermissions повертає всі дозволи для певної ролі
// ✅ ВІДПОВІДАЄ Frontend: RolePermissions mapping
func GetRolePermissions(role UserRole) []Permission {
	// Базові права користувача
	basePermissions := []Permission{
		PermissionViewProfile,
		PermissionEditOwnProfile,
		PermissionCreateAnnouncement,
		PermissionEditOwnAnnouncement,
		PermissionDeleteOwnAnnouncement,
		PermissionCreateEvent,
		PermissionEditOwnEvent,
		PermissionDeleteOwnEvent,
		PermissionCreatePetition,
		PermissionSignPetition,
		PermissionCreatePoll,
		PermissionVotePoll,
		PermissionReportCityIssue,
		PermissionCreateGroup,
		PermissionJoinGroup,
		PermissionSendMessage,
	}

	// Права модератора
	moderatorPermissions := []Permission{
		PermissionModerateAnnouncement,
		PermissionModerateEvent,
		PermissionModerateGroup,
		PermissionModerateCityIssue,
		PermissionViewReports,
	}

	// Права адміністратора
	adminPermissions := []Permission{
		PermissionManageUsers,
		PermissionUsersManage, // ✅ ДОДАНО для відповідності Frontend
		PermissionBlockUser,
		PermissionVerifyUser,
		PermissionPromoteModerator,
		PermissionViewAnalytics,
		PermissionManageTransport,
		PermissionSendNotifications,
	}

	// Права супер-адміністратора
	superAdminPermissions := []Permission{
		PermissionManageAdmins,
		PermissionManageSystemSettings,
		PermissionViewAuditLogs,
		PermissionManageRoles,
	}

	// Повертаємо права в залежності від ролі (з успадкуванням)
	switch role {
	case RoleUser:
		return basePermissions

	case RoleModerator:
		return append(basePermissions, moderatorPermissions...)

	case RoleAdmin:
		allPerms := append(basePermissions, moderatorPermissions...)
		return append(allPerms, adminPermissions...)

	case RoleSuperAdmin:
		allPerms := append(basePermissions, moderatorPermissions...)
		allPerms = append(allPerms, adminPermissions...)
		return append(allPerms, superAdminPermissions...)

	default:
		return []Permission{}
	}
}

// HasAnyPermission перевіряє чи має роль хоча б одне з дозволів
// ✅ ВІДПОВІДАЄ Frontend: hasAnyPermission()
func HasAnyPermission(role UserRole, permissions []Permission) bool {
	for _, permission := range permissions {
		if role.HasPermission(permission) {
			return true
		}
	}
	return false
}

// HasAllPermissions перевіряє чи має роль всі дозволи
// ✅ ВІДПОВІДАЄ Frontend: hasAllPermissions()
func HasAllPermissions(role UserRole, permissions []Permission) bool {
	for _, permission := range permissions {
		if !role.HasPermission(permission) {
			return false
		}
	}
	return true
}

// ========================================
// UTILITY FUNCTIONS
// ========================================

// AllRoles повертає список всіх доступних ролей
func AllRoles() []UserRole {
	return []UserRole{
		RoleUser,
		RoleModerator,
		RoleAdmin,
		RoleSuperAdmin,
	}
}

// RoleFromString конвертує string в UserRole
func RoleFromString(role string) (UserRole, bool) {
	r := UserRole(role)
	if r.IsValid() {
		return r, true
	}
	return "", false
}

// GetRoleDisplayName повертає локалізовану назву ролі
func GetRoleDisplayName(role UserRole) string {
	switch role {
	case RoleUser:
		return "Користувач"
	case RoleModerator:
		return "Модератор"
	case RoleAdmin:
		return "Адміністратор"
	case RoleSuperAdmin:
		return "Супер-адміністратор"
	default:
		return "Невідома роль"
	}
}

// PermissionFromString конвертує string в Permission
func PermissionFromString(permission string) (Permission, bool) {
	p := Permission(permission)
	// Перевіряємо чи permission існує в будь-якій ролі
	for _, role := range AllRoles() {
		permissions := GetRolePermissions(role)
		for _, validPermission := range permissions {
			if validPermission == p {
				return p, true
			}
		}
	}
	return "", false
}
