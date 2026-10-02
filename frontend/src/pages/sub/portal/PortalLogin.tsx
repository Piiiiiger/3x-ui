import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Button, Form, Input } from 'antd';
import { LockOutlined, UserOutlined } from '@ant-design/icons';
import { FormProvider, useForm } from 'react-hook-form';

import { FormField, rhfZodValidate } from '@/components/form/rhf';
import { PANEL_NAME } from '@/lib/brand';
import {
  PortalLoginSchema,
  PortalRegistrationSchema,
  PortalAdminHandoffSchema,
  type PortalAuthValues,
} from '@/schemas/portal';
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
  const methods = useForm<PortalAuthValues>({
    defaultValues: { username: '', password: '', code: '', twoFactorCode: '' },
  });
  const [registering, setRegistering] = useState(false);
  const [adminMode, setAdminMode] = useState(false);
  const [errorKey, setErrorKey] = useState('');
  const [submitting, setSubmitting] = useState(false);

  async function onSubmit(values: PortalAuthValues) {
    setSubmitting(true);
    setErrorKey('');
    try {
      const res = await fetch(
        `${base}/${adminMode ? 'admin-login' : registering ? 'register' : 'login'}`,
        {
          method: 'POST',
          credentials: 'same-origin',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            username: values.username.trim(),
            password: values.password,
            ...(registering ? { code: values.code.trim() } : {}),
            ...(adminMode ? { twoFactorCode: values.twoFactorCode } : {}),
          }),
        },
      );
      const text = await res.text().catch(() => '');
      if (res.ok) {
        if (adminMode) {
          const handoff = PortalAdminHandoffSchema.parse(JSON.parse(text));
          const exchanged = await fetch(handoff.path, {
            method: 'POST',
            credentials: 'same-origin',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ token: handoff.token }),
          });
          const body = await exchanged.json();
          if (
            !exchanged.ok ||
            typeof body.redirect !== 'string' ||
            !/^\/(?!\/)/.test(body.redirect)
          )
            throw new Error('Admin session failed');
          window.location.assign(body.redirect);
          return;
        }
        onSignedIn();
        return;
      }
      if (adminMode && res.status === 403) setErrorKey('subscription.portal.adminRequires2FA');
      else if (registering && res.status === 409) setErrorKey('subscription.portal.taken');
      else if (registering && text.includes('"code"')) setErrorKey('subscription.portal.badCode');
      else setErrorKey(loginErrorKey(res.status));
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
        <h1 className="portal-login-title">
          {t(
            adminMode
              ? 'subscription.portal.adminSignIn'
              : registering
                ? 'subscription.portal.register'
                : 'subscription.portal.title',
          )}
        </h1>
        {!adminMode && (
          <p className="portal-login-intro">
            {t(registering ? 'subscription.portal.registerIntro' : 'subscription.portal.intro')}
          </p>
        )}
        {errorKey && (
          <Alert type="error" showIcon title={t(errorKey)} className="portal-login-error" />
        )}
        <FormProvider {...methods}>
          <Form layout="vertical" onFinish={methods.handleSubmit(onSubmit)}>
            <FormField
              name="username"
              label={t('subscription.portal.username')}
              rules={{
                validate: rhfZodValidate(
                  (registering ? PortalRegistrationSchema : PortalLoginSchema).shape.username,
                ),
              }}
            >
              <Input prefix={<UserOutlined />} autoComplete="username" size="large" autoFocus />
            </FormField>
            <FormField
              name="password"
              label={t('subscription.portal.password')}
              rules={{
                validate: rhfZodValidate(
                  (registering ? PortalRegistrationSchema : PortalLoginSchema).shape.password,
                ),
              }}
            >
              <Input.Password
                prefix={<LockOutlined />}
                autoComplete={registering ? 'new-password' : 'current-password'}
                size="large"
              />
            </FormField>
            {registering && (
              <FormField
                name="code"
                label={t('subscription.portal.code')}
                rules={{ validate: rhfZodValidate(PortalRegistrationSchema.shape.code) }}
              >
                <Input autoComplete="off" size="large" maxLength={64} />
              </FormField>
            )}
            {adminMode && (
              <FormField
                name="twoFactorCode"
                label={t('subscription.portal.twoFactorCode')}
                rules={{ required: t('subscription.portal.required') }}
              >
                <Input
                  autoComplete="one-time-code"
                  inputMode="numeric"
                  maxLength={6}
                  size="large"
                />
              </FormField>
            )}
            <Button type="primary" htmlType="submit" size="large" block loading={submitting}>
              {t(
                adminMode
                  ? 'subscription.portal.adminSignIn'
                  : registering
                    ? 'subscription.portal.register'
                    : 'subscription.portal.signIn',
              )}
            </Button>
            <Button
              type="link"
              block
              disabled={submitting}
              onClick={() => {
                setRegistering(!registering);
                setAdminMode(false);
                setErrorKey('');
                methods.clearErrors();
              }}
            >
              {t(registering ? 'subscription.portal.signIn' : 'subscription.portal.register')}
            </Button>
            <Button
              type="link"
              block
              disabled={submitting}
              onClick={() => {
                setAdminMode(!adminMode);
                setRegistering(false);
                setErrorKey('');
                methods.clearErrors();
              }}
            >
              {t(adminMode ? 'subscription.portal.signIn' : 'subscription.portal.adminSignIn')}
            </Button>
          </Form>
        </FormProvider>
      </div>
    </SubShell>
  );
}
