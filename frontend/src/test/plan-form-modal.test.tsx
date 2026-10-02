import { fireEvent, screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import PlanFormModal from '@/pages/plans/PlanFormModal';
import type { PlanSummary } from '@/generated/zod';
import { renderWithProviders } from './test-utils';

const plan: PlanSummary = {
  id: 7,
  name: 'Monthly',
  totalGB: 100 * 1024 ** 3,
  durationDays: 30,
  trafficReset: 'monthly',
  trafficResetDay: 1,
  limitIp: 2,
  remark: '',
  clashRules: '',
  inboundIds: [1, 2],
  memberCount: 3,
  sortIndex: 0,
  createdAt: 0,
  updatedAt: 0,
};

function save() {
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));
}

describe('PlanFormModal', () => {
  // Ticked by default, every edit re-stamped the members' limits, even one that only added a server.
  it('saves an edit without re-applying limits unless the box is ticked', async () => {
    const onConfirm = vi.fn();
    renderWithProviders(
      <PlanFormModal open plan={plan} onClose={() => {}} onConfirm={onConfirm} />,
    );
    const reapply = screen.getByRole('checkbox', {
      name: 'Also re-apply quota, IP limit and reset schedule to the 3 client(s) on this plan',
    }) as HTMLInputElement;
    expect(reapply.checked).toBe(false);
    expect(
      screen.getByText(
        "Servers you add or remove always reach this plan's clients, with or without this option.",
      ),
    ).toBeTruthy();

    save();
    await waitFor(() => expect(onConfirm).toHaveBeenCalledTimes(1));
    expect(onConfirm.mock.calls[0][1]).toBe(false);

    fireEvent.click(reapply);
    save();
    await waitFor(() => expect(onConfirm).toHaveBeenCalledTimes(2));
    expect(onConfirm.mock.calls[1][1]).toBe(true);
  });
});
