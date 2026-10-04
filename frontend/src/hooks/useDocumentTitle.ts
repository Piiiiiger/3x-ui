import { useEffect } from 'react';

import { PANEL_NAME } from '@/lib/brand';

export function useDocumentTitle(page: string, username?: string) {
  useEffect(() => {
    document.title = [username, page === PANEL_NAME ? undefined : page, PANEL_NAME]
      .filter(Boolean)
      .join(' · ');
  }, [page, username]);
}
