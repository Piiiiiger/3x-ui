import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Alert,
  Button,
  Col,
  Form,
  Input,
  InputNumber,
  Modal,
  Row,
  Segmented,
  Select,
  Switch,
  message,
} from 'antd';
import { FormProvider, useForm, useWatch } from 'react-hook-form';
import type { NodeRecord } from '@/api/queries/useNodesQuery';
import type { RemoteInboundOption } from '@/api/queries/useNodeMutations';
import type { Msg } from '@/utils';
import { NodeFormSchema, type NodeFormValues, type ProbeResult } from '@/schemas/node';
import { FormField, rhfZodValidate } from '@/components/form/rhf';
import { useOutboundTagGroups } from '@/api/queries/useOutboundTags';
import type { AgentSecretView, ProbeServer } from '@/generated/zod';
import AgentSecretModal from './AgentSecretModal';
import './NodeFormModal.css';

type Mode = 'add' | 'edit';

interface NodeFormModalProps {
  open: boolean;
  mode: Mode;
  node: NodeRecord | null;
  probeServers?: Pick<ProbeServer, 'id' | 'name' | 'linked'>[];
  testConnection: (payload: Partial<NodeRecord>) => Promise<Msg<ProbeResult>>;
  fetchFingerprint: (payload: Partial<NodeRecord>) => Promise<Msg<string>>;
  fetchInbounds: (payload: Partial<NodeRecord>) => Promise<Msg<RemoteInboundOption[]>>;
  save: (payload: Partial<NodeRecord>) => Promise<Msg<unknown>>;
  mintAgentSecret: (id: number) => Promise<Msg<AgentSecretView>>;
  onOpenChange: (open: boolean) => void;
}

function defaultValues(): NodeFormValues {
  return {
    id: 0,
    name: '',
    remark: '',
    kind: 'panel',
    scheme: 'https',
    address: '',
    probeServerId: '',
    port: 2053,
    basePath: '/',
    apiToken: '',
    hasStoredToken: false,
    enable: true,
    allowPrivateAddress: false,
    tlsVerifyMode: 'verify',
    pinnedCertSha256: '',
    inboundSyncMode: 'all',
    inboundTags: [],
    outboundTag: '',
    trafficMultiplier: 1,
  };
}

