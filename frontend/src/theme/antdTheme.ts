import { theme as antdThemeModule, type ThemeConfig } from 'antd';
import type { ThemeId } from '../hooks/useTheme';

const { darkAlgorithm, defaultAlgorithm } = antdThemeModule;

interface ThemeTokens {
  bgBase: string;
  bgSurface: string;
  bgElevated: string;
  bgHover: string;
  bgInput: string;
  textPrimary: string;
  textSecondary: string;
  textMuted: string;
  textInverse: string;
  borderSubtle: string;
  borderDefault: string;
  accent: string;
  accentHover: string;
  accentDim: string;
  accentStrong: string;
  success: string;
  warning: string;
  danger: string;
  info: string;
  focusRing: string;
}

const themeTokens: Record<ThemeId, ThemeTokens> = {
  assistente: {
    bgBase: '#0a1628',
    bgSurface: '#0f1f3a',
    bgElevated: '#162a4d',
    bgHover: '#1e3660',
    bgInput: '#0c1a30',
    textPrimary: '#eef2f9',
    textSecondary: '#a8bdd8',
    textMuted: '#7e97b8',
    textInverse: '#0a1628',
    borderSubtle: '#1c3158',
    borderDefault: '#294572',
    accent: '#5799f5',
    accentHover: '#83b4f8',
    accentDim: 'rgba(43, 126, 244, 0.18)',
    accentStrong: '#2b7ef4',
    success: '#4ade80',
    warning: '#fbbf24',
    danger: '#f87171',
    info: '#60a5fa',
    focusRing: 'rgba(43, 126, 244, 0.4)',
  },
  amethyst: {
    bgBase: '#12082a',
    bgSurface: '#1c1040',
    bgElevated: '#271852',
    bgHover: '#332264',
    bgInput: '#150c30',
    textPrimary: '#f0ecf9',
    textSecondary: '#beb5d6',
    textMuted: '#9990b8',
    textInverse: '#12082a',
    borderSubtle: '#2e1f5a',
    borderDefault: '#3c2e6e',
    accent: '#a78bfa',
    accentHover: '#c4b5fd',
    accentDim: 'rgba(167, 139, 250, 0.18)',
    accentStrong: '#8b5cf6',
    success: '#4ade80',
    warning: '#fbbf24',
    danger: '#f87171',
    info: '#60a5fa',
    focusRing: 'rgba(167, 139, 250, 0.45)',
  },
  midnight: {
    bgBase: '#0c0f14',
    bgSurface: '#151921',
    bgElevated: '#1e232d',
    bgHover: '#272d39',
    bgInput: '#111419',
    textPrimary: '#e8ecf2',
    textSecondary: '#9ca8be',
    textMuted: '#7a879e',
    textInverse: '#0c0f14',
    borderSubtle: '#232a36',
    borderDefault: '#2f3848',
    accent: '#60a5fa',
    accentHover: '#93c5fd',
    accentDim: 'rgba(96, 165, 250, 0.15)',
    accentStrong: '#3b82f6',
    success: '#4ade80',
    warning: '#fbbf24',
    danger: '#f87171',
    info: '#60a5fa',
    focusRing: 'rgba(96, 165, 250, 0.4)',
  },
  light: {
    bgBase: '#f0f4fa',
    bgSurface: '#ffffff',
    bgElevated: '#e8eef6',
    bgHover: '#dce4f0',
    bgInput: '#ffffff',
    textPrimary: '#0a1628',
    textSecondary: '#374a64',
    textMuted: '#5a7090',
    textInverse: '#eef2f9',
    borderSubtle: '#dce4f0',
    borderDefault: '#c5d0e0',
    accent: '#2b7ef4',
    accentHover: '#1a60c4',
    accentDim: 'rgba(43, 126, 244, 0.1)',
    accentStrong: '#1550a8',
    success: '#16a34a',
    warning: '#d97706',
    danger: '#dc2626',
    info: '#2563eb',
    focusRing: 'rgba(43, 126, 244, 0.3)',
  },
  'high-contrast': {
    bgBase: '#000000',
    bgSurface: '#0a0a0a',
    bgElevated: '#1a1a1a',
    bgHover: '#2a2a2a',
    bgInput: '#0a0a0a',
    textPrimary: '#ffffff',
    textSecondary: '#e0e0e0',
    textMuted: '#bbbbbb',
    textInverse: '#000000',
    borderSubtle: '#555555',
    borderDefault: '#777777',
    accent: '#5babff',
    accentHover: '#8cc8ff',
    accentDim: 'rgba(91, 171, 255, 0.2)',
    accentStrong: '#2b7ef4',
    success: '#86efac',
    warning: '#fde68a',
    danger: '#fca5a5',
    info: '#93c5fd',
    focusRing: 'rgba(91, 171, 255, 0.6)',
  },
};

