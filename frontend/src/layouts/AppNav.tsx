import { useCallback, useMemo, useState } from 'react';
import type { ComponentType } from 'react';
import { Link, useLocation, useNavigate } from 'react-router';
import { useTranslation } from 'react-i18next';
import { Drawer, Dropdown, Menu } from 'antd';
import type { MenuProps } from 'antd';
import {
  ApiOutlined,
  ApartmentOutlined,
  AppstoreOutlined,
  CloseOutlined,
  CloudServerOutlined,
  ClusterOutlined,
  CodeOutlined,
  CrownOutlined,
  DashboardOutlined,
  DatabaseOutlined,
  DiscordOutlined,
  DownOutlined,
  EllipsisOutlined,
  ExportOutlined,
  GithubOutlined,
  GlobalOutlined,
  HeartOutlined,
  ImportOutlined,
  LogoutOutlined,
  MailOutlined,
  MenuOutlined,
  MessageOutlined,
  MoonFilled,
  MoonOutlined,
  ProfileOutlined,
  RadarChartOutlined,
  ReadOutlined,
  SafetyOutlined,
  SearchOutlined,
  SettingOutlined,
  SunOutlined,
  SwapOutlined,
  TagsOutlined,
  TeamOutlined,
  ToolOutlined,
} from '@ant-design/icons';

import { HttpUtil } from '@/utils';
import { formatPanelVersion } from '@/lib/panel-version';
import { PANEL_NAME } from '@/lib/brand';
import { pauseAnimationsUntilLeave, useTheme } from '@/hooks/useTheme';
import { useMediaQuery } from '@/hooks/useMediaQuery';
import { useAllSettings } from '@/api/queries/useAllSettings';
import { useCommandPalette } from '@/components/command-palette/useCommandPalette';
import SponsorSlot from '@/components/sponsor/SponsorSlot';
import './AppNav.css';

const DONATE_URL = 'https://donate.sanaei.dev/';
// The palette listens for Ctrl as well as Cmd, so the chip must not show a
// Mac glyph to the Linux and Windows operators who are most of this panel's.
const SHORTCUT_MODIFIER = /Mac|iPhone|iPad|iPod/.test(navigator.userAgent) ? '⌘' : 'Ctrl';
const DOCS_URL = 'https://docs.sanaei.dev/';
const REPO_URL = 'https://github.com/MHSanaei/3x-ui';
const LOGOUT_KEY = '__logout__';
// Below this width the page buttons stop fitting beside the actions, so the bar
// keeps only the brand, search and a menu button that opens the drawer.
const NAV_COLLAPSE_MAX_PX = 991;
// Secondary pages live behind the bar's "more" button instead of taking a slot.
const MORE_PAGES = new Set(['/api-docs', '/sponsors']);

type IconName =
  | 'dashboard'
  | 'probe'
  | 'inbound'
  | 'team'
  | 'groups'
  | 'plans'
  | 'setting'
  | 'tool'
  | 'cluster'
  | 'hosts'
  | 'logout'
  | 'sponsors'
  | 'apidocs'
  | 'outbound'
  | 'routing';

const iconByName: Record<IconName, ComponentType> = {
  dashboard: DashboardOutlined,
  probe: RadarChartOutlined,
  inbound: ImportOutlined,
  team: TeamOutlined,
  groups: TagsOutlined,
  plans: ProfileOutlined,
  setting: SettingOutlined,
  tool: ToolOutlined,
  cluster: ClusterOutlined,
  hosts: GlobalOutlined,
  logout: LogoutOutlined,
  sponsors: CrownOutlined,
  apidocs: ApiOutlined,
  outbound: ExportOutlined,
  routing: SwapOutlined,
};

function ThemeIcon({ isDark, isUltra }: { isDark: boolean; isUltra: boolean }) {
  return !isDark ? <SunOutlined /> : !isUltra ? <MoonOutlined /> : <MoonFilled />;
}

function ShortcutChip() {
  return (
    <span className="nav-search-kbd">
      <span className="kbd-cmd">{SHORTCUT_MODIFIER}</span>
      <span className="kbd-key">K</span>
    </span>
  );
}

