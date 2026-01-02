/**
 * Chat Components - TypeScript Types
 * 
 * Tipos exportados para projetos TypeScript.
 */

// ========================================
// Message Types
// ========================================

export interface Message {
  id: string | number;
  content: string;
  timestamp?: Date | string | number;
  media?: MediaItem[];
  metadata?: Record<string, unknown>;
}

export interface Author {
  id: string;
  name: string;
  avatar?: string;
  color?: string;
  role?: 'user' | 'assistant' | 'agent' | 'tool' | 'system' | string;
}

export interface MessageNode {
  message: Message;
  author: Author;
  isMe: boolean;
  level: number;
  children?: MessageNode[];
  childCount?: number;
}

// ========================================
// Media Types
// ========================================

export interface MediaItem {
  type: 'image' | 'audio' | 'document' | 'screenshot' | 'webcam';
  file?: File;
  preview?: string;
  base64?: string;
  url?: string;
  altText?: string;
  mimeType?: string;
}

// ========================================
// Config Types
// ========================================

export interface MessageConfig {
  showAvatar?: boolean;
  showTimestamp?: boolean;
  showActions?: boolean;
  editable?: boolean;
  deletable?: boolean;
  pinnable?: boolean;
  speakable?: boolean;
}

export interface MediaConfig {
  allowImages?: boolean;
  allowAudio?: boolean;
  allowDocuments?: boolean;
  allowScreenshot?: boolean;
  allowWebcam?: boolean;
  maxFiles?: number;
  maxFileSize?: number;
}

export interface ChatConfig {
  // Features
  enableTTS?: boolean;
  enableThreads?: boolean;
  enablePinning?: boolean;
  enableEditing?: boolean;
  enableMedia?: boolean;
  
  // History
  lazyLoadChildren?: boolean;
  autoScroll?: boolean;
  groupByDate?: boolean;
  
  // Input
  placeholder?: string;
  maxMediaFiles?: number;
  allowedMediaTypes?: string[];
  
  // Header
  showHeader?: boolean;
  title?: string;
}

// ========================================
// Handler Types
// ========================================

export interface ChatHandlers {
  // === Messages (backend) ===
  onSend?: (content: string, media: MediaItem[]) => Promise<Message>;
  onEdit?: (messageId: string, newContent: string) => Promise<void>;
  onDelete?: (messageId: string) => Promise<void>;
  onPin?: (messageId: string, pinned: boolean) => Promise<void>;
  onResend?: (message: Message) => Promise<void>;
  
  // === Loading (backend) ===
  onLoadHistory?: () => Promise<Message[]>;
  onLoadChildren?: (messageId: string) => Promise<MessageNode[]>;
  onLoadMore?: (direction: 'up' | 'down') => Promise<Message[]>;
  
  // === TTS (external service) ===
  onSpeak?: (text: string) => void;
  onStopSpeaking?: () => void;
  
  // === Media (local or backend) ===
  onCaptureScreen?: () => Promise<MediaItem>;
  onCaptureWebcam?: () => Promise<MediaItem>;
  onRecordAudio?: () => Promise<MediaItem>;
  onGenerateAltText?: (imageBase64: string) => Promise<string>;
  
  // === Notifications (optional) ===
  onPlaySound?: (type: 'send' | 'receive' | 'error') => void;
  onAnnounce?: (message: string, priority: 'polite' | 'assertive') => void;
  onError?: (error: Error) => void;
  
  // === Streaming LLM (optional) ===
  onStreamStart?: () => void;
  onStreamChunk?: (chunk: string, messageId: string) => void;
  onStreamEnd?: (messageId: string) => void;
}

// ========================================
// Label Types
// ========================================

export interface ChatLabels {
  // Actions
  copy: string;
  copyMarkdown: string;
  copyCode: string;
  edit: string;
  delete: string;
  pin: string;
  unpin: string;
  speak: string;
  stopSpeaking: string;
  resend: string;
  reply: string;
  moreActions: string;
  
