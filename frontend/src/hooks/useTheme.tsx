import { createContext, useCallback, useContext, useLayoutEffect, useMemo, useState } from 'react';
import type { ReactNode } from 'react';
import { theme as antdTheme } from 'antd';
import type { MappingAlgorithm, ThemeConfig } from 'antd';

const STORAGE_DARK = 'dark-mode';
const STORAGE_ULTRA = 'isUltraDarkThemeEnabled';

function readBool(key: string, fallback: boolean): boolean {
  const raw = localStorage.getItem(key);
  if (raw === null) return fallback;
  return raw === 'true';
}

function applyDom(isDark: boolean, isUltra: boolean) {
  document.body.classList.remove('dark', 'light');
  document.body.classList.add(isDark ? 'dark' : 'light');
  // Native scrollbars read color-scheme, not the body class.
  document.documentElement.style.colorScheme = isDark ? 'dark' : 'light';
  if (isUltra) {
    document.documentElement.setAttribute('data-theme', 'ultra-dark');
  } else {
    document.documentElement.removeAttribute('data-theme');
  }
  const msg = document.getElementById('message');
  if (msg) {
    msg.classList.remove('dark', 'light');
    msg.classList.add(isDark ? 'dark' : 'light');
  }
}

// module load so the document is in the right theme before React mounts.
const initialDark = readBool(STORAGE_DARK, true);
const initialUltra = readBool(STORAGE_ULTRA, false);
applyDom(initialDark, initialUltra);

// 600 and darker are the shades that keep white text on coral, or coral used as
// text, at WCAG AA on the light surfaces; 500 is the decorative brand coral.
const CORAL = {
  50: '#fef5f2',
  100: '#fde8e2',
  200: '#fbd4c9',
  300: '#f7b5a3',
  400: '#f18c6e',
  500: '#d97757',
  600: '#c25236',
  650: '#b5482d',
  700: '#a4432d',
} as const;

const FONT_SANS =
  "Inter, 'OPPO Sans', OPlusSans3, 'HarmonyOS Sans SC', -apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', 'Noto Sans', 'Noto Sans SC', sans-serif, 'Apple Color Emoji', 'Segoe UI Emoji'";
const FONT_MONO =
  "'JetBrains Mono', ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace";

const SHAPE_TOKENS = {
  borderRadius: 0,
  borderRadiusLG: 0,
  borderRadiusSM: 0,
  borderRadiusXS: 0,
  fontFamily: FONT_SANS,
  fontFamilyCode: FONT_MONO,
};

const LIGHT_TOKENS = {
  ...SHAPE_TOKENS,
  colorPrimary: CORAL[500],
  colorLink: CORAL[650],
  colorLinkHover: CORAL[600],
  colorLinkActive: CORAL[700],
  colorTextBase: '#271610',
  colorBgBase: '#fdfaf8',
  colorBgLayout: '#fdfaf8',
  colorBgContainer: '#fffdfc',
  colorBgElevated: '#fffdfc',
  colorBorder: 'rgba(137, 110, 96, 0.45)',
  colorBorderSecondary: 'rgba(137, 110, 96, 0.28)',
  colorTextDescription: 'rgba(39, 22, 16, 0.62)',
  colorTextTertiary: 'rgba(39, 22, 16, 0.62)',
  colorTextPlaceholder: '#7c6b65',
  colorError: '#cf1322',
  colorErrorText: '#cf1322',
  colorSuccessText: '#237804',
  colorWarningText: '#874d00',
  boxShadow: '0 0 0 1px rgba(137, 110, 96, 0.32), 6px 6px 0 rgba(39, 22, 16, 0.14)',
  boxShadowSecondary: '0 0 0 1px rgba(137, 110, 96, 0.28), 3px 3px 0 rgba(39, 22, 16, 0.12)',
};
const DARK_TOKENS = {
  ...SHAPE_TOKENS,
  colorPrimary: CORAL[400],
  colorLink: CORAL[300],
  colorLinkHover: CORAL[200],
  colorLinkActive: CORAL[400],
  colorTextBase: '#f9f4f1',
  colorBgBase: '#10131c',
  colorBgLayout: '#0d1018',
  colorBgContainer: '#131722',
  colorBgElevated: '#1a1f2c',
  colorBorder: 'rgba(255, 255, 255, 0.16)',
  colorBorderSecondary: 'rgba(255, 255, 255, 0.09)',
  colorTextPlaceholder: '#9a8a84',
  boxShadow: '0 0 0 1px rgba(255, 255, 255, 0.1), 6px 6px 0 rgba(0, 0, 0, 0.55)',
  boxShadowSecondary: '0 0 0 1px rgba(255, 255, 255, 0.1), 3px 3px 0 rgba(0, 0, 0, 0.5)',
};
const ULTRA_DARK_TOKENS = {
  ...DARK_TOKENS,
  colorBgBase: '#000',
  colorBgLayout: '#000',
  colorBgContainer: '#0b0b0e',
  colorBgElevated: '#15151a',
  colorBorderSecondary: 'rgba(255, 255, 255, 0.07)',
};

