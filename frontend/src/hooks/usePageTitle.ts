import { useLocation } from 'react-router';
import { useTranslation } from 'react-i18next';

import { PANEL_NAME } from '@/lib/brand';
import { useDocumentTitle } from './useDocumentTitle';

const TITLE_KEYS: Record<string, string> = {
  '/': 'menu.dashboard',
  '/inbounds': 'menu.inbounds',
  '/clients': 'menu.clients',
  '/plans': 'menu.plans',
  '/rules': 'menu.rules',
  '/nodes': 'menu.nodes',
  '/settings': 'menu.settings',
  '/xray': 'menu.xray',
};

export function usePageTitle() {
  const { pathname } = useLocation();
  const { t } = useTranslation();

  const key = TITLE_KEYS[pathname] ?? TITLE_KEYS[`/${pathname.split('/')[1]}`];
  useDocumentTitle(key ? t(key) : PANEL_NAME);
}
