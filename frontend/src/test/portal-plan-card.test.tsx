import { screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { PortalPlanCard } from '@/pages/sub/portal/PortalCards';
import { renderWithProviders } from './test-utils';

function facts(): Record<string, string> {
  const out: Record<string, string> = {};
  for (const row of document.querySelectorAll('.portal-facts > div')) {
    out[row.querySelector('dt')?.textContent ?? ''] = row.querySelector('dd')?.textContent ?? '';
  }
  return out;
}

describe('PortalPlanCard', () => {
  // The plan is named; the limits are the person's own, and a plan has no validity of its own.
  it("names the plan and shows the person's own quota, reset and IP limit", () => {
    renderWithProviders(
      <PortalPlanCard
        plan={{
          name: 'Monthly',
          totalGB: 50 * 1024 ** 3,
          trafficReset: 'monthly',
          trafficResetDay: 9,
          limitIp: 2,
        }}
      />,
    );
    expect(screen.getByText('Monthly')).toBeTruthy();
    expect(facts()).toEqual({
      Traffic: '50.00 GB',
      'Traffic Reset': 'Monthly on day 9',
      'IP Limit': '2',
    });
  });
});
