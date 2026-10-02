import { useEffect } from 'react';
import { useLocation } from 'react-router';
import { useTranslation } from 'react-i18next';

import { PANEL_NAME } from '@/lib/brand';

const TITLE_KEYS: Record<string, string> = {
  '/': 'menu.dashboard',
  '/probe': 'menu.probe',
  '/inbounds': 'menu.inbounds',
  '/clients': 'menu.clients',
  '/plans': 'menu.plans',
  '/rules': 'menu.rules',
  '/nodes': 'menu.nodes',
  '/settings': 'menu.settings',
  '/xray': 'menu.xray',
  '/api-docs': 'menu.apiDocs',
  '/sponsors': 'menu.sponsors',
};

export function usePageTitle() {
  const { pathname } = useLocation();
  const { t } = useTranslation();

  useEffect(() => {
    const key = TITLE_KEYS[pathname] ?? TITLE_KEYS[`/${pathname.split('/')[1]}`];
    const title = key ? t(key) : PANEL_NAME;
    const host = window.location.hostname;
    document.title = host ? `${host} - ${title}` : title;
  }, [pathname, t]);
}
