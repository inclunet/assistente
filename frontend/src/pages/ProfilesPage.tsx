import { useState, useEffect, useMemo, useCallback, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { useLocation, useNavigate } from 'react-router-dom';
import {
  CheckOutlined,
  CopyOutlined,
  DeleteOutlined,
  EditOutlined,
  PlusOutlined,
} from '@ant-design/icons';
import {
  GetProfiles,
  GetActiveProfileSlug,
  GetProfileSearchPaths,
} from '@wailsjs/go/wailsapi/Profiles';
import { profiles } from '../../wailsjs/go/models';
import { DataGrid, DataGridColumn } from '../components/ui/DataGrid';
import { MenuButton } from '../components/layout/MenuButton';
import { Toolbar } from '../components/ui/Toolbar';
import { Button, PageLoading } from '../components';
import { Modal } from '../components/ui/Modal';
import { EditorPanelFooter } from '../components/ui/EditorPanel';
import { ProfileEditorTabs } from '../components/profiles/ProfileEditorTabs';
import { useGridFocus } from '../hooks/useGridFocus';
import { useGridPageLandmarks } from '../hooks/useGridPageLandmarks';
import { useAnnouncer } from '../hooks/useAnnouncer';
import { useUIStore } from '../store/uiStore';
import { useEditableList } from '../hooks/useEditableList';
import { useProfileDependencies } from '../hooks/useProfileDependencies';
import { useResourceEditRequest } from '../hooks/useResourceEditRequest';
import type { ResourceEditRequest } from '../store/navigationStore';
import { useWorkspaceStore } from '../store/workspaceStore';
import { useAuthStore } from '../store/authStore';
import { useWorkspaceChatModalStore } from '../store/workspaceChatModalStore';
import {
  buildTabChatSurfaceId,
  buildWorkspaceModalChatSurfaceId,
} from '../services/chatSessionRegistry';
import { profileDisplayDescription } from '../lib/profileDescription';
import { usePagePresentationCommands, type PagePresentationCommandID } from '../lib/commandPagePresentation';
import { getModalRegistrySnapshot } from '../lib/modalRegistry';
import { useCommandShortcutHint } from '../lib/commandShortcutHints';
import { readProfileCommandTarget } from '../lib/commandPageMutationWails';
import {
  usePageMutationCommands,
  type PageMutationID,
  type PageMutationRequest,
  type PageMutationResult,
} from '../lib/commandPageMutation';
import './ProfilesPage.css';

type ProfileInfo = profiles.ProfileInfo;
type Profile = profiles.Profile;

interface ProfileRow extends Profile {
  id: string; // slug as id
  slug: string;
  source?: string;
  builtin?: boolean;
  isActive?: boolean;
  [key: string]: unknown;
}

interface ProfileEditorSnapshot {
  slug: string | null;
  fingerprint: string;
  version: number;
}

interface ProfileMutationContext {
  pathname: string;
  ownerId: string;
  sessionId: string;
  workspaceId: string;
  activeTabId: string | null;
}

interface ProfileMutationSuccessGuard {
  context: ProfileMutationContext;
  editorVersion?: number;
  draftVersion?: number;
  targetSlug?: string;
  presentationGeneration?: number;
  capturedTarget?: ProfileRow;
}

function cloneProfile(profile: Profile): Profile {
  return profiles.Profile.createFrom(JSON.parse(JSON.stringify(profile))) as Profile;
}

function profileContextKey(context: ProfileMutationContext | null): string {
  return context
    ? `${context.pathname}|${context.ownerId}|${context.sessionId}|${context.workspaceId}|${context.activeTabId ?? ''}`
    : '';
}

export default function ProfilesPage() {
  const { t } = useTranslation();
  const { pathname } = useLocation();
  const navigate = useNavigate();
  const createProfileShortcut = useCommandShortcutHint('profiles.create.open', 'profiles');
  const addToast = useUIStore((s) => s.addToast);
  const { announce } = useAnnouncer();
  const { handleGridReady } = useGridFocus();
  useGridPageLandmarks({ pageClass: 'profiles-page' });

  const getErrorMessage = (error: unknown) =>
    error instanceof Error ? error.message : String(error ?? '');

  // Grid state
  const [activeSlug, setActiveSlug] = useState<string>('padrao');
  const [searchTerm, setSearchTerm] = useState('');
  const [selectedIds, setSelectedIds] = useState<Set<string | number>>(new Set());
  const [searchPaths, setSearchPaths] = useState<string[]>([]);
  const [focusedRow, setFocusedRow] = useState<ProfileRow | null>(null);
  const [editorRequest, setEditorRequest] = useState<ResourceEditRequest | null>(null);
  const [editorReadLoading, setEditorReadLoading] = useState(false);
  const [mutationBusy, setMutationBusy] = useState(false);
  const pageRef = useRef<HTMLDivElement>(null);
  const editorRootRef = useRef<HTMLDivElement>(null);
  const presentationTargetRef = useRef<ProfileRow | null>(null);
  const profileLoadRequestRef = useRef(0);
  const profileListRequestRef = useRef(0);
  const profileRowsRef = useRef<ProfileRow[]>([]);
  const committedProfileContextRef = useRef('');
  const editorSnapshotRef = useRef<ProfileEditorSnapshot>({ slug: null, fingerprint: '', version: 0 });
  const editorVersionRef = useRef(0);
  const editorDraftVersionRef = useRef(0);
  const editorOpenRef = useRef(false);
  const editorReadLoadingRef = useRef(false);
  const mutationBusyRef = useRef(false);
  const mutationRunRef = useRef(0);
  const inlineUpdateRef = useRef<{ row: ProfileRow; name: string } | null>(null);
  const presentationGenerationRef = useRef(0);
  const mountedRef = useRef(true);
  const pathnameRef = useRef(pathname);
  pathnameRef.current = pathname;

  const setEditorReadBusy = useCallback((busy: boolean) => {
    editorReadLoadingRef.current = busy;
    setEditorReadLoading(busy);
  }, []);

  const setMutationBusyState = useCallback((busy: boolean) => {
    mutationBusyRef.current = busy;
    setMutationBusy(busy);
  }, []);

  const invalidateProfileLoad = useCallback(() => {
    profileLoadRequestRef.current += 1;
    profileListRequestRef.current += 1;
  }, []);

  const readProfileContext = useCallback((): ProfileMutationContext | null => {
    const auth = useAuthStore.getState();
    const workspace = useWorkspaceStore.getState().workspace;
    if (!auth.isAuthenticated || !auth.user || !workspace) return null;
    return {
      pathname: pathnameRef.current,
      ownerId: auth.user.userId,
      sessionId: auth.user.sessionId,
      workspaceId: workspace.id,
      activeTabId: workspace.activeTabId ?? null,
    };
  }, []);

  const isProfileContextCurrent = useCallback((expected: ProfileMutationContext) => {
    const current = readProfileContext();
    return mountedRef.current && current !== null &&
      current.pathname === expected.pathname &&
      current.ownerId === expected.ownerId &&
      current.sessionId === expected.sessionId &&
      current.workspaceId === expected.workspaceId &&
      current.activeTabId === expected.activeTabId;
  }, [readProfileContext]);

  const captureProfileRow = useCallback((row: ProfileRow | null) => {
    if (presentationTargetRef.current !== row) ++presentationGenerationRef.current;
    presentationTargetRef.current = row;
    setFocusedRow(row);
  }, []);

  useEffect(() => {
    mountedRef.current = true;
    const unsubscribeAuth = useAuthStore.subscribe(invalidateProfileLoad);
    const unsubscribeWorkspace = useWorkspaceStore.subscribe(invalidateProfileLoad);
    const invalidatePresentationContext = () => invalidateProfileLoad();
    window.addEventListener('blur', invalidatePresentationContext);
    document.addEventListener('compositionstart', invalidatePresentationContext, true);
    return () => {
      mountedRef.current = false;
      invalidateProfileLoad();
      unsubscribeAuth();
      unsubscribeWorkspace();
      window.removeEventListener('blur', invalidatePresentationContext);
      document.removeEventListener('compositionstart', invalidatePresentationContext, true);
    };
  }, [invalidateProfileLoad]);

  useEffect(() => {
    invalidateProfileLoad();
  }, [pathname, invalidateProfileLoad]);

  const returnToCaller = useCallback((request: ResourceEditRequest | null) => {
    const caller = request?.caller;
    if (!caller) return;

    const workspaceState = useWorkspaceStore.getState();
    const callerTab = workspaceState.workspace?.tabs.find((tab) => tab.id === caller.tabId);
    const surfaceMatches = caller.surfaceType === 'modal'
      ? caller.surfaceId === buildWorkspaceModalChatSurfaceId(caller.tabId)
      : caller.surfaceType === 'page'
        ? caller.surfaceId === buildTabChatSurfaceId(caller.tabId, 'page')
        : caller.surfaceId.trim().length > 0;
    const conversationMatches = (callerTab?.conversationId ?? null) === caller.conversationId;
    if (!callerTab || !surfaceMatches || !conversationMatches) return;

    navigate('/');
    requestAnimationFrame(() => {
      workspaceState.setActiveTab(caller.tabId);
      if (caller.surfaceType === 'modal') {
        void useWorkspaceChatModalStore.getState().requestOpen(caller.tabId);
      }
    });
  }, [navigate]);

  const { tools: availableTools, skills: availableSkills, allowlists: availableAllowlists, contextProviders: availableContextProviders, loading: depsLoading } =
    useProfileDependencies();

  const crud = useEditableList<ProfileRow, Profile, Profile>(
    {
      loadItems: async () => {
        const requestId = ++profileListRequestRef.current;
        const contextAtStart = readProfileContext();
        const contextKeyAtStart = profileContextKey(contextAtStart);
        const [allProfiles, currentSlug] = await Promise.all([
          GetProfiles(),
          GetActiveProfileSlug(),
        ]);
        const contextIsCurrent = contextAtStart !== null &&
          requestId === profileListRequestRef.current &&
          contextKeyAtStart === profileContextKey(readProfileContext());
        if (!mountedRef.current || !contextIsCurrent) {
          return contextKeyAtStart === committedProfileContextRef.current
            ? profileRowsRef.current
            : [];
        }
        const resolvedSlug = currentSlug || 'padrao';
        setActiveSlug(resolvedSlug);

        const rows = (allProfiles || []).map((p: ProfileInfo) => ({
          id: p.slug,
          slug: p.slug,
          name: p.name,
          description: p.description || '',
          icon: p.icon || '',
          source: p.source,
          builtin: p.builtin,
          isActive: p.slug === resolvedSlug,
        })) as ProfileRow[];
        profileRowsRef.current = rows;
        committedProfileContextRef.current = contextKeyAtStart;
        return rows;
      },
      loadItem: async (id) => {
        const requestId = ++profileLoadRequestRef.current;
        const contextAtStart = readProfileContext();
        const targetAtStart = presentationTargetRef.current;
        const modalGenerationAtStart = getModalRegistrySnapshot().generation;
        const editorVersionAtStart = editorVersionRef.current;
        const target = await readProfileCommandTarget(String(id));
        if (
          !mountedRef.current
          || contextAtStart === null
          || !isProfileContextCurrent(contextAtStart)
          || requestId !== profileLoadRequestRef.current
          || getModalRegistrySnapshot().generation !== modalGenerationAtStart
          || (targetAtStart !== null && presentationTargetRef.current !== targetAtStart)
          || editorVersionAtStart !== editorVersionRef.current
          || !target.profile
        ) {
          throw new Error('stale profile presentation');
        }
        const profile = profiles.Profile.createFrom(target.profile) as ProfileRow;
        editorSnapshotRef.current = {
          slug: String(id),
          fingerprint: target.fingerprint,
          version: editorVersionAtStart,
        };
        editorDraftVersionRef.current += 1;
        setEditorReadBusy(false);
        const row = profile;
        row.id = String(id);
        row.slug = String(id);
        row.source = (target.profile as { source?: string }).source ?? 'workdir';
        row.isActive = row.slug === activeSlug;
        return row;
      },
    },
    {
      entityName: t('profiles.entityName', 'Perfil'),
      messages: {
        loadError: t('profiles.loadError', 'Erro ao carregar perfis'),
        createSuccess: t('profiles.created', 'Perfil criado com sucesso!'),
        createError: t('profiles.saveError', 'Erro ao criar perfil'),
        updateSuccess: t('profiles.updated', 'Perfil atualizado com sucesso!'),
        updateError: t('profiles.saveError', 'Erro ao atualizar perfil'),
        deleteSuccess: t('profiles.deleted', 'Perfil excluído!'),
        deleteError: t('profiles.deleteError', 'Erro ao excluir perfil'),
      },
      validate: (item) => {
        if (!item.name?.trim()) {
          return t('profiles.nameRequired', 'Nome é obrigatório');
        }
        return null;
      },
      canDelete: (item) => {
        if (item.isActive) {
          return t('profiles.cannotDeleteActive', 'Não é possível excluir o perfil ativo');
        }
        return true;
      },
      createDefault: () => {
        const defaultProfile = profiles.Profile.createFrom({
          name: 'Novo Perfil',
          description: '',
          icon: 'chatbox',
          chat: {
            llm_provider: '',
            model: '',
            temperature: 0.7,
            max_tokens: 4096,
            top_p: 1.0,
            response_timeout: 180,
            rate_limit_enabled: true,
            rate_limit_rpm: 60,
            rate_limit_burst: 30,
            reasoning_effort: '',
            prompt_cache: {
              enabled: false,
              provider_hints: false,
              explicit_cache_control: false,
            },
            debug: {
              enabled: false,
              dump_requests: true,
              dump_responses: true,
              max_files: 200,
            },
            streaming_recovery_enabled: true,
            streaming_recovery_max_attempts: 3,
            streaming_recovery_show_continue: true,
            system_prompt: '',
            system_prompt_position: 'after',
          },
          voice: {
            assistant: {
              enabled: false,
              provider: 'disabled',
              voice_id: '',
              rate: 1.0,
              pitch: 1.0,
              volume: 1.0,
            },
            user: {
              enabled: false,
              provider: 'disabled',
              rate: 1.0,
              pitch: 1.0,
              volume: 1.0,
            },
            system: {
              enabled: false,
              provider: 'disabled',
              rate: 1.0,
              pitch: 1.0,
              volume: 1.0,
            },
          },
          input: {
            enabled: true,
            stt_provider: 'webspeech',
            language: 'pt-BR',
            feedback_sounds: true,
            triggers: [],
          },
          channels: {
            response_mode: 'mirror',
          },
          context_providers: {},
        }) as ProfileRow;
        defaultProfile.id = '';
        defaultProfile.isActive = false;
        defaultProfile.source = 'workdir';
        return defaultProfile;
      },
      skipBuiltInDeleteConfirm: true,
    }
  );

  const startNewProfile = useCallback((request: ResourceEditRequest | null = null) => {
    invalidateProfileLoad();
    editorOpenRef.current = true;
    setEditorReadBusy(true);
    const version = ++editorVersionRef.current;
    const contextAtStart = readProfileContext();
    editorSnapshotRef.current = { slug: null, fingerprint: '', version };
    editorDraftVersionRef.current += 1;
    setEditorRequest(request);
    crud.openNew();
    void readProfileCommandTarget('').then((target) => {
      if (!mountedRef.current || editorVersionRef.current !== version || !editorOpenRef.current ||
        !contextAtStart || !isProfileContextCurrent(contextAtStart)) return;
      editorSnapshotRef.current = { slug: null, fingerprint: target.fingerprint, version };
      setEditorReadBusy(false);
    }).catch((error: unknown) => {
      if (!mountedRef.current || editorVersionRef.current !== version || !editorOpenRef.current ||
        !contextAtStart || !isProfileContextCurrent(contextAtStart)) return;
      setEditorReadBusy(false);
      const message = getErrorMessage(error) || t('profiles.loadError', 'Erro ao carregar perfil');
      addToast(message, 'error', undefined, undefined, { suppressAnnounce: true });
    });
  }, [addToast, crud, invalidateProfileLoad, isProfileContextCurrent, readProfileContext, setEditorReadBusy, t]);

  useEffect(() => {
    void crud.loadItems();
    void GetProfileSearchPaths().then((paths) => {
      if (mountedRef.current) setSearchPaths(paths || []);
    });
  }, []);

  useResourceEditRequest('profiles', {
    onEdit: (slug, request) => {
      invalidateProfileLoad();
      editorOpenRef.current = true;
      setEditorReadBusy(true);
      const version = ++editorVersionRef.current;
      editorSnapshotRef.current = { slug: String(slug), fingerprint: '', version };
      editorDraftVersionRef.current += 1;
      setEditorRequest(request);
      void crud.openEdit({ id: slug, slug } as ProfileRow);
    },
    onNew: (request) => {
      startNewProfile(request);
    },
    ready: !crud.loading && crud.items.length > 0,
  });

  // --- Grid actions ---

  const handleEditProfile = useCallback(async (row: ProfileRow) => {
    invalidateProfileLoad();
    editorOpenRef.current = true;
    setEditorReadBusy(true);
    const version = ++editorVersionRef.current;
    editorSnapshotRef.current = { slug: row.slug, fingerprint: '', version };
    editorDraftVersionRef.current += 1;
    setEditorRequest(null);
    await crud.openEdit(row);
    if (mountedRef.current && editorVersionRef.current === version && editorOpenRef.current) {
      setEditorReadBusy(false);
    }
  }, [crud, invalidateProfileLoad, setEditorReadBusy]);

  const handleNewProfile = () => {
    startNewProfile();
  };

  const pagePresentationCommands: readonly PagePresentationCommandID[] = [
    'profiles.create.open',
    'profiles.edit.open',
    'profiles.search.focus',
  ];

  const { request: requestPagePresentationCommand } = usePagePresentationCommands({
    root: pageRef,
    pathname,
    allowedCommands: pagePresentationCommands,
    readTarget: () => presentationTargetRef.current,
    isCurrent: () => pathname === '/profiles',
    canOpen: (id) => {
      if (crud.editingItem) return false;
      if (id === 'profiles.edit.open') {
        const target = presentationTargetRef.current;
        return target !== null && crud.items.some((item) => item === target);
      }
      if (id === 'profiles.search.focus') {
        return Boolean(pageRef.current?.querySelector<HTMLInputElement>('.toolbar__search'));
      }
      return true;
    },
    open: (id) => {
      if (crud.editingItem) return false;
      if (id === 'profiles.create.open') {
        handleNewProfile();
        return true;
      }
      if (id === 'profiles.edit.open') {
        const target = presentationTargetRef.current;
        const row = target === null ? undefined : crud.items.find((item) => item === target);
        if (!row) return false;
        void handleEditProfile(row);
        return true;
      }
      if (id === 'profiles.search.focus') {
        const search = pageRef.current?.querySelector<HTMLInputElement>('.toolbar__search');
        if (!search) return false;
        search.focus();
        return true;
      }
      return false;
    },
  });

  const handleCloseEditor = useCallback((force = false) => {
    invalidateProfileLoad();
    if (!force && mutationBusyRef.current) return;
    ++editorVersionRef.current;
    ++editorDraftVersionRef.current;
    editorOpenRef.current = false;
    editorSnapshotRef.current = { slug: null, fingerprint: '', version: editorVersionRef.current };
    setEditorReadBusy(false);
    const request = editorRequest;
    crud.closeEditor();
    setEditorRequest(null);
    returnToCaller(request);
  }, [crud, editorRequest, invalidateProfileLoad, returnToCaller, setEditorReadBusy]);

  const updateFields = (updates: Record<string, unknown>) => {
    if (!crud.editingItem) return;
    const updated = JSON.parse(JSON.stringify(crud.editingItem));

    const setDeepValue = (target: Record<string, unknown>, path: string, value: unknown) => {
      const keys = path.split('.');
      let obj = target as Record<string, unknown>;
      for (let i = 0; i < keys.length - 1; i++) {
        const key = keys[i];
        const next = obj[key];
        if (!next || typeof next !== 'object') {
          obj[key] = {};
        }
        obj = obj[key] as Record<string, unknown>;
      }
      obj[keys[keys.length - 1]] = value;
    };

    for (const [path, value] of Object.entries(updates)) {
      setDeepValue(updated, path, value);
    }

    ++editorDraftVersionRef.current;
    const next = profiles.Profile.createFrom(updated) as ProfileRow;
    next.id = crud.editingItem.id || next.slug || String(crud.editingId || '');
    next.source = crud.editingItem.source;
    next.isActive = crud.editingItem.isActive;
    crud.setEditingItem(next);
  };

  const updateField = (path: string, value: unknown) => {
    updateFields({ [path]: value });
  };

  const handlePageMutationSucceeded = useCallback(async (
    commandId: PageMutationID,
    result: PageMutationResult,
    guard: ProfileMutationSuccessGuard,
  ) => {
    if (!isProfileContextCurrent(guard.context)) return;
    await crud.loadItems();
    if (!isProfileContextCurrent(guard.context)) return;
    if (guard.editorVersion !== undefined && (
      !editorOpenRef.current ||
      editorVersionRef.current !== guard.editorVersion ||
      (guard.draftVersion !== undefined && editorDraftVersionRef.current !== guard.draftVersion)
    )) return;
    if (guard.capturedTarget && guard.targetSlug && (
      presentationTargetRef.current?.slug !== guard.targetSlug ||
      guard.presentationGeneration !== presentationGenerationRef.current
    )) return;

    const successMessage = commandId === 'profiles.create'
      ? t('profiles.created', 'Perfil criado com sucesso!')
      : commandId === 'profiles.update'
        ? t('profiles.updated', 'Perfil atualizado com sucesso!')
        : commandId === 'profiles.duplicate'
          ? t('profiles.duplicated', 'Perfil duplicado!')
          : commandId === 'profiles.activate'
            ? t('profiles.activated', `Perfil "${result.title}" ativado!`)
            : t('profiles.deleted', 'Perfil excluído!');
    addToast(successMessage, 'success', undefined, undefined, { suppressAnnounce: true });
    announce(commandId === 'profiles.activate'
      ? t('profiles.activatedAnnounce', `Perfil ${result.title} ativado`)
      : successMessage);

    if (commandId === 'profiles.create' || commandId === 'profiles.update' || commandId === 'profiles.delete') {
      if (editorOpenRef.current) handleCloseEditor(true);
    }
    if (commandId === 'profiles.duplicate' && result.id) {
      setEditorRequest(null);
      await handleEditProfile({ id: result.id, slug: result.id, name: result.title } as ProfileRow);
    }
    if (commandId === 'profiles.delete' && guard.targetSlug && presentationTargetRef.current?.slug === guard.targetSlug) {
      captureProfileRow(null);
    }
  }, [addToast, announce, captureProfileRow, crud, handleCloseEditor, handleEditProfile, isProfileContextCurrent, t]);

  const { request: requestEditorPageMutation } = usePageMutationCommands({
    root: editorRootRef,
    pathname,
    allowedCommands: ['profiles.create', 'profiles.update', 'profiles.activate', 'profiles.delete'],
    canStart: (commandId) => {
      if (!editorOpenRef.current || editorReadLoadingRef.current || mutationBusyRef.current || !crud.editingItem) return false;
      if (commandId === 'profiles.create') return crud.isNew && Boolean(editorSnapshotRef.current.fingerprint);
      if (commandId === 'profiles.update') return !crud.isNew && Boolean(editorSnapshotRef.current.slug && editorSnapshotRef.current.fingerprint);
      return !crud.isNew && Boolean(crud.editingId && editorSnapshotRef.current.fingerprint) &&
        (commandId === 'profiles.activate' ? activeSlug !== String(crud.editingId) : activeSlug !== String(crud.editingId));
    },
    prepare: (commandId) => {
      const capturedEditor = crud.editingItem;
      const capturedId = crud.editingId === null ? '' : String(crud.editingId);
      const snapshot = { ...editorSnapshotRef.current };
      const version = editorVersionRef.current;
      const draftVersion = editorDraftVersionRef.current;
      const context = readProfileContext();
      if (!capturedEditor || !context) return undefined;
      const profile = commandId === 'profiles.create' || commandId === 'profiles.update'
        ? cloneProfile(capturedEditor)
        : undefined;
      const expectedFingerprint = snapshot.fingerprint;
      if (commandId === 'profiles.create' && !expectedFingerprint) return undefined;
      if (commandId !== 'profiles.create' && (!capturedId || !expectedFingerprint)) return undefined;
      return {
        readRequest: async (): Promise<PageMutationRequest> => {
          if (commandId === 'profiles.create') {
            return { targetId: '', expectedFingerprint, title: '', description: '', profile };
          }
          return { targetId: capturedId, expectedFingerprint, title: '', description: '', profile };
        },
        isCurrent: () => editorOpenRef.current &&
          editorVersionRef.current === version &&
          editorDraftVersionRef.current === draftVersion &&
          crud.editingItem === capturedEditor &&
          (commandId === 'profiles.create' ? crud.isNew : !crud.isNew && String(crud.editingId) === capturedId) &&
          isProfileContextCurrent(context),
        canPresent: () => editorOpenRef.current &&
          editorVersionRef.current === version &&
          editorDraftVersionRef.current === draftVersion &&
          isProfileContextCurrent(context),
        succeeded: async (result: PageMutationResult) => handlePageMutationSucceeded(commandId, result, {
          context,
          editorVersion: version,
          draftVersion,
          targetSlug: capturedId || undefined,
        }),
      };
    },
  });

  const { request: requestRootPageMutation } = usePageMutationCommands({
    root: pageRef,
    pathname,
    allowedCommands: ['profiles.update', 'profiles.duplicate', 'profiles.delete', 'profiles.activate'],
    canStart: (commandId) => {
      if (editorOpenRef.current || mutationBusyRef.current) return false;
      const captured = presentationTargetRef.current;
      if (!captured || !crud.items.some((item) => item === captured)) return false;
      if (commandId === 'profiles.update') return inlineUpdateRef.current?.row === captured;
      return commandId === 'profiles.duplicate' || !captured.isActive;
    },
    prepare: (commandId) => {
      const captured = presentationTargetRef.current;
      const context = readProfileContext();
      if (!captured || !context) return undefined;
      const targetSlug = captured.slug;
      const presentationGeneration = presentationGenerationRef.current;
      const inlineUpdate = commandId === 'profiles.update' ? inlineUpdateRef.current : null;
      if (commandId === 'profiles.update' && (!inlineUpdate || inlineUpdate.row !== captured)) return undefined;
      return {
        readRequest: async (): Promise<PageMutationRequest> => {
          const target = await readProfileCommandTarget(targetSlug);
          const profile = commandId === 'profiles.update' && target.profile
            ? cloneProfile(target.profile)
            : undefined;
          if (profile && inlineUpdate) profile.name = inlineUpdate.name;
          return { targetId: targetSlug, expectedFingerprint: target.fingerprint, title: '', description: '', profile };
        },
        isCurrent: () => !editorOpenRef.current &&
          presentationTargetRef.current === captured &&
          presentationGenerationRef.current === presentationGeneration &&
          crud.items.some((item) => item === captured) &&
          (commandId !== 'profiles.update' || inlineUpdateRef.current === inlineUpdate) &&
          isProfileContextCurrent(context),
        canPresent: () => !editorOpenRef.current &&
          presentationTargetRef.current?.slug === targetSlug &&
          isProfileContextCurrent(context),
        succeeded: async (result: PageMutationResult) => handlePageMutationSucceeded(commandId, result, {
          context,
          targetSlug,
          presentationGeneration,
          capturedTarget: captured,
        }),
      };
    },
  });

  const runProfileMutation = useCallback(async (
    commandId: PageMutationID,
    request: (id: PageMutationID) => Promise<{ status: string }>,
  ) => {
    const context = readProfileContext();
    if (!context) return;
    const runId = ++mutationRunRef.current;
    const outcomePromise = request(commandId);
    // A captura acontece antes de marcar busy; marcar antes faria o próprio
    // alvo nativo recusar a operação durante o dispatch síncrono.
    setMutationBusyState(true);
    try {
      const outcome = await outcomePromise;
      if (outcome.status !== 'succeeded' && outcome.status !== 'cancelled' && isProfileContextCurrent(context)) {
        addToast(t('common.error', 'Erro ao alterar perfil'), 'error', undefined, undefined, {
          suppressAnnounce: true,
        });
      }
    } catch (error: unknown) {
      if (!isProfileContextCurrent(context)) return;
      addToast(getErrorMessage(error) || t('common.error', 'Erro ao alterar perfil'), 'error', undefined, undefined, {
        suppressAnnounce: true,
      });
    } finally {
      if (commandId === 'profiles.update') inlineUpdateRef.current = null;
      if (mountedRef.current && mutationRunRef.current === runId) setMutationBusyState(false);
    }
  }, [addToast, isProfileContextCurrent, setMutationBusyState, t]);

  const handleSave = useCallback(async () => {
    if (!crud.editingItem?.name?.trim()) {
      const message = t('profiles.nameRequired', 'Nome é obrigatório');
      addToast(message, 'error', undefined, undefined, { suppressAnnounce: true });
      announce(message);
      return;
    }
    await runProfileMutation(crud.isNew ? 'profiles.create' : 'profiles.update', requestEditorPageMutation);
  }, [addToast, announce, crud.editingItem, crud.isNew, requestEditorPageMutation, runProfileMutation, t]);

  const handleDuplicateProfile = useCallback((row: ProfileRow) => {
    captureProfileRow(row);
    void runProfileMutation('profiles.duplicate', requestRootPageMutation);
  }, [captureProfileRow, requestRootPageMutation, runProfileMutation]);

  const handleActivateProfile = useCallback((row: ProfileRow) => {
    captureProfileRow(row);
    const request = editorOpenRef.current ? requestEditorPageMutation : requestRootPageMutation;
    void runProfileMutation('profiles.activate', request);
  }, [captureProfileRow, requestEditorPageMutation, requestRootPageMutation, runProfileMutation]);

  const handleDeleteProfile = useCallback((row: ProfileRow) => {
    captureProfileRow(row);
    const request = editorOpenRef.current ? requestEditorPageMutation : requestRootPageMutation;
    void runProfileMutation('profiles.delete', request);
  }, [captureProfileRow, requestEditorPageMutation, requestRootPageMutation, runProfileMutation]);

  // --- Grid columns ---

  const columns: DataGridColumn<ProfileRow>[] = [
    {
      key: 'name',
      label: t('profiles.colName', 'Nome'),
      width: '22%',
      editable: true,
    },
    {
      key: 'description',
      label: t('profiles.colDescription', 'Descrição'),
      width: '28%',
      truncate: true,
      format: (_value, row) => profileDisplayDescription(t, row),
    },
    {
      key: 'source',
      label: t('profiles.colSource', 'Origem'),
      width: '12%',
      format: (value) => {
        switch (String(value || '')) {
          case 'workdir':
            return t('profiles.sourceWorkdir', 'Projeto');
          case 'home':
            return t('profiles.sourceHome', 'Global');
          case 'exe':
            return t('profiles.sourceExe', 'Embutido');
          default:
            return String(value || '-');
        }
      },
    },
    {
      key: 'isActive',
      label: t('profiles.colStatus', 'Status'),
      width: '10%',
      format: (value) => {
        const isActive = Boolean(value);
        return (
          <span className={isActive ? 'profiles-badge profiles-badge--active' : 'profiles-badge profiles-badge--inactive'}>
            {isActive ? t('profiles.active', 'Ativo') : t('profiles.inactive', 'Inativo')}
          </span>
        );
      },
    },
    {
      key: 'actions',
      label: '',
      width: '8%',
      format: (_value, item) => (
        <MenuButton
          items={getProfileRowActions(item)}
          buttonLabel={t('profiles.actions', 'Ações')}
        />
      ),
    },
  ];


  // Gera as ações contextuais para cada linha
  function getProfileRowActions(item: ProfileRow) {
    return [
      {
        id: 'activate',
        label: t('profiles.activate', 'Ativar perfil'),
        icon: <CheckOutlined />,
        onClick: () => handleActivateProfile(item),
        disabled: !!item.isActive,
      },
      {
        id: 'edit',
        label: t('profiles.edit', 'Editar perfil'),
        icon: <EditOutlined />,
        onClick: () => {
          captureProfileRow(item);
          requestPagePresentationCommand('profiles.edit.open');
        },
      },
      {
        id: 'duplicate',
        label: t('profiles.duplicate', 'Duplicar'),
        icon: <CopyOutlined />,
        onClick: () => handleDuplicateProfile(item),
      },
      {
        id: 'delete',
        label: t('profiles.delete', 'Excluir perfil'),
        icon: <DeleteOutlined />,
        onClick: () => handleDeleteProfile(item),
        danger: true,
        disabled: !!item.isActive,
      },
    ];
  }

  const handleCellEdit = async (item: ProfileRow, column: DataGridColumn<ProfileRow>, newValue: string) => {
    if (column.key === 'name') {
      captureProfileRow(item);
      inlineUpdateRef.current = { row: item, name: newValue };
      await runProfileMutation('profiles.update', requestRootPageMutation);
    }
  };

  // --- Filtering ---

  const filteredRows = useMemo(
    () =>
      crud.items.filter(
        (row) =>
          row.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
          profileDisplayDescription(t, row).toLowerCase().includes(searchTerm.toLowerCase())
      ),
    [crud.items, searchTerm, t]
  );

  const getItemId = useCallback((item: ProfileRow) => item.id, []);
  const handleActivateRow = useCallback((item: ProfileRow) => {
    captureProfileRow(item);
    requestPagePresentationCommand('profiles.edit.open');
  }, [captureProfileRow, requestPagePresentationCommand]);
  const handleDeleteRow = useCallback(
    (item: ProfileRow) => handleDeleteProfile(item),
    [handleDeleteProfile]
  );
  const handleFocusChange = useCallback((item: ProfileRow | null) => {
    captureProfileRow(item);
  }, [captureProfileRow]);

  // --- Loading ---

  const loading = crud.loading || depsLoading;

  if (loading) {
    return (
      <div className="profiles-page">
        <PageLoading message={t('profiles.loading', 'Carregando perfis...')} />
      </div>
    );
  }

  // --- Render ---

  const editingProfile = crud.editingItem;
  const editingSlug = crud.editingId ? String(crud.editingId) : null;
  const isNew = crud.isNew;
  const saving = mutationBusy || editorReadLoading;

  const editorTitle = isNew
    ? t('profiles.newProfileTitle', 'Novo Perfil')
    : editingProfile?.name || '';

  return (
    <div ref={pageRef} className="profiles-page">
      <Toolbar
        left={
          <h1 className="page-toolbar__title">
            {t('profiles.pageTitle', 'Perfis')}
          </h1>
        }
        searchPlaceholder={t('profiles.search', 'Buscar perfis...')}
        searchValue={searchTerm}
        onSearchChange={setSearchTerm}
        actions={[
          {
            key: 'new-profile',
            label: t('profiles.newProfile', 'Novo Perfil'),
            icon: <PlusOutlined />,
            onClick: () => requestPagePresentationCommand('profiles.create.open'),
            shortcut: createProfileShortcut,
            variant: 'primary',
          },
          {
            key: 'activate-profile',
            label: t('profiles.activate', 'Ativar perfil'),
            icon: <CheckOutlined />,
            onClick: () => focusedRow && handleActivateProfile(focusedRow),
            disabled: !focusedRow || !!focusedRow?.isActive,
          },
          {
            key: 'edit-profile',
            label: t('profiles.edit', 'Editar perfil'),
            icon: <EditOutlined />,
            onClick: () => {
              if (!focusedRow) return;
              captureProfileRow(focusedRow);
              requestPagePresentationCommand('profiles.edit.open');
            },
            disabled: !focusedRow,
          },
          {
            key: 'duplicate-profile',
            label: t('profiles.duplicate', 'Duplicar'),
            icon: <CopyOutlined />,
            onClick: () => focusedRow && handleDuplicateProfile(focusedRow),
            disabled: !focusedRow,
          },
          {
            key: 'delete-profile',
            label: t('profiles.delete', 'Excluir perfil'),
            icon: <DeleteOutlined />,
            onClick: () => focusedRow && handleDeleteProfile(focusedRow),
            disabled: !focusedRow || !!focusedRow?.isActive,
            variant: 'danger',
          },
        ]}
      />

      <DataGrid
        items={filteredRows}
        columns={columns}
        label={t('profiles.gridLabel', 'Lista de perfis')}
        getItemId={getItemId}
        selectedIds={selectedIds}
        onSelectionChange={setSelectedIds}
        onActivate={handleActivateRow}
        onDelete={handleDeleteRow}
        onCellEdit={handleCellEdit}
        onGridReady={handleGridReady}
        getRowActions={getProfileRowActions}
        onFocusChange={handleFocusChange}
      />

      {/* Editor Modal */}
      <Modal
        isOpen={!!editingProfile}
        onClose={handleCloseEditor}
        title={editorTitle}
        size="xl"
        allowClose={!saving}
        initialFocusSelector={editorRequest?.tab ? '[role="tab"][aria-selected="true"]' : undefined}
      >
        {editingProfile && (
            <div ref={editorRootRef} className="profiles-editor">
            <ProfileEditorTabs
              editingProfile={editingProfile}
              availableTools={availableTools}
              availableSkills={availableSkills}
              availableContextProviders={availableContextProviders}
              availableAllowlists={availableAllowlists}
              updateField={updateField}
              updateFields={updateFields}
              initialTab={editorRequest?.tab}
            />

            <EditorPanelFooter className="profiles-editor__footer">
              {editingSlug && activeSlug !== editingSlug && (
                <Button
                  variant="secondary"
                  onClick={() => handleActivateProfile({ slug: editingSlug, name: editingProfile.name } as ProfileRow)}
                  aria-label={t('profiles.activateBtnLabel', `Ativar perfil ${editingProfile.name}`)}
                >
                  {t('profiles.activateBtn', 'Ativar')}
                </Button>
              )}
              <Button onClick={handleSave} loading={saving}>
                {t('profiles.saveBtn', 'Salvar')}
              </Button>
              <Button variant="secondary" onClick={() => handleCloseEditor()} disabled={saving}>
                {t('common.cancel', 'Cancelar')}
              </Button>
              <div className="profiles-editor__footer-spacer" />
              {editingSlug && activeSlug !== editingSlug && (
                <Button
                  variant="danger"
                  onClick={() => {
                    const deleteTarget = profiles.Profile.createFrom(editingProfile) as ProfileRow;
                    deleteTarget.id = editingSlug;
                    deleteTarget.slug = editingSlug;
                    deleteTarget.isActive = activeSlug === editingSlug;
                    deleteTarget.source = (editingProfile as ProfileRow).source;
                    handleDeleteProfile(deleteTarget);
                  }}
                  aria-label={t('profiles.deleteBtnLabel', `Excluir perfil ${editingProfile.name}`)}
                >
                  {t('profiles.deleteBtn', 'Excluir')}
                </Button>
              )}
            </EditorPanelFooter>
          </div>
        )}
      </Modal>

      {/* Empty state when no profile is being edited */}
      {!editingProfile && crud.items.length > 0 && (
        <div className="profiles-empty">
          <p>
            {t('profiles.selectHint', 'Pressione Enter ou clique para editar.')}
            {' '}
            <EditOutlined aria-hidden="true" />
          </p>
        </div>
      )}

      {/* Search paths footer */}
      {searchPaths.length > 0 && (
        <div className="profiles-search-paths" role="contentinfo" aria-label={t('profiles.searchPathsLabel', 'Caminhos de busca de perfis')}>
          <p className="profiles-search-paths__title">
            {t('profiles.searchPaths', 'Caminhos de busca:')}
          </p>
          {searchPaths.map((path, i) => (
            <p key={i} className="profiles-search-paths__item">{path}</p>
          ))}
        </div>
      )}
    </div>
  );
}
