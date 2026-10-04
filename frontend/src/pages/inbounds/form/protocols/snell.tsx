import { Alert, Button, Form, Input, Switch } from 'antd';
import { useTranslation } from 'react-i18next';
import { useFormContext } from 'react-hook-form';
import { FormField } from '@/components/form/rhf';
import { generateSnellPsk } from '@/schemas/protocols/inbound/snell';

export default function SnellFields() {
  const { t } = useTranslation();
  const { setValue } = useFormContext();
  return (
    <>
      <Alert
        type="info"
        showIcon
        title={t('pages.inbounds.snell.managed')}
        description={t('pages.inbounds.snell.help')}
        style={{ marginBottom: 20 }}
      />
      <Form.Item label={t('pages.inbounds.protocol')}>
        <Input value="Snell v5 · TCP / UDP" readOnly />
      </Form.Item>
      <FormField
        name="settings.psk"
        label={t('pages.inbounds.snell.psk')}
        rules={{
          required: true,
          pattern: {
            value: /^[A-Za-z0-9_+/=-]{16,256}$/,
            message: t('pages.inbounds.snell.pskHelp'),
          },
        }}
      >
        <Input.Password autoComplete="new-password" />
      </FormField>
      <Form.Item>
        <Button onClick={() => setValue('settings.psk', generateSnellPsk(), { shouldDirty: true })}>
          {t('pages.inbounds.snell.regenerate')}
        </Button>
      </Form.Item>
      <FormField name="settings.ipv6" label="IPv6" valueProp="checked">
        <Switch />
      </FormField>
      <FormField name="settings.reuse" label={t('pages.inbounds.snell.reuse')} valueProp="checked">
        <Switch />
      </FormField>
    </>
  );
}