const LIGHT_MENU_TOKENS = {
  itemBg: 'transparent',
  subMenuItemBg: 'transparent',
  itemSelectedBg: CORAL[100],
  itemSelectedColor: CORAL[700],
  itemHoverBg: CORAL[50],
  itemHoverColor: CORAL[700],
  itemActiveBg: CORAL[100],
  subMenuItemSelectedColor: CORAL[700],
  horizontalItemSelectedBg: 'transparent',
  horizontalItemSelectedColor: CORAL[700],
  horizontalItemHoverBg: 'transparent',
  horizontalItemHoverColor: CORAL[700],
  activeBarHeight: 0,
};
const DARK_MENU_TOKENS = {
  darkItemBg: '#0b0e15',
  darkSubMenuItemBg: '#0d1018',
  darkPopupBg: '#1a1f2c',
  darkItemColor: 'rgba(249, 244, 241, 0.72)',
  darkItemHoverBg: 'rgba(241, 140, 110, 0.08)',
  darkItemHoverColor: CORAL[300],
  darkItemSelectedBg: 'rgba(241, 140, 110, 0.16)',
  darkItemSelectedColor: CORAL[300],
  horizontalItemSelectedBg: 'transparent',
  horizontalItemSelectedColor: CORAL[300],
  horizontalItemHoverBg: 'transparent',
  horizontalItemHoverColor: CORAL[300],
  activeBarHeight: 0,
};
const ULTRA_DARK_MENU_TOKENS = {
  ...DARK_MENU_TOKENS,
  darkItemBg: '#050507',
  darkSubMenuItemBg: '#000',
  darkPopupBg: '#15151a',
};

const LIGHT_CARD_TOKENS = {
  colorBorderSecondary: 'rgba(137, 110, 96, 0.38)',
};
const DARK_CARD_TOKENS = {
  colorBorderSecondary: 'rgba(241, 140, 110, 0.2)',
};
const ULTRA_DARK_CARD_TOKENS = {
  colorBorderSecondary: 'rgba(241, 140, 110, 0.16)',
};

const STATISTIC_TOKENS = {
  contentFontSize: 17,
  titleFontSize: 11,
};

// The hard offset shadow is the signature of the flat theme: buttons sit 2px
// off the page, cards and dialogs further (see styles/page-cards.css).
const LIGHT_BUTTON_TOKENS = {
  colorPrimary: CORAL[600],
  colorPrimaryHover: '#b84c31',
  colorPrimaryActive: CORAL[700],
  fontWeight: 500,
  defaultShadow: '2px 2px 0 rgba(39, 22, 16, 0.16)',
  primaryShadow: '2px 2px 0 rgba(39, 22, 16, 0.22)',
  dangerShadow: '2px 2px 0 rgba(39, 22, 16, 0.22)',
};
const DARK_BUTTON_TOKENS = {
  primaryColor: '#2a130c',
  fontWeight: 500,
  defaultShadow: '2px 2px 0 rgba(0, 0, 0, 0.5)',
  primaryShadow: '2px 2px 0 rgba(0, 0, 0, 0.55)',
  dangerShadow: '2px 2px 0 rgba(0, 0, 0, 0.55)',
};

const LIGHT_TABS_TOKENS = {
  inkBarColor: CORAL[500],
  itemSelectedColor: CORAL[650],
  itemHoverColor: CORAL[650],
  itemActiveColor: CORAL[700],
};

// Header rows read as plain bold text over the card, like 妙妙屋X's tables; the
// fill stays opaque so fixed columns still cover the cells scrolling under them.
const LIGHT_TABLE_TOKENS = {
  headerBg: '#fffdfc',
  headerSplitColor: 'transparent',
  headerColor: '#271610',
  rowHoverBg: CORAL[50],
  rowSelectedBg: CORAL[100],
  rowSelectedHoverBg: CORAL[200],
};
const DARK_TABLE_TOKENS = {
  headerBg: '#131722',
  headerSplitColor: 'transparent',
  rowHoverBg: 'rgba(241, 140, 110, 0.06)',
  rowSelectedBg: 'rgba(241, 140, 110, 0.12)',
  rowSelectedHoverBg: 'rgba(241, 140, 110, 0.18)',
};

const LIGHT_PAGINATION_TOKENS = {
  itemActiveColor: CORAL[650],
  itemActiveColorHover: CORAL[700],
};

const LIGHT_TAG_TOKENS = {
  defaultBg: '#f8f1ec',
  defaultColor: '#4a2f25',
};

