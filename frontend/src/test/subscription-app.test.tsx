import { fireEvent, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import SubscriptionApp from '@/pages/sub/SubscriptionApp';
import { renderWithProviders } from './test-utils';

afterEach(() => vi.unstubAllGlobals());

describe('subscription overview and probe', () => {
  it('shows the scheduled reset and loads the probe only when selected', async () => {
    const request = vi
      .fn()
      .mockResolvedValue(
        new Response('{"enabled":true,"fetchedAt":0,"stale":false,"servers":[]}', { status: 200 }),
      );
    vi.stubGlobal('fetch', request);
    renderWithProviders(
      <SubscriptionApp
        data={{
          sId: 'subscription-a',
          enabled: true,
          probeBase: '/x/subscription-a',
          nextReset: Date.UTC(2026, 10, 1),
        }}
      />,
    );
    expect(screen.getByText('Next traffic reset')).toBeTruthy();
    expect(request).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('radio', { name: 'Probe' }));
    await waitFor(() => expect(request).toHaveBeenCalledOnce());
    expect(request.mock.calls[0][0]).toBe('/x/subscription-a/probe');
    fireEvent.click(screen.getByRole('radio', { name: 'Overview' }));
    expect(screen.getByText('Next traffic reset')).toBeTruthy();
  });

  it('offers no probe or reset date when neither exists', () => {
    renderWithProviders(<SubscriptionApp data={{ enabled: true }} />);
    expect(screen.queryByRole('radio', { name: 'Probe' })).toBeNull();
    expect(screen.queryByText('Next traffic reset')).toBeNull();
  });
});
