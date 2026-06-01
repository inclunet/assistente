/**
 * Logger condicional centralizado.
 *
 * Este é o ÚNICO ponto autorizado a chamar `console.*` diretamente no app
 * (a regra ESLint `no-console` é desligada apenas para este arquivo).
 * Todo o restante do frontend deve usar `logger` em vez de `console`.
 *
 * Política de níveis (dev vs. produção):
 * - `debug` / `log` / `info`: emitidos apenas em desenvolvimento
 *   (`import.meta.env.DEV`) ou quando `VITE_DEBUG` está habilitado.
 *   Em produção ficam silenciados para não poluir o console nem expor
 *   informações sensíveis.
 * - `warn` e `error`: preservados sempre (também em produção), pois
 *   sinalizam problemas reais úteis para diagnóstico.
 *
 * O nível pode ser ajustado em runtime via `logger.setLevel(...)`, útil
 * para depuração pontual sem recompilar.
 */

export type LogLevel = 'silent' | 'error' | 'warn' | 'info' | 'debug';

const LEVEL_PRIORITY: Record<LogLevel, number> = {
  silent: 0,
  error: 1,
  warn: 2,
  info: 3,
  debug: 4,
};

const readEnvFlag = (): boolean => {
  try {
    const env = import.meta.env as ImportMetaEnv | undefined;
    if (!env) return false;
    // Aceita VITE_DEBUG="true" | "1" para habilitar logs de debug em qualquer build.
    const flag = env.VITE_DEBUG;
    return flag === 'true' || flag === '1';
  } catch {
    return false;
  }
};

const isDev = (): boolean => {
  try {
    return Boolean((import.meta.env as ImportMetaEnv | undefined)?.DEV);
  } catch {
    return false;
  }
};

const resolveDefaultLevel = (): LogLevel => {
  // Dev (ou VITE_DEBUG): tudo. Produção: apenas warn/error.
  return isDev() || readEnvFlag() ? 'debug' : 'warn';
};

let currentLevel: LogLevel = resolveDefaultLevel();

const shouldLog = (level: Exclude<LogLevel, 'silent'>): boolean => (
  LEVEL_PRIORITY[currentLevel] >= LEVEL_PRIORITY[level]
);

export interface Logger {
  /** Log de debug detalhado — silenciado em produção. */
  debug: (...args: unknown[]) => void;
  /** Alias de `debug` para substituição direta de `console.log`. */
  log: (...args: unknown[]) => void;
  /** Mensagem informativa — silenciada em produção. */
  info: (...args: unknown[]) => void;
  /** Aviso — preservado também em produção. */
  warn: (...args: unknown[]) => void;
  /** Erro — preservado também em produção. */
  error: (...args: unknown[]) => void;
  /** Define o nível mínimo de log em runtime. */
  setLevel: (level: LogLevel) => void;
  /** Retorna o nível atual de log. */
  getLevel: () => LogLevel;
}

export const logger: Logger = {
  debug: (...args: unknown[]): void => {
    if (shouldLog('debug')) console.debug(...args);
  },
  log: (...args: unknown[]): void => {
    if (shouldLog('debug')) console.debug(...args);
  },
  info: (...args: unknown[]): void => {
    if (shouldLog('info')) console.info(...args);
  },
  warn: (...args: unknown[]): void => {
    if (shouldLog('warn')) console.warn(...args);
  },
  error: (...args: unknown[]): void => {
    if (shouldLog('error')) console.error(...args);
  },
  setLevel: (level: LogLevel): void => {
    currentLevel = level;
  },
  getLevel: (): LogLevel => currentLevel,
};

export default logger;
