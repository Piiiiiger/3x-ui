import { fireEvent, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import CustomUserGroups from '@/pages/clients/CustomUserGroups';
import { HttpUtil, Msg } from '@/utils';
import { renderWithProviders } from './test-utils';
const get = vi.mocked(HttpUtil.get);
const post = vi.mocked(HttpUtil.post);
const originalGet = get.getMockImplementation();
const originalPost = post.getMockImplementation();
afterEach(() => {
  if (originalGet) get.mockImplementation(originalGet);
  if (originalPost) post.mockImplementation(originalPost);
});
it('filters by a custom group and creates a named group', async () => {
  get.mockResolvedValue(new Msg(true, '', [{ id: 7, name: 'Friends', emails: ['alice'] }]));
  post.mockResolvedValue(new Msg(true, '', null));
  const onChange = vi.fn();
  renderWithProviders(<CustomUserGroups selected={[]} onChange={onChange} onSaved={() => {}} />);
  fireEvent.click(await screen.findByText('Friends (1)'));
  expect(onChange).toHaveBeenCalledWith('7');
  fireEvent.click(screen.getByText('Ungrouped'));
  expect(onChange).toHaveBeenCalledWith('0');
  fireEvent.click(screen.getByText('Manage groups'));
  fireEvent.change(screen.getByRole('textbox', { name: 'Group name' }), {
    target: { value: 'Family' },
  });
  fireEvent.click(screen.getByText('Create group'));
  await waitFor(() =>
    expect(post).toHaveBeenCalledWith(
      '/panel/api/clients/customGroups/save',
      { id: 0, name: 'Family' },
      expect.anything(),
    ),
  );
});
