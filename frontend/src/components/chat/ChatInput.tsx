import React, { useState, useRef, KeyboardEvent, useEffect, forwardRef, useCallback } from 'react';
import { Button } from '../ui/Button';
import { MediaPreview } from './MediaPreview';
import { VoiceButton } from './VoiceButton';
import { SlashCommandMenu, countFilteredSkills } from './SlashCommandMenu';
import { MediaFile, processMediaFiles } from '../../services/mediaService';
import { DIMENSIONS } from '../../constants/chat';
import { GetUserInvocableSkills } from '@wailsjs/go/main/App';
import type { skills } from '../../../wailsjs/go/models';
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

  // Slash command state
  const [showSlashMenu, setShowSlashMenu] = useState(false);
  const [slashFilter, setSlashFilter] = useState('');
  const [slashSelectedIndex, setSlashSelectedIndex] = useState(0);
  const [invocableSkills, setInvocableSkills] = useState<skills.SkillInfo[]>([]);
  
  // Use external ref if provided, otherwise use internal ref
  const textareaRef = (ref as React.RefObject<HTMLTextAreaElement>) || internalTextareaRef;

  // Carrega skills invocáveis quando o componente monta
  useEffect(() => {
    GetUserInvocableSkills()
      .then((result) => setInvocableSkills(result || []))
      .catch(() => setInvocableSkills([]));
  }, []);

  // Handler para transcrição de voz
  const handleVoiceTranscription = (text: string) => {
    if (text.trim()) {
      // Envia diretamente o texto transcrito
      onSend(text.trim(), mediaFiles.length > 0 ? mediaFiles : undefined);
      setMediaFiles([]);
    }
  };

  // Detecta slash command no texto
  const updateSlashMenu = useCallback((text: string) => {
    if (invocableSkills.length === 0) {
      setShowSlashMenu(false);
      return;
    }

    // Verifica se o texto começa com /
    if (text.startsWith('/') && !text.includes('\n')) {
      const afterSlash = text.slice(1);
      // Se tem espaço, o slug está completo — fecha o menu
      if (afterSlash.includes(' ')) {
        setShowSlashMenu(false);
        return;
      }
      setSlashFilter(afterSlash);
      setSlashSelectedIndex(0);
      setShowSlashMenu(true);
    } else {
      setShowSlashMenu(false);
    }
  }, [invocableSkills]);

  // Quando um skill é selecionado no menu
  const handleSlashSelect = useCallback((skill: skills.SkillInfo) => {
    const hint = skill.argumentHint ? ` ` : '';
    setMessage(`/${skill.slug}${hint}`);
    setShowSlashMenu(false);
    // Foca o textarea
    requestAnimationFrame(() => {
      textareaRef.current?.focus();
    });
  }, [textareaRef]);

  const adjustTextareaHeight = () => {
    const textarea = textareaRef.current;
    if (textarea) {
      textarea.style.height = 'auto';
      const newHeight = Math.min(textarea.scrollHeight, DIMENSIONS.TEXTAREA_MAX_HEIGHT);
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

      // Reset textarea height and restore focus
      if (textareaRef.current) {
        textareaRef.current.style.height = 'auto';
        // Ensure focus returns to textarea after send
        requestAnimationFrame(() => {
          textareaRef.current?.focus();
        });
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
    // Navegação no menu slash
    if (showSlashMenu) {
      const totalFiltered = countFilteredSkills(invocableSkills, slashFilter);

      if (e.key === 'ArrowDown') {
        e.preventDefault();
        setSlashSelectedIndex((prev) => (prev + 1) % Math.max(totalFiltered, 1));
        return;
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault();
        setSlashSelectedIndex((prev) => (prev - 1 + Math.max(totalFiltered, 1)) % Math.max(totalFiltered, 1));
        return;
      }
      if (e.key === 'Tab' || (e.key === 'Enter' && !e.shiftKey)) {
        e.preventDefault();
        // Seleciona o skill na posição atual
        const filtered = invocableSkills.filter((s) => {
          const searchText = slashFilter.toLowerCase();
          if (!searchText) return true;
          const name = (s.displayName || s.name || '').toLowerCase();
          const slug = (s.slug || '').toLowerCase();
          const desc = (s.description || '').toLowerCase();
          return name.includes(searchText) || slug.includes(searchText) || desc.includes(searchText);
        });
        if (filtered[slashSelectedIndex]) {
          handleSlashSelect(filtered[slashSelectedIndex]);
        }
        return;
      }
      if (e.key === 'Escape') {
        e.preventDefault();
        setShowSlashMenu(false);
        return;
      }
    }

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

      {showSlashMenu && invocableSkills.length > 0 && (
        <SlashCommandMenu
          skills={invocableSkills}
          filter={slashFilter}
          selectedIndex={slashSelectedIndex}
          onSelect={handleSlashSelect}
          onClose={() => setShowSlashMenu(false)}
          anchorRef={textareaRef}
        />
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
          onChange={(e) => {
            setMessage(e.target.value);
            updateSlashMenu(e.target.value);
          }}
          onKeyDown={handleKeyDown}
          onPaste={handlePaste}
          placeholder={placeholder}
          rows={1}
          aria-label="Mensagem"
        />
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
