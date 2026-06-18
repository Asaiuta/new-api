package controller

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

const (
	userBatchActionEnable     = "enable"
	userBatchActionDisable    = "disable"
	userBatchActionHardDelete = "hard_delete"
	userBatchActionPromote    = "promote"
	userBatchActionDemote     = "demote"
	userBatchActionAddQuota   = "add_quota"
	userBatchActionSetGroup   = "set_group"
	userBatchQuotaModeAdd     = "add"
	userBatchQuotaModeSub     = "subtract"
	userBatchQuotaModeSet     = "override"
	userBatchMaxSize          = 100
)

type UserBatchManageRequest struct {
	Ids    []int  `json:"ids"`
	Action string `json:"action"`
	Mode   string `json:"mode,omitempty"`
	Value  int    `json:"value,omitempty"`
	Group  string `json:"group,omitempty"`
}

type UserBatchManageFailure struct {
	Id       int    `json:"id"`
	Username string `json:"username,omitempty"`
	Reason   string `json:"reason"`
}

type UserBatchManageResult struct {
	Action    string                   `json:"action"`
	Succeeded int                      `json:"succeeded"`
	Failed    int                      `json:"failed"`
	Failures  []UserBatchManageFailure `json:"failures,omitempty"`
}

func normalizeUserBatchIds(ids []int) []int {
	seen := make(map[int]struct{}, len(ids))
	normalized := make([]int, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		normalized = append(normalized, id)
	}
	return normalized
}

func isValidUserBatchAction(action string) bool {
	switch action {
	case userBatchActionEnable,
		userBatchActionDisable,
		userBatchActionHardDelete,
		userBatchActionPromote,
		userBatchActionDemote,
		userBatchActionAddQuota,
		userBatchActionSetGroup:
		return true
	default:
		return false
	}
}

func isValidUserBatchQuotaMode(mode string) bool {
	switch mode {
	case userBatchQuotaModeAdd, userBatchQuotaModeSub, userBatchQuotaModeSet:
		return true
	default:
		return false
	}
}

func validateUserBatchRequest(req UserBatchManageRequest) bool {
	switch strings.TrimSpace(req.Action) {
	case userBatchActionAddQuota:
		mode := strings.TrimSpace(req.Mode)
		if !isValidUserBatchQuotaMode(mode) {
			return false
		}
		return mode == userBatchQuotaModeSet || req.Value > 0
	case userBatchActionSetGroup:
		return strings.TrimSpace(req.Group) != ""
	default:
		return true
	}
}

func userBatchActionAuditLabel(action string, mode string) string {
	switch action {
	case userBatchActionEnable:
		return "enable"
	case userBatchActionDisable:
		return "disable"
	case userBatchActionHardDelete:
		return "hard_delete"
	case userBatchActionPromote:
		return "promote"
	case userBatchActionDemote:
		return "demote"
	case userBatchActionAddQuota:
		if mode != "" {
			return "quota_" + mode
		}
		return "quota"
	case userBatchActionSetGroup:
		return "set_group"
	default:
		return action
	}
}

func validateUserBatchTarget(myRole int, user model.User, action string) string {
	if !canManageTargetRole(myRole, user.Role) {
		return "no permission to manage this user"
	}
	if action != userBatchActionHardDelete && user.DeletedAt.Valid {
		return "deleted users can only be hard deleted"
	}
	switch action {
	case userBatchActionDisable:
		if user.Role == common.RoleRootUser {
			return "root user cannot be disabled"
		}
	case userBatchActionHardDelete:
		if user.Role == common.RoleRootUser {
			return "root user cannot be deleted"
		}
	case userBatchActionPromote:
		if myRole != common.RoleRootUser {
			return "only root users can promote users"
		}
		if user.Role >= common.RoleAdminUser {
			return "user is already an admin"
		}
	case userBatchActionDemote:
		if user.Role == common.RoleRootUser {
			return "root user cannot be demoted"
		}
		if user.Role == common.RoleCommonUser {
			return "user is already a regular user"
		}
	}
	return ""
}

func usersByIdUnscoped(ids []int) (map[int]model.User, error) {
	users := make([]model.User, 0, len(ids))
	if err := model.DB.Unscoped().Omit("password").Where("id IN ?", ids).Find(&users).Error; err != nil {
		return nil, err
	}

	usersById := make(map[int]model.User, len(users))
	for _, user := range users {
		usersById[user.Id] = user
	}
	return usersById, nil
}

func invalidateManagedUserCaches(userId int, includeTokens bool) {
	if err := model.InvalidateUserCache(userId); err != nil {
		common.SysLog(fmt.Sprintf("failed to invalidate user cache for user %d: %s", userId, err.Error()))
	}
	if includeTokens {
		if err := model.InvalidateUserTokensCache(userId); err != nil {
			common.SysLog(fmt.Sprintf("failed to invalidate tokens cache for user %d: %s", userId, err.Error()))
		}
	}
}

