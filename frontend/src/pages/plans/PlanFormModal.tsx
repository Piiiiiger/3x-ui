import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Checkbox, Form, Input, InputNumber, Modal, Select } from 'antd';
import { FormProvider, useForm, useWatch } from 'react-hook-form';

import { FormField, rhfZodValidate } from '@/components/form/rhf';
import SelectAllClearButtons from '@/components/form/SelectAllClearButtons';
import type { PlanSummary } from '@/generated/zod';
import {
  PlanFormSchema,
  PlanTrafficResetSchema,
  type PlanFormValues,
  type PlanTrafficReset,
} from '@/schemas/plan';
import { useInboundChoices } from './planText';

const GIB = 1024 ** 3;

// applyToMembers rides along in the form so reopening the modal resets it too.
type PlanFormState = PlanFormValues & { applyToMembers: boolean };

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
    inboundIds: [...(plan?.inboundIds ?? [])],
    applyToMembers: true,
  };
}

interface PlanFormModalProps {
  open: boolean;
  plan: PlanSummary | null;
  onClose: () => void;
  onConfirm: (values: PlanFormValues, applyToMembers: boolean) => Promise<void> | void;
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
  const { options: inboundGroups, flat: inboundOptions } = useInboundChoices();

  const resetOptions = PlanTrafficResetSchema.options.map((value: PlanTrafficReset) => ({
    value,
    label: t(`pages.inbounds.periodicTrafficReset.${value}`),
  }));

  async function onFinish({ applyToMembers, ...values }: PlanFormState) {
    setSaving(true);
    try {
      await onConfirm(PlanFormSchema.parse(values), isEdit && members > 0 && applyToMembers);
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

          <FormField label={t('remark')} name="remark">
            <Input maxLength={256} />
          </FormField>

          {isEdit && members > 0 && (
            <FormField name="applyToMembers" valueProp="checked">
              <Checkbox>{t('pages.plans.applyToMembers', { count: members })}</Checkbox>
            </FormField>
          )}
        </Form>
      </FormProvider>
    </Modal>
  );
}
