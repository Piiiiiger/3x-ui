import { useCallback, useMemo, useState } from 'react';
import type { ReactNode } from 'react';
import { Card, ConfigProvider, Layout } from 'antd';

import { LanguageManager } from '@/utils';
import { useTheme } from '@/hooks/useTheme';
import './SubPage.css';

const RTL_LANGUAGES = new Set(['fa-IR', 'ar-EG']);

// The sub page pins the AA-safe coral shades (deeper than the panel's decorative
// coral in light mode) so its text-heavy controls stay readable. Mirrored in SubPage.css.
const ACCENT = {
  light: {
    primary: '#b5482d',
    hover: '#c25236',
    active: '#a4432d',
    rail: 'rgba(217, 119, 87, 0.16)',
  },
  dark: {
    primary: '#f18c6e',
    hover: '#f7b5a3',
    active: '#d97757',
    rail: 'rgba(241, 140, 110, 0.18)',
  },
};

// The subscription page keeps its own language choice, apart from the panel's.
export function useSubLanguage() {
  const [lang, setLang] = useState<string>(() => LanguageManager.getLanguage('subscription'));
  const onLangChange = useCallback((next: string) => {
    setLang(next);
    LanguageManager.setLanguage(next, 'subscription');
  }, []);
  return { lang, onLangChange };
}

// The themed frame shared by the subscription page and the client portal.
export default function SubShell({ lang, children }: { lang: string; children: ReactNode }) {
  const { isDark, isUltra, antdThemeConfig } = useTheme();
  const direction = RTL_LANGUAGES.has(lang) ? 'rtl' : 'ltr';
  const pageClass = ['subscription-page', isDark && 'is-dark', isUltra && 'is-ultra']
    .filter(Boolean)
    .join(' ');

  const themeConfig = useMemo(() => {
    const accent = isDark ? ACCENT.dark : ACCENT.light;
    const primary = {
      colorPrimary: accent.primary,
      colorPrimaryHover: accent.hover,
      colorPrimaryActive: accent.active,
    };
    return {
      ...antdThemeConfig,
      token: {
        ...antdThemeConfig.token,
        ...primary,
        colorLink: accent.primary,
        colorInfo: accent.primary,
      },
      components: {
        ...antdThemeConfig.components,
        Button: { ...antdThemeConfig.components?.Button, ...primary },
        Progress: { ...antdThemeConfig.components?.Progress, remainingColor: accent.rail },
      },
    };
  }, [antdThemeConfig, isDark]);

  return (
    <ConfigProvider theme={themeConfig} direction={direction}>
      <Layout className={pageClass} dir={direction}>
        <div className="sub-aurora" aria-hidden="true">
          <span className="sub-aurora-grid" />
        </div>
        <Layout.Content className="sub-content">
          <Card className="sub-card">{children}</Card>
        </Layout.Content>
      </Layout>
    </ConfigProvider>
  );
}