export default function AppNav() {
  const { t } = useTranslation();
  const { isDark, isUltra, toggleTheme, toggleUltra } = useTheme();
  const { open: openCommandPalette } = useCommandPalette();
  const navigate = useNavigate();
  const { pathname, hash } = useLocation();
  const { allSetting } = useAllSettings();
  const { isMobile: navCollapsed } = useMediaQuery(NAV_COLLAPSE_MAX_PX);
  const showSubFormats = !!(allSetting.subJsonEnable || allSetting.subClashEnable);
  const showSubBalancers = !!allSetting.subJsonEnable;
  const [drawerOpen, setDrawerOpen] = useState(false);

  const currentTheme: 'light' | 'dark' = isDark ? 'dark' : 'light';
  const panelVersion = window.X_UI_CUR_VER || '';
  const versionLabel = panelVersion ? formatPanelVersion(panelVersion) : 'GitHub';

  const tabs = useMemo<{ key: string; icon: IconName; title: string }[]>(
    () => [
      { key: '/', icon: 'dashboard', title: t('menu.dashboard') },
      // Second on purpose: the bar folds the items that do not fit from the end.
      { key: '/probe', icon: 'probe', title: t('menu.probe') },
      { key: '/inbounds', icon: 'inbound', title: t('menu.inbounds') },
      { key: '/clients', icon: 'team', title: t('menu.clients') },
      { key: '/plans', icon: 'plans', title: t('menu.plans') },
      { key: '/groups', icon: 'groups', title: t('menu.groups') },
      { key: '/nodes', icon: 'cluster', title: t('menu.nodes') },
      { key: '/hosts', icon: 'hosts', title: t('menu.hosts') },
      { key: '/outbound', icon: 'outbound', title: t('menu.outbounds') },
      { key: '/routing', icon: 'routing', title: t('menu.routing') },
      { key: '/settings', icon: 'setting', title: t('menu.settings') },
      { key: '/xray', icon: 'tool', title: t('menu.xray') },
      { key: '/api-docs', icon: 'apidocs', title: t('menu.apiDocs') },
      { key: '/sponsors', icon: 'sponsors', title: t('menu.sponsors') },
      { key: LOGOUT_KEY, icon: 'logout', title: t('logout') },
    ],
    [t],
  );

  const drawerItems = useMemo(() => tabs.filter((tab) => tab.icon !== 'logout'), [tabs]);
  const barItems = useMemo(
    () => drawerItems.filter((tab) => !MORE_PAGES.has(tab.key)),
    [drawerItems],
  );
  const utilItems = useMemo(() => tabs.filter((tab) => tab.icon === 'logout'), [tabs]);

  const settingsChildren = useMemo<NonNullable<MenuProps['items']>>(() => {
    const children: NonNullable<MenuProps['items']> = [
      {
        key: '/settings#general',
        icon: <SettingOutlined />,
        label: t('pages.settings.panelSettings'),
      },
      {
        key: '/settings#security',
        icon: <SafetyOutlined />,
        label: t('pages.settings.securitySettings'),
      },
      {
        key: '/settings#telegram',
        icon: <MessageOutlined />,
        label: t('pages.settings.TGBotSettings'),
      },
      { key: '/settings#email', icon: <MailOutlined />, label: t('pages.settings.emailSettings') },
      {
        key: '/settings#discord',
        icon: <DiscordOutlined />,
        label: t('pages.settings.discordSettings'),
      },
      {
        key: '/settings#subscription',
        icon: <CloudServerOutlined />,
        label: t('pages.settings.subSettings'),
      },
    ];
    if (showSubFormats) {
      children.push({
        key: '/settings#subscription-formats',
        icon: <CodeOutlined />,
        label: t('menu.subFormats'),
      });
    }
    if (showSubBalancers) {
      children.push({
        key: '/settings#subscription-balancers',
        icon: <ApartmentOutlined />,
        label: t('pages.settings.subBalancers.menu'),
      });
    }
    return children;
  }, [t, showSubFormats, showSubBalancers]);

  const xrayChildren = useMemo<NonNullable<MenuProps['items']>>(
    () => [
      { key: '/xray#basic', icon: <SettingOutlined />, label: t('pages.xray.basicTemplate') },
      { key: '/xray#balancer', icon: <ClusterOutlined />, label: t('pages.xray.Balancers') },
      { key: '/xray#dns', icon: <DatabaseOutlined />, label: 'DNS' },
      { key: '/xray#advanced', icon: <CodeOutlined />, label: t('pages.xray.advancedTemplate') },
    ],
    [t],
  );

  const settingsActive = pathname === '/settings';
  const xrayActive = pathname === '/xray';
  const selectedKey = settingsActive
    ? `/settings${hash || '#general'}`
    : xrayActive
      ? `/xray${hash || '#basic'}`
      : pathname === ''
        ? '/'
        : pathname;
  const moreActive = MORE_PAGES.has(selectedKey);

  const openSubmenu = settingsActive ? '/settings' : xrayActive ? '/xray' : null;
  const [openKeys, setOpenKeys] = useState<string[]>(() => (openSubmenu ? [openSubmenu] : []));
  if (openSubmenu && !openKeys.includes(openSubmenu)) {
    setOpenKeys([...openKeys, openSubmenu]);
  }

  // In the bar the button face is drawn on an inner pill: rc-overflow measures each
  // item's own box, so the spacing must be padding inside it, never margin outside.
  const toMenuItems = useCallback(
    (items: typeof tabs, opts: { bar?: boolean } = {}): MenuProps['items'] =>
      items.map((tab) => {
        const Icon = iconByName[tab.icon];
        const children =
          tab.key === '/settings' ? settingsChildren : tab.key === '/xray' ? xrayChildren : null;
        if (!opts.bar) {
          return children
            ? { key: tab.key, icon: <Icon />, label: tab.title, children }
            : { key: tab.key, icon: <Icon />, label: tab.title, title: '' };
        }
        const label = (
          <span className="app-nav-pill">
            <Icon />
            <span>{tab.title}</span>
            {children && <DownOutlined className="app-nav-caret" />}
          </span>
        );
        return children ? { key: tab.key, label, children } : { key: tab.key, label, title: '' };
      }),
    [settingsChildren, xrayChildren],
  );

  const logout = useCallback(async () => {
    await HttpUtil.post('/logout');
    window.location.href = window.X_UI_BASE_PATH || '/';
  }, []);

  const onMenuClick = useCallback<NonNullable<MenuProps['onClick']>>(
    ({ key }) => {
      if (key === LOGOUT_KEY) {
        void logout();
        return;
      }
      navigate(String(key));
    },
    [logout, navigate],
  );

  const cycleTheme = useCallback(
    (id: string) => {
      pauseAnimationsUntilLeave(id);
      if (!isDark) {
        toggleTheme();
        if (isUltra) toggleUltra();
      } else if (!isUltra) {
        toggleUltra();
      } else {
        toggleUltra();
        toggleTheme();
      }
    },
    [isDark, isUltra, toggleTheme, toggleUltra],
  );

  const moreItems = useMemo<NonNullable<MenuProps['items']>>(
    () => [
      { key: '/api-docs', icon: <ApiOutlined />, label: t('menu.apiDocs') },
      { key: '/sponsors', icon: <CrownOutlined />, label: t('menu.sponsors') },
      { type: 'divider' },
      {
        key: 'docs',
        icon: <ReadOutlined />,
        label: (
          <a href={DOCS_URL} target="_blank" rel="noopener noreferrer">
            {t('menu.docs')}
          </a>
        ),
      },
      {
        key: 'donate',
        icon: <HeartOutlined />,
        label: (
          <a href={DONATE_URL} target="_blank" rel="noopener noreferrer">
            {t('menu.donate')}
          </a>
        ),
      },
      {
        key: 'repo',
        icon: <GithubOutlined />,
        label: (
          <a href={REPO_URL} target="_blank" rel="noopener noreferrer">
            {versionLabel}
          </a>
        ),
      },
    ],
    [t, versionLabel],
  );

  const searchLabel = t('commandPalette.title') || 'Command Palette (Ctrl + K)';

  return (
    <>
      <header className="app-nav">
        <Link to="/" className="app-nav-brand">
          {PANEL_NAME}
        </Link>
        {!navCollapsed && (
          <Menu
            mode="horizontal"
            className="app-nav-menu"
            selectedKeys={[selectedKey]}
            items={toMenuItems(barItems, { bar: true })}
            overflowedIndicator={
              <span className="app-nav-pill">
                <EllipsisOutlined />
              </span>
            }
            onClick={onMenuClick}
          />
        )}
        <div className="app-nav-actions">
          <button
            type="button"
            className="app-nav-btn nav-search"
            onClick={openCommandPalette}
            aria-label={searchLabel}
            title={searchLabel}
          >
            <SearchOutlined />
            {!navCollapsed && <ShortcutChip />}
          </button>
          {!navCollapsed && (
            <>
              <button
                id="theme-cycle"
                type="button"
                className="app-nav-btn app-nav-theme"
                onClick={() => cycleTheme('theme-cycle')}
              >
                <ThemeIcon isDark={isDark} isUltra={isUltra} />
                <span className="app-nav-btn-label">{t('menu.theme')}</span>
              </button>
              <SponsorSlot slot="sidebar" variant="compact" iconOnly rotate />
              <Dropdown
                trigger={['click']}
                placement="bottomRight"
                menu={{
                  items: moreItems,
                  selectedKeys: moreActive ? [selectedKey] : [],
                  onClick: ({ key }) => {
                    if (MORE_PAGES.has(key)) navigate(key);
                  },
                }}
              >
                <button
                  type="button"
                  className={`app-nav-btn app-nav-more${moreActive ? ' is-active' : ''}`}
                  aria-label={t('more')}
                  title={t('more')}
                >
                  <AppstoreOutlined />
                </button>
              </Dropdown>
              <button type="button" className="app-nav-btn app-nav-logout" onClick={logout}>
                <LogoutOutlined />
                <span className="app-nav-btn-label">{t('logout')}</span>
              </button>
            </>
          )}
          {navCollapsed && (
            <button
              type="button"
              className="app-nav-btn app-nav-burger"
              aria-label={t('menu.openMenu')}
              onClick={() => setDrawerOpen(true)}
            >
              <MenuOutlined />
            </button>
          )}
        </div>
      </header>

      <Drawer
        placement="left"
        closable={false}
        open={drawerOpen}
        rootClassName={currentTheme}
        size="min(82vw, 320px)"
        styles={{
          wrapper: { padding: 0 },
          body: { padding: 0, display: 'flex', flexDirection: 'column', height: '100%' },
          header: { display: 'none' },
        }}
        onClose={() => setDrawerOpen(false)}
      >
        <div className="drawer-header">
          <span className="drawer-brand">{PANEL_NAME}</span>
          <div className="drawer-header-actions">
            <a
              href={DOCS_URL}
              target="_blank"
              rel="noopener noreferrer"
              className="drawer-icon-btn"
              aria-label={t('menu.docs')}
              title={t('menu.docs')}
            >
              <ReadOutlined />
            </a>
            <a
              href={DONATE_URL}
              target="_blank"
              rel="noopener noreferrer"
              className="drawer-icon-btn drawer-donate"
              aria-label={t('menu.donate')}
              title={t('menu.donate')}
            >
              <HeartOutlined />
            </a>
            <button
              id="theme-cycle-drawer"
              type="button"
              className="drawer-icon-btn"
              aria-label={t('menu.theme')}
              title={t('menu.theme')}
              onClick={() => cycleTheme('theme-cycle-drawer')}
            >
              <ThemeIcon isDark={isDark} isUltra={isUltra} />
            </button>
            <button
              className="drawer-icon-btn"
              type="button"
              aria-label={t('close')}
              onClick={() => setDrawerOpen(false)}
            >
              <CloseOutlined />
            </button>
          </div>
        </div>
        <button
          type="button"
          className="nav-search nav-search-wide"
          onClick={() => {
            setDrawerOpen(false);
            openCommandPalette();
          }}
          aria-label={searchLabel}
        >
          <span className="nav-search-left">
            <SearchOutlined />
            <span>{t('commandPalette.search') || 'Search...'}</span>
          </span>
          <ShortcutChip />
        </button>
        <Menu
          theme={currentTheme}
          mode="inline"
          selectedKeys={[selectedKey]}
          openKeys={openKeys}
          onOpenChange={(keys) => setOpenKeys(keys as string[])}
          className="drawer-menu drawer-nav"
          items={toMenuItems(drawerItems)}
          onClick={(info) => {
            onMenuClick(info);
            setDrawerOpen(false);
          }}
        />
        <Menu
          theme={currentTheme}
          mode="inline"
          selectedKeys={[selectedKey]}
          className="drawer-menu drawer-utility"
          items={toMenuItems(utilItems)}
          onClick={(info) => {
            onMenuClick(info);
            setDrawerOpen(false);
          }}
        />
        <div className="drawer-footer">
          <SponsorSlot slot="sidebar" variant="compact" rotate className="drawer-sponsor" />
          <a
            href={REPO_URL}
            target="_blank"
            rel="noopener noreferrer"
            className="drawer-version"
            aria-label={`GitHub ${versionLabel}`}
          >
            <GithubOutlined />
            <span>{versionLabel}</span>
          </a>
        </div>
      </Drawer>
    </>
  );
}
