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
import { useMemo, type ChangeEvent } from 'react'
import { z } from 'zod'
import type { Resolver } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useTranslation } from 'react-i18next'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useSettingsForm } from '../hooks/use-settings-form'
import { useUpdateOption } from '../hooks/use-update-option'

const schema = z.object({
  enabled: z.boolean(),
  minQuota: z.coerce.number().min(0),
  maxQuota: z.coerce.number().min(0),
})

type Values = z.infer<typeof schema>

const DEFAULT_QUOTA_PER_USD = 500000

function normalizeQuotaPerUnit(value: number) {
  return Number.isFinite(value) && value > 0 ? value : DEFAULT_QUOTA_PER_USD
}

function quotaToUsd(quota: number, quotaPerUnit: number) {
  return quota / quotaPerUnit
}

function usdToQuota(usd: number, quotaPerUnit: number) {
  return Math.max(0, Math.round(usd * quotaPerUnit))
}

function formatQuota(value: number) {
  return value.toLocaleString(undefined, { maximumFractionDigits: 0 })
}

export function CheckinSettingsSection({
  defaultValues,
  quotaPerUnit,
}: {
  defaultValues: {
    enabled: boolean
    minQuota: number
    maxQuota: number
  }
  quotaPerUnit: number
}) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const {
    enabled: defaultEnabled,
    minQuota: defaultMinQuota,
    maxQuota: defaultMaxQuota,
  } = defaultValues
  const normalizedQuotaPerUnit = normalizeQuotaPerUnit(quotaPerUnit)
  const formDefaults = useMemo(
    () => ({
      enabled: defaultEnabled,
      minQuota: quotaToUsd(defaultMinQuota, normalizedQuotaPerUnit),
      maxQuota: quotaToUsd(defaultMaxQuota, normalizedQuotaPerUnit),
    }),
    [defaultEnabled, defaultMinQuota, defaultMaxQuota, normalizedQuotaPerUnit]
  )

  const handleNumberChange =
    (onChange: (value: number | string) => void) =>
    (event: ChangeEvent<HTMLInputElement>) => {
      onChange(
        event.target.value === '' ? '' : event.currentTarget.valueAsNumber
      )
    }

  const { form, handleSubmit, isDirty, isSubmitting } =
    useSettingsForm<Values>({
      resolver: zodResolver(schema) as Resolver<Values, unknown, Values>,
      defaultValues: formDefaults,
      onSubmit: async (_data, changedFields) => {
        for (const [key, value] of Object.entries(changedFields)) {
          const option =
            key === 'minQuota'
              ? {
                  key: 'checkin_setting.min_quota',
                  value: usdToQuota(Number(value), normalizedQuotaPerUnit),
                }
              : key === 'maxQuota'
                ? {
                    key: 'checkin_setting.max_quota',
                    value: usdToQuota(Number(value), normalizedQuotaPerUnit),
                  }
                : {
                    key: 'checkin_setting.enabled',
                    value: Boolean(value),
                  }

          await updateOption.mutateAsync(option)
        }
      },
    })

  const enabled = form.watch('enabled')

  const getQuotaPreview = (usd: number | string) =>
    formatQuota(usdToQuota(Number(usd) || 0, normalizedQuotaPerUnit))

  return (
    <SettingsSection title={t('Check-in Settings')}>
      <Form {...form}>
        <SettingsForm onSubmit={handleSubmit} autoComplete='off'>
          <SettingsPageFormActions
            onSave={handleSubmit}
            isSaving={updateOption.isPending || isSubmitting}
            isSaveDisabled={!isDirty}
            saveLabel='Save check-in settings'
          />
          <FormField
            control={form.control}
            name='enabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable check-in feature')}</FormLabel>
                  <FormDescription>
                    {t(
                      'Allow users to check in daily for random quota rewards'
                    )}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                    disabled={updateOption.isPending || isSubmitting}
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />

          {enabled && (
            <div className='grid gap-6 sm:grid-cols-2'>
              <FormField
                control={form.control}
                name='minQuota'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Minimum check-in quota (USD)')}</FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        min={0}
                        step='0.001'
                        value={field.value ?? ''}
                        onChange={handleNumberChange(field.onChange)}
                        name={field.name}
                        onBlur={field.onBlur}
                        ref={field.ref}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Saved as {{quota}} internal quota.', {
                        quota: getQuotaPreview(field.value),
                      })}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='maxQuota'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Maximum check-in quota (USD)')}</FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        min={0}
                        step='0.001'
                        value={field.value ?? ''}
                        onChange={handleNumberChange(field.onChange)}
                        name={field.name}
                        onBlur={field.onBlur}
                        ref={field.ref}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Saved as {{quota}} internal quota.', {
                        quota: getQuotaPreview(field.value),
                      })}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>
          )}
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
