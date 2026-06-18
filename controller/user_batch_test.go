package controller

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"gorm.io/gorm"
)

func TestNormalizeUserBatchIds(t *testing.T) {
	got := normalizeUserBatchIds([]int{0, 3, -1, 3, 2, 2, 1})
	want := []int{3, 2, 1}

	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %d, want %d", i, got[i], want[i])
		}
	}
}

func TestIsValidUserBatchAction(t *testing.T) {
	validActions := []string{
		userBatchActionEnable,
		userBatchActionDisable,
		userBatchActionHardDelete,
		userBatchActionPromote,
		userBatchActionDemote,
		userBatchActionAddQuota,
		userBatchActionSetGroup,
	}
	for _, action := range validActions {
		if !isValidUserBatchAction(action) {
			t.Fatalf("expected %q to be valid", action)
		}
	}

	if isValidUserBatchAction("reset_password") {
		t.Fatal("expected reset_password to be invalid for batch management")
	}
}

func TestValidateUserBatchRequest(t *testing.T) {
	validRequests := []UserBatchManageRequest{
		{Action: userBatchActionEnable},
		{Action: userBatchActionAddQuota, Mode: userBatchQuotaModeAdd, Value: 1},
		{Action: userBatchActionAddQuota, Mode: userBatchQuotaModeSub, Value: 1},
		{Action: userBatchActionAddQuota, Mode: userBatchQuotaModeSet, Value: 0},
		{Action: userBatchActionSetGroup, Group: "default"},
	}
	for _, req := range validRequests {
		if !validateUserBatchRequest(req) {
			t.Fatalf("expected %#v to be valid", req)
		}
	}

	invalidRequests := []UserBatchManageRequest{
		{Action: userBatchActionAddQuota, Mode: "multiply", Value: 1},
		{Action: userBatchActionAddQuota, Mode: userBatchQuotaModeAdd, Value: 0},
		{Action: userBatchActionAddQuota, Mode: userBatchQuotaModeSub, Value: -1},
		{Action: userBatchActionSetGroup, Group: ""},
		{Action: userBatchActionSetGroup, Group: "   "},
	}
	for _, req := range invalidRequests {
		if validateUserBatchRequest(req) {
			t.Fatalf("expected %#v to be invalid", req)
		}
	}
}

func TestValidateUserBatchTarget(t *testing.T) {
	commonUser := model.User{Id: 1, Role: common.RoleCommonUser}
	if reason := validateUserBatchTarget(common.RoleAdminUser, commonUser, userBatchActionDisable); reason != "" {
		t.Fatalf("unexpected reason: %s", reason)
	}

	adminUser := model.User{Id: 2, Role: common.RoleAdminUser}
	if reason := validateUserBatchTarget(common.RoleAdminUser, adminUser, userBatchActionDisable); reason == "" {
		t.Fatal("expected same-level admin management to be rejected")
	}

	deletedUser := model.User{
		Id:        3,
		Role:      common.RoleCommonUser,
		DeletedAt: gorm.DeletedAt{Time: time.Now(), Valid: true},
	}
	if reason := validateUserBatchTarget(common.RoleAdminUser, deletedUser, userBatchActionEnable); reason == "" {
		t.Fatal("expected enabling a deleted user to be rejected")
	}
	if reason := validateUserBatchTarget(common.RoleAdminUser, deletedUser, userBatchActionHardDelete); reason != "" {
		t.Fatalf("unexpected hard-delete reason: %s", reason)
	}

	rootUser := model.User{Id: 4, Role: common.RoleRootUser}
	if reason := validateUserBatchTarget(common.RoleRootUser, rootUser, userBatchActionDisable); reason == "" {
		t.Fatal("expected disabling root user to be rejected")
	}
	if reason := validateUserBatchTarget(common.RoleRootUser, rootUser, userBatchActionHardDelete); reason == "" {
		t.Fatal("expected hard deleting root user to be rejected")
	}
	if reason := validateUserBatchTarget(common.RoleRootUser, rootUser, userBatchActionDemote); reason == "" {
		t.Fatal("expected demoting root user to be rejected")
	}

	adminUserForDemotion := model.User{Id: 5, Role: common.RoleAdminUser}
	if reason := validateUserBatchTarget(common.RoleRootUser, adminUserForDemotion, userBatchActionDemote); reason != "" {
		t.Fatalf("unexpected demotion reason: %s", reason)
	}
	if reason := validateUserBatchTarget(common.RoleRootUser, adminUserForDemotion, userBatchActionPromote); reason == "" {
		t.Fatal("expected promoting existing admin to be rejected")
	}

	if reason := validateUserBatchTarget(common.RoleAdminUser, commonUser, userBatchActionPromote); reason == "" {
		t.Fatal("expected non-root promotion to be rejected")
	}
	if reason := validateUserBatchTarget(common.RoleRootUser, commonUser, userBatchActionPromote); reason != "" {
		t.Fatalf("unexpected promotion reason: %s", reason)
	}
}
