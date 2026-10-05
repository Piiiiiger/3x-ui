import { lazy, Suspense } from 'react';
import { Navigate, createBrowserRouter, type RouteObject } from 'react-router';
import { Spin } from 'antd';

import PanelLayout from '@/layouts/PanelLayout';
import RouteLoadError from '@/components/feedback/RouteLoadError';

const IndexPage = lazy(() => import('@/pages/index/IndexPage'));
const InboundsPage = lazy(() => import('@/pages/inbounds/InboundsPage'));
const ClientsPage = lazy(() => import('@/pages/clients/ClientsPage'));
const PlansPage = lazy(() => import('@/pages/plans/PlansPage'));
const RulesPage = lazy(() => import('@/pages/rules/RulesPage'));
const NodesPage = lazy(() => import('@/pages/nodes/NodesPage'));
const HostPage = lazy(() => import('@/pages/nodes/HostPage'));
const SettingsPage = lazy(() => import('@/pages/settings/SettingsPage'));
const XrayPage = lazy(() => import('@/pages/xray/XrayPage'));
const ChainsPage = lazy(() => import('@/pages/chains/ChainsPage'));
const AiUsagePage = lazy(() => import('@/pages/ai-usage/AiUsagePage'));

function withSuspense(node: React.ReactNode) {
  return (
    <Suspense
      fallback={
        <div
          style={{
            display: 'flex',
            justifyContent: 'center',
            alignItems: 'center',
            minHeight: '60vh',
          }}
        >
          <Spin size="large" />
        </div>
      }
    >
      {node}
    </Suspense>
  );
}

const routes: RouteObject[] = [
  {
    path: '/',
    element: <PanelLayout />,
    errorElement: <RouteLoadError />,
    children: [
      { index: true, element: withSuspense(<IndexPage />) },
      // The probe's servers moved onto the hosts page; old links and bookmarks follow them.
      { path: 'probe', element: <Navigate to="/nodes" replace /> },
      { path: 'inbounds', element: withSuspense(<InboundsPage />) },
      { path: 'clients', element: withSuspense(<ClientsPage />) },
      { path: 'plans', element: withSuspense(<PlansPage />) },
      { path: 'rules', element: withSuspense(<RulesPage />) },
      { path: 'ai-usage', element: withSuspense(<AiUsagePage />) },
      { path: 'nodes', element: withSuspense(<NodesPage />) },
      { path: 'nodes/:hostId', element: withSuspense(<HostPage />) },
      { path: 'settings', element: withSuspense(<SettingsPage />) },
      { path: 'xray', element: withSuspense(<XrayPage />) },
      { path: 'chains', element: withSuspense(<ChainsPage />) },
    ],
  },
];

function computeBasename() {
  const raw = (typeof window !== 'undefined' && window.X_UI_BASE_PATH) || '/';
  const trimmed = raw.replace(/\/+$/, '');
  return `${trimmed}/panel`;
}

export const router = createBrowserRouter(routes, {
  basename: computeBasename(),
});
