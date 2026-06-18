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
import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { type Table } from '@tanstack/react-table'
import {
  ArrowDown,
  ArrowUp,
  Coins,
  FolderInput,
  Power,
  PowerOff,
  Trash2,
  type LucideIcon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { getCurrencyDisplay, getCurrencyLabel } from '@/lib/currency'
import { formatQuota, parseQuotaFromDollars } from '@/lib/format'
import { DataTableBulkActions as BulkActionsToolbar } from '@/components/data-table'
import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  ToggleGroup,
  ToggleGroupItem,
} from '@/components/ui/toggle-group'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { batchManageUsers, getGroups } from '../api'
import { ERROR_MESSAGES } from '../constants'
import { getBatchUserActionMessage } from '../lib'
import {
  type BatchManageUserAction,
  type BatchManageUsersPayload,
  type QuotaAdjustMode,
  type User,
} from '../types'
import { useUsers } from './users-provider'

interface DataTableBulkActionsProps {
  table: Table<User>
}

type BatchActionPayload = Omit<BatchManageUsersPayload, 'ids'>

type BulkActionIconButtonProps = {
  label: string
  icon: LucideIcon
  onClick: () => void
  disabled?: boolean
  variant?: 'outline' | 'destructive'
}

function BulkActionIconButton({
  label,
  icon: Icon,
  onClick,
  disabled,
  variant = 'outline',
}: BulkActionIconButtonProps) {
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button
            variant={variant}
            size='icon'
            onClick={onClick}
            disabled={disabled}
            className='size-8'
            aria-label={label}
            title={label}
          />
        }
      >
        <Icon />
        <span className='sr-only'>{label}</span>
      </TooltipTrigger>
      <TooltipContent>
        <p>{label}</p>
      </TooltipContent>
    </Tooltip>
  )
}

