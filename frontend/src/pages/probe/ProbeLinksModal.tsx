import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Button, Flex, Form, Modal, Select, Spin, Typography } from 'antd';
import { FormProvider, useForm, useWatch } from 'react-hook-form';

import { FormField } from '@/components/form/rhf';
import { regionFlag } from '@/components/probe/regionFlag';
import { useProbeLinksQuery } from '@/api/queries/useProbeLinksQuery';
import { useProbeMutations } from '@/api/queries/useProbeMutations';
import type { ProbeLinkView, ProbeServer } from '@/generated/zod';
import { probeHostLabel } from './probeHostLabel';
import { useOpenings } from './useOpenings';
import './ProbeModals.css';

interface LinksFormValues {
  rows: { nodeId: number; serverId: string }[];
}

interface ProbeLinksFormProps {
  links: ProbeLinkView[];
  // null while Lite cannot be read: which servers it lists is then unknown.
  servers: ProbeServer[] | null;
  onClose: () => void;
  onSaved: () => void;
}

function ProbeLinksForm({ links, servers, onClose, onSaved }: ProbeLinksFormProps) {
  const { t } = useTranslation();
  const { saveLinks } = useProbeMutations();
  const methods = useForm<LinksFormValues>({
    defaultValues: { rows: links.map(({ nodeId, serverId }) => ({ nodeId, serverId })) },
  });
  const rows = useWatch({ control: methods.control, name: 'rows' });
  const [saving, setSaving] = useState(false);

  async function submit(values: LinksFormValues) {
    setSaving(true);
    try {
      const msg = await saveLinks(values.rows.filter((row) => row.serverId !== ''));
      if (msg?.success) onSaved();
    } finally {
      setSaving(false);
    }
  }

  return (
    <FormProvider {...methods}>
      <Form
        layout="horizontal"
        labelCol={{ xs: 24, sm: 10 }}
        wrapperCol={{ xs: 24, sm: 14 }}
        labelAlign="left"
        labelWrap
        colon={false}
        onFinish={methods.handleSubmit(submit)}
      >
        <Typography.Paragraph type="secondary">{t('pages.probe.linksIntro')}</Typography.Paragraph>
        {servers === null && (
          <Alert
            type="warning"
            showIcon
            title={t('pages.probe.linksUnreadable')}
            style={{ marginBottom: 16 }}
          />
        )}
        {links.map((host, index) => {
          const serverId = rows[index]?.serverId ?? '';
          const missing =
            servers !== null &&
            serverId !== '' &&
            !servers.some((server) => server.id === serverId);
          return (
            <FormField
              key={host.nodeId}
              name={['rows', index, 'serverId']}
              label={
                <span className="probe-link-host">
                  <span dir="auto">{probeHostLabel(host, t)}</span>
                  {host.address && <bdi className="probe-link-address">{host.address}</bdi>}
                </span>
              }
              extra={
                missing ? (
                  <Typography.Text type="danger">{t('pages.probe.linkMissing')}</Typography.Text>
                ) : undefined
              }
              transform={{ input: (value) => value || undefined, output: (value) => value ?? '' }}
            >
              <Select
                id={`probe-link-${host.nodeId}`}
                allowClear
                showSearch={{ optionFilterProp: 'label' }}
                placeholder={t('pages.probe.notLinked')}
                status={missing ? 'error' : undefined}
                options={(servers ?? []).map((server) => ({
                  value: server.id,
                  label: `${regionFlag(server.region)} ${server.name}`.trim(),
                  // The API refuses one server for two hosts.
                  disabled: rows.some((row, at) => at !== index && row.serverId === server.id),
                }))}
              />
            </FormField>
          );
        })}
        <Flex justify="end" gap={8}>
          <Button onClick={onClose}>{t('cancel')}</Button>
          <Button type="primary" htmlType="submit" loading={saving}>
            {t('save')}
          </Button>
        </Flex>
      </Form>
    </FormProvider>
  );
}

// Saving a form that failed to load would store an empty set and unlink every
// host, so the form exists only once the stored links have arrived.
function ProbeLinksBody(props: Omit<ProbeLinksFormProps, 'links'>) {
  const { links, fetchError } = useProbeLinksQuery();
  if (fetchError) return <Alert type="error" showIcon title={fetchError} />;
  if (!links) return <Spin />;
  return <ProbeLinksForm links={links} {...props} />;
}

interface ProbeLinksModalProps {
  open: boolean;
  servers: ProbeServer[] | null;
  onClose: () => void;
  onSaved: () => void;
}

export default function ProbeLinksModal({ open, servers, onClose, onSaved }: ProbeLinksModalProps) {
  const { t } = useTranslation();
  const openings = useOpenings(open);
  return (
    <Modal
      open={open}
      title={t('pages.probe.linkNodes')}
      width="600px"
      footer={null}
      destroyOnHidden
      onCancel={onClose}
    >
      <ProbeLinksBody key={openings} servers={servers} onClose={onClose} onSaved={onSaved} />
    </Modal>
  );
}
