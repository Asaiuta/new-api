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
import * as z from 'zod'
import type { Resolver } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
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
import { FormDirtyIndicator } from '../components/form-dirty-indicator'
import { FormNavigationGuard } from '../components/form-navigation-guard'
import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
  SettingsFormGrid,
  SettingsFormGridItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useSettingsForm } from '../hooks/use-settings-form'
import { useUpdateOption } from '../hooks/use-update-option'

const quotaSchema = z.object({
  QuotaForNewUser: z.coerce.number().min(0),
  PreConsumedQuota: z.coerce.number().min(0),
  QuotaForInviter: z.coerce.number().min(0),
  QuotaForInvitee: z.coerce.number().min(0),
  TopUpLink: z.string(),
  general_setting: z.object({
    docs_link: z.string(),
  }),
  quota_setting: z.object({
    enable_free_model_pre_consume: z.boolean(),
    fast_pre_consume_estimate: z.boolean(),
  }),
})

type QuotaFormValues = z.infer<typeof quotaSchema>

type RawQuotaSettingsValues = QuotaFormValues

type QuotaSettingsSectionProps = {
  defaultValues: RawQuotaSettingsValues
  quotaPerUnit: number
  complianceConfirmed?: boolean
}

const DEFAULT_QUOTA_PER_USD = 500000
const usdBackedQuotaKeys = [
  'QuotaForNewUser',
  'QuotaForInviter',
  'QuotaForInvitee',
] as const

type UsdBackedQuotaKey = (typeof usdBackedQuotaKeys)[number]

function isUsdBackedQuotaKey(key: string): key is UsdBackedQuotaKey {
  return (usdBackedQuotaKeys as readonly string[]).includes(key)
}

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

function formatUsd(value: number) {
  const precision = value > 0 && value < 0.01 ? 6 : 3
  return value.toLocaleString(undefined, {
    minimumFractionDigits: 0,
    maximumFractionDigits: precision,
  })
}

function buildFormDefaults(
  defaults: RawQuotaSettingsValues,
  quotaPerUnit: number
): QuotaFormValues {
  return {
    ...defaults,
    QuotaForNewUser: quotaToUsd(defaults.QuotaForNewUser, quotaPerUnit),
    QuotaForInviter: quotaToUsd(defaults.QuotaForInviter, quotaPerUnit),
    QuotaForInvitee: quotaToUsd(defaults.QuotaForInvitee, quotaPerUnit),
  }
}

