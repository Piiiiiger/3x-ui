import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Checkbox, Form, Input, InputNumber, Modal, Select } from 'antd';
import { StarFilled } from '@ant-design/icons';
import { FormProvider, useForm, useWatch } from 'react-hook-form';

import { FormField, rhfZodValidate } from '@/components/form/rhf';
import SelectAllClearButtons from '@/components/form/SelectAllClearButtons';
import { useRuleTemplatesQuery } from '@/api/queries/useRuleTemplates';
import type { PlanSummary } from '@/generated/zod';
import { PlanFormSchema, type PlanFormValues } from '@/schemas/plan';
import { useInboundChoices } from './planText';

// reapplyLimits rides along in the form so reopening the modal resets it too.
type PlanFormState = PlanFormValues & { reapplyLimits: boolean };

function initialState(plan: PlanSummary | null): PlanFormState {
  return {
    name: plan?.name ?? '',
    limitIp: plan?.limitIp ?? 0,
    remark: plan?.remark ?? '',
    templateId: plan?.templateId ?? 0,
    inboundIds: [...(plan?.inboundIds ?? [])],
    reapplyLimits: false,
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

  const inboundIds = useWatch({ control: methods.control, name: 'inboundIds' });
  const { templates } = useRuleTemplatesQuery();
  const defaultTemplate = templates.find((tpl) => tpl.isDefault);
  const templateOptions = [
    {
      value: 0,
      label: defaultTemplate
        ? t('pages.plans.templateDefault', { name: defaultTemplate.name })
        : t('pages.plans.templateNoDefault'),
    },
    ...templates.map((tpl) => ({
      value: tpl.id,
      label: tpl.isDefault ? (
        <span>
          {tpl.name} <StarFilled className="plan-template-star" />
        </span>
      ) : (
        tpl.name
      ),
    })),
  ];
  const { options: inboundGroups, flat: inboundOptions } = useInboundChoices();

  async function onFinish({ reapplyLimits, ...values }: PlanFormState) {
    setSaving(true);
    try {
      const parsed = PlanFormSchema.parse(values);
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
            label={t('pages.plans.name')}
            name="name"
            required
            rules={{ validate: rhfZodValidate(PlanFormSchema.shape.name) }}
          >
            <Input maxLength={64} />
          </FormField>

          <FormField
            label={t('pages.clients.limitIp')}
            name="limitIp"
            tooltip={t('pages.plans.zeroUnlimited')}
          >
            <InputNumber min={0} precision={0} style={{ width: '100%' }} />
          </FormField>

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
            label={t('pages.plans.template')}
            name="templateId"
            tooltip={t('pages.plans.templateHint')}
          >
            <Select options={templateOptions} />
          </FormField>

          <FormField label={t('remark')} name="remark">
            <Input maxLength={256} />
          </FormField>

          {isEdit && members > 0 && (
            <FormField
              name="reapplyLimits"
              valueProp="checked"
              extra={t('pages.plans.serverChangesApply')}
            >
              <Checkbox>{t('pages.plans.reapplyIpLimit', { count: members })}</Checkbox>
            </FormField>
          )}
        </Form>
      </FormProvider>
    </Modal>
  );
}
