import { theme as antdThemeModule, type ThemeConfig } from 'antd';
import type { ThemeId } from '../hooks/useTheme';

const { darkAlgorithm, defaultAlgorithm } = antdThemeModule;

/**
 * Tema do Ant Design derivado do sistema central de design tokens (`theme.css`).
 *
 * Em vez de manter uma paleta paralela com cores hardcoded (que precisava ser
 * sincronizada manualmente), lemos os valores reais das variáveis CSS definidas
 * para cada `[data-theme="..."]` em `theme.css`. Assim:
 *  - existe UMA única fonte de verdade para as cores (o `theme.css`);
 *  - qualquer ajuste de contraste/WCAG feito no CSS reflete automaticamente no antd;
 *  - todos os temas (Assistente, Ametista, Meia-Noite, Claro, Alto Contraste)
 *    funcionam sem manutenção extra.
 *
 * O `App` chama `getAntdTheme(theme)` a cada troca de tema, então o `ConfigProvider`
 * recebe os tokens corretos e o `data-theme` no <html> mantém o restante da UI em sincronia.
 */

/** Variáveis CSS lidas do `theme.css` para montar o tema antd. */
const TOKEN_VARS = [
  '--bg-base',
  '--bg-surface',
  '--bg-elevated',
  '--bg-hover',
  '--bg-input',
  '--text-primary',
  '--text-secondary',
  '--text-muted',
  '--text-inverse',
  '--border-subtle',
  '--border-default',
  '--accent',
  '--accent-hover',
  '--accent-dim',
  '--accent-strong',
  '--color-success',
  '--color-warning',
  '--color-danger',
  '--color-info',
  '--focus-ring',
  '--shadow-sm',
  '--shadow-md',
] as const;

type TokenVar = (typeof TOKEN_VARS)[number];
type TokenMap = Record<TokenVar, string>;

/** Seletor de bloco de tema corresponde ao id? (assistente também vive em `:root`). */
function selectorMatchesTheme(selectorText: string, id: ThemeId): boolean {
  const target = `[data-theme="${id}"]`;
  return selectorText.split(',').some((sel) => {
    const s = sel.trim();
    return s === target || (id === 'assistente' && s === ':root');
  });
}

/** Lê os tokens diretamente das regras de `theme.css` para o tema informado. */
function readTokensFromStylesheets(id: ThemeId): Partial<TokenMap> {
  const result: Partial<TokenMap> = {};
  if (typeof document === 'undefined') return result;

  for (const sheet of Array.from(document.styleSheets)) {
    let rules: CSSRuleList | null = null;
    try {
      rules = sheet.cssRules;
    } catch {
      // Folhas de estilo cross-origin lançam ao acessar cssRules — ignorar.
      continue;
    }
    if (!rules) continue;

    for (const rule of Array.from(rules)) {
      if (!(rule instanceof CSSStyleRule)) continue;
      if (!selectorMatchesTheme(rule.selectorText, id)) continue;
      for (const v of TOKEN_VARS) {
        const value = rule.style.getPropertyValue(v).trim();
        if (value) result[v] = value;
      }
    }
  }
  return result;
}

/** Fallback: lê os tokens computados do <html> (válido quando o tema já está ativo). */
function readTokensFromComputed(): Partial<TokenMap> {
  const result: Partial<TokenMap> = {};
  if (typeof window === 'undefined' || typeof document === 'undefined') return result;

  const cs = getComputedStyle(document.documentElement);
  for (const v of TOKEN_VARS) {
    const value = cs.getPropertyValue(v).trim();
    if (value) result[v] = value;
  }
  return result;
}

/** Resolve o conjunto completo de tokens para um tema; `null` se ainda indisponível. */
function resolveTokens(id: ThemeId): TokenMap | null {
  const merged: Partial<TokenMap> = { ...readTokensFromStylesheets(id) };

  const missing = TOKEN_VARS.filter((v) => !merged[v]);
  if (missing.length > 0 && typeof document !== 'undefined') {
    const active = document.documentElement.getAttribute('data-theme') ?? 'assistente';
    if (active === id) {
      const computed = readTokensFromComputed();
      for (const v of missing) {
        if (computed[v]) merged[v] = computed[v];
      }
    }
  }

  if (TOKEN_VARS.every((v) => merged[v])) return merged as TokenMap;
  return null;
}

