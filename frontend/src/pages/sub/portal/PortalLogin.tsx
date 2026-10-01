import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Button, Form, Input } from 'antd';
import { LockOutlined, UserOutlined } from '@ant-design/icons';
import { FormProvider, useForm } from 'react-hook-form';

import { FormField, rhfZodValidate } from '@/components/form/rhf';
import { PANEL_NAME } from '@/lib/brand';
import { PortalLoginSchema, type PortalLoginValues } from '@/schemas/portal';
import SubHeader from '../SubHeader';
import SubShell, { useSubLanguage } from '../SubShell';

// The sign-in endpoint answers with a status, mapped here to what the visitor reads.
function loginErrorKey(status: number): string {
  if (status === 401) return 'subscription.portal.invalid';
  if (status === 429) return 'subscription.portal.blocked';
  return 'subscription.portal.failed';
}

interface PortalLoginProps {
  base: string;
  onSignedIn: () => void;
}

export default function PortalLogin({ base, onSignedIn }: PortalLoginProps) {
  const { t } = useTranslation();
  const { lang, onLangChange } = useSubLanguage();
  const methods = useForm<PortalLoginValues>({ defaultValues: { username: '', password: '' } });
  const [errorKey, setErrorKey] = useState('');
  const [submitting, setSubmitting] = useState(false);

  async function onSubmit(values: PortalLoginValues) {
    setSubmitting(true);
    setErrorKey('');
    try {
      const res = await fetch(`${base}/login`, {
        method: 'POST',
        credentials: 'same-origin',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username: values.username.trim(), password: values.password }),
      });
      await res.text().catch(() => '');
      if (res.ok) {
        onSignedIn();
        return;
      }
      setErrorKey(loginErrorKey(res.status));
    } catch {
      setErrorKey('subscription.portal.failed');
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <SubShell lang={lang}>
      <SubHeader title={PANEL_NAME} sId="" email="" lang={lang} onLangChange={onLangChange} />
      <div className="portal-login">
        <h1 className="portal-login-title">{t('subscription.portal.title')}</h1>
        <p className="portal-login-intro">{t('subscription.portal.intro')}</p>
        {errorKey && (
          <Alert type="error" showIcon title={t(errorKey)} className="portal-login-error" />
        )}
        <FormProvider {...methods}>
          <Form layout="vertical" onFinish={methods.handleSubmit(onSubmit)}>
            <FormField
              name="username"
              label={t('subscription.portal.username')}
              rules={{ validate: rhfZodValidate(PortalLoginSchema.shape.username) }}
            >
              <Input prefix={<UserOutlined />} autoComplete="username" size="large" autoFocus />
            </FormField>
            <FormField
              name="password"
              label={t('subscription.portal.password')}
              rules={{ validate: rhfZodValidate(PortalLoginSchema.shape.password) }}
            >
              <Input.Password
                prefix={<LockOutlined />}
                autoComplete="current-password"
                size="large"
              />
            </FormField>
            <Button type="primary" htmlType="submit" size="large" block loading={submitting}>
              {t('subscription.portal.signIn')}
            </Button>
          </Form>
        </FormProvider>
      </div>
    </SubShell>
  );
}
