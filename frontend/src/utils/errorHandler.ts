/**
 * Central Error Handling Utility
 * Provides consistent error handling across the application
 */

import { logger } from './logger';
import { announce } from '../hooks/useAnnouncer';

export enum ErrorSeverity {
  /** User error - validation, bad input */
  USER = 'user',
  /** Recoverable error - can retry */
  RECOVERABLE = 'recoverable',
  /** Recoverable error that should interrupt screen readers */
  RECOVERABLE_ASSERTIVE = 'recoverable_assertive',
  /** Fatal error - requires app restart or navigation */
  FATAL = 'fatal',
}

export interface ErrorContext {
  /** Where the error occurred */
  source: string;
  /** User-friendly error message */
  userMessage: string;
  /** Technical error message (for logs) */
  technicalMessage?: string;
  /** Error severity level */
  severity: ErrorSeverity;
  /** Retry callback if recoverable */
  onRetry?: () => void | Promise<void>;
  /** Additional context data */
  metadata?: Record<string, unknown>;
}

/**
 * Handle an error with consistent logging, user feedback, and recovery options
 */
export function handleError(error: unknown, context: ErrorContext): void {
  const errorMessage = error instanceof Error ? error.message : String(error);
  
  // Route the log through the centralized logger (not directly to console)
  logger.error(`[${context.source}] Error (${context.severity}):`, {
    userMessage: context.userMessage,
    technicalMessage: context.technicalMessage || errorMessage,
    error,
    metadata: context.metadata,
  });

  // Announce to screen readers
  const priority = context.severity === ErrorSeverity.FATAL
    || context.severity === ErrorSeverity.RECOVERABLE_ASSERTIVE
    ? 'assertive'
    : 'polite';
  announce(context.userMessage, priority);

  // TODO: Send to error tracking service (Sentry, etc.)
  // if (context.severity === ErrorSeverity.FATAL) {
  //   sendToErrorTracking(error, context);
  // }
}

/**
 * Wrap an async function with error handling
 */
export function withErrorHandling<T extends (...args: unknown[]) => Promise<unknown>>(
  fn: T,
  errorContext: Omit<ErrorContext, 'onRetry'>
): T {
  return (async (...args: Parameters<T>) => {
    try {
      return await fn(...args);
    } catch (error) {
      handleError(error, errorContext);
      throw error; // Re-throw so caller can handle if needed
    }
  }) as T;
}

/**
 * Common error messages for consistency
 */
export const ErrorMessages = {
  NETWORK: {
    OFFLINE: 'Sem conexão com a internet. Verifique sua conexão.',
    TIMEOUT: 'A requisição demorou muito. Tente novamente.',
    SERVER_ERROR: 'Erro no servidor. Tente novamente em alguns instantes.',
  },
  VALIDATION: {
    EMPTY_MESSAGE: 'A mensagem não pode estar vazia.',
    TOO_LARGE: 'O conteúdo é muito grande. Limite: ',
    INVALID_FILE: 'Arquivo inválido ou formato não suportado.',
  },
  CHAT: {
    SEND_FAILED: 'Falha ao enviar mensagem. Tente novamente.',
    DELETE_FAILED: 'Falha ao excluir mensagem. Tente novamente.',
    LOAD_FAILED: 'Falha ao carregar conversa. Tente novamente.',
  },
} as const;

/**
 * Retry helper with exponential backoff
 */
export async function retryWithBackoff<T>(
  fn: () => Promise<T>,
  options: {
    maxRetries?: number;
    initialDelay?: number;
    maxDelay?: number;
    onRetry?: (attempt: number, error: unknown) => void;
  } = {}
): Promise<T> {
  const {
    maxRetries = 3,
    initialDelay = 1000,
    maxDelay = 10000,
    onRetry,
  } = options;

  let lastError: unknown;

  for (let attempt = 0; attempt <= maxRetries; attempt++) {
    try {
      return await fn();
    } catch (error) {
      lastError = error;

      if (attempt < maxRetries) {
        const delay = Math.min(initialDelay * Math.pow(2, attempt), maxDelay);
        onRetry?.(attempt + 1, error);
        await new Promise(resolve => setTimeout(resolve, delay));
      }
    }
  }

  throw lastError;
}