function buildThemeConfig(t: TokenMap, isDark: boolean): ThemeConfig {
  return {
    algorithm: isDark ? darkAlgorithm : defaultAlgorithm,
    token: {
      colorPrimary: t['--accent-strong'],
      colorSuccess: t['--color-success'],
      colorError: t['--color-danger'],
      colorWarning: t['--color-warning'],
      colorInfo: t['--color-info'],
      colorTextBase: t['--text-primary'],
      colorTextSecondary: t['--text-secondary'],
      colorTextTertiary: t['--text-muted'],
      colorBgBase: t['--bg-base'],
      colorBgContainer: t['--bg-surface'],
      colorBgElevated: t['--bg-elevated'],
      colorBgLayout: t['--bg-base'],
      colorBorder: t['--border-default'],
      colorBorderSecondary: t['--border-subtle'],
      colorFillSecondary: t['--bg-hover'],
      colorFillTertiary: t['--bg-elevated'],
      borderRadius: 8,
      borderRadiusSM: 4,
      borderRadiusLG: 12,
      borderRadiusXS: 4,
      fontSize: 14,
      fontSizeSM: 12,
      fontSizeLG: 16,
      fontSizeXL: 20,
      fontSizeHeading1: 28,
      fontSizeHeading2: 24,
      fontSizeHeading3: 20,
      fontSizeHeading4: 16,
      fontFamily: "-apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif",
      boxShadow: t['--shadow-sm'],
      boxShadowSecondary: t['--shadow-md'],
      motionDurationFast: '0.1s',
      motionDurationMid: '0.2s',
      motionDurationSlow: '0.3s',
      controlHeight: 32,
      controlHeightSM: 24,
      controlHeightLG: 40,
    },
    components: {
      Button: {
        borderRadius: 8,
        primaryShadow: 'none',
        defaultShadow: 'none',
        colorPrimaryHover: t['--accent-hover'],
        colorPrimaryActive: t['--accent-strong'],
        defaultBorderColor: t['--border-default'],
        fontWeight: 500,
      },
      Card: {
        borderRadiusLG: 8,
        boxShadowTertiary: t['--shadow-sm'],
      },
      Input: {
        borderRadius: 8,
        activeBorderColor: t['--accent'],
        hoverBorderColor: t['--accent'],
        activeShadow: `0 0 0 2px ${t['--focus-ring']}`,
        colorBgContainer: t['--bg-input'],
      },
      Select: {
        borderRadius: 8,
        colorBorder: t['--border-default'],
        optionSelectedBg: t['--accent-dim'],
      },
      Modal: {
        borderRadiusLG: 12,
      },
      Table: {
        borderRadius: 8,
        headerBg: t['--bg-elevated'],
        rowHoverBg: t['--bg-hover'],
      },
      Tabs: {
        borderRadius: 8,
        inkBarColor: t['--accent'],
        itemActiveColor: t['--accent'],
        itemSelectedColor: t['--accent'],
        itemHoverColor: t['--accent-hover'],
      },
      Tag: {
        borderRadiusSM: 8,
      },
      Menu: {
        itemBorderRadius: 8,
        subMenuItemBorderRadius: 8,
        itemSelectedBg: t['--accent-dim'],
        itemSelectedColor: t['--accent'],
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

/** Config mínima (apenas algoritmo + raios) usada enquanto o `theme.css` não está disponível. */
function buildFallbackConfig(isDark: boolean): ThemeConfig {
  return {
    algorithm: isDark ? darkAlgorithm : defaultAlgorithm,
    token: {
      borderRadius: 8,
      borderRadiusSM: 4,
      borderRadiusLG: 12,
      borderRadiusXS: 4,
      fontSize: 14,
    },
  };
}

const themeConfigCache = new Map<ThemeId, ThemeConfig>();

export function getAntdTheme(id: ThemeId): ThemeConfig {
  const cached = themeConfigCache.get(id);
  if (cached) return cached;

  const isDark = id !== 'light';
  const tokens = resolveTokens(id);

  if (!tokens) {
    // theme.css ainda não foi aplicado/parsável: devolve config mínima sem cachear,
    // para que uma chamada posterior consiga resolver os tokens reais.
    return buildFallbackConfig(isDark);
  }

  const config = buildThemeConfig(tokens, isDark);
  themeConfigCache.set(id, config);
  return config;
}
