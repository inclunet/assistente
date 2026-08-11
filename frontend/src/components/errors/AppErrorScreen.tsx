import { useCallback, useEffect, useId, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '../ui/Button';
import './AppErrorScreen.css';

export interface AppErrorScreenProps {
  error: unknown;
  /**
   * Árvore de componentes até onde o erro estourou. É a única pista que nomeia
   * o culpado: o stack do erro em si costuma ter só quadros internos do React.
   */
  componentStack?: string | null;
}

type CopyState = 'idle' | 'done' | 'failed';

function descreverErro(error: unknown): { message: string; stack: string | null } {
  if (error instanceof Error) {
    return { message: error.message, stack: error.stack ?? null };
  }
  if (typeof error === 'string') {
    return { message: error, stack: null };
  }
  try {
    return { message: JSON.stringify(error), stack: null };
  } catch {
    return { message: String(error), stack: null };
  }
}

export function AppErrorScreen({ error, componentStack }: AppErrorScreenProps) {
  const { t } = useTranslation();
  const containerRef = useRef<HTMLElement>(null);
  const [copyState, setCopyState] = useState<CopyState>('idle');

  const headingId = useId();
  const descriptionId = useId();

  const { message, stack } = useMemo(() => descreverErro(error), [error]);

  const detalhes = useMemo(() => {
    const partes = [`${t('errorScreen.labels.message', 'Mensagem')}: ${message}`];
    if (stack) {
      partes.push(`\n${t('errorScreen.labels.stack', 'Pilha de chamadas')}:\n${stack}`);
    }
    if (componentStack) {
      partes.push(`\n${t('errorScreen.labels.components', 'Componentes')}:${componentStack}`);
    }
    return partes.join('\n');
  }, [componentStack, message, stack, t]);

  useEffect(() => {
    // O #root é `role="application"`, o que prende o leitor de telas em modo de
    // foco: ele anuncia "aplicativo" e não lê mais nada. Enquanto esta tela
    // estiver no ar não há aplicação nenhuma para operar — só texto para ler —,
    // então devolvemos o documento à navegação normal e restauramos ao sair.
    const root = document.getElementById('root');
    const roleAnterior = root?.getAttribute('role') ?? null;
    const rotuloAnterior = root?.getAttribute('aria-label') ?? null;

    root?.removeAttribute('role');
    root?.removeAttribute('aria-label');

    // Com o documento navegável, mover o foco para cá faz o leitor começar a
    // ler pelo título em vez de deixar o usuário procurando o que aconteceu.
    containerRef.current?.focus();

    return () => {
      if (roleAnterior !== null) root?.setAttribute('role', roleAnterior);
      if (rotuloAnterior !== null) root?.setAttribute('aria-label', rotuloAnterior);
    };
  }, []);

  const copiarDetalhes = useCallback(() => {
    void (async () => {
      try {
        await navigator.clipboard.writeText(detalhes);
        setCopyState('done');
      } catch {
        setCopyState('failed');
      }
    })();
  }, [detalhes]);

  const recarregar = useCallback(() => {
    window.location.reload();
  }, []);

  const avisoCopia = copyState === 'done'
    ? t('errorScreen.copyDone', 'Detalhes copiados para a área de transferência.')
    : copyState === 'failed'
      ? t('errorScreen.copyFailed', 'Não foi possível copiar. Selecione o texto dos detalhes e copie manualmente.')
      : '';

  return (
    <main
      className="app-error"
      ref={containerRef}
      tabIndex={-1}
      aria-labelledby={headingId}
      aria-describedby={descriptionId}
    >
      <div className="app-error__card">
        <h1 id={headingId} className="app-error__title">
          {t('errorScreen.title', 'O aplicativo encontrou um erro')}
        </h1>

        <p id={descriptionId} className="app-error__description">
          {t(
            'errorScreen.description',
            'A tela não pôde ser desenhada. Recarregar costuma resolver; se o erro voltar, copie os detalhes e registre uma issue.',
          )}
        </p>

        <p className="app-error__message">{message}</p>

        <div className="app-error__actions">
          <Button type="button" onClick={recarregar}>
            {t('errorScreen.reload', 'Recarregar o aplicativo')}
          </Button>
          <Button type="button" variant="secondary" onClick={copiarDetalhes}>
            {t('errorScreen.copy', 'Copiar detalhes')}
          </Button>
        </div>

        {/* A live region é local: quem anuncia no app é o ScreenReaderAnnouncer,
            que vive dentro da árvore que acabou de cair. */}
        <p className="app-error__copy-status" role="status" aria-live="polite">
          {avisoCopia}
        </p>

        <details className="app-error__details">
          <summary className="app-error__summary">
            {t('errorScreen.detailsSummary', 'Detalhes técnicos')}
          </summary>
          <pre className="app-error__stack">{detalhes}</pre>
        </details>
      </div>
    </main>
  );
}
