import { fireEvent, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { afterEach, describe, expect, it, vi } from 'vitest';

import RulesPage from '@/pages/rules/RulesPage';
import { HttpUtil, Msg } from '@/utils';
import { chooseSelectOption, renderWithProviders } from './test-utils';

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
    baseId: 0,
    changes: [],
  },
  {
    id: 5,
    name: 'remote_rules',
    isDefault: false,
    kind: 'remote',
    size: 40,
    planCount: 0,
    updatedAt: 0,
    baseId: 0,
    changes: [],
  },
  {
    id: 7,
    name: 'mine_v3',
    isDefault: false,
    kind: 'yaml',
    size: 120,
    planCount: 1,
    updatedAt: 0,
    baseId: 3,
    changes: [
      { key: 'dns', replaced: true, added: 0 },
      { key: 'proxy-groups', replaced: true, added: 0 },
      { key: 'rules', replaced: false, added: 5 },
    ],
  },
  {
    id: 8,
    name: 'beta_v3',
    isDefault: false,
    kind: 'yaml',
    size: 2000,
    planCount: 1,
    updatedAt: 0,
    baseId: 0,
    changes: [],
  },
];
const MINE_CONTENT = 'prepend-rules:\n  - DOMAIN,mine.example,PROXY\n';
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
  conversion = keepsListeners;
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
    if (url === '/panel/api/ruleTemplates/get/7')
      return new Msg(true, '', { ...TEMPLATES[2], content: MINE_CONTENT, createdAt: 0 });
    if (url === '/panel/api/plans/list') return new Msg(true, '', PLANS);
    return new Msg(true, '', []);
  });
  postStub.mockImplementation(async (url: string, body?: unknown) => {
    if (url === '/panel/api/ruleTemplates/preview')
      return new Msg(true, '', 'proxies:\n  - name: preview-node\nrules:\n  - MATCH,PROXY\n');
    if (url === '/panel/api/ruleTemplates/variantOf/8') return conversion(body);
    return new Msg(true, '', { id: 9 });
  });
}

// What the server says converting beta_v3 does; a test replaces it to try other answers.
const keepsListeners = () =>
  new Msg(true, '', {
    identical: false,
    changes: [{ key: 'listeners', replaced: true, added: 0 }],
    moved: 0,
    size: 300,
    planCount: 1,
  });
let conversion: (body: unknown) => Msg = keepsListeners;

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

