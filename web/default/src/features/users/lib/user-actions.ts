/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { type BatchManageUserAction, type ManageUserAction } from '../types'

// ============================================================================
// User Action Messages
// ============================================================================

const ACTION_MESSAGES: Record<ManageUserAction, string> = {
  enable: 'User enabled successfully',
  disable: 'User disabled successfully',
  promote: 'User promoted to admin successfully',
  demote: 'User demoted to regular user successfully',
  delete: 'User deleted successfully',
  add_quota: 'Quota adjusted successfully',
}

const BATCH_ACTION_MESSAGES: Record<BatchManageUserAction, string> = {
  enable: '{{count}} user(s) enabled',
  disable: '{{count}} user(s) disabled',
  hard_delete: '{{count}} user(s) deleted',
  promote: '{{count}} user(s) promoted to admin',
  demote: '{{count}} user(s) demoted to regular user',
  add_quota: 'Quota adjusted for {{count}} user(s)',
  set_group: 'Group updated for {{count}} user(s)',
}

/**
 * Get success message for user management action
 */
export function getUserActionMessage(action: ManageUserAction): string {
  return ACTION_MESSAGES[action]
}

/**
 * Get success message for batch user management action
 */
export function getBatchUserActionMessage(
  action: BatchManageUserAction
): string {
  return BATCH_ACTION_MESSAGES[action]
}
