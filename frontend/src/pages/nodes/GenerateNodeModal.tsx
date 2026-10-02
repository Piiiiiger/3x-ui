import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Alert,
  Button,
  Checkbox,
  Form,
  Input,
  InputNumber,
  Modal,
  Typography,
  message,
} from 'antd';
import { FormProvider, useForm, useWatch } from 'react-hook-form';
import { useQueryClient } from '@tanstack/react-query';

import { FormField } from '@/components/form/rhf';
import { keys } from '@/api/queryKeys';
import { useInboundOptions } from '@/api/queries/useInboundOptions';
import { usePlansQuery } from '@/api/queries/usePlansQuery';
import { buildClonePayload, cloneShareFor, type CloneShare } from '@/lib/xray/inbound-clone';
import { DBInbound, coerceInboundJsonField } from '@/models/dbinbound';
import { freshRealityFor } from '@/pages/inbounds/realityKeys';
import { HttpUtil } from '@/utils';

import {
  isNatHost,
  pickTemplate,
  pretickedPlans,
  uniqueNodeName,
  withRealityTarget,
} from './generateNode';

const JSON_HEADERS = { headers: { 'Content-Type': 'application/json' } } as const;

interface GenerateValues {
  name: string;
  port: number | null;
  publicPort: number | null;
  target: string;
  serverName: string;
  planIds: number[];
}

const EMPTY: GenerateValues = {
  name: '',
  port: null,
  publicPort: null,
  target: '',
  serverName: '',
  planIds: [],
};

interface GenerateNodeModalProps {
  open: boolean;
  /** The host the node is generated on; id 0 is the local panel. */
  host: { id: number; name: string; remark?: string };
  onClose: () => void;
}

/*
 * Generates the next node of a host by copying a VLESS REALITY node with fresh
 * keys, the host's own when it has one. Mount it per opening: it keeps no reset.
 */
