import { useState } from 'react';
import { z } from 'zod';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { Button, Input, Modal, Select, Space, Alert, Popconfirm, message } from 'antd';
import { useTranslation } from 'react-i18next';
import { HttpUtil } from '@/utils';

interface Group {
  id: number;
  name: string;
  emails: string[];
}
const key = ['clients', 'customGroups'];
const base = '/panel/api/clients/customGroups';
function useGroups() {
  return useQuery<Group[]>({
    queryKey: key,
    queryFn: async () => {
      const result = await HttpUtil.get(base);
      if (!result?.success) throw new Error(result?.msg || 'Unable to load groups');
      return z
        .array(z.object({ id: z.number(), name: z.string(), emails: z.array(z.string()) }))
        .parse(result.obj ?? []);
    },
    staleTime: 30_000,
  });
}
function useSave() {
  const cache = useQueryClient();
  const [busy, setBusy] = useState(false);
  const [feedback, context] = message.useMessage();
  async function save(action: string, body: unknown) {
    setBusy(true);
    try {
      const result = await HttpUtil.post(`${base}/${action}`, body, {
        headers: { 'Content-Type': 'application/json' },
      });
      if (!result?.success) throw new Error(result?.msg || 'Unable to save group');
      await cache.invalidateQueries({ queryKey: ['clients'] });
      return true;
    } catch (error) {
      feedback.error(String(error));
      return false;
    } finally {
      setBusy(false);
    }
  }
  return { save, busy, context };
}
export function CustomGroupCell({ email }: { email: string }) {
  const { t } = useTranslation();
  const { data: groups = [], isError, isPending } = useGroups();
  const { save, busy, context } = useSave();
  return (
    <>
      {context}
      <Select
        aria-label={t('pages.clients.groups.title')}
        style={{ minWidth: 110, maxWidth: 180 }}
        loading={busy || isPending}
        disabled={busy || isError || isPending}
        value={groups.find((g) => g.emails.includes(email))?.id ?? 0}
        options={[
          { value: 0, label: t('pages.clients.groups.ungrouped') },
          ...groups.map((g) => ({ value: g.id, label: g.name })),
        ]}
        onChange={(id) => void save('assign', { id, emails: [email] })}
      />
    </>
  );
}
export default function CustomUserGroups({
  selected,
  value,
  onChange,
  onSaved,
}: {
  selected: string[];
  value?: string;
  onChange: (value?: string) => void;
  onSaved: () => void;
}) {
  const { t } = useTranslation();
  const { data: groups = [], isError, refetch } = useGroups();
  const { save, busy, context } = useSave();
  const [open, setOpen] = useState(false);
  const [name, setName] = useState('');
  const [editing, setEditing] = useState(0);
  const [assignment, setAssignment] = useState<number>();
  const label = (part: string) => t(`pages.clients.groups.${part}`);
  async function saveName() {
    if (await save('save', { id: editing, name })) {
      setName('');
      setEditing(0);
    }
  }
  return (
    <>
      {context}
      <div className="client-chips">
        <span>{label('title')}</span>
        <Button type={!value ? 'primary' : 'default'} onClick={() => onChange()}>
          {label('all')}
        </Button>
        <Button type={value === '0' ? 'primary' : 'default'} onClick={() => onChange('0')}>
          {label('ungrouped')}
        </Button>
        {groups.map((g) => (
          <Button
            key={g.id}
            type={value === String(g.id) ? 'primary' : 'default'}
            onClick={() => onChange(String(g.id))}
          >
            {g.name} ({g.emails.length})
          </Button>
        ))}
        <Button onClick={() => setOpen(true)}>{label('manage')}</Button>
        {selected.length > 0 && (
          <>
            <Select
              aria-label={label('assign')}
              placeholder={label('assign')}
              style={{ minWidth: 160 }}
              value={assignment}
              onChange={setAssignment}
              options={[
                { value: 0, label: label('ungrouped') },
                ...groups.map((g) => ({ value: g.id, label: g.name })),
              ]}
            />
            <Button
              loading={busy}
              disabled={assignment === undefined}
              onClick={async () => {
                if (await save('assign', { id: assignment, emails: selected })) onSaved();
              }}
            >
              {label('apply')} ({selected.length})
            </Button>
          </>
        )}
        {isError && (
          <Button danger onClick={() => void refetch()}>
            {label('retry')}
          </Button>
        )}
      </div>
      <Modal title={label('manage')} open={open} onCancel={() => setOpen(false)} footer={null}>
        <Alert title={label('hint')} type="info" style={{ marginBottom: 16 }} />
        <Space.Compact style={{ width: '100%', marginBottom: 16 }}>
          <Input
            aria-label={label('name')}
            placeholder={label('name')}
            maxLength={64}
            value={name}
            onChange={(e) => setName(e.target.value)}
            onPressEnter={() => void saveName()}
          />
          <Button loading={busy} disabled={!name.trim()} onClick={() => void saveName()}>
            {editing ? label('rename') : label('create')}
          </Button>
          {editing > 0 && (
            <Button
              onClick={() => {
                setEditing(0);
                setName('');
              }}
            >
              {t('cancel')}
            </Button>
          )}
        </Space.Compact>
        {groups.map((g) => (
          <div
            key={g.id}
            style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '8px 0' }}
          >
            <span style={{ flex: 1, overflowWrap: 'anywhere' }}>
              {g.name} ({g.emails.length})
            </span>
            <Button
              disabled={busy}
              onClick={() => {
                setEditing(g.id);
                setName(g.name);
              }}
            >
              {label('rename')}
            </Button>
            <Popconfirm
              title={label('deleteConfirm')}
              onConfirm={async () => {
                if (await save('delete', { id: g.id })) {
                  if (value === String(g.id)) onChange();
                  if (editing === g.id) {
                    setEditing(0);
                    setName('');
                  }
                  onSaved();
                }
              }}
            >
              <Button danger disabled={busy}>
                {t('delete')}
              </Button>
            </Popconfirm>
          </div>
        ))}
      </Modal>
    </>
  );
}
