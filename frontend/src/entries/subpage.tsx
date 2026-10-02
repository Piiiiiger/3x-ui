import { createRoot } from 'react-dom/client';
import { message } from 'antd';
import 'antd/dist/reset.css';

import { readyI18n } from '@/i18n/react';
import { ThemeProvider } from '@/hooks/useTheme';
import { QueryProvider } from '@/api/QueryProvider';
import SubscriptionApp from '@/pages/sub/SubscriptionApp';
import PortalApp from '@/pages/sub/portal/PortalApp';

const messageContainer = document.getElementById('message');
if (messageContainer) {
  message.config({ getContainer: () => messageContainer });
}

readyI18n('subscription').then(() => {
  const root = document.getElementById('app');
  if (root) {
    createRoot(root).render(
      <ThemeProvider>
        <QueryProvider>
          {window.__SUB_PORTAL__ ? (
            <PortalApp base={window.__SUB_PORTAL__.base} />
          ) : (
            <SubscriptionApp data={window.__SUB_PAGE_DATA__ ?? {}} />
          )}
        </QueryProvider>
      </ThemeProvider>,
    );
  }
});