export default function GenerateNodeModal({ open, host, onClose }: GenerateNodeModalProps) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [messageApi, messageContextHolder] = message.useMessage();
  const { data: options, isFetched: optionsFetched } = useInboundOptions();
  const { plans, fetched: plansFetched } = usePlansQuery();
  const methods = useForm<GenerateValues>({ defaultValues: EMPTY });
  const [template, setTemplate] = useState<DBInbound | null>(null);
  const [prefilled, setPrefilled] = useState(false);
  const [refusal, setRefusal] = useState('');
  const [saving, setSaving] = useState(false);

  const hostNodes = useMemo(
    () => (options ?? []).filter((o) => (o.nodeId ?? 0) === host.id),
    [options, host.id],
  );
  const templateOption = useMemo(() => pickTemplate(options ?? [], host.id), [options, host.id]);
  const nat = useMemo(() => isNatHost(hostNodes), [hostNodes]);
  const hostShare = useMemo(() => {
    const shares = new Map<number, CloneShare>();
    const first = hostNodes[0];
    if (first) {
      shares.set(host.id, {
        shareAddrStrategy: first.shareAddrStrategy || 'node',
        shareAddr: first.shareAddr || '',
      });
    }
    return shares;
  }, [hostNodes, host.id]);

  useEffect(() => {
    if (!open || prefilled || !templateOption || !plansFetched) return;
    let cancelled = false;
    void (async () => {
      const [source, freePort] = await Promise.all([
        HttpUtil.get(`/panel/api/inbounds/get/${templateOption.id}`, undefined, { silent: true }),
        HttpUtil.get<{ port: number }>(`/panel/api/inbounds/freePort/${host.id}`, undefined, {
          silent: true,
        }),
      ]);
      if (cancelled) return;
      if (!source?.success || !source.obj) {
        setRefusal(source?.msg || t('somethingWentWrong'));
        return;
      }
      const copied = new DBInbound(source.obj as ConstructorParameters<typeof DBInbound>[0]);
      const stream = coerceInboundJsonField(copied.streamSettings) as {
        realitySettings?: { target?: string; dest?: string; serverNames?: string[] };
      };
      setTemplate(copied);
      methods.reset({
        name: uniqueNodeName(
          hostNodes[0]?.remark || host.remark || host.name,
          (options ?? []).map((o) => o.remark ?? ''),
        ),
        port: freePort?.success && freePort.obj ? freePort.obj.port : null,
        publicPort: null,
        target: stream.realitySettings?.target || stream.realitySettings?.dest || '',
        serverName: stream.realitySettings?.serverNames?.[0] ?? '',
        planIds: pretickedPlans(
          plans,
          hostNodes.map((n) => n.id),
        ),
      });
      setPrefilled(true);
    })();
    return () => {
      cancelled = true;
    };
  }, [open, prefilled, templateOption, plansFetched, plans, host, hostNodes, options, methods, t]);

  const chosenPlans = useWatch({ control: methods.control, name: 'planIds' }) ?? [];
  const reach = plans
    .filter((p) => chosenPlans.includes(p.id))
    .reduce((sum, p) => sum + p.memberCount, 0);

  async function submit(values: GenerateValues) {
    if (!template) return;
    if (!values.name.trim() || !values.port) {
      messageApi.error(t('pages.nodes.generate.needNameAndPort'));
      return;
    }
    if (nat && !values.publicPort) {
      messageApi.error(t('pages.nodes.generate.publicPortNeeded'));
      return;
    }
    setSaving(true);
    setRefusal('');
    try {
      const fresh = await freshRealityFor(template);
      const copy = buildClonePayload(
        template,
        values.port,
        host.id > 0 ? host.id : null,
        cloneShareFor(template, host.id, hostShare),
        fresh,
      );
      const inbound = {
        ...copy,
        remark: values.name.trim(),
        enable: true,
        sharePort: values.publicPort ?? 0,
        streamSettings: withRealityTarget(
          copy.streamSettings,
          values.target.trim(),
          values.serverName.trim(),
        ),
      };
      const msg = await HttpUtil.post(
        '/panel/api/inbounds/generate',
        { inbound, planIds: values.planIds },
        { ...JSON_HEADERS, silent: true },
      );
      void queryClient.invalidateQueries({ queryKey: keys.inbounds.root() });
      void queryClient.invalidateQueries({ queryKey: keys.plans.root() });
      if (msg?.success) {
        messageApi.success(t('pages.nodes.generate.done', { host: host.name }));
        onClose();
      } else {
        setRefusal(msg?.msg || t('somethingWentWrong'));
      }
    } catch (e) {
      setRefusal(e instanceof Error ? e.message : String(e));
    } finally {
      setSaving(false);
    }
  }

  const noTemplate = optionsFetched && !templateOption;

  return (
    <Modal
      open={open}
      title={t('pages.nodes.generate.title', { host: host.name })}
      width="560px"
      onCancel={onClose}
      footer={[
        <Button key="cancel" onClick={onClose}>
          {t('cancel')}
        </Button>,
        <Button
          key="generate"
          type="primary"
          loading={saving}
          disabled={!template}
          onClick={methods.handleSubmit(submit)}
        >
          {t('pages.nodes.generate.submit')}
        </Button>,
      ]}
    >
      {messageContextHolder}
      {noTemplate && <Alert type="info" showIcon title={t('pages.nodes.generate.noTemplate')} />}
      {refusal && <Alert type="error" showIcon title={refusal} style={{ marginBottom: 12 }} />}
      <FormProvider {...methods}>
        <Form layout="vertical">
          <FormField name="name" label={t('pages.nodes.name')} required>
            <Input />
          </FormField>
          <FormField name="port" label={t('pages.inbounds.port')} required>
            <InputNumber min={1} max={65535} style={{ width: '100%' }} />
          </FormField>
          {nat && (
            <FormField
              name="publicPort"
              label={t('pages.nodes.generate.publicPort')}
              extra={t('pages.nodes.generate.publicPortHint')}
              required
            >
              <InputNumber min={1} max={65535} style={{ width: '100%' }} />
            </FormField>
          )}
          <FormField name="target" label={t('pages.nodes.generate.target')} required>
            <Input placeholder="www.example.com:443" />
          </FormField>
          <FormField name="serverName" label={t('pages.nodes.generate.sni')}>
            <Input />
          </FormField>
          <FormField name="planIds" label={t('pages.nodes.generate.plans')}>
            <Checkbox.Group options={plans.map((p) => ({ value: p.id, label: p.name }))} />
          </FormField>
          <Typography.Text type="secondary">
            {t('pages.nodes.generate.reach', { count: reach })}
          </Typography.Text>
        </Form>
      </FormProvider>
    </Modal>
  );
}