function buildThemeConfig(id: ThemeId): ThemeConfig {
  const t = themeTokens[id];
  const isDark = id !== 'light';

  return {
    algorithm: isDark ? darkAlgorithm : defaultAlgorithm,
    token: {
      colorPrimary: t.accentStrong,
      colorSuccess: t.success,
      colorError: t.danger,
      colorWarning: t.warning,
      colorInfo: t.info,
      colorTextBase: t.textPrimary,
      colorTextSecondary: t.textSecondary,
      colorTextTertiary: t.textMuted,
      colorBgBase: t.bgBase,
      colorBgContainer: t.bgSurface,
      colorBgElevated: t.bgElevated,
      colorBgLayout: t.bgBase,
      colorBorder: t.borderDefault,
      colorBorderSecondary: t.borderSubtle,
      colorFillSecondary: t.bgHover,
      colorFillTertiary: t.bgElevated,
      borderRadius: 6,
      borderRadiusSM: 4,
      borderRadiusLG: 8,
      borderRadiusXS: 4,
      fontSize: 14,
      fontSizeSM: 13,
      fontSizeLG: 16,
      fontSizeXL: 20,
      fontSizeHeading1: 28,
      fontSizeHeading2: 24,
      fontSizeHeading3: 20,
      fontSizeHeading4: 16,
      fontFamily: "-apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif",
      boxShadow: '0px 2px 4px rgba(0, 0, 0, 0.06)',
      boxShadowSecondary: '0px 4px 8px rgba(0, 0, 0, 0.10)',
      motionDurationFast: '0.1s',
      motionDurationMid: '0.2s',
      motionDurationSlow: '0.3s',
      controlHeight: 36,
      controlHeightSM: 28,
      controlHeightLG: 44,
    },
    components: {
      Button: {
        borderRadius: 6,
        primaryShadow: 'none',
        defaultShadow: 'none',
        colorPrimaryHover: t.accentHover,
        colorPrimaryActive: t.accentStrong,
        defaultBorderColor: t.borderDefault,
      },
      Card: {
        borderRadiusLG: 8,
      },
      Input: {
        borderRadius: 6,
        activeBorderColor: t.accent,
        hoverBorderColor: t.accent,
        activeShadow: `0 0 0 2px ${t.focusRing}`,
        colorBgContainer: t.bgInput,
      },
      Select: {
        borderRadius: 6,
        colorBorder: t.borderDefault,
        optionSelectedBg: t.accentDim,
      },
      Modal: {
        borderRadiusLG: 12,
      },
      Table: {
        borderRadius: 8,
        headerBg: t.bgElevated,
        rowHoverBg: t.bgHover,
      },
      Tabs: {
        borderRadius: 6,
        inkBarColor: t.accent,
        itemActiveColor: t.accent,
        itemSelectedColor: t.accent,
        itemHoverColor: t.accentHover,
      },
      Tag: {
        borderRadiusSM: 6,
      },
      Menu: {
        itemBorderRadius: 6,
        subMenuItemBorderRadius: 6,
        itemSelectedBg: t.accentDim,
        itemSelectedColor: t.accent,
      },
      Tooltip: {
        borderRadius: 4,
      },
      Dropdown: {
        borderRadiusLG: 8,
      },
    },
  };
}

const themeConfigCache = new Map<ThemeId, ThemeConfig>();

export function getAntdTheme(id: ThemeId): ThemeConfig {
  let config = themeConfigCache.get(id);
  if (!config) {
    config = buildThemeConfig(id);
    themeConfigCache.set(id, config);
  }
  return config;
}