export default function NodeFormModal({
  open,
  mode,
  node,
  probeServers = [],
  testConnection,
  fetchFingerprint,
  fetchInbounds,
  save,
  mintAgentSecret,
  onOpenChange,
}: NodeFormModalProps) {
  const { t } = useTranslation();
  const methods = useForm<NodeFormValues>({ defaultValues: defaultValues() });
  const [messageApi, messageContextHolder] = message.useMessage();
  const [modal, modalContextHolder] = Modal.useModal();
  const [shownSecret, setShownSecret] = useState<{ secret: string; name: string } | null>(null);

  const [submitting, setSubmitting] = useState(false);
  const [testing, setTesting] = useState(false);
  const [fetchingPin, setFetchingPin] = useState(false);
  const [fetchingInbounds, setFetchingInbounds] = useState(false);
  const [inboundOptions, setInboundOptions] = useState<RemoteInboundOption[]>([]);
  const [testResult, setTestResult] = useState<ProbeResult | null>(null);
  const scheme = useWatch({ control: methods.control, name: 'scheme' }) ?? 'https';
  const tlsVerifyMode = useWatch({ control: methods.control, name: 'tlsVerifyMode' }) ?? 'verify';
  const inboundSyncMode = useWatch({ control: methods.control, name: 'inboundSyncMode' }) ?? 'all';
  const isAgent = (useWatch({ control: methods.control, name: 'kind' }) ?? 'panel') === 'agent';
  const { data: outboundGroups } = useOutboundTagGroups({ excludeBlackhole: true });

  // Outbounds and balancers share one picker (like the panel-outbound selector);
  // when balancers exist they get a labeled group so it's clear the selection
  // routes through a balancer. Empty falls back to the placeholder ("Direct
  // connection") rather than a synthetic option, so it can't read as a second
  // "direct" next to a real freedom outbound.
  const outboundOptions = useMemo<
    (
      | { label: string; value: string }
      | { label: string; options: { label: string; value: string }[] }
    )[]
  >(() => {
    const outOpts = (outboundGroups?.outbounds ?? []).map((tag) => ({ label: tag, value: tag }));
    if (!outboundGroups?.balancers.length) return outOpts;
    return [
      { label: t('pages.xray.Outbounds'), options: outOpts },
      {
        label: t('pages.xray.Balancers'),
        options: outboundGroups.balancers.map((tag) => ({ label: tag, value: tag })),
      },
    ];
  }, [outboundGroups, t]);

  // Reset during render, not in an effect, so the first frame is already clean.
  const [synced, setSynced] = useState<{ mode: string; node: NodeRecord | null } | null>(null);
  if (!open) {
    if (synced) setSynced(null);
  } else if (!synced || synced.mode !== mode || synced.node !== (node ?? null)) {
    setSynced({ mode, node: node ?? null });
    const base = defaultValues();
    const next: NodeFormValues =
      mode === 'edit' && node
        ? {
            ...base,
            ...(node as unknown as Partial<NodeFormValues>),
            id: node.id,
            kind: node.kind ?? base.kind,
            scheme: (node.scheme as 'http' | 'https') || base.scheme,
            inboundSyncMode: (node.inboundSyncMode as 'all' | 'selected') || base.inboundSyncMode,
            inboundTags: node.inboundTags ?? [],
            apiToken: '',
            hasStoredToken: node.hasApiToken ?? false,
          }
        : base;
    if (next.scheme === 'http') next.tlsVerifyMode = 'skip';
    methods.reset(next);
    setInboundOptions((next.inboundTags || []).map((tag) => ({ tag })));
    setTestResult(null);
  }

  const title = useMemo(
    () => (mode === 'edit' ? t('pages.nodes.editNode') : t('pages.nodes.addNode')),
    [mode, t],
  );

  const editingWithToken = mode === 'edit' && Boolean(node?.hasApiToken);

  function buildPayload(values: NodeFormValues): Partial<NodeRecord> {
    if (values.kind === 'agent') {
      return {
        id: values.id || 0,
        kind: 'agent',
        name: values.name.trim(),
        remark: values.remark?.trim() || '',
        address: values.address.trim(),
        enable: values.enable,
        trafficMultiplier: values.trafficMultiplier,
        ...(values.probeServerId ? { probeServerId: values.probeServerId } : {}),
      };
    }
    const token = values.apiToken.trim();
    const payload: Partial<NodeRecord> = {
      id: values.id || 0,
      kind: 'panel',
      name: values.name.trim(),
      remark: values.remark?.trim() || '',
      scheme: values.scheme,
      address: values.address.trim(),
      port: values.port,
      basePath: values.basePath.trim() || '/',
      enable: values.enable,
      allowPrivateAddress: values.allowPrivateAddress,
      tlsVerifyMode: values.tlsVerifyMode,
      pinnedCertSha256: values.tlsVerifyMode === 'pin' ? values.pinnedCertSha256.trim() : '',
      inboundSyncMode: values.inboundSyncMode,
      inboundTags: values.inboundSyncMode === 'selected' ? values.inboundTags : [],
      outboundTag: values.outboundTag || '',
    };
    if (token) payload.apiToken = token;
    return payload;
  }

  async function onTest() {
    if (!(await methods.trigger(['name', 'address', 'port']))) return;
    setTesting(true);
    setTestResult(null);
    try {
      const payload = buildPayload(methods.getValues());
      const msg = await testConnection(payload);
      if (msg?.success && msg.obj) {
        setTestResult(msg.obj);
      } else {
        setTestResult({ status: 'offline', error: msg?.msg || 'unknown error' });
      }
    } finally {
      setTesting(false);
    }
  }

  async function onFetchPin() {
    if (!(await methods.trigger(['name', 'address', 'port']))) return;
    setFetchingPin(true);
    try {
      const payload = buildPayload(methods.getValues());
      const msg = await fetchFingerprint(payload);
      if (msg?.success && msg.obj) {
        methods.setValue('pinnedCertSha256', msg.obj);
        messageApi.success(t('pages.nodes.pinFetched'));
      } else {
        messageApi.error(msg?.msg || t('pages.nodes.pinFetchFailed'));
      }
    } finally {
      setFetchingPin(false);
    }
  }

  async function onFetchInbounds() {
    if (!(await methods.trigger(['name', 'address', 'port', 'apiToken']))) return;
    setFetchingInbounds(true);
    try {
      const msg = await fetchInbounds(buildPayload(methods.getValues()));
      if (msg?.success && Array.isArray(msg.obj)) {
        setInboundOptions(msg.obj);
        messageApi.success(t('pages.nodes.inboundsLoaded', { count: msg.obj.length }));
      } else {
        messageApi.error(msg?.msg || t('pages.nodes.inboundsLoadFailed'));
      }
    } finally {
      setFetchingInbounds(false);
    }
  }

  // A secret is shown once; the agent on that server needs it to dial in.
  async function revealNewSecret(id: number, name: string) {
    const msg = await mintAgentSecret(id);
    if (msg?.success && msg.obj) {
      setShownSecret({ secret: msg.obj.secret, name });
    } else {
      messageApi.error(msg?.msg || t('pages.nodes.agentSecretFailed'));
    }
  }

  function confirmNewSecret() {
    if (!node?.id) return;
    const { id, name } = node;
    modal.confirm({
      title: t('pages.nodes.agentNewSecret'),
      content: t('pages.nodes.agentNewSecretConfirm'),
      okText: t('confirm'),
      cancelText: t('cancel'),
      onOk: () => revealNewSecret(id, name || ''),
    });
  }

  async function saveAgent(payload: Partial<NodeRecord>) {
    const msg = await save(payload);
    if (!msg?.success) return;
    onOpenChange(false);
    const created = (msg.obj as { id?: number } | null)?.id;
    const id = mode === 'edit' ? node?.id : created;
    // A new agent, or a panel node turned into one, has no secret yet.
    if (id && (mode === 'add' || node?.kind !== 'agent')) {
      await revealNewSecret(id, payload.name || '');
    }
  }

  async function onFinish(values: NodeFormValues) {
    const result = NodeFormSchema.safeParse(values);
    if (!result.success) {
      messageApi.error(t(result.error.issues[0]?.message ?? 'pages.nodes.toasts.fillRequired'));
      return;
    }
    setSubmitting(true);
    try {
      const payload = buildPayload(result.data);
      if (result.data.kind === 'agent') {
        await saveAgent(payload);
        return;
      }
      const test = await testConnection(payload);
      const probe = test?.success ? test.obj : null;
      if (!probe || probe.status !== 'online') {
        setTestResult(
          probe ?? { status: 'offline', error: test?.msg || t('pages.nodes.connectionFailed') },
        );
        return;
      }
      setTestResult(probe);
      const msg = await save(payload);
      if (msg?.success) {
        onOpenChange(false);
      }
    } finally {
      setSubmitting(false);
    }
  }

  function close() {
    if (!submitting) onOpenChange(false);
  }

  return (
    <>
      {messageContextHolder}
      {modalContextHolder}
      <AgentSecretModal
        secret={shownSecret?.secret ?? null}
        nodeName={shownSecret?.name ?? ''}
        onClose={() => setShownSecret(null)}
      />
      <Modal
        open={open}
        title={title}
        confirmLoading={submitting}
        okText={t('save')}
        cancelText={t('cancel')}
        mask={{ closable: false }}
        width="640px"
        onOk={methods.handleSubmit(onFinish)}
        onCancel={close}
      >
        <FormProvider {...methods}>
          <Form layout="vertical">
            <FormField
              label={t('pages.nodes.kind')}
              name="kind"
              tooltip={t('pages.nodes.kindHint')}
              onAfterChange={(value) => {
                if (value !== 'agent') methods.setValue('probeServerId', '');
              }}
            >
              <Segmented
                block
                options={[
                  { value: 'panel', label: t('pages.nodes.kindPanel') },
                  { value: 'agent', label: t('pages.nodes.kindAgent') },
                ]}
              />
            </FormField>

            {mode === 'add' && probeServers.some((server) => !server.linked) && (
              <FormField
                name="probeServerId"
                label={t('pages.nodes.fromProbe')}
                onAfterChange={(value) => {
                  const server = probeServers.find((server) => server.id === value);
                  if (server) {
                    methods.setValue('name', server.name);
                    methods.setValue('kind', 'agent');
                  }
                }}
              >
                <Select
                  allowClear
                  showSearch={{ optionFilterProp: 'label' }}
                  options={probeServers
                    .filter((server) => !server.linked)
                    .map((server) => ({ value: server.id, label: server.name }))}
                />
              </FormField>
            )}

            <Row gutter={16}>
              <Col xs={24} md={12}>
                <FormField
                  label={t('pages.nodes.name')}
                  name="name"
                  rules={{ validate: rhfZodValidate(NodeFormSchema.shape.name) }}
                >
                  <Input placeholder={t('pages.nodes.namePlaceholder')} />
                </FormField>
              </Col>
              <Col xs={24} md={12}>
                <FormField label={t('pages.nodes.remark')} name="remark">
                  <Input />
                </FormField>
              </Col>
            </Row>

            <Row gutter={16}>
              {!isAgent && (
                <Col xs={24} md={6}>
                  <FormField
                    label={t('pages.nodes.scheme')}
                    name="scheme"
                    onAfterChange={(value) => {
                      if (value === 'http') methods.setValue('tlsVerifyMode', 'skip');
                    }}
                  >
                    <Select
                      options={[
                        { value: 'https', label: 'https' },
                        { value: 'http', label: 'http' },
                      ]}
                    />
                  </FormField>
                </Col>
              )}
              <Col xs={24} md={isAgent ? 24 : 12}>
                <FormField
                  label={isAgent ? t('pages.nodes.publicAddress') : t('pages.nodes.address')}
                  tooltip={isAgent ? t('pages.nodes.publicAddressAutoHint') : undefined}
                  name="address"
                  rules={{ required: !isAgent && t('pages.nodes.toasts.fillRequired') }}
                >
                  <Input placeholder={t('pages.nodes.addressPlaceholder')} />
                </FormField>
              </Col>
              {!isAgent && (
                <Col xs={24} md={6}>
                  <FormField
                    label={t('pages.nodes.port')}
                    name="port"
                    rules={{ validate: rhfZodValidate(NodeFormSchema.shape.port) }}
                  >
                    <InputNumber min={1} max={65535} style={{ width: '100%' }} />
                  </FormField>
                </Col>
              )}
            </Row>

            <Row gutter={16}>
              {!isAgent && (
                <Col xs={24} md={12}>
                  <FormField label={t('pages.nodes.basePath')} name="basePath">
                    <Input placeholder="/" />
                  </FormField>
                </Col>
              )}
              <Col xs={24} md={12}>
                <FormField label={t('pages.nodes.enable')} name="enable" valueProp="checked">
                  <Switch />
                </FormField>
              </Col>
              {isAgent && (
                <Col xs={24} md={12}>
                  <FormField
                    label={t('pages.nodes.trafficMultiplier')}
                    name="trafficMultiplier"
                    tooltip={t('pages.nodes.trafficMultiplierHint')}
                  >
                    <InputNumber
                      min={0}
                      max={100}
                      step={0.1}
                      suffix="×"
                      style={{ width: '100%' }}
                    />
                  </FormField>
                </Col>
              )}
            </Row>

            {isAgent && (
              <>
                <Alert
                  type="info"
                  showIcon
                  style={{ marginBottom: 16 }}
                  title={t('pages.nodes.agentHint')}
                />
                {mode === 'edit' && node?.kind === 'agent' && (
                  <Button onClick={confirmNewSecret}>{t('pages.nodes.agentNewSecret')}</Button>
                )}
              </>
            )}

            {!isAgent && (
              <>
                <FormField
                  label={t('pages.nodes.allowPrivateAddress')}
                  name="allowPrivateAddress"
                  valueProp="checked"
                  tooltip={t('pages.nodes.allowPrivateAddressHint')}
                >
                  <Switch />
                </FormField>

                <FormField
                  label={t('pages.nodes.tlsVerifyMode')}
                  name="tlsVerifyMode"
                  tooltip={t('pages.nodes.tlsVerifyModeHint')}
                >
                  <Select
                    disabled={scheme === 'http'}
                    options={[
                      { value: 'verify', label: t('pages.nodes.tlsVerify') },
                      { value: 'pin', label: t('pages.nodes.tlsPin') },
                      { value: 'skip', label: t('pages.nodes.tlsSkip') },
                      { value: 'mtls', label: t('pages.nodes.tlsMtls') },
                    ]}
                  />
                </FormField>

                {tlsVerifyMode === 'skip' && (
                  <Alert
                    type="warning"
                    showIcon
                    style={{ marginBottom: 16 }}
                    title={t('pages.nodes.tlsSkipWarning')}
                  />
                )}

                {tlsVerifyMode === 'mtls' && (
                  <Alert
                    type="info"
                    showIcon
                    style={{ marginBottom: 16 }}
                    title={t('pages.nodes.mtlsFormHint')}
                  />
                )}

                {tlsVerifyMode === 'pin' && (
                  <FormField
                    label={t('pages.nodes.pinnedCert')}
                    name="pinnedCertSha256"
                    tooltip={t('pages.nodes.pinnedCertHint')}
                  >
                    <Input.Search
                      placeholder={t('pages.nodes.pinnedCertPlaceholder')}
                      enterButton={t('pages.nodes.fetchPin')}
                      loading={fetchingPin}
                      onSearch={onFetchPin}
                    />
                  </FormField>
                )}

                <FormField
                  label={t('pages.nodes.apiToken')}
                  name="apiToken"
                  rules={{ validate: rhfZodValidate(NodeFormSchema.shape.apiToken) }}
                  tooltip={t('pages.nodes.apiTokenHint')}
                  extra={editingWithToken ? t('pages.nodes.apiTokenKeepHint') : undefined}
                >
                  <Input.Password
                    placeholder={
                      editingWithToken
                        ? t('pages.nodes.apiTokenKeepHint')
                        : t('pages.nodes.apiTokenPlaceholder')
                    }
                  />
                </FormField>

                <FormField
                  label={t('pages.nodes.outboundTag')}
                  name="outboundTag"
                  tooltip={t('pages.nodes.outboundTagHint')}
                  transform={{ input: (v) => (v as string) || undefined }}
                >
                  <Select
                    allowClear
                    showSearch
                    placeholder={t('pages.nodes.outboundTagPlaceholder')}
                    options={outboundOptions}
                  />
                </FormField>

                <FormField
                  label={t('pages.nodes.inboundSyncMode')}
                  name="inboundSyncMode"
                  tooltip={t('pages.nodes.inboundSyncModeHint')}
                >
                  <Select
                    options={[
                      { value: 'all', label: t('pages.nodes.allInbounds') },
                      { value: 'selected', label: t('pages.nodes.selectedInbounds') },
                    ]}
                  />
                </FormField>

                {inboundSyncMode === 'selected' && (
                  <FormField
                    label={t('pages.nodes.inboundTags')}
                    name="inboundTags"
                    tooltip={t('pages.nodes.inboundTagsHint')}
                  >
                    <Select
                      mode="multiple"
                      allowClear
                      loading={fetchingInbounds}
                      placeholder={t('pages.nodes.inboundTagsPlaceholder')}
                      popupRender={(menu) => (
                        <>
                          <Button
                            type="text"
                            block
                            loading={fetchingInbounds}
                            onClick={onFetchInbounds}
                          >
                            {t('pages.nodes.loadInbounds')}
                          </Button>
                          {menu}
                        </>
                      )}
                      options={inboundOptions.map((inbound) => ({
                        value: inbound.tag,
                        label: `${inbound.remark || inbound.tag}${inbound.protocol ? ` (${inbound.protocol}:${inbound.port || 0})` : ''}`,
                      }))}
                    />
                  </FormField>
                )}

                <div className="test-row">
                  <Button type="default" loading={testing} onClick={onTest}>
                    {t('pages.nodes.testConnection')}
                  </Button>
                  {testResult && (
                    <div className="test-result">
                      {testResult.status === 'online' ? (
                        <Alert
                          type="success"
                          showIcon
                          title={t('pages.nodes.connectionOk', { ms: testResult.latencyMs })}
                          description={
                            testResult.xrayVersion ? `Xray ${testResult.xrayVersion}` : undefined
                          }
                        />
                      ) : (
                        <Alert
                          type="error"
                          showIcon
                          title={t('pages.nodes.connectionFailed')}
                          description={testResult.error}
                        />
                      )}
                    </div>
                  )}
                </div>
              </>
            )}
          </Form>
        </FormProvider>
      </Modal>
    </>
  );
}
