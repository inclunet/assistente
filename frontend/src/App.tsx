import { logger } from './utils/logger';
import { useCallback, useEffect, useRef, useState } from 'react';
import { Outlet, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { RespondQuestionnaire, NeedsWelcomeWizard, RunWelcomeWizard } from "@wailsjs/go/app/App";
import { EventsOn } from "@wailsjs/runtime/runtime";
import { useAuthStore } from './store/authStore';
import { useUIStore } from './store/uiStore';
import { useChatStore } from './store/chatStore';
import { parseDeepLink, executeDeepLink } from './lib/deepLinks';
import { ScreenReaderAnnouncer } from './components/ui/ScreenReaderAnnouncer';
import { ConfirmHost } from './components/ui/ConfirmHost';
import { QuestionnaireDialog, QuestionnairePayload } from './components/ui/QuestionnaireDialog';
import { useQuestionnaireUIStore } from './store/questionnaireUIStore';
import { useConnectionStatusListener } from './hooks/useConnectionStatusListener';
import { usePartialRuntimeInitListener } from './hooks/usePartialRuntimeInitListener';
import { ToastHost } from './components/ui/ToastHost';
import { useTheme } from './hooks/useTheme';
import { ConfigProvider } from 'antd';
import type { Locale } from 'antd/es/locale';
import { getAntdTheme } from './theme/antdTheme';
import { waitForWailsBridge } from './lib/waitForWailsBridge';
import { summaryErrorMessage } from './lib/summaryError';
import { chatNoticeMessage, type ChatNoticeEvent } from './lib/chatNotice';
import { AuthGate } from './components/auth/AuthGate';

function useAntdLocale(lang: string): Locale | undefined {
    const [locale, setLocale] = useState<Locale | undefined>(undefined);
    useEffect(() => {
        let cancelled = false;
        const loaders: Record<string, () => Promise<{ default: Locale }>> = {
            'pt-BR': () => import('antd/locale/pt_BR'),
            'en':    () => import('antd/locale/en_US'),
            'es':    () => import('antd/locale/es_ES'),
        };
        const load = loaders[lang] ?? loaders['en'];
        void load().then((m) => { if (!cancelled) setLocale(m.default); });
        return () => { cancelled = true; };
    }, [lang]);
    return locale;
}

type LegacyImportSummaryEvent = {
    userId?: string;
    imported?: number;
    skipped?: number;
    failed?: number;
    warningCount?: number;
    errorCount?: number;
};

function getCurrentAuthSnapshot() {
    const auth = useAuthStore.getState();
    return {
        isAuthenticated: auth.isAuthenticated,
        isLoading: auth.isLoading,
        userId: auth.user?.userId,
    };
}

function App() {
    const { theme } = useTheme();
    const { t, i18n } = useTranslation();
    const navigate = useNavigate();
    const antLocale = useAntdLocale(i18n.language);
    const isAuthenticated = useAuthStore((s) => s.isAuthenticated);
    const authUser = useAuthStore((s) => s.user);
    const authLoading = useAuthStore((s) => s.isLoading);
    const addToast = useUIStore((s) => s.addToast);
    const handleConversationDeleted = useChatStore((s) => s.handleConversationDeleted);
    const handleConversationCleared = useChatStore((s) => s.handleConversationCleared);
    const handleConversationRenamed = useChatStore((s) => s.handleConversationRenamed);
    const handleDatabaseReset = useChatStore((s) => s.handleDatabaseReset);
    const handleExternalIncoming = useChatStore((s) => s.handleExternalIncoming);
    const wasQuestionnaireOpenRef = useRef(false);
    const lastFocusedElementRef = useRef<HTMLElement | null>(null);
    const pendingLegacyImportSummaryRef = useRef<LegacyImportSummaryEvent | null>(null);

    // Estado do dialog de questionário (tool: collect_responses e aprovações)
    const [questionnaireOpen, setQuestionnaireOpen] = useState(false);
    const [questionnaireData, setQuestionnaireData] = useState<QuestionnairePayload | null>(null);

    // Questionários disparados pela UI (reusa o mesmo QuestionnaireDialog)
    const uiQuestionnaireData = useQuestionnaireUIStore((s) => s.active);
    const uiSubmit = useQuestionnaireUIStore((s) => s.submit);
    const uiCancel = useQuestionnaireUIStore((s) => s.cancel);

    // Status de conexão com a API LLM (Issue #38): assinatura única do evento,
    // anúncios de queda/restauração via announcer global + toast.
    useConnectionStatusListener();

    // Aviso não-bloqueante de runtime parcialmente inicializado pós-login
    // (issue #250): toast + announce com ação "Tentar novamente".
    usePartialRuntimeInitListener();

    const showLegacyImportSummary = useCallback((eventData: LegacyImportSummaryEvent) => {
        const currentUserId = getCurrentAuthSnapshot().userId;
        if (eventData.userId && currentUserId && eventData.userId !== currentUserId) return;

        const imported = eventData.imported ?? 0;
        const skipped = eventData.skipped ?? 0;
        const failed = eventData.failed ?? 0;
        const warnings = eventData.warningCount ?? 0;
        const errors = eventData.errorCount ?? 0;
        if (imported === 0 && skipped === 0 && failed === 0 && warnings === 0 && errors === 0) return;

        const toastType = failed > 0 || errors > 0 ? 'error' : warnings > 0 ? 'warning' : 'success';
        addToast(t('app.legacyImport.summary', { imported, skipped, failed, warnings }), toastType, 10000);
    }, [addToast, t]);

    useEffect(() => {
        if (!isAuthenticated || !authUser || !pendingLegacyImportSummaryRef.current) return;

        const pending = pendingLegacyImportSummaryRef.current;
        pendingLegacyImportSummaryRef.current = null;
        showLegacyImportSummary(pending);
    }, [authUser, isAuthenticated, showLegacyImportSummary]);

    useEffect(() => {
        if (!isAuthenticated && !authLoading) {
            pendingLegacyImportSummaryRef.current = null;
        }
    }, [authLoading, isAuthenticated]);

    useEffect(() => {
        const controller = new AbortController();

        const runBoot = async () => {
            await waitForWailsBridge({ signal: controller.signal });
            if (controller.signal.aborted) return;
            if (!isAuthenticated || !authUser) return;

            // Verifica se precisa do wizard de boas-vindas. Toda a configuração
            // de LLM/modelo/voz vive em profiles + provider registry; não há mais
            // config legado para carregar no boot (#299).
            //
            // A própria checagem (NeedsWelcomeWizard) também roda dentro do try:
            // se a ponte/IPC falhar, o usuário recebe feedback (app.wizard.error)
            // em vez de a falha cair silenciosamente só no .catch externo.
            try {
                const needsWizard = await NeedsWelcomeWizard();
                if (needsWizard) {
                    const completed = await RunWelcomeWizard();
                    if (!completed) {
                        addToast(t('app.wizard.cancelled'), 'warning');
                    } else {
                        addToast(t('app.wizard.success'), 'success', 5000);
                    }
                }
            } catch (error) {
                logger.error('[App] Erro no wizard de boas-vindas:', error);
                addToast(t('app.wizard.error'), 'error');
            }
        };

        void runBoot().catch((error) => {
            if (error instanceof DOMException && error.name === 'AbortError') {
                return;
            }
            logger.error('[App] Erro no boot:', error);
        });
        return () => controller.abort();
    }, [addToast, isAuthenticated, authUser, t]);

    // Escuta eventos de conversa deletada/limpa
    useEffect(() => {
        const unsubs: Array<() => void> = [];

        unsubs.push(EventsOn('conversation:deleted', (data: unknown) => {
            const eventData = data as { conversation_id?: string };
            if (eventData.conversation_id) {
                handleConversationDeleted(eventData.conversation_id);
            }
        }));

        unsubs.push(EventsOn('conversation:cleared', (data: unknown) => {
            const eventData = data as { conversation_id?: string };
            if (eventData.conversation_id) {
                handleConversationCleared(eventData.conversation_id);
            }
        }));

        unsubs.push(EventsOn('conversation:renamed', (data: unknown) => {
            const eventData = data as { conversationId?: string; newTitle?: string };
            if (eventData.conversationId && eventData.newTitle) {
                handleConversationRenamed(eventData.conversationId, eventData.newTitle);
            }
        }));

        unsubs.push(EventsOn('database:reset', () => {
            handleDatabaseReset();
        }));

        unsubs.push(EventsOn('navigate:update', () => {
            navigate('/update');
        }));

        unsubs.push(EventsOn('deeplink:execute', (uri: unknown) => {
            if (typeof uri !== 'string') return;
            const action = parseDeepLink(uri);
            if (action) {
                void executeDeepLink(action, { navigate });
            }
        }));

        unsubs.push(EventsOn('chat:summary_started', (data: unknown) => {
            const eventData = data as { messageCount?: number };
            addToast(t('app.summary.started', { count: eventData.messageCount ?? 0 }), 'info', 10000);
        }));

        unsubs.push(EventsOn('chat:summary_completed', (data: unknown) => {
            const eventData = data as { messageCount?: number };
            addToast(t('app.summary.completed', { count: eventData.messageCount ?? 0 }), 'success', 5000);
        }));

        unsubs.push(EventsOn('chat:summary_error', (data: unknown) => {
            const eventData = data as { error?: string; code?: string };
            addToast(summaryErrorMessage(t, eventData), 'error');
        }));

        // Aviso sobre o turno (ex.: anexo que o provedor não recebe). É global
        // de propósito: o controller de streaming vive só entre o envio e o
        // chat:done, e o aviso pode chegar antes de ele estar de pé.
        unsubs.push(EventsOn('chat:notice', (data: unknown) => {
            const message = chatNoticeMessage(t, data as ChatNoticeEvent);
            if (message) {
                addToast(message, 'warning', 10000);
            }
        }));

        unsubs.push(EventsOn('legacy:import_summary', (data: unknown) => {
            const eventData = data as LegacyImportSummaryEvent;
            const authSnapshot = getCurrentAuthSnapshot();
            if (!authSnapshot.isAuthenticated || !authSnapshot.userId) {
                if (authSnapshot.isLoading) {
                    pendingLegacyImportSummaryRef.current = eventData;
                }
                return;
            }
            showLegacyImportSummary(eventData);
        }));

        return () => {
            unsubs.forEach(fn => fn());
        };
    }, [addToast, handleConversationDeleted, handleConversationCleared, handleConversationRenamed, handleDatabaseReset, navigate, showLegacyImportSummary, t]);

    // Listener para mensagens de canais externos (Signal, Telegram).
    // Quando messaging:incoming chega, delega ao chatStore que monta placeholders
    // e registra listeners de streaming (chat:stream, chat:done, etc.) — mesmo fluxo
    // do sendMessage, com som, TTS, announcer e streaming em tempo real.
    useEffect(() => {
        const unsubIncoming = EventsOn('messaging:incoming', (data: unknown) => {
            const eventData = data as {
                channel?: string;
                from?: string;
                fromId?: string;
                text?: string;
                conversationId?: string;
                newConversation?: boolean;
            };
            handleExternalIncoming({
                channel: eventData.channel || '',
                from: eventData.from || '',
                fromId: eventData.fromId || '',
                text: eventData.text || '',
                conversationId: eventData.conversationId || '',
                newConversation: eventData.newConversation || false,
            });
        });

        const unsubLegacy = EventsOn('messaging:legacy_channel_dropped', (data: unknown) => {
            const eventData = data as { channel?: string; from?: string; reason?: string };
            const channel = eventData.channel || '';
            const message = t('channels.toast.legacyChannelDropped', { channel });
            addToast(message, 'warning', 10000);
        });

        const unsubPairing = EventsOn('messaging:pairing_pending', (data: unknown) => {
            const eventData = data as { channel?: string };
            addToast(t('channels.toast.pairingPending', { channel: eventData.channel || '' }), 'info', 8000);
        });

        const unsubAuthorized = EventsOn('messaging:contact_authorized', (data: unknown) => {
            const eventData = data as { channel?: string };
            addToast(t('channels.toast.contactAuthorized', { channel: eventData.channel || '' }), 'success', 8000);
        });

        return () => {
            unsubIncoming();
            unsubLegacy();
            unsubPairing();
            unsubAuthorized();
        };
    }, [handleExternalIncoming, addToast, t]);

    useEffect(() => {
        const unsub = EventsOn('tool:questionnaire', (data: QuestionnairePayload) => {
            lastFocusedElementRef.current = document.activeElement as HTMLElement;
            setQuestionnaireData(data);
            setQuestionnaireOpen(true);
        });
        return unsub;
    }, []);

    // Quando um questionário da UI abre, captura o foco atual
    useEffect(() => {
        if (uiQuestionnaireData && !questionnaireOpen) {
            lastFocusedElementRef.current = document.activeElement as HTMLElement;
        }
    }, [uiQuestionnaireData, questionnaireOpen]);

    // Previne menu de contexto nativo quando tecla ContextMenu ou Shift+F10 for pressionada
    useEffect(() => {
        const preventNativeContextMenu = (e: KeyboardEvent) => {
            if (e.key === 'ContextMenu' || (e.shiftKey && e.key === 'F10')) {
                e.preventDefault();
                e.stopPropagation();
            }
        };

        document.addEventListener('keydown', preventNativeContextMenu, true);
        return () => document.removeEventListener('keydown', preventNativeContextMenu, true);
    }, []);

    const restoreFocus = () => {
        requestAnimationFrame(() => {
            const el = lastFocusedElementRef.current;
            if (el && document.contains(el)) {
                el.focus();
                return;
            }
            const textarea = document.querySelector('.chat-input__textarea') as HTMLTextAreaElement | null;
            textarea?.focus();
        });
    };

    const effectiveQuestionnaireOpen = questionnaireOpen || !!uiQuestionnaireData;
    const effectiveQuestionnaireData = questionnaireData || uiQuestionnaireData;

    // Restaura foco quando qualquer questionário fecha
    useEffect(() => {
        if (!effectiveQuestionnaireOpen && wasQuestionnaireOpenRef.current) {
            restoreFocus();
        }
        wasQuestionnaireOpenRef.current = effectiveQuestionnaireOpen;
    }, [effectiveQuestionnaireOpen]);

    const handleQuestionnaireSubmit = async (answers: Record<string, unknown>) => {
        if (questionnaireOpen && questionnaireData) {
            try {
                await RespondQuestionnaire(questionnaireData.id, answers, false);
                setQuestionnaireOpen(false);
                setQuestionnaireData(null);
                restoreFocus();
            } catch (err) {
                logger.error('[App] Erro ao enviar questionário:', err);
                addToast(t('app.questionnaire.submitError'), 'error');
            }
            return;
        }

        // Questionário disparado pela UI
        uiSubmit(answers);
        restoreFocus();
    };

    const handleQuestionnaireCancel = async (answers?: Record<string, unknown>) => {
        if (questionnaireOpen && questionnaireData) {
            try {
                await RespondQuestionnaire(questionnaireData.id, answers ?? {}, true);
                setQuestionnaireOpen(false);
                setQuestionnaireData(null);
                restoreFocus();
            } catch (err) {
                logger.error('[App] Erro ao cancelar questionário:', err);
                addToast(t('app.questionnaire.submitError'), 'error');
            }
            return;
        }

        // Questionário disparado pela UI
        uiCancel(answers);
        restoreFocus();
    };

    return (
        <ConfigProvider theme={getAntdTheme(theme)} locale={antLocale}>
            <ScreenReaderAnnouncer />
            <ToastHost />
            <AuthGate>
                <Outlet />
                <ConfirmHost />
                <QuestionnaireDialog
                    isOpen={effectiveQuestionnaireOpen}
                    data={effectiveQuestionnaireData}
                    onSubmit={handleQuestionnaireSubmit}
                    onCancel={handleQuestionnaireCancel}
                />
            </AuthGate>
        </ConfigProvider>
    )
}

export default App
