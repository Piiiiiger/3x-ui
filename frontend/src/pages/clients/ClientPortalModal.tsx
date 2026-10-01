import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Button, Descriptions, Form, Input, Modal, Space, Typography, message } from 'antd';
import { CopyOutlined, KeyOutlined, ReloadOutlined } from '@ant-design/icons';
import { FormProvider, useForm } from 'react-hook-form';
import { useQuery, useQueryClient } from '@tanstack/react-query';

import { FormField } from '@/components/form/rhf';
import { keys } from '@/api/queryKeys';
import { ClientPortalStatusSchema, type ClientPortalStatus } from '@/generated/zod';
import { ClipboardManager, HttpUtil, IntlUtil, RandomUtil } from '@/utils';
import { useDatepicker } from '@/hooks/useDatepicker';
import { parseMsg } from '@/utils/zodValidate';

const JSON_HEADERS = { headers: { 'Content-Type': 'application/json' } } as const;
// Mirrors the server's bounds; bcrypt ignores bytes past 72.
const PASSWORD_MIN = 6;
const PASSWORD_MAX = 72;

function portalApi(email: string) {
  return `/panel/api/clients/${encodeURIComponent(email)}/portal`;
}

async function fetchStatus(email: string): Promise<ClientPortalStatus | null> {
  const msg = await HttpUtil.get(portalApi(email), undefined, { silent: true });
  if (!msg?.success) throw new Error(msg?.msg || 'Failed to read the portal login');
  return parseMsg(msg, ClientPortalStatusSchema, 'clients/portal').obj ?? null;
}

interface ClientPortalModalProps {
  email: string | null;
  // The portal's address, or '' while the subscription server is off.
  portalUrl: string;
  onClose: () => void;
}

export default function ClientPortalModal({ email, portalUrl, onClose }: ClientPortalModalProps) {
  const { t } = useTranslation();
  const { datepicker } = useDatepicker();
  const queryClient = useQueryClient();
  const [messageApi, messageContextHolder] = message.useMessage();
  const [modal, modalContextHolder] = Modal.useModal();
  const methods = useForm<{ password: string }>({ defaultValues: { password: '' } });
  const [saving, setSaving] = useState(false);
  // The password just saved, shown once so it can be handed to the client.
  const [handout, setHandout] = useState('');
  const open = email !== null;

  const status = useQuery({
    queryKey: keys.clients.portal(email ?? ''),
    queryFn: () => fetchStatus(email ?? ''),
    enabled: open,
  });

  function close() {
    setHandout('');
    methods.reset({ password: '' });
    onClose();
  }

  function storeStatus(obj: unknown) {
    const parsed = ClientPortalStatusSchema.safeParse(obj);
    if (parsed.success && email) queryClient.setQueryData(keys.clients.portal(email), parsed.data);
  }

  async function save({ password }: { password: string }) {
    if (!email) return;
    setSaving(true);
    try {
      const msg = await HttpUtil.post(portalApi(email), { password }, JSON_HEADERS);
      if (msg?.success) {
        storeStatus(msg.obj);
        setHandout(password);
        methods.reset({ password: '' });
        messageApi.success(t('pages.clients.portal.savedToast'));
      }
    } finally {
      setSaving(false);
    }
  }

  function revoke() {
    if (!email) return;
    modal.confirm({
      title: t('pages.clients.portal.revokeConfirm', { email }),
      okText: t('pages.clients.portal.revoke'),
      okType: 'danger',
      cancelText: t('cancel'),
      onOk: async () => {
        const msg = await HttpUtil.post(`${portalApi(email)}/clear`, {}, JSON_HEADERS);
        if (msg?.success) {
          storeStatus(msg.obj);
          setHandout('');
          messageApi.success(t('pages.clients.portal.revoked'));
        }
      },
    });
  }

  async function copyHandout() {
    const text = t('pages.clients.portal.loginText', {
      url: portalUrl,
      username: email ?? '',
      password: handout,
    });
    if (await ClipboardManager.copyText(text)) messageApi.success(t('copied'));
  }

  const enabled = status.data?.enabled === true;
  const passwordError = methods.formState.errors.password?.message;
  return (
    <Modal
      open={open}
      title={t('pages.clients.portal.title')}
      width="560px"
      onCancel={close}
      footer={
        <Space>
          {enabled && (
            <Button danger onClick={revoke}>
              {t('pages.clients.portal.revoke')}
            </Button>
          )}
          <Button
            type="primary"
            icon={<KeyOutlined />}
            loading={saving}
            onClick={methods.handleSubmit(save)}
          >
            {t('pages.clients.portal.save')}
          </Button>
        </Space>
      }
    >
      {messageContextHolder}
      {modalContextHolder}
      <Typography.Paragraph type="secondary">
        {t('pages.clients.portal.intro')}
      </Typography.Paragraph>
      {!portalUrl && (
        <Alert
          type="warning"
          showIcon
          title={t('pages.clients.portal.subDisabled')}
          style={{ marginBottom: 16 }}
        />
      )}
      <Descriptions
        size="small"
        column={1}
        bordered
        items={[
          { key: 'user', label: t('subscription.portal.username'), children: email },
          {
            key: 'status',
            label: t('status'),
            children: enabled
              ? t('pages.clients.portal.statusOn', {
                  time: IntlUtil.formatDate(status.data?.updatedAt ?? 0, datepicker),
                })
              : t('pages.clients.portal.statusOff'),
          },
          ...(portalUrl
            ? [
                {
                  key: 'url',
                  label: t('pages.clients.portal.url'),
                  children: <Typography.Text copyable>{portalUrl}</Typography.Text>,
                },
              ]
            : []),
        ]}
        style={{ marginBottom: 16 }}
      />
      {handout && (
        <Alert
          type="success"
          showIcon
          title={t('pages.clients.portal.handout')}
          description={
            <Space orientation="vertical" size={4}>
              <Typography.Text code>{handout}</Typography.Text>
              <Button size="small" icon={<CopyOutlined />} onClick={copyHandout}>
                {t('pages.clients.portal.copyLogin')}
              </Button>
            </Space>
          }
          style={{ marginBottom: 16 }}
        />
      )}
      <FormProvider {...methods}>
        <Form layout="vertical">
          <Form.Item
            label={
              enabled ? t('pages.clients.portal.newPassword') : t('subscription.portal.password')
            }
            extra={t('pages.clients.portal.passwordHint')}
            validateStatus={passwordError ? 'error' : undefined}
            help={passwordError ? t(passwordError) : undefined}
          >
            <Space.Compact style={{ display: 'flex' }}>
              <FormField
                name="password"
                noStyle
                rules={{
                  validate: (value) => {
                    const length = new TextEncoder().encode(String(value ?? '')).length;
                    return (
                      (length >= PASSWORD_MIN && length <= PASSWORD_MAX) ||
                      'pages.clients.portal.passwordLength'
                    );
                  },
                }}
              >
                <Input.Password autoComplete="new-password" style={{ flex: 1 }} />
              </FormField>
              <Button
                aria-label={t('regenerate')}
                title={t('regenerate')}
                icon={<ReloadOutlined />}
                onClick={() =>
                  methods.setValue('password', RandomUtil.randomSeq(12), { shouldValidate: true })
                }
              />
            </Space.Compact>
          </Form.Item>
        </Form>
      </FormProvider>
    </Modal>
  );
}