export function QuotaSettingsSection({
  defaultValues,
  quotaPerUnit,
  complianceConfirmed = true,
}: QuotaSettingsSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const normalizedQuotaPerUnit = normalizeQuotaPerUnit(quotaPerUnit)
  const formDefaults = useMemo(
    () => buildFormDefaults(defaultValues, normalizedQuotaPerUnit),
    [
      defaultValues.QuotaForNewUser,
      defaultValues.PreConsumedQuota,
      defaultValues.QuotaForInviter,
      defaultValues.QuotaForInvitee,
      defaultValues.TopUpLink,
      defaultValues.general_setting.docs_link,
      defaultValues.quota_setting.enable_free_model_pre_consume,
      defaultValues.quota_setting.fast_pre_consume_estimate,
      normalizedQuotaPerUnit,
    ]
  )
  const handleNumberChange =
    (onChange: (value: number | string) => void) =>
    (event: ChangeEvent<HTMLInputElement>) => {
      onChange(
        event.target.value === '' ? '' : event.currentTarget.valueAsNumber
      )
    }

  const { form, handleSubmit, isDirty, isSubmitting } =
    useSettingsForm<QuotaFormValues>({
      resolver: zodResolver(quotaSchema) as Resolver<
        QuotaFormValues,
        unknown,
        QuotaFormValues
      >,
      defaultValues: formDefaults,
      onSubmit: async (_data, changedFields) => {
        for (const [key, value] of Object.entries(changedFields)) {
          const optionValue =
            isUsdBackedQuotaKey(key) && typeof value === 'number'
              ? usdToQuota(value, normalizedQuotaPerUnit)
              : value

          await updateOption.mutateAsync({
            key,
            value: optionValue as string | number | boolean,
          })
        }
      },
    })

  const getQuotaPreview = (usd: number) =>
    formatQuota(usdToQuota(usd || 0, normalizedQuotaPerUnit))
  const preConsumedUsd = quotaToUsd(
    Number(form.watch('PreConsumedQuota') || 0),
    normalizedQuotaPerUnit
  )

  return (
    <SettingsSection title={t('Quota Settings')}>
      <FormNavigationGuard when={isDirty} />

      {!complianceConfirmed ? (
        <Alert variant='destructive'>
          <AlertDescription>
            {t(
              'Non-zero invitation rewards require compliance confirmation in Payment Gateway settings.'
            )}
          </AlertDescription>
        </Alert>
      ) : null}

      <Form {...form}>
        <SettingsForm onSubmit={handleSubmit}>
          <SettingsPageFormActions
            onSave={handleSubmit}
            isSaving={updateOption.isPending || isSubmitting}
          />
          <FormDirtyIndicator isDirty={isDirty} />
          <SettingsFormGrid>
            <FormField
              control={form.control}
              name='QuotaForNewUser'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('New User Credit (USD)')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min='0'
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
              name='PreConsumedQuota'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Pre-Consumed Quota')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      value={field.value ?? ''}
                      onChange={handleNumberChange(field.onChange)}
                      name={field.name}
                      onBlur={field.onBlur}
                      ref={field.ref}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Internal quota reserved before final settlement. Approximately ${{amount}}.',
                      { amount: formatUsd(preConsumedUsd) }
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='QuotaForInviter'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Inviter Reward (USD)')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min='0'
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
              name='QuotaForInvitee'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Invitee Reward (USD)')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min='0'
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

            <SettingsFormGridItem span='full'>
              <FormField
                control={form.control}
                name='quota_setting.enable_free_model_pre_consume'
                render={({ field }) => (
                  <SettingsSwitchItem>
                    <SettingsSwitchContent>
                      <FormLabel>{t('Pre-Consume for Free Models')}</FormLabel>
                      <FormDescription>
                        {t(
                          'When enabled, zero-cost models also pre-consume quota before final settlement.'
                        )}
                      </FormDescription>
                    </SettingsSwitchContent>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                        disabled={updateOption.isPending}
                      />
                    </FormControl>
                  </SettingsSwitchItem>
                )}
              />
            </SettingsFormGridItem>

            <SettingsFormGridItem span='full'>
              <FormField
                control={form.control}
                name='quota_setting.fast_pre_consume_estimate'
                render={({ field }) => (
                  <SettingsSwitchItem>
                    <SettingsSwitchContent>
                      <FormLabel>{t('Fast Pre-Consume Estimate')}</FormLabel>
                      <FormDescription>
                        {t(
                          'Use the fast token estimator during pre-consume to reduce first-token latency for large requests. Final settlement still uses upstream usage when available.'
                        )}
                      </FormDescription>
                    </SettingsSwitchContent>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                        disabled={updateOption.isPending}
                      />
                    </FormControl>
                  </SettingsSwitchItem>
                )}
              />
            </SettingsFormGridItem>

            <FormField
              control={form.control}
              name='TopUpLink'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Top-Up Link')}</FormLabel>
                  <FormControl>
                    <Input
                      placeholder={t('https://example.com/topup')}
                      {...field}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('External link for users to purchase quota')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='general_setting.docs_link'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Documentation Link')}</FormLabel>
                  <FormControl>
                    <Input
                      placeholder={t('https://docs.example.com')}
                      {...field}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Link to your documentation site')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </SettingsFormGrid>
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
