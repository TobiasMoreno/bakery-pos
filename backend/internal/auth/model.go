package auth

import (
	"slices"
	"strings"

	"github.com/google/uuid"
)

type Role string

const (
	RoleSeller Role = "seller"
	RoleOwner  Role = "owner"
)

type Permission string

const (
	PermissionSalesWrite      Permission = "sales:write"
	PermissionCashOperate     Permission = "cash:operate"
	PermissionReturnsWrite    Permission = "returns:write"
	PermissionCatalogRead     Permission = "catalog:read"
	PermissionInventoryRead   Permission = "inventory:read"
	PermissionProductionWrite Permission = "production:write"
	PermissionInventoryAdjust Permission = "inventory:adjust"
	PermissionPricingWrite    Permission = "pricing:write"
	PermissionReportsGlobal   Permission = "reports:global"
	PermissionUsersManage     Permission = "users:manage"
	PermissionRecoveryForce   Permission = "recovery:force"
)

var sellerPermissions = []Permission{
	PermissionSalesWrite,
	PermissionCashOperate,
	PermissionReturnsWrite,
	PermissionCatalogRead,
	PermissionInventoryRead,
}

var ownerPermissions = []Permission{
	PermissionSalesWrite,
	PermissionCashOperate,
	PermissionReturnsWrite,
	PermissionCatalogRead,
	PermissionInventoryRead,
	PermissionProductionWrite,
	PermissionInventoryAdjust,
	PermissionPricingWrite,
	PermissionReportsGlobal,
	PermissionUsersManage,
	PermissionRecoveryForce,
}

type User struct {
	ID              uuid.UUID `json:"id"`
	Name            string    `json:"name"`
	Username        string    `json:"username"`
	PasswordHash    string    `json:"-"`
	Role            Role      `json:"role"`
	ProductionWrite bool      `json:"-"`
	InventoryAdjust bool      `json:"-"`
	Active          bool      `json:"-"`
}

type PublicUser struct {
	ID          uuid.UUID    `json:"id"`
	Name        string       `json:"name"`
	Username    string       `json:"username"`
	Role        Role         `json:"role"`
	Permissions []Permission `json:"permissions"`
}

func NormalizeUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

func (u User) Permissions() []Permission {
	var permissions []Permission
	if u.Role == RoleOwner {
		permissions = slices.Clone(ownerPermissions)
	} else {
		permissions = slices.Clone(sellerPermissions)
	}
	if u.ProductionWrite && !slices.Contains(permissions, PermissionProductionWrite) {
		permissions = append(permissions, PermissionProductionWrite)
	}
	if u.InventoryAdjust && !slices.Contains(permissions, PermissionInventoryAdjust) {
		permissions = append(permissions, PermissionInventoryAdjust)
	}
	return permissions
}

func (u User) Has(permission Permission) bool {
	return slices.Contains(u.Permissions(), permission)
}

func (u User) Public() PublicUser {
	return PublicUser{
		ID:          u.ID,
		Name:        u.Name,
		Username:    u.Username,
		Role:        u.Role,
		Permissions: u.Permissions(),
	}
}

func (u PublicUser) Has(permission Permission) bool {
	return slices.Contains(u.Permissions, permission)
}