func applyUserBatchAction(user model.User, req UserBatchManageRequest) error {
	switch req.Action {
	case userBatchActionEnable:
		if err := model.DB.Model(&model.User{}).Where("id = ?", user.Id).Update("status", common.UserStatusEnabled).Error; err != nil {
			return err
		}
		invalidateManagedUserCaches(user.Id, false)
	case userBatchActionDisable:
		if err := model.DB.Model(&model.User{}).Where("id = ?", user.Id).Update("status", common.UserStatusDisabled).Error; err != nil {
			return err
		}
		invalidateManagedUserCaches(user.Id, true)
	case userBatchActionHardDelete:
		if err := model.HardDeleteUserById(user.Id); err != nil {
			return err
		}
		invalidateManagedUserCaches(user.Id, true)
	case userBatchActionPromote:
		if err := model.DB.Model(&model.User{}).Where("id = ?", user.Id).Update("role", common.RoleAdminUser).Error; err != nil {
			return err
		}
		invalidateManagedUserCaches(user.Id, true)
	case userBatchActionDemote:
		if err := model.DB.Model(&model.User{}).Where("id = ?", user.Id).Update("role", common.RoleCommonUser).Error; err != nil {
			return err
		}
		invalidateManagedUserCaches(user.Id, true)
	case userBatchActionAddQuota:
		switch req.Mode {
		case userBatchQuotaModeAdd:
			if err := model.IncreaseUserQuota(user.Id, req.Value, true); err != nil {
				return err
			}
		case userBatchQuotaModeSub:
			if err := model.DecreaseUserQuota(user.Id, req.Value, true); err != nil {
				return err
			}
		case userBatchQuotaModeSet:
			if err := model.DB.Model(&model.User{}).Where("id = ?", user.Id).Update("quota", req.Value).Error; err != nil {
				return err
			}
			invalidateManagedUserCaches(user.Id, false)
		default:
			return fmt.Errorf("invalid quota mode")
		}
	case userBatchActionSetGroup:
		if err := model.DB.Model(&model.User{}).Where("id = ?", user.Id).Update("group", req.Group).Error; err != nil {
			return err
		}
		if err := model.UpdateUserGroupCache(user.Id, req.Group); err != nil {
			common.SysLog(fmt.Sprintf("failed to update user group cache for user %d: %s", user.Id, err.Error()))
		}
		invalidateManagedUserCaches(user.Id, true)
	default:
		return fmt.Errorf("invalid action")
	}
	return nil
}

func appendUserBatchFailure(result *UserBatchManageResult, id int, username string, reason string) {
	result.Failures = append(result.Failures, UserBatchManageFailure{
		Id:       id,
		Username: username,
		Reason:   reason,
	})
}

func BatchManageUsers(c *gin.Context) {
	var req UserBatchManageRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}

	action := strings.TrimSpace(req.Action)
	req.Action = action
	req.Mode = strings.TrimSpace(req.Mode)
	req.Group = strings.TrimSpace(req.Group)
	ids := normalizeUserBatchIds(req.Ids)
	if !isValidUserBatchAction(action) || !validateUserBatchRequest(req) || len(ids) == 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if len(ids) > userBatchMaxSize {
		common.ApiErrorI18n(c, i18n.MsgBatchTooMany, map[string]any{"Max": userBatchMaxSize})
		return
	}

	usersById, err := usersByIdUnscoped(ids)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	result := UserBatchManageResult{
		Action:   action,
		Failures: make([]UserBatchManageFailure, 0),
	}
	myRole := c.GetInt("role")
	for _, id := range ids {
		user, ok := usersById[id]
		if !ok {
			appendUserBatchFailure(&result, id, "", "user not found")
			continue
		}
		if reason := validateUserBatchTarget(myRole, user, action); reason != "" {
			appendUserBatchFailure(&result, id, user.Username, reason)
			continue
		}
		if err := applyUserBatchAction(user, req); err != nil {
			appendUserBatchFailure(&result, id, user.Username, err.Error())
			continue
		}
		result.Succeeded++
	}
	result.Failed = len(result.Failures)

	if result.Succeeded > 0 {
		recordManageAudit(c, "user.batch_manage", map[string]interface{}{
			"action": userBatchActionAuditLabel(action, req.Mode),
			"count":  result.Succeeded,
			"failed": result.Failed,
			"group":  req.Group,
			"mode":   req.Mode,
			"value":  req.Value,
		})
	}

	common.ApiSuccess(c, result)
}
