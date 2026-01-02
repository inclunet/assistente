<script>
  import { createEventDispatcher } from 'svelte';
  import { _ } from 'svelte-i18n';
  
  export let disabled = false;
  export let isRecording = false;
  export let mode = 'ptt'; // 'ptt' | 'toggle' | 'vad'
  
  const dispatch = createEventDispatcher();
  
  function handleMouseDown() {
    if (mode === 'ptt') {
      dispatch('recordStart');
    }
  }
  
  function handleMouseUp() {
    if (mode === 'ptt' && isRecording) {
      dispatch('recordStop');
    }
  }
  
  function handleClick() {
    if (mode === 'toggle' || mode === 'vad') {
      if (isRecording) {
        dispatch('recordStop');
      } else {
        dispatch('recordStart');
      }
    }
  }
  
  function handleKeyDown(event) {
    if (event.key === ' ' || event.key === 'Enter') {
      event.preventDefault();
      if (mode === 'ptt') {
        dispatch('recordStart');
      } else {
        handleClick();
      }
    }
  }
  
  function handleKeyUp(event) {
    if ((event.key === ' ' || event.key === 'Enter') && mode === 'ptt' && isRecording) {
      dispatch('recordStop');
    }
  }
  
  $: ariaLabel = isRecording 
    ? $_('chat.stopRecording') 
    : $_('chat.startRecording');
</script>

<button 
  type="button"
  class="btn-primary voice-btn"
  class:recording={isRecording}
  {disabled}
  aria-label={ariaLabel}
  aria-pressed={isRecording}
  on:mousedown={handleMouseDown}
  on:mouseup={handleMouseUp}
  on:mouseleave={handleMouseUp}
  on:click={handleClick}
  on:keydown={handleKeyDown}
  on:keyup={handleKeyUp}
>
  <slot>
    {#if isRecording}
      ⏹️
    {:else}
      🎤
    {/if}
  </slot>
</button>

<style>
  .voice-btn {
    padding: var(--chat-space-3, 0.75rem);
    border-radius: var(--chat-radius-full, 9999px);
    font-size: 1.25rem;
    min-width: 44px;
    min-height: 44px;
    display: flex;
    align-items: center;
    justify-content: center;
  }
  
  .btn-primary {
    background: var(--chat-btn-primary-bg, #3b82f6);
    color: var(--chat-btn-primary-text, #ffffff);
    border: none;
    cursor: pointer;
    transition: background-color var(--chat-transition-fast, 150ms ease);
  }
  
  .btn-primary:hover:not(:disabled) {
    background: var(--chat-btn-primary-hover, #2563eb);
  }
  
  .btn-primary:focus-visible {
    outline: 2px solid var(--chat-color-border-focus, #3b82f6);
    outline-offset: 2px;
  }
  
  .btn-primary:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }
  
  .voice-btn.recording {
    background: var(--chat-color-error, #dc2626);
    animation: pulse-recording 1s ease-in-out infinite;
  }
  
  @keyframes pulse-recording {
    0%, 100% { 
      box-shadow: 0 0 0 0 rgba(220, 53, 69, 0.4);
    }
    50% { 
      box-shadow: 0 0 0 8px rgba(220, 53, 69, 0);
    }
  }
</style>


