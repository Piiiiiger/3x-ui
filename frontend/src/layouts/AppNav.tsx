import { useCallback, useMemo, useState } from 'react';
import type { ComponentType } from 'react';
import { Link, useLocation, useNavigate } from 'react-router';
import { useTranslation } from 'react-i18next';
import { Drawer, Menu } from 'antd';
import type { MenuProps } from 'antd';
import {
  ApartmentOutlined,
  CloseOutlined,
  CloudServerOutlined,
  ClusterOutlined,
  CodeOutlined,
  DashboardOutlined,
  DatabaseOutlined,
  DiscordOutlined,
  DownOutlined,
  EllipsisOutlined,
  FileTextOutlined,
  ImportOutlined,
  LogoutOutlined,
  MailOutlined,
  MenuOutlined,
  MessageOutlined,
  MoonFilled,
  MoonOutlined,
  ProfileOutlined,
  RobotOutlined,
  SafetyOutlined,
  SearchOutlined,
  SettingOutlined,
  SunOutlined,
  TeamOutlined,
  ToolOutlined,
} from '@ant-design/icons';

import { HttpUtil } from '@/utils';
import { BrandMark } from '@/components/ui';
import { pauseAnimationsUntilLeave, useTheme } from '@/hooks/useTheme';
import { useMediaQuery } from '@/hooks/useMediaQuery';
import { useAllSettings } from '@/api/queries/useAllSettings';
import { useCommandPalette } from '@/components/command-palette/useCommandPalette';
import './AppNav.css';

// The palette listens for Ctrl as well as Cmd, so the chip must not show a
// Mac glyph to the Linux and Windows operators who are most of this panel's.
const SHORTCUT_MODIFIER = /Mac|iPhone|iPad|iPod/.test(navigator.userAgent) ? '⌘' : 'Ctrl';
const LOGOUT_KEY = '__logout__';
// Below this width the page buttons stop fitting beside the actions, so the bar
// keeps only the brand, search and a menu button that opens the drawer.
const NAV_COLLAPSE_MAX_PX = 991;

type IconName =
  | 'dashboard'
  | 'inbound'
  | 'team'
  | 'plans'
  | 'rules'
  | 'ai'
  | 'setting'
  | 'tool'
  | 'cluster'
  | 'logout'
  | 'shield';

const iconByName: Record<IconName, ComponentType> = {
  dashboard: DashboardOutlined,
  inbound: ImportOutlined,
  team: TeamOutlined,
  plans: ProfileOutlined,
  rules: FileTextOutlined,
  ai: RobotOutlined,
  setting: SettingOutlined,
  tool: ToolOutlined,
  cluster: ClusterOutlined,
  logout: LogoutOutlined,
  shield: SafetyOutlined,
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

  const tabs = useMemo<{ key: string; icon: IconName; title: string }[]>(
    () => [
      { key: '/', icon: 'dashboard', title: t('menu.dashboard') },
      // Second on purpose: the bar folds the items that do not fit from the end.
      { key: '/nodes', icon: 'cluster', title: t('menu.nodes') },
      { key: '/inbounds', icon: 'inbound', title: t('menu.inbounds') },
      { key: '/clients', icon: 'team', title: t('menu.clients') },
      { key: '/plans', icon: 'plans', title: t('menu.plans') },
      { key: '/rules', icon: 'rules', title: t('menu.rules') },
      { key: '/ai-usage', icon: 'ai', title: t('menu.aiUsage') },
      { key: '/settings', icon: 'setting', title: t('menu.settings') },
      { key: '/xray', icon: 'tool', title: t('menu.xray') },
      { key: '/chains', icon: 'tool', title: '链式管理' },
      { key: '/abuse', icon: 'shield', title: '防滥用' },
      { key: LOGOUT_KEY, icon: 'logout', title: t('logout') },
    ],
    [t],
  );

  const drawerItems = useMemo(() => tabs.filter((tab) => tab.icon !== 'logout'), [tabs]);
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
    children.push({
      key: '/settings#backup',
      icon: <CloudServerOutlined />,
      label: t('pages.index.backupTitle'),
    });
    return children;
  }, [t, showSubFormats, showSubBalancers]);

  const xrayChildren = useMemo<NonNullable<MenuProps['items']>>(
    () => [
      { key: '/xray#basic', icon: <SettingOutlined />, label: t('pages.xray.basicTemplate') },
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
        : pathname.startsWith('/nodes/')
          ? '/nodes'
          : pathname;

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

  const searchLabel = t('commandPalette.title') || 'Command Palette (Ctrl + K)';

  return (
    <>
      <header className="app-nav">
        <Link to="/" className="app-nav-brand">
          <BrandMark />
        </Link>
        {!navCollapsed && (
          <Menu
            mode="horizontal"
            className="app-nav-menu"
            selectedKeys={[selectedKey]}
            items={toMenuItems(drawerItems, { bar: true })}
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
          <span className="drawer-brand">
            <BrandMark />
          </span>
          <div className="drawer-header-actions">
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
      </Drawer>
    </>
  );
}
