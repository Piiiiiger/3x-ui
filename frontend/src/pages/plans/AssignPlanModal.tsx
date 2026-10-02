import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Form, Modal, Select, Typography, message } from 'antd';
import { FormProvider, useForm } from 'react-hook-form';

import { FormField } from '@/components/form/rhf';
import { useClientOptions } from '@/api/queries/useClientOptions';
import { usePlanMutations } from '@/api/queries/usePlanMutations';
import type { PlanSummary } from '@/generated/zod';

interface AssignFormValues {
  planId: number | null;
  emails: string[];
}

function initialState(planId?: number): AssignFormValues {
  return { planId: planId ?? null, emails: [] };
}

interface AssignPlanModalProps {
  open: boolean;
  plans: PlanSummary[];
  // Fixed clients (from the clients table); empty lets the admin pick them here.
  emails: string[];
  planId?: number;
  onClose: () => void;
  onAssigned?: () => void;
}

export default function AssignPlanModal({
  open,
  plans,
  emails,
  planId,
  onClose,
  onAssigned,
}: AssignPlanModalProps) {
  const { t } = useTranslation();
  const [messageApi, messageContextHolder] = message.useMessage();
  const { assign } = usePlanMutations();
  const pickPeople = emails.length === 0;
  const { data: clientEmails } = useClientOptions(open && pickPeople);
  const methods = useForm<AssignFormValues>({ defaultValues: initialState(planId) });
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (open) methods.reset(initialState(planId));
  }, [open, planId, methods]);

  const planOptions = useMemo(() => plans.map((p) => ({ value: p.id, label: p.name })), [plans]);
  const peopleOptions = useMemo(
    () => (clientEmails ?? []).map((email) => ({ value: email, label: email })),
    [clientEmails],
  );

  async function submit(values: AssignFormValues) {
    const targets = pickPeople ? values.emails : emails;
    if (values.planId == null) {
      messageApi.error(t('pages.plans.needPlan'));
      return;
    }
    if (targets.length === 0) {
      messageApi.error(t('pages.plans.needPeople'));
      return;
    }
    setSaving(true);
    try {
      const msg = await assign({ emails: targets, planId: values.planId });
      if (msg?.success) {
        messageApi.success(t('pages.plans.toasts.assigned', { count: targets.length }));
        onAssigned?.();
        onClose();
      }
    } finally {
      setSaving(false);
    }
  }

  return (
    <Modal
      open={open}
      title={t('pages.plans.assign')}
      okText={t('confirm')}
      cancelText={t('cancel')}
      confirmLoading={saving}
      width="520px"
      onOk={methods.handleSubmit(submit)}
      onCancel={onClose}
    >
      {messageContextHolder}
      <FormProvider {...methods}>
        <Form layout="vertical">
          {pickPeople ? (
            <FormField label={t('pages.plans.people')} name="emails" required>
              <Select
                mode="multiple"
                options={peopleOptions}
                maxTagCount="responsive"
                showSearch={{ optionFilterProp: 'label' }}
              />
            </FormField>
          ) : (
            <Typography.Paragraph type="secondary">
              {emails.length === 1
                ? emails[0]
                : t('pages.plans.selectedPeople', { count: emails.length })}
            </Typography.Paragraph>
          )}
          <FormField label={t('menu.plans')} name="planId" required>
            <Select options={planOptions} />
          </FormField>
        </Form>
      </FormProvider>
    </Modal>
  );
}
