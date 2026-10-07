import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Form, InputNumber, Modal } from 'antd';
import { FormProvider, useForm } from 'react-hook-form';

import { FormField } from '@/components/form/rhf';
import type { Msg } from '@/utils';

interface LocalHostValues {
  trafficMultiplier: number | null;
}

interface LocalHostModalProps {
  multiplier: number;
  save: (multiplier: number) => Promise<Msg<unknown>>;
  onClose: () => void;
}

/** This panel's own host has no node row; its traffic multiplier is all there is to edit. */
export default function LocalHostModal({ multiplier, save, onClose }: LocalHostModalProps) {
  const { t } = useTranslation();
  const methods = useForm<LocalHostValues>({ defaultValues: { trafficMultiplier: multiplier } });
  const [saving, setSaving] = useState(false);

  async function onFinish(values: LocalHostValues) {
    setSaving(true);
    try {
      const msg = await save(values.trafficMultiplier ?? 1);
      if (msg?.success) onClose();
    } finally {
      setSaving(false);
    }
  }

  return (
    <Modal
      open
      title={t('pages.nodes.editNode')}
      okText={t('save')}
      cancelText={t('cancel')}
      confirmLoading={saving}
      onOk={methods.handleSubmit(onFinish)}
      onCancel={onClose}
    >
      <FormProvider {...methods}>
        <Form layout="vertical">
          <Form.Item label={t('pages.nodes.name')}>{t('pages.inbounds.localPanel')}</Form.Item>
          <FormField
            label={t('pages.nodes.trafficMultiplier')}
            name="trafficMultiplier"
            tooltip={t('pages.nodes.trafficMultiplierHint')}
          >
            <InputNumber min={0} max={100} step={0.1} suffix="×" style={{ width: '100%' }} />
          </FormField>
        </Form>
      </FormProvider>
    </Modal>
  );
}