export function DataTableBulkActions({ table }: DataTableBulkActionsProps) {
  const { t } = useTranslation()
  const { triggerRefresh } = useUsers()
  const [showPromoteConfirm, setShowPromoteConfirm] = useState(false)
  const [showDemoteConfirm, setShowDemoteConfirm] = useState(false)
  const [showQuotaDialog, setShowQuotaDialog] = useState(false)
  const [showGroupDialog, setShowGroupDialog] = useState(false)
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false)
  const [quotaMode, setQuotaMode] = useState<QuotaAdjustMode>('add')
  const [quotaAmount, setQuotaAmount] = useState('')
  const [selectedGroup, setSelectedGroup] = useState('')
  const [submittingAction, setSubmittingAction] =
    useState<BatchManageUserAction | null>(null)

  const { data: groupsData } = useQuery({
    queryKey: ['groups'],
    queryFn: getGroups,
    staleTime: 5 * 60 * 1000,
  })

  const groups = groupsData?.data || []
  const selectedRows = table.getFilteredSelectedRowModel().rows
  const selectedIds = selectedRows.reduce<number[]>((ids, row) => {
    const id = row.original.id
    if (typeof id === 'number') {
      ids.push(id)
    }
    return ids
  }, [])

  const isSubmitting = submittingAction !== null
  const { meta: currencyMeta } = getCurrencyDisplay()
  const currencyLabel = getCurrencyLabel()
  const tokensOnly = currencyMeta.kind === 'tokens'

  const amountValue = Number.parseFloat(quotaAmount) || 0
  const quotaValue =
    quotaMode === 'override'
      ? parseQuotaFromDollars(amountValue)
      : parseQuotaFromDollars(Math.abs(amountValue))
  const quotaPreview = getBatchQuotaPreview(quotaMode, quotaValue, t)

  const clearSelection = () => table.resetRowSelection()

  const handleBatchAction = async (
    payload: BatchActionPayload,
    onSuccess?: () => void
  ) => {
    if (selectedIds.length === 0) {
      toast.error(t('No users selected'))
      return
    }

    setSubmittingAction(payload.action)
    try {
      const result = await batchManageUsers({ ids: selectedIds, ...payload })
      if (!result.success || !result.data) {
        toast.error(result.message || t('Failed to manage users'))
        return
      }

      const { succeeded, failed } = result.data
      if (succeeded > 0) {
        toast.success(
          t(getBatchUserActionMessage(payload.action), { count: succeeded })
        )
        triggerRefresh()
        clearSelection()
        onSuccess?.()
      }

      if (failed > 0) {
        toast.error(t('{{count}} user(s) failed', { count: failed }))
      }
    } catch (_error) {
      toast.error(t(ERROR_MESSAGES.UNEXPECTED))
    } finally {
      setSubmittingAction(null)
    }
  }

  const resetQuotaDialog = () => {
    setShowQuotaDialog(false)
    setQuotaMode('add')
    setQuotaAmount('')
  }

  const handleQuotaConfirm = () => {
    if (quotaMode !== 'override' && quotaValue <= 0) {
      toast.error(t('Amount must be greater than 0'))
      return
    }

    handleBatchAction(
      {
        action: 'add_quota',
        mode: quotaMode,
        value: quotaValue,
      },
      resetQuotaDialog
    )
  }

  const resetGroupDialog = () => {
    setShowGroupDialog(false)
    setSelectedGroup('')
  }

  const handleGroupConfirm = () => {
    const group = selectedGroup.trim()
    if (!group) {
      toast.error(t('Please select a group'))
      return
    }

    handleBatchAction(
      {
        action: 'set_group',
        group,
      },
      resetGroupDialog
    )
  }

  const quotaPlaceholder = tokensOnly
    ? t('Enter amount in tokens')
    : t('Enter amount in {{currency}}', { currency: currencyLabel })

  return (
    <>
      <BulkActionsToolbar table={table} entityName='user'>
        <BulkActionIconButton
          label={t('Enable selected users')}
          icon={Power}
          onClick={() => handleBatchAction({ action: 'enable' })}
          disabled={isSubmitting}
        />

        <BulkActionIconButton
          label={t('Disable selected users')}
          icon={PowerOff}
          onClick={() => handleBatchAction({ action: 'disable' })}
          disabled={isSubmitting}
        />

        <BulkActionIconButton
          label={t('Promote selected users')}
          icon={ArrowUp}
          onClick={() => setShowPromoteConfirm(true)}
          disabled={isSubmitting}
        />

        <BulkActionIconButton
          label={t('Demote selected users')}
          icon={ArrowDown}
          onClick={() => setShowDemoteConfirm(true)}
          disabled={isSubmitting}
        />

        <BulkActionIconButton
          label={t('Adjust quota for selected users')}
          icon={Coins}
          onClick={() => setShowQuotaDialog(true)}
          disabled={isSubmitting}
        />

        <BulkActionIconButton
          label={t('Set group for selected users')}
          icon={FolderInput}
          onClick={() => setShowGroupDialog(true)}
          disabled={isSubmitting}
        />

        <BulkActionIconButton
          label={t('Delete selected users')}
          icon={Trash2}
          onClick={() => setShowDeleteConfirm(true)}
          disabled={isSubmitting}
          variant='destructive'
        />
      </BulkActionsToolbar>

      <Dialog
        open={showPromoteConfirm}
        onOpenChange={setShowPromoteConfirm}
        title={t('Promote Users?')}
        description={t(
          'Promote {{count}} selected user(s) to admin? This may grant access to administrative features.',
          { count: selectedIds.length }
        )}
        contentHeight='auto'
        footer={
          <>
            <Button
              variant='outline'
              onClick={() => setShowPromoteConfirm(false)}
              disabled={isSubmitting}
            >
              {t('Cancel')}
            </Button>
            <Button
              onClick={() =>
                handleBatchAction({ action: 'promote' }, () =>
                  setShowPromoteConfirm(false)
                )
              }
              disabled={isSubmitting}
            >
              {isSubmitting ? t('Processing...') : t('Promote')}
            </Button>
          </>
        }
      >
        {' '}
      </Dialog>

      <Dialog
        open={showDemoteConfirm}
        onOpenChange={setShowDemoteConfirm}
        title={t('Demote Users?')}
        description={t(
          'Demote {{count}} selected user(s) to regular user? Administrative access will be removed.',
          { count: selectedIds.length }
        )}
        contentHeight='auto'
        footer={
          <>
            <Button
              variant='outline'
              onClick={() => setShowDemoteConfirm(false)}
              disabled={isSubmitting}
            >
              {t('Cancel')}
            </Button>
            <Button
              onClick={() =>
                handleBatchAction({ action: 'demote' }, () =>
                  setShowDemoteConfirm(false)
                )
              }
              disabled={isSubmitting}
            >
              {isSubmitting ? t('Processing...') : t('Demote')}
            </Button>
          </>
        }
      >
        {' '}
      </Dialog>

      <Dialog
        open={showQuotaDialog}
        onOpenChange={setShowQuotaDialog}
        title={t('Adjust Quota')}
        description={t('Adjust quota for {{count}} selected user(s).', {
          count: selectedIds.length,
        })}
        contentHeight='auto'
        bodyClassName='flex flex-col gap-4'
        footer={
          <>
            <Button
              variant='outline'
              onClick={resetQuotaDialog}
              disabled={isSubmitting}
            >
              {t('Cancel')}
            </Button>
            <Button onClick={handleQuotaConfirm} disabled={isSubmitting}>
              {isSubmitting ? t('Processing...') : t('Confirm')}
            </Button>
          </>
        }
      >
        <FieldGroup>
          <Field>
            <FieldLabel>{t('Mode')}</FieldLabel>
            <ToggleGroup
              value={[quotaMode]}
              onValueChange={(value) => {
                const nextMode = value[0]
                if (nextMode) {
                  setQuotaMode(nextMode as QuotaAdjustMode)
                  setQuotaAmount('')
                }
              }}
              variant='outline'
              size='sm'
              spacing={1}
            >
              <ToggleGroupItem value='add'>{t('Add')}</ToggleGroupItem>
              <ToggleGroupItem value='subtract'>
                {t('Subtract')}
              </ToggleGroupItem>
              <ToggleGroupItem value='override'>
                {t('Override')}
              </ToggleGroupItem>
            </ToggleGroup>
          </Field>

          <Field>
            <FieldLabel htmlFor='batch-user-quota-amount'>
              {t('Amount')} ({currencyLabel})
            </FieldLabel>
            <Input
              id='batch-user-quota-amount'
              type='number'
              step={tokensOnly ? 1 : 0.000001}
              min={quotaMode === 'override' ? undefined : 0}
              placeholder={quotaPlaceholder}
              value={quotaAmount}
              onChange={(event) => setQuotaAmount(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === 'Enter') handleQuotaConfirm()
              }}
            />
            <FieldDescription>{quotaPreview}</FieldDescription>
          </Field>
        </FieldGroup>
      </Dialog>

      <Dialog
        open={showGroupDialog}
        onOpenChange={setShowGroupDialog}
        title={t('Set User Group')}
        description={t('Set group for {{count}} selected user(s).', {
          count: selectedIds.length,
        })}
        contentHeight='auto'
        footer={
          <>
            <Button
              variant='outline'
              onClick={resetGroupDialog}
              disabled={isSubmitting}
            >
              {t('Cancel')}
            </Button>
            <Button
              onClick={handleGroupConfirm}
              disabled={isSubmitting || !selectedGroup}
            >
              {isSubmitting ? t('Processing...') : t('Confirm')}
            </Button>
          </>
        }
      >
        <FieldGroup>
          <Field>
            <FieldLabel>{t('Group')}</FieldLabel>
            <Select
              items={groups.map((group) => ({ value: group, label: group }))}
              value={selectedGroup}
              onValueChange={(value) => {
                if (value !== null) setSelectedGroup(value)
              }}
            >
              <SelectTrigger className='w-full'>
                <SelectValue placeholder={t('Select a group')} />
              </SelectTrigger>
              <SelectContent alignItemWithTrigger={false}>
                <SelectGroup>
                  {groups.map((group) => (
                    <SelectItem key={group} value={group}>
                      {group}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          </Field>
        </FieldGroup>
      </Dialog>

      <Dialog
        open={showDeleteConfirm}
        onOpenChange={setShowDeleteConfirm}
        title={t('Delete Users?')}
        description={t(
          'Are you sure you want to permanently delete {{count}} user(s)? This action cannot be undone.',
          { count: selectedIds.length }
        )}
        contentHeight='auto'
        footer={
          <>
            <Button
              variant='outline'
              onClick={() => setShowDeleteConfirm(false)}
              disabled={isSubmitting}
            >
              {t('Cancel')}
            </Button>
            <Button
              variant='destructive'
              onClick={() =>
                handleBatchAction({ action: 'hard_delete' }, () =>
                  setShowDeleteConfirm(false)
                )
              }
              disabled={isSubmitting}
            >
              {isSubmitting ? t('Processing...') : t('Delete')}
            </Button>
          </>
        }
      >
        {' '}
      </Dialog>
    </>
  )
}

function getBatchQuotaPreview(
  mode: QuotaAdjustMode,
  value: number,
  t: ReturnType<typeof useTranslation>['t']
) {
  switch (mode) {
    case 'add':
      return t('Each selected user will receive {{quota}}.', {
        quota: formatQuota(value),
      })
    case 'subtract':
      return t('Each selected user will lose {{quota}}.', {
        quota: formatQuota(value),
      })
    case 'override':
      return t("Each selected user's quota will be set to {{quota}}.", {
        quota: formatQuota(value),
      })
    default: {
      const _exhaustive: never = mode
      return _exhaustive
    }
  }
}
