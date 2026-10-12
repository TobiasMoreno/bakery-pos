package auth

import "testing"

func TestSellerOptionalPermissions(t *testing.T) {
	user := User{Role: RoleSeller, ProductionWrite: true}
	if !user.Has(PermissionSalesWrite) {
		t.Fatal("seller should sell")
	}
	if !user.Has(PermissionProductionWrite) {
		t.Fatal("seller with capability should register production")
	}
	if user.Has(PermissionPricingWrite) {
		t.Fatal("seller should not edit prices")
	}
}

func TestOwnerHasAdministrativePermissions(t *testing.T) {
	user := User{Role: RoleOwner}
	for _, permission := range []Permission{PermissionPricingWrite, PermissionUsersManage, PermissionRecoveryForce} {
		if !user.Has(permission) {
			t.Fatalf("owner should have %s", permission)
		}
	}
}

func TestSellerReadsInventoryWithoutManualAdjustmentPermission(t *testing.T) {
	user := User{Role: RoleSeller}
	if !user.Has(PermissionInventoryRead) {
		t.Fatal("seller should read inventory")
	}
	if user.Has(PermissionInventoryAdjust) {
		t.Fatal("seller without optional capability should not adjust inventory")
	}
}
