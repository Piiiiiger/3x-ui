import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';

// Stable per language: the columns memo depends on it, and a fresh function each
// render rebuilt every column, re-rendering all rows on each heartbeat push.
export function useRelativeTime() {
  const { t } = useTranslation();
  return useMemo(
    () => (unixSeconds?: number) => {
      if (!unixSeconds) return t('pages.nodes.never');
      const diffSec = Math.max(0, Math.floor(Date.now() / 1000 - unixSeconds));
      if (diffSec < 5) return t('pages.nodes.justNow');
      if (diffSec < 60) return `${diffSec}s`;
      if (diffSec < 3600) return `${Math.floor(diffSec / 60)}m`;
      if (diffSec < 86400) return `${Math.floor(diffSec / 3600)}h`;
      return `${Math.floor(diffSec / 86400)}d`;
    },
    [t],
  );
}
