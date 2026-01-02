<script>
  import { _ } from 'svelte-i18n';
  
  // Props
  export let visible = true;
  export let text = '';      // Texto opcional (ex: "Assistente está digitando...")
  
  $: displayText = text || $_('chat.loading');
</script>

{#if visible}
  <div class="typing-indicator" role="status" aria-live="polite" aria-label={displayText}>
    <span class="typing-dots" aria-hidden="true">
      <span class="dot"></span>
      <span class="dot"></span>
      <span class="dot"></span>
    </span>
    {#if text}
      <span class="typing-text">{displayText}</span>
    {/if}
  </div>
{/if}

<style>
  .typing-indicator {
    display: inline-flex;
    align-items: center;
    gap: var(--chat-space-2);
    padding: var(--chat-space-2) var(--chat-space-4);
    background: var(--chat-color-assistant-bg);
    border-radius: var(--chat-radius-xl);
    color: var(--chat-color-text-muted);
  }
  
  .typing-dots {
    display: inline-flex;
    gap: 4px;
  }
  
  .dot {
    width: 8px;
    height: 8px;
    border-radius: var(--chat-radius-full);
    background: var(--chat-color-text-muted);
    animation: bounce 1.4s infinite ease-in-out both;
  }
  
  .dot:nth-child(1) { animation-delay: -0.32s; }
  .dot:nth-child(2) { animation-delay: -0.16s; }
  .dot:nth-child(3) { animation-delay: 0s; }
  
  @keyframes bounce {
    0%, 80%, 100% { transform: scale(0.6); opacity: 0.5; }
    40% { transform: scale(1); opacity: 1; }
  }
  
  .typing-text {
    font-size: var(--chat-font-size-sm);
    font-style: italic;
  }
</style>