  // Navigation
  expand: string;
  collapse: string;
  interactions: string;
  showMore: string;
  showLess: string;
  
  // Input
  placeholder: string;
  send: string;
  cancel: string;
  save: string;
  
  // Media
  addMedia: string;
  removeMedia: string;
  captureScreen: string;
  captureWebcam: string;
  recordAudio: string;
  uploadFile: string;
  download: string;
  zoom: string;
  
  // Feedback
  copied: string;
  deleted: string;
  saved: string;
  error: string;
  loading: string;
  sending: string;
  typing: string;
  
  // States
  emptyChat: string;
  emptyDescription: string;
  noMessages: string;
  
  // Accessibility
  you: string;
  assistant: string;
  agent: string;
  tool: string;
  system: string;
  messageOf: string;
  startOfMessages: string;
  endOfMessages: string;
  
  // Errors
  errorSending: string;
  errorLoading: string;
  errorDeleting: string;
  errorSaving: string;
  
  // Timestamps
  justNow: string;
  minutesAgo: string;
  hoursAgo: string;
  yesterday: string;
}

// ========================================
// Theme Types
// ========================================

export type ChatTheme = Record<string, string>;

// ========================================
// Event Types
// ========================================

export interface CopyEvent {
  message: Message;
  format: 'text' | 'markdown';
}

export interface SpeakEvent {
  message: Message;
  text: string;
}

export interface DeleteEvent {
  message: Message;
}

export interface PinEvent {
  message: Message;
  pinned: boolean;
}

export interface EditEvent {
  message: Message;
  newContent?: string;
}

export interface ToggleEvent {
  path: string;
  expand: boolean;
}

export interface BoundaryEvent {
  edge: 'start' | 'end';
  level: number;
  path: string;
}

export interface MediaClickEvent {
  media: MediaItem;
  index: number;
}

export interface SubmitEvent {
  content: string;
  media: MediaItem[];
}

export interface AnnounceEvent {
  message: string;
  priority: 'polite' | 'assertive';
}

export interface KeyActionEvent {
  /** Identificador da tecla (ex: "Enter", "Space", "Ctrl+C", "Shift+E") */
  key: string;
  /** Tecla original (event.key) */
  originalKey: string;
  /** Ctrl/Cmd pressionado */
  ctrlKey: boolean;
  /** Shift pressionado */
  shiftKey: boolean;
  /** Alt pressionado */
  altKey: boolean;
  /** Meta/Cmd pressionado */
  metaKey: boolean;
  /** Objeto da mensagem */
  message: Message;
  /** Índice da mensagem na lista */
  index: number;
  /** Nível na árvore de threads */
  level: number;
  /** Caminho na árvore (ex: "0", "0-1", "0-1-2") */
  path: string;
  /** Evento original do teclado (para preventDefault) */
  originalEvent: KeyboardEvent;
}

export interface ContextMenuEvent {
  /** Evento DOM original */
  event: MouseEvent | KeyboardEvent;
  /** Objeto da mensagem */
  message: Message;
  /** Índice da mensagem */
  index: number;
  /** Nível na árvore */
  level: number;
  /** Posição X sugerida para o menu */
  x: number;
  /** Posição Y sugerida para o menu */
  y: number;
}

export interface ImageZoomEvent {
  /** URL da imagem */
  src: string;
  /** Texto alternativo */
  alt?: string;
}

export interface DetailEvent {
  /** Mensagem a exibir em detalhe */
  message: Message;
  /** Índice da mensagem */
  index: number;
  /** Caminho na árvore */
  path: string;
}

export interface RecordEvent {
  /** Tipo de evento de gravação */
  type: 'start' | 'stop' | 'cancel';
}

// ========================================
// Action Types
// ========================================

export interface Action {
  id: string;
  label: string;
  icon?: string;
  shortcut?: string;
  disabled?: boolean;
  danger?: boolean;
}

