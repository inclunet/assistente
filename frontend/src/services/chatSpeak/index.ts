import { DispatchSpeech } from '@wailsjs/go/app/App';
import i18next from 'i18next';

import { messageAudioService } from '../messageAudio';
import { ttsService } from '../tts';
import type { VoiceRole } from '../tts';
import { announceWithOrigin } from '../voiceAccessibility/announcerBroker';
import { requestVoiceTTS } from '../voiceAccessibility/ttsBroker';
import type { VoiceAccessibilityOrigin } from '../voiceAccessibility/types';

export type ChatSpeakStrategy = 'announce' | 'webspeech' | 'backend_audio' | 'none';

export type ChatSpeakOrigin =
  | 'assistant_message'
  | 'user_message'
  | 'system_message'
  | 'thinking'
  | 'tool_status'
  | 'segment';

export interface ChatSpeakRequest {
  conversationId: string;
  messageId?: string;
  profileSlug?: string;
  role: VoiceRole;
  text: string;
  origin: ChatSpeakOrigin;
  interrupt?: boolean;
}

export interface ChatSpeakEvent {
  messageId?: string;
  conversationId?: string;
  role?: VoiceRole;
  text?: string;
  strategy?: ChatSpeakStrategy;
  fallbackStrategy?: ChatSpeakStrategy;
  autoRead?: boolean;
  providerId?: string;
  voiceId?: string;
  model?: string;
  rate?: number;
  pitch?: number;
  volume?: number;
  origin?: ChatSpeakOrigin;
  accessibilityOrigin?: VoiceAccessibilityOrigin;
  interrupt?: boolean;
}

function getRolePrefix(role: VoiceRole): string {
  switch (role) {
    case 'user':
      return i18next.t('chat.you');
    case 'system':
      return i18next.t('chat.system');
    default:
      return i18next.t('chat.assistant');
  }
}

function getAnnounceEventType(origin: ChatSpeakOrigin | undefined) {
  switch (origin) {
    case 'assistant_message':
      return 'completion';
    case 'segment':
    case 'thinking':
    case 'tool_status':
      return 'progress';
    case 'user_message':
      return 'user-action';
    case 'system_message':
    default:
      return 'system';
  }
}

function buildFallbackEvent(event: ChatSpeakEvent, strategy: ChatSpeakStrategy): ChatSpeakEvent {
  return { ...event, strategy };
}

async function executeFallback(event: ChatSpeakEvent): Promise<void> {
  if (!event.fallbackStrategy || event.fallbackStrategy === event.strategy) {
    return;
  }

  await handleChatSpeak(buildFallbackEvent(event, event.fallbackStrategy));
}

export async function dispatchChatSpeech(request: ChatSpeakRequest): Promise<void> {
  if (!request.text?.trim()) {
    return;
  }

  await DispatchSpeech(request);
}

export async function handleChatSpeak(event: ChatSpeakEvent): Promise<void> {
  if (!event.strategy || event.strategy === 'none') {
    return;
  }

  const role: VoiceRole = event.role ?? 'assistant';
  const text = event.text?.trim() ?? '';
  const stopCurrent = () => {
    messageAudioService.stopCurrentAudio();
    ttsService.stop();
  };

  if (event.strategy === 'announce' || event.autoRead === false) {
    if (text) {
      announceWithOrigin({
        message: `${getRolePrefix(role)}: ${text}`,
        origin: event.accessibilityOrigin,
        eventType: getAnnounceEventType(event.origin),
      });
    }
    return;
  }

  if (event.strategy === 'backend_audio') {
    if (event.messageId) {
      const played = await requestVoiceTTS({
        text,
        origin: event.accessibilityOrigin,
        priority: 'automatic-active',
        eventType: 'completion',
        interrupt: event.interrupt,
        stopCurrent,
        speak: () => messageAudioService.speakMessage(
          event.messageId!,
          event.volume ?? ttsService.getVolume(),
          event.providerId
            ? {
                providerId: event.providerId,
                voiceId: event.voiceId ?? '',
                model: event.model ?? '',
                rate: event.rate ?? 1.0,
              }
            : undefined,
        ),
      });

      if (!played) {
        // TTS falhou — delega ao fallback (que pode incluir announce)
        await executeFallback(event);
      }
      return;
    }

    // Sem messageId — degrada para fallback em mensagens do assistente e segmentos.
    // Segmentos intermediários são verbalizados via fallback (announce/webspeech)
    // enquanto o assistant_message final usará SpeakMessage com messageId.
    if (event.origin === 'assistant_message' || event.origin === 'segment') {
      await executeFallback(event);
    }
    return;
  }

  if (!text) {
    return;
  }

  if (event.strategy === 'webspeech') {
    await requestVoiceTTS({
      text,
      origin: event.accessibilityOrigin,
      priority: 'automatic-active',
      eventType: 'completion',
      interrupt: event.interrupt,
      stopCurrent,
      speak: () => ttsService.speakWithOverride(text, {
        providerId: event.providerId ?? 'webspeech',
        voiceName: event.voiceId,
        ttsModel: event.model,
        rate: event.rate,
        pitch: event.pitch,
        volume: event.volume,
      }),
    });
  }
}
