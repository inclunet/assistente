/** @vitest-environment jsdom */
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { VoiceButton } from './VoiceButton';
import { WorkspacePanelProvider } from '../workspace/WorkspacePanelContext';

const toggleInteractionSpy = vi.fn();
const startInteractionSpy = vi.fn();
const stopInteractionSpy = vi.fn();
const requestSTTStartSpy: ReturnType<typeof vi.fn<(request: unknown) => boolean>> = vi.fn(() => true);
const finishSTTSessionSpy: ReturnType<typeof vi.fn<(origin: unknown) => void>> = vi.fn();
const announceRequestSpy = vi.fn();
let triggerType = 'button_toggle';
let interimText = '';

const panelTab = {
  id: 'chat-tab',
  type: 'chat' as const,
  title: 'Chat',
  position: 0,
  conversationId: 'conversation-1',
};

function renderVoiceButton() {
  return render(
    <WorkspacePanelProvider value={{ tab: panelTab, isActive: true }}>
      <VoiceButton onTranscription={() => {}} />
    </WorkspacePanelProvider>,
  );
}

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock('../../hooks/useInteractionProfile', () => ({
  useInteractionProfile: () => ({
    isActive: false,
    isListening: false,
    isRecording: false,
    isProcessing: false,
    volume: 0,
    interimText,
    activeProfile: {
      input: {
        triggers: [{ type: triggerType, enabled: true }],
      },
    },
    startInteraction: startInteractionSpy,
    stopInteraction: stopInteractionSpy,
    cancelInteraction: vi.fn(),
    toggleInteraction: toggleInteractionSpy,
    isWakewordListening: false,
  }),
}));

vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({
    announceRequest: announceRequestSpy,
  }),
}));

vi.mock('../../services/voiceAccessibility/sttGate', () => ({
  requestSTTStart: (request: unknown) => requestSTTStartSpy(request),
  finishSTTSession: (origin: unknown) => finishSTTSessionSpy(origin),
}));

describe('VoiceButton', () => {
  beforeEach(() => {
    triggerType = 'button_toggle';
    interimText = '';
    toggleInteractionSpy.mockClear();
    startInteractionSpy.mockClear();
    stopInteractionSpy.mockClear();
    announceRequestSpy.mockClear();
    requestSTTStartSpy.mockReset();
    requestSTTStartSpy.mockReturnValue(true);
    finishSTTSessionSpy.mockClear();
  });

  it('aciona toggle no clique', () => {
    renderVoiceButton();

    fireEvent.click(screen.getByRole('button'));
    expect(toggleInteractionSpy).toHaveBeenCalled();
  });

  it('usa aria-label do modo', () => {
    renderVoiceButton();

    expect(screen.getByRole('button')).toHaveAttribute('aria-label', 'voice.clickToRecord');
  });

  it('não entra em PTT quando o gate STT nega pointer down', () => {
    triggerType = 'button_ptt';
    requestSTTStartSpy.mockReturnValue(false);
    renderVoiceButton();

    fireEvent.pointerDown(screen.getByRole('button'), { pointerId: 1 });

    expect(startInteractionSpy).not.toHaveBeenCalled();
    expect(screen.getByRole('button')).toHaveAttribute('aria-pressed', 'false');
  });

  it('não entra em PTT quando o gate STT nega teclado', () => {
    triggerType = 'button_ptt';
    requestSTTStartSpy.mockReturnValue(false);
    renderVoiceButton();

    fireEvent.keyDown(screen.getByRole('button'), { key: ' ' });

    expect(startInteractionSpy).not.toHaveBeenCalled();
    expect(screen.getByRole('button')).toHaveAttribute('aria-pressed', 'false');
  });

  it('envia texto interim para o announcer global sem live region local', () => {
    interimText = 'texto parcial';

    renderVoiceButton();

    expect(screen.getByText('texto parcial')).not.toHaveAttribute('aria-live');
    expect(announceRequestSpy).toHaveBeenCalledWith({
      message: 'texto parcial',
      origin: {
        tabId: 'chat-tab',
        surfaceId: 'chat-tab',
        conversationId: 'conversation-1',
        surfaceType: 'chat',
        profileSlug: null,
        title: 'Chat',
      },
      eventType: 'progress',
      deduplicate: true,
    });
  });
});
