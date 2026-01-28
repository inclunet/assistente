import React, { useState, useRef, KeyboardEvent, useEffect, forwardRef } from 'react';
import { Button } from '../ui/Button';
import { MediaPreview } from './MediaPreview';
import { VoiceButton } from './VoiceButton';
import { MediaFile, processMediaFiles } from '../../services/mediaService';
import './ChatInput.css';

export interface ChatInputProps {
  onSend: (message: string, mediaFiles?: MediaFile[]) => void;
  disabled?: boolean;
  placeholder?: string;
  maxFiles?: number;
  onArrowUp?: () => void;
  /** Se o controle de voz está habilitado */
  voiceEnabled?: boolean;
}

export const ChatInput = forwardRef<HTMLTextAreaElement, ChatInputProps>((
  { onSend, disabled = false, placeholder = 'Digite sua mensagem...', maxFiles = 5, onArrowUp, voiceEnabled = false },
  ref
) => {
  const [message, setMessage] = useState('');
  const [mediaFiles, setMediaFiles] = useState<MediaFile[]>([]);
  const [isDragging, setIsDragging] = useState(false);
  const [isProcessing, setIsProcessing] = useState(false);
  const internalTextareaRef = useRef<HTMLTextAreaElement>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const hintId = 'chat-input-hint';
  
  // Use external ref if provided, otherwise use internal ref
  const textareaRef = (ref as React.RefObject<HTMLTextAreaElement>) || internalTextareaRef;

  // Handler para transcrição de voz
  const handleVoiceTranscription = (text: string) => {
    if (text.trim()) {
      // Envia diretamente o texto transcrito
      onSend(text.trim(), mediaFiles.length > 0 ? mediaFiles : undefined);
      setMediaFiles([]);
    }
  };

  const adjustTextareaHeight = () => {
    const textarea = textareaRef.current;
    if (textarea) {
      textarea.style.height = 'auto';
      const newHeight = Math.min(textarea.scrollHeight, 200); // Max 200px
      textarea.style.height = `${newHeight}px`;
    }
  };

  useEffect(() => {
    adjustTextareaHeight();
  }, [message]);

  const handleSend = () => {
    const trimmedMessage = message.trim();
    if ((trimmedMessage || mediaFiles.length > 0) && !disabled && !isProcessing) {
      onSend(trimmedMessage, mediaFiles.length > 0 ? mediaFiles : undefined);
      setMessage('');
      setMediaFiles([]);
      
      // Reset textarea height
      if (textareaRef.current) {
        textareaRef.current.style.height = 'auto';
      }
    }
  };

  const handleFileSelect = async (files: File[]) => {
    if (files.length === 0 || mediaFiles.length >= maxFiles) return;
    
    setIsProcessing(true);
    try {
      const remainingSlots = maxFiles - mediaFiles.length;
      const filesToProcess = files.slice(0, remainingSlots);
      const processed = await processMediaFiles(filesToProcess);
      setMediaFiles(prev => [...prev, ...processed]);
    } catch (error) {
      console.error('Erro ao processar arquivos:', error);
    } finally {
      setIsProcessing(false);
    }
  };

  const handleRemoveMedia = (id: string) => {
    setMediaFiles(prev => prev.filter(m => m.id !== id));
  };

  const handleFileInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const files = e.target.files;
    if (files) {
      handleFileSelect(Array.from(files));
    }
    if (fileInputRef.current) {
      fileInputRef.current.value = '';
    }
  };

  // Drag & Drop handlers
  const handleDragEnter = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setIsDragging(true);
  };

  const handleDragOver = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
  };

  const handleDragLeave = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    if (e.currentTarget === e.target) {
      setIsDragging(false);
    }
  };

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setIsDragging(false);
    
    const files = Array.from(e.dataTransfer.files);
    handleFileSelect(files);
  };

  // Paste handler para imagens
  const handlePaste = async (e: React.ClipboardEvent<HTMLTextAreaElement>) => {
    const items = Array.from(e.clipboardData.items);
    const imageItems = items.filter(item => item.type.startsWith('image/'));
    
    if (imageItems.length > 0) {
      e.preventDefault();
      const files = imageItems
        .map(item => item.getAsFile())
        .filter((file): file is File => file !== null);
      
      if (files.length > 0) {
        await handleFileSelect(files);
      }
    }
  };

  const handleKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      if (!disabled) {
        handleSend();
      }
    }
    
    // ArrowUp no início do texto navega para a lista de mensagens
    if (e.key === 'ArrowUp' && onArrowUp) {
      const textarea = textareaRef.current;
      if (textarea && textarea.selectionStart === 0 && textarea.selectionEnd === 0) {
        e.preventDefault();
        onArrowUp();
      }
    }
  };

  return (
    <div 
      className={`chat-input ${isDragging ? 'dragging' : ''}`}
      onDragEnter={handleDragEnter}
      onDragOver={handleDragOver}
      onDragLeave={handleDragLeave}
      onDrop={handleDrop}
    >
      {isDragging && (
        <div className="drag-overlay">
          <div className="drag-overlay__content">
            📎 Solte os arquivos aqui
          </div>
        </div>
      )}

      <MediaPreview media={mediaFiles} onRemove={handleRemoveMedia} />

      <div className="chat-input__container">
        <input
          ref={fileInputRef}
          type="file"
          multiple
          accept="image/*,audio/*,video/*,application/pdf,.doc,.docx,.txt,.md,.csv,.xlsx"
          onChange={handleFileInputChange}
          style={{ display: 'none' }}
          aria-label="Selecionar arquivos"
        />
        
        <button
          type="button"
          className="chat-input__attach-button"
          onClick={() => fileInputRef.current?.click()}
          disabled={disabled || mediaFiles.length >= maxFiles}
          aria-label="Anexar arquivo"
          title="Anexar arquivo"
        >
          📎
        </button>

        <textarea
          ref={textareaRef}
          className="chat-input__textarea"
          value={message}
          onChange={(e) => setMessage(e.target.value)}
          onKeyDown={handleKeyDown}
          onPaste={handlePaste}
          placeholder={placeholder}
          rows={1}
          aria-label="Digite sua mensagem"
          aria-describedby={hintId}
          aria-multiline="true"
        />
        {/* #region agent log */}
        {(() => { const showVoice = voiceEnabled && !message.trim() && mediaFiles.length === 0; fetch('http://127.0.0.1:7242/ingest/c14faa4a-a682-41c0-9f93-65632102ad3e',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({location:'ChatInput.tsx:render',message:'Button render decision',data:{showVoice,voiceEnabled,messageEmpty:!message.trim(),noMedia:mediaFiles.length===0},timestamp:Date.now(),sessionId:'debug-session',hypothesisId:'C'})}).catch(()=>{}); return null; })()}
        {/* #endregion */}
        {/* Mostra botão de voz quando input vazio, senão botão de enviar */}
        {voiceEnabled && !message.trim() && mediaFiles.length === 0 ? (
          <VoiceButton
            onTranscription={handleVoiceTranscription}
            disabled={disabled}
            className="chat-input__voice-button"
            textareaRef={textareaRef}
          />
        ) : (
          <Button
            onClick={handleSend}
            disabled={disabled || (!message.trim() && mediaFiles.length === 0) || isProcessing}
            variant="primary"
            size="md"
            className="chat-input__button"
            aria-label={disabled ? "Aguarde o término da resposta para enviar" : "Enviar mensagem"}
          >
            <svg
              width="20"
              height="20"
              viewBox="0 0 20 20"
              fill="none"
              xmlns="http://www.w3.org/2000/svg"
              aria-hidden="true"
            >
              <path
                d="M18.3327 10L1.66602 1.66669L5.83268 10L1.66602 18.3334L18.3327 10Z"
                fill="currentColor"
              />
            </svg>
          </Button>
        )}
      </div>
    </div>
  );
});

ChatInput.displayName = 'ChatInput';
