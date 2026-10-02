import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Checkbox, Spin } from 'antd';
import { DownloadOutlined, UploadOutlined } from '@ant-design/icons';

import { HttpUtil, PromiseUtil } from '@/utils';
import './BackupTab.css';

/** Backing up and restoring the panel's database, a section of the panel settings. */
export default function BackupTab() {
  const { t } = useTranslation();
  const isPostgres = window.X_UI_DB_TYPE === 'postgres';
  const [keepHostSettings, setKeepHostSettings] = useState(true);
  const [busyTip, setBusyTip] = useState<string | null>(null);

  function exportDb() {
    window.location.href = (window.X_UI_BASE_PATH || '') + 'panel/api/server/getDb';
  }

  function exportMigration() {
    window.location.href = (window.X_UI_BASE_PATH || '') + 'panel/api/server/getMigration';
  }

  function importDb() {
    const fileInput = document.createElement('input');
    fileInput.type = 'file';
    fileInput.accept = '.dump,.db';
    fileInput.addEventListener('change', async (e) => {
      const dbFile = (e.target as HTMLInputElement).files?.[0];
      if (!dbFile) return;

      const formData = new FormData();
      formData.append('db', dbFile);
      formData.append('keepHostSettings', String(keepHostSettings));

      setBusyTip(`${t('pages.index.importDatabase')}…`);

      const upload = await HttpUtil.post('/panel/api/server/importDB', formData, {
        headers: { 'Content-Type': 'multipart/form-data' },
      });
      if (!upload?.success) {
        setBusyTip(null);
        return;
      }

      // importDB schedules the panel restart server-side; wait it out, then reload.
      setBusyTip(`${t('pages.settings.restartPanel')}…`);
      await PromiseUtil.sleep(5000);
      window.location.reload();
    });
    fileInput.click();
  }

  return (
    <>
      <Spin fullscreen spinning={busyTip !== null} description={busyTip ?? undefined} />
      {isPostgres && (
        <div className="backup-description" style={{ marginBottom: 16 }}>
          {t('pages.index.backupPostgresNote')}
        </div>
      )}
      <div className="backup-list">
        <div className="backup-item">
          <div className="backup-meta">
            <div className="backup-title">{t('pages.index.exportDatabase')}</div>
            <div className="backup-description">
              {isPostgres
                ? t('pages.index.exportDatabasePgDesc')
                : t('pages.index.exportDatabaseDesc')}
            </div>
          </div>
          <Button
            type="primary"
            aria-label={t('pages.index.exportDatabase')}
            onClick={exportDb}
            icon={<DownloadOutlined />}
          />
        </div>

        {isPostgres && (
          <div className="backup-item">
            <div className="backup-meta">
              <div className="backup-title">{t('pages.index.migrationDownload')}</div>
              <div className="backup-description">{t('pages.index.migrationDownloadPgDesc')}</div>
            </div>
            <Button
              type="primary"
              aria-label={t('pages.index.migrationDownload')}
              onClick={exportMigration}
              icon={<DownloadOutlined />}
            />
          </div>
        )}

        <div className="backup-item">
          <div className="backup-meta">
            <div className="backup-title">{t('pages.index.importDatabase')}</div>
            <div className="backup-description">
              {isPostgres
                ? t('pages.index.importDatabasePgDesc')
                : t('pages.index.importDatabaseDesc')}
            </div>
          </div>
          <Button
            type="primary"
            aria-label={t('pages.index.importDatabase')}
            onClick={importDb}
            icon={<UploadOutlined />}
          />
        </div>

        <div className="backup-item">
          <div className="backup-meta">
            <Checkbox
              checked={keepHostSettings}
              onChange={(e) => setKeepHostSettings(e.target.checked)}
            >
              {t('pages.index.importKeepHostSettings')}
            </Checkbox>
            <div className="backup-description">{t('pages.index.importKeepHostSettingsDesc')}</div>
          </div>
        </div>
      </div>
    </>
  );
}
