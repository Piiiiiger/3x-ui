import { fireEvent, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { afterEach, describe, expect, it, vi } from 'vitest';

import RulesPage from '@/pages/rules/RulesPage';
import { HttpUtil, Msg } from '@/utils';
import { renderWithProviders } from './test-utils';

// The top bar has its own tests and needs settings the page itself does not.
vi.mock('@/layouts/AppNav', () => ({ default: () => null }));

const TEMPLATES = [
  {
    id: 3,
    name: 'alpha_v3',
    isDefault: true,
    kind: 'yaml',
    size: 2048,
    planCount: 2,
    updatedAt: 0,
  },
  {
    id: 5,
    name: 'remote_rules',
    isDefault: false,
    kind: 'remote',
    size: 40,
    planCount: 0,
    updatedAt: 0,
  },
];
const PLANS = [
  { id: 1, name: 'Empty', memberCount: 0, inboundIds: [], templateId: 0 },
  { id: 2, name: 'Monthly', memberCount: 3, inboundIds: [], templateId: 3 },
];

// The setup file stubs HttpUtil for every test; both stubs are put back afterwards.
const getStub = vi.mocked(HttpUtil.get);
const postStub = vi.mocked(HttpUtil.post);
const setupGet = getStub.getMockImplementation();
const setupPost = postStub.getMockImplementation();

afterEach(() => {
  if (setupGet) getStub.mockImplementation(setupGet);
  if (setupPost) postStub.mockImplementation(setupPost);
});

function serve() {
  getStub.mockImplementation(async (url: string) => {
    if (url === '/panel/api/ruleTemplates/list') return new Msg(true, '', TEMPLATES);
    if (url === '/panel/api/ruleTemplates/get/3')
      return new Msg(true, '', {
        ...TEMPLATES[0],
        content: 'rules:\n  - MATCH,PROXY\n',
        createdAt: 0,
      });
    if (url === '/panel/api/plans/list') return new Msg(true, '', PLANS);
    return new Msg(true, '', []);
  });
  postStub.mockImplementation(async (url: string) =>
    url === '/panel/api/ruleTemplates/preview'
      ? new Msg(true, '', 'proxies:\n  - name: preview-node\nrules:\n  - MATCH,PROXY\n')
      : new Msg(true, '', { id: 9 }),
  );
}

function renderPage() {
  serve();
  renderWithProviders(
    <MemoryRouter initialEntries={['/rules']}>
      <RulesPage />
    </MemoryRouter>,
  );
}

function rowOf(name: string) {
  return screen.getByText(name).closest('tr') as HTMLElement;
}

describe('RulesPage', () => {
  it('lists each template with its kind, the default star and the plans using it', async () => {
    renderPage();
    await screen.findByText('alpha_v3');
    const fallback = rowOf('alpha_v3');
    expect(within(fallback).getByText('YAML')).toBeTruthy();
    expect(within(fallback).getByText('2 plan(s)')).toBeTruthy();
    expect(within(fallback).getByRole('img', { name: 'Default template' })).toBeTruthy();
    expect(within(rowOf('remote_rules')).getByText('Remote')).toBeTruthy();
    expect(
      within(rowOf('remote_rules')).queryByRole('img', { name: 'Default template' }),
    ).toBeNull();
  });

  it('makes another template the default', async () => {
    renderPage();
    await screen.findByText('remote_rules');
    expect(
      within(rowOf('alpha_v3'))
        .getByRole('button', { name: /Make default/ })
        .hasAttribute('disabled'),
    ).toBe(true);

    fireEvent.click(within(rowOf('remote_rules')).getByRole('button', { name: /Make default/ }));

    await waitFor(() =>
      expect(postStub).toHaveBeenCalledWith('/panel/api/ruleTemplates/setDefault/5'),
    );
  });

  it('creates a template from the editor', async () => {
    renderPage();
    await screen.findByText('alpha_v3');
    fireEvent.click(screen.getByRole('button', { name: /New template/ }));
    const dialog = await screen.findByRole('dialog');
    fireEvent.change(within(dialog).getByLabelText('Name'), { target: { value: 'pigger_v3' } });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Save' }));

    await waitFor(() =>
      expect(postStub).toHaveBeenCalledWith(
        '/panel/api/ruleTemplates/add',
        expect.objectContaining({
          name: 'pigger_v3',
          content: expect.stringContaining('__PROXY_NODES__'),
        }),
        expect.anything(),
      ),
    );
  });

  // A preview is useless on a plan nobody is on, so it opens on the first plan with users.
  it('previews a saved template on the first plan with users', async () => {
    renderPage();
    await screen.findByText('alpha_v3');
    fireEvent.click(within(rowOf('alpha_v3')).getByRole('button', { name: /Preview/ }));

    await screen.findByText('preview-node', { exact: false });
    expect(postStub).toHaveBeenCalledWith(
      '/panel/api/ruleTemplates/preview',
      { planId: 2, content: 'rules:\n  - MATCH,PROXY\n' },
      expect.anything(),
    );
  });
});
