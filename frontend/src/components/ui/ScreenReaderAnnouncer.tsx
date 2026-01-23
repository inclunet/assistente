import { useAnnouncerState } from '../../hooks/useAnnouncer';
import './ScreenReaderAnnouncer.css';

/**
 * Componente de anúncios para leitores de tela.
 * Fornece duas regiões aria-live ocultas visualmente:
 * - Uma polite (não interrompe o leitor)
 * - Uma assertive (interrompe o leitor imediatamente)
 */
export function ScreenReaderAnnouncer() {
  const { politeMessage, assertiveMessage } = useAnnouncerState();

  return (
    <div className="sr-announcer">
      {/* Região polite - não interrompe a leitura atual */}
      <div
        className="sr-only"
        role="status"
        aria-live="polite"
        aria-atomic="true"
      >
        {politeMessage}
      </div>

      {/* Região assertive - interrompe a leitura atual */}
      <div
        className="sr-only"
        role="alert"
        aria-live="assertive"
        aria-atomic="true"
      >
        {assertiveMessage}
      </div>
    </div>
  );
}
