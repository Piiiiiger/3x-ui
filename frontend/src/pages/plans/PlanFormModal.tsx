import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Checkbox, Form, Input, InputNumber, Modal, Radio, Select } from 'antd';
import { FormProvider, useForm, useWatch } from 'react-hook-form';

import { FormField, rhfZodValidate } from '@/components/form/rhf';
import SelectAllClearButtons from '@/components/form/SelectAllClearButtons';
import { remoteSourceBadge } from '@/pages/settings/subscriptionShared';
import type { PlanSummary } from '@/generated/zod';
import {
  PlanClashModeSchema,
  PlanFormSchema,
  PlanTrafficResetSchema,
  type PlanClashMode,
  type PlanFormValues,
  type PlanTrafficReset,
} from '@/schemas/plan';
import { useInboundChoices } from './planText';

const GIB = 1024 ** 3;

// reapplyLimits and clashMode ride along in the form so reopening the modal resets them too.
type PlanFormState = PlanFormValues & { reapplyLimits: boolean; clashMode: PlanClashMode };

const CLASH_MODE_LABEL_KEYS: Record<PlanClashMode, string> = {
  inherit: 'pages.plans.clashInherit',
  custom: 'pages.plans.clashCustom',
};

const CLASH_RULES_PLACEHOLDER =
  'https://…/rules.yaml\n\nDOMAIN-SUFFIX,example.com,DIRECT\nGEOIP,CN,DIRECT';

function initialState(plan: PlanSummary | null): PlanFormState {
  const reset = PlanTrafficResetSchema.safeParse(plan?.trafficReset);
  return {
    name: plan?.name ?? '',
    quotaGB: plan ? Math.round((plan.totalGB / GIB) * 100) / 100 : 0,
    durationDays: plan?.durationDays ?? 30,
    trafficReset: reset.success ? reset.data : 'never',
    trafficResetDay: plan?.trafficResetDay || 1,
    limitIp: plan?.limitIp ?? 0,
    remark: plan?.remark ?? '',
    clashRules: plan?.clashRules ?? '',
    inboundIds: [...(plan?.inboundIds ?? [])],
    reapplyLimits: false,
    clashMode: plan?.clashRules ? 'custom' : 'inherit',
  };
}

interface PlanFormModalProps {
  open: boolean;
  plan: PlanSummary | null;
  onClose: () => void;
  onConfirm: (values: PlanFormValues, reapplyLimits: boolean) => Promise<void> | void;
}

export default function PlanFormModal({ open, plan, onClose, onConfirm }: PlanFormModalProps) {
  const { t } = useTranslation();
  const methods = useForm<PlanFormState>({ defaultValues: initialState(plan) });
  const [saving, setSaving] = useState(false);
  const isEdit = plan != null;
  const members = plan?.memberCount ?? 0;

  useEffect(() => {
    if (open) methods.reset(initialState(plan));
  }, [open, plan, methods]);

  const trafficReset = useWatch({ control: methods.control, name: 'trafficReset' });
  const inboundIds = useWatch({ control: methods.control, name: 'inboundIds' });
  const clashMode = useWatch({ control: methods.control, name: 'clashMode' });
  const clashRules = useWatch({ control: methods.control, name: 'clashRules' });
  const { options: inboundGroups, flat: inboundOptions } = useInboundChoices();

  const resetOptions = PlanTrafficResetSchema.options.map((value: PlanTrafficReset) => ({
    value,
    label: t(`pages.inbounds.periodicTrafficReset.${value}`),
  }));

  async function onFinish({ reapplyLimits, clashMode: mode, ...values }: PlanFormState) {
    setSaving(true);
    try {
      const parsed = PlanFormSchema.parse({
        ...values,
        clashRules: mode === 'custom' ? values.clashRules : '',
      });
      await onConfirm(parsed, isEdit && members > 0 && reapplyLimits);
    } finally {
      setSaving(false);
    }
  }

  return (
    <Modal
      open={open}
      title={isEdit ? t('pages.plans.edit') : t('pages.plans.add')}
      okText={isEdit ? t('save') : t('create')}
      cancelText={t('cancel')}
      confirmLoading={saving}
      mask={{ closable: false }}
      width="600px"
      onOk={methods.handleSubmit(onFinish)}
      onCancel={onClose}
    >
      <FormProvider {...methods}>
        <Form layout="vertical">
          <FormField
            label={t('pages.groups.name')}
            name="name"
            required
            rules={{ validate: rhfZodValidate(PlanFormSchema.shape.name) }}
          >
            <Input maxLength={64} />
          </FormField>

          <div className="plan-form-row">
            <FormField
              label={t('pages.plans.quota')}
              name="quotaGB"
              tooltip={t('pages.plans.zeroUnlimited')}
            >
              <InputNumber min={0} precision={2} suffix="GB" style={{ width: '100%' }} />
            </FormField>
            <FormField
              label={t('pages.plans.duration')}
              name="durationDays"
              tooltip={t('pages.plans.durationHint')}
            >
              <InputNumber
                min={0}
                precision={0}
                suffix={t('pages.plans.daysUnit')}
                style={{ width: '100%' }}
              />
            </FormField>
          </div>

          <div className="plan-form-row">
            <FormField label={t('pages.inbounds.periodicTrafficResetTitle')} name="trafficReset">
              <Select options={resetOptions} />
            </FormField>
            <FormField
              label={t('pages.clients.limitIp')}
              name="limitIp"
              tooltip={t('pages.plans.zeroUnlimited')}
            >
              <InputNumber min={0} precision={0} style={{ width: '100%' }} />
            </FormField>
          </div>
          {trafficReset === 'monthly' && (
            <FormField label={t('pages.plans.resetDay')} name="trafficResetDay">
              <InputNumber min={1} max={31} precision={0} style={{ width: '100%' }} />
            </FormField>
          )}

          <FormField label={t('pages.plans.servers')} name="inboundIds">
            <Select
              mode="multiple"
              options={inboundGroups}
              maxTagCount="responsive"
              listHeight={240}
              placeholder={t('pages.plans.noServers')}
              showSearch={{ optionFilterProp: 'label' }}
            />
          </FormField>
          <SelectAllClearButtons
            options={inboundOptions}
            value={inboundIds || []}
            onChange={(v) => methods.setValue('inboundIds', v, { shouldDirty: true })}
          />

          <FormField
            label={t('pages.plans.clashRules')}
            name="clashMode"
            tooltip={t('pages.plans.clashRulesHint')}
          >
            <Radio.Group
              optionType="button"
              options={PlanClashModeSchema.options.map((value) => ({
                value,
                label: t(CLASH_MODE_LABEL_KEYS[value]),
              }))}
            />
          </FormField>
          {clashMode === 'custom' && (
            <FormField
              name="clashRules"
              extra={remoteSourceBadge(clashRules ?? '')}
              rules={{
                validate: (value) =>
                  String(value ?? '').trim() !== '' || 'pages.plans.errClashRulesRequired',
              }}
            >
              <Input.TextArea rows={6} placeholder={CLASH_RULES_PLACEHOLDER} spellCheck={false} />
            </FormField>
          )}

          <FormField label={t('remark')} name="remark">
            <Input maxLength={256} />
          </FormField>

          {isEdit && members > 0 && (
            <FormField
              name="reapplyLimits"
              valueProp="checked"
              extra={t('pages.plans.serverChangesApply')}
            >
              <Checkbox>{t('pages.plans.reapplyLimits', { count: members })}</Checkbox>
            </FormField>
          )}
        </Form>
      </FormProvider>
    </Modal>
  );
}