function rowNames(): string[] {
  return Array.from(document.querySelectorAll('.rule-template-name-text')).map(
    (el) => el.textContent ?? '',
  );
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
    fireEvent.change(within(dialog).getByLabelText('Name'), { target: { value: 'beta_v3' } });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Save' }));

    await waitFor(() =>
      expect(postStub).toHaveBeenCalledWith(
        '/panel/api/ruleTemplates/add',
        expect.objectContaining({
          name: 'beta_v3',
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
      { planId: 2, content: 'rules:\n  - MATCH,PROXY\n', baseId: 0 },
      expect.anything(),
    );
  });

  // One base, each variant under it saying what it changes there.
  it('lists each variant under its base with what it changes', async () => {
    renderPage();
    await screen.findByText('mine_v3');
    expect(rowNames()).toEqual(['alpha_v3', 'mine_v3', 'remote_rules', 'beta_v3']);
    expect(within(rowOf('alpha_v3')).getByText('Base')).toBeTruthy();
    const mine = rowOf('mine_v3');
    for (const chip of ['DNS', 'Proxy groups', 'Rules +5']) {
      expect(within(mine).getByText(chip)).toBeTruthy();
    }
    expect(within(mine).queryByText('YAML')).toBeNull();
    // Only a full YAML template can take variants or become one.
    expect(within(mine).queryByRole('button', { name: /New variant/ })).toBeNull();
    expect(within(rowOf('remote_rules')).queryByRole('button', { name: /New variant/ })).toBeNull();
    expect(within(rowOf('alpha_v3')).queryByRole('button', { name: /Make variant/ })).toBeNull();
  });

  it('creates a variant of a base holding only its differences', async () => {
    renderPage();
    await screen.findByText('alpha_v3');
    fireEvent.click(within(rowOf('alpha_v3')).getByRole('button', { name: /New variant/ }));
    const dialog = await screen.findByRole('dialog', { name: 'New variant of alpha_v3' });
    fireEvent.change(within(dialog).getByLabelText('Name'), { target: { value: 'gamma_v3' } });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Save' }));

    await waitFor(() =>
      expect(postStub).toHaveBeenCalledWith(
        '/panel/api/ruleTemplates/add',
        { name: 'gamma_v3', content: expect.not.stringContaining('__PROXY_NODES__'), baseId: 3 },
        expect.anything(),
      ),
    );
  });

  // Editing shows the variant's own differences; its preview is the merge on the base.
  it('edits a variant and previews it merged onto its base', async () => {
    renderPage();
    await screen.findByText('mine_v3');
    fireEvent.click(within(rowOf('mine_v3')).getByRole('button', { name: /Edit/ }));
    const dialog = await screen.findByRole('dialog', { name: 'Edit template' });
    expect(await within(dialog).findByText('Based on alpha_v3')).toBeTruthy();
    fireEvent.click(within(dialog).getByRole('button', { name: /Preview/ }));

    await screen.findByText('preview-node', { exact: false });
    expect(postStub).toHaveBeenCalledWith(
      '/panel/api/ruleTemplates/preview',
      { planId: 2, content: MINE_CONTENT, baseId: 3 },
      expect.anything(),
    );
  });

  it('makes a template a variant after showing what it keeps', async () => {
    renderPage();
    await screen.findByText('beta_v3');
    fireEvent.click(within(rowOf('beta_v3')).getByRole('button', { name: /Make variant/ }));
    const dialog = await screen.findByRole('dialog', { name: 'Make beta_v3 a variant' });
    chooseSelectOption('variant-base', 'alpha_v3');

    expect(await within(dialog).findByText('Listeners')).toBeTruthy();
    expect(postStub).toHaveBeenCalledWith(
      '/panel/api/ruleTemplates/variantOf/8',
      { baseId: 3, allowReorder: false, apply: false },
      expect.anything(),
    );
    fireEvent.click(within(dialog).getByRole('button', { name: 'Convert' }));
    await waitFor(() =>
      expect(postStub).toHaveBeenCalledWith(
        '/panel/api/ruleTemplates/variantOf/8',
        { baseId: 3, allowReorder: false, apply: true },
        expect.anything(),
      ),
    );
  });

  // Rules that move to the top may match first, so converting waits for a yes.
  it('asks before moving rules to the top', async () => {
    conversion = () =>
      new Msg(true, '', { identical: false, changes: [], moved: 5, size: 300, planCount: 1 });
    renderPage();
    await screen.findByText('beta_v3');
    fireEvent.click(within(rowOf('beta_v3')).getByRole('button', { name: /Make variant/ }));
    const dialog = await screen.findByRole('dialog', { name: 'Make beta_v3 a variant' });
    chooseSelectOption('variant-base', 'alpha_v3');

    const convert = await within(dialog).findByRole('button', { name: 'Convert' });
    await within(dialog).findByText(/5 of its rules/);
    expect(convert.hasAttribute('disabled')).toBe(true);
    fireEvent.click(within(dialog).getByRole('checkbox', { name: 'Move them to the top' }));
    fireEvent.click(convert);
    await waitFor(() =>
      expect(postStub).toHaveBeenCalledWith(
        '/panel/api/ruleTemplates/variantOf/8',
        { baseId: 3, allowReorder: true, apply: true },
        expect.anything(),
      ),
    );
  });

  it('says a copy of the base will be folded into it', async () => {
    conversion = () =>
      new Msg(true, '', { identical: true, changes: [], moved: 0, size: 0, planCount: 1 });
    renderPage();
    await screen.findByText('beta_v3');
    fireEvent.click(within(rowOf('beta_v3')).getByRole('button', { name: /Make variant/ }));
    const dialog = await screen.findByRole('dialog', { name: 'Make beta_v3 a variant' });
    chooseSelectOption('variant-base', 'alpha_v3');
    expect(
      await within(dialog).findByText(
        'beta_v3 is the same as alpha_v3: its 1 plan(s) will use alpha_v3 directly, and beta_v3 will be deleted.',
      ),
    ).toBeTruthy();
  });
});
