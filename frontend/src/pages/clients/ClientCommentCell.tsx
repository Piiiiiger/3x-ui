import { memo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Input, Tooltip } from 'antd';
import { EditOutlined } from '@ant-design/icons';

interface ClientCommentCellProps {
  email: string;
  comment?: string;
  onSave: (email: string, comment: string) => Promise<boolean>;
}

/** 备注, edited in place: Enter or leaving the field saves, Esc cancels. */
const ClientCommentCell = memo(function ClientCommentCell({
  email,
  comment = '',
  onSave,
}: ClientCommentCellProps) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  async function commit() {
    if (draft === null || saving) return;
    if (draft.trim() === comment.trim()) {
      setDraft(null);
      return;
    }
    setSaving(true);
    try {
      if (await onSave(email, draft)) setDraft(null);
    } finally {
      setSaving(false);
    }
  }

  if (draft !== null) {
    return (
      <Input
        size="small"
        autoFocus
        value={draft}
        disabled={saving}
        maxLength={500}
        aria-label={t('pages.clients.commentEdit')}
        onChange={(e) => setDraft(e.target.value)}
        onPressEnter={commit}
        onBlur={commit}
        onKeyDown={(e) => {
          if (e.key === 'Escape') setDraft(null);
        }}
      />
    );
  }
  return (
    <span className="client-comment">
      <Tooltip title={comment || undefined}>
        <span className="client-comment-text">{comment || '—'}</span>
      </Tooltip>
      <Button
        type="text"
        size="small"
        icon={<EditOutlined />}
        aria-label={t('pages.clients.commentEdit')}
        onClick={() => setDraft(comment)}
      />
    </span>
  );
});

export default ClientCommentCell;
