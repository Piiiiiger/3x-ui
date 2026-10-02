import { fireEvent, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import ActivationCodesModal from '@/pages/plans/ActivationCodesModal';
import { HttpUtil, Msg } from '@/utils';
import { renderWithProviders } from './test-utils';

const plan = {
  id: 2,
  name: 'Standard',
  limitIp: 0,
  remark: '',
  templateId: 0,
  inboundIds: [1],
  memberCount: 0,
  sortIndex: 0,
  createdAt: 0,
  updatedAt: 0,
};
const code = {
  id: 3,
  code: 'ABCD-EFGH-JKLM-NPQR',
  planId: 2,
  totalGB: 10 * 1024 ** 3,
  days: 30,
  resetDay: 22,
  note: '',
  usedAt: 0,
  usedBy: '',
  createdAt: 0,
};
const get = vi.mocked(HttpUtil.get);
const post = vi.mocked(HttpUtil.post);
const originalGet = get.getMockImplementation();
const originalPost = post.getMockImplementation();
afterEach(() => {
  if (originalGet) get.mockImplementation(originalGet);
  if (originalPost) post.mockImplementation(originalPost);
});

describe('activation codes', () => {
  it('creates codes with a per-user quota and refreshes the list', async () => {
    get.mockResolvedValue(new Msg(true, '', []));
    post.mockImplementation(async () => {
      get.mockResolvedValue(new Msg(true, '', [code]));
      return new Msg(true, '', [code]);
    });
    renderWithProviders(<ActivationCodesModal plan={plan} onClose={() => {}} />);
    expect(screen.getByRole('dialog', { name: 'Activation codes · Standard' })).toBeTruthy();
    fireEvent.change(screen.getByLabelText('Traffic'), { target: { value: '10' } });
    fireEvent.change(screen.getByLabelText('Monthly reset day'), { target: { value: '22' } });
    fireEvent.click(screen.getByRole('button', { name: 'Create codes' }));
    await waitFor(() => expect(screen.getByText(code.code)).toBeTruthy());
    expect(post).toHaveBeenCalledWith(
      '/panel/api/plans/codes/add',
      { planId: 2, count: 1, totalGB: 10 * 1024 ** 3, days: 30, resetDay: 22, note: '' },
      expect.anything(),
    );
  });
});