// The dark algorithm darkens seed colours (#f18c6e renders as #d07a61); re-pin
// the primary and link shades so dark mode keeps the bright coral it is built on.
const pinDarkAccents: MappingAlgorithm = (seed, map = antdTheme.darkAlgorithm(seed)) => ({
  ...map,
  colorPrimary: CORAL[400],
  colorPrimaryHover: CORAL[300],
  colorPrimaryActive: CORAL[500],
  colorPrimaryText: CORAL[400],
  colorPrimaryTextHover: CORAL[300],
  colorPrimaryTextActive: CORAL[500],
  colorLink: CORAL[300],
  colorLinkHover: CORAL[200],
  colorLinkActive: CORAL[400],
});

// Class antd puts on every component root; plain elements opt into the theme's
// CSS variables by carrying it too (see CommandPalette).
export const THEME_CSS_VAR_SCOPE = 'xui';

// hashed:false drops the `:where(.css-<hash>)` wrapper antd puts around every
// rule. It costs nothing in specificity — `:where()` contributes zero, so the
// panel's own `.ant-*` overrides still win — and it removes roughly 5,700
// wrappers, 16% of the generated stylesheet, from what the browser has to parse.
//
// cssVar.key pins the CSS-variable scope. Every panel page mounts its own
// ConfigProvider (there is no root one), and without a fixed key each mints a
// fresh useId-derived scope, so navigating re-serialises and re-injects the whole
// token block under a new class instead of reusing the one already in the head.
const SHARED_STYLE_CONFIG = {
  hashed: false,
  cssVar: { key: THEME_CSS_VAR_SCOPE },
} as const;

export function buildAntdThemeConfig(isDark: boolean, isUltra: boolean): ThemeConfig {
  if (!isDark) {
    return {
      ...SHARED_STYLE_CONFIG,
      algorithm: antdTheme.defaultAlgorithm,
      token: LIGHT_TOKENS,
      components: {
        Statistic: STATISTIC_TOKENS,
        Button: LIGHT_BUTTON_TOKENS,
        Menu: LIGHT_MENU_TOKENS,
        Card: LIGHT_CARD_TOKENS,
        Tabs: LIGHT_TABS_TOKENS,
        Table: LIGHT_TABLE_TOKENS,
        Pagination: LIGHT_PAGINATION_TOKENS,
        Tag: LIGHT_TAG_TOKENS,
      },
    };
  }
  return {
    ...SHARED_STYLE_CONFIG,
    algorithm: [antdTheme.darkAlgorithm, pinDarkAccents],
    token: isUltra ? ULTRA_DARK_TOKENS : DARK_TOKENS,
    components: {
      Menu: isUltra ? ULTRA_DARK_MENU_TOKENS : DARK_MENU_TOKENS,
      Card: isUltra ? ULTRA_DARK_CARD_TOKENS : DARK_CARD_TOKENS,
      Statistic: STATISTIC_TOKENS,
      Button: DARK_BUTTON_TOKENS,
      Table: isUltra ? { ...DARK_TABLE_TOKENS, headerBg: '#0b0b0e' } : DARK_TABLE_TOKENS,
    },
  };
}

export function pauseAnimationsUntilLeave(elementId: string): void {
  document.documentElement.setAttribute('data-theme-animations', 'off');
  const el = document.getElementById(elementId);
  if (!el) return;
  const restore = () => {
    document.documentElement.removeAttribute('data-theme-animations');
    el.removeEventListener('mouseleave', restore);
    el.removeEventListener('touchend', restore);
  };
  el.addEventListener('mouseleave', restore);
  el.addEventListener('touchend', restore);
}

interface ThemeContextValue {
  isDark: boolean;
  isUltra: boolean;
  toggleTheme: () => void;
  toggleUltra: () => void;
  antdThemeConfig: ThemeConfig;
}

const ThemeContext = createContext<ThemeContextValue | null>(null);

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [isDark, setIsDark] = useState<boolean>(initialDark);
  const [isUltra, setIsUltra] = useState<boolean>(initialUltra);

  useLayoutEffect(() => {
    applyDom(isDark, isUltra);
    localStorage.setItem(STORAGE_DARK, String(isDark));
    localStorage.setItem(STORAGE_ULTRA, String(isUltra));
  }, [isDark, isUltra]);

  const toggleTheme = useCallback(() => setIsDark((v) => !v), []);
  const toggleUltra = useCallback(() => setIsUltra((v) => !v), []);

  const antdThemeConfig = useMemo(() => buildAntdThemeConfig(isDark, isUltra), [isDark, isUltra]);

  const value = useMemo<ThemeContextValue>(
    () => ({ isDark, isUltra, toggleTheme, toggleUltra, antdThemeConfig }),
    [isDark, isUltra, toggleTheme, toggleUltra, antdThemeConfig],
  );

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}

export function useTheme(): ThemeContextValue {
  const ctx = useContext(ThemeContext);
  if (!ctx) throw new Error('useTheme must be used inside <ThemeProvider>');
  return ctx;
}
