<script>
  // Props
  export let author = {};
  export let size = 'md';  // 'sm' | 'md' | 'lg'
  
  // Avatar pode ser: emoji, URL, ou iniciais
  $: avatarType = getAvatarType(author.avatar);
  $: initials = getInitials(author.name);
  $: bgColor = author.color || getDefaultColor(author.role);
  
  function getAvatarType(avatar) {
    if (!avatar) return 'initials';
    if (avatar.startsWith('http') || avatar.startsWith('data:') || avatar.startsWith('/')) {
      return 'image';
    }
    // Provavelmente é um emoji
    return 'emoji';
  }
  
  function getInitials(name) {
    if (!name) return '?';
    const parts = name.split(' ');
    if (parts.length === 1) {
      return name.substring(0, 2).toUpperCase();
    }
    return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase();
  }
  
  function getDefaultColor(role) {
    const colors = {
      user: 'var(--chat-color-user-border)',
      assistant: 'var(--chat-color-assistant-border)',
      agent: 'var(--chat-color-agent-border)',
      tool: 'var(--chat-color-tool-border)',
      system: 'var(--chat-color-text-muted)'
    };
    return colors[role] || colors.system;
  }
</script>

<div 
  class="avatar {size}"
  style="--avatar-bg: {bgColor}"
  role="img"
  aria-label={author.name || 'Avatar'}
>
  {#if avatarType === 'image'}
    <img src={author.avatar} alt="" class="avatar-img" />
  {:else if avatarType === 'emoji'}
    <span class="avatar-emoji">{author.avatar}</span>
  {:else}
    <span class="avatar-initials">{initials}</span>
  {/if}
</div>

<style>
  .avatar {
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: var(--chat-radius-full);
    background: var(--avatar-bg, var(--chat-color-bg-tertiary));
    color: var(--chat-color-primary-text);
    font-weight: var(--chat-font-weight-bold);
    flex-shrink: 0;
    overflow: hidden;
  }
  
  /* Tamanhos */
  .avatar.sm {
    width: 24px;
    height: 24px;
    font-size: var(--chat-font-size-xs);
  }
  
  .avatar.md {
    width: 32px;
    height: 32px;
    font-size: var(--chat-font-size-xs);
  }
  
  .avatar.lg {
    width: 40px;
    height: 40px;
    font-size: var(--chat-font-size-sm);
  }
  
  .avatar-img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
  
  .avatar-emoji {
    font-size: var(--chat-font-size-lg);
  }
  
  .avatar-initials {
    text-transform: uppercase;
  }
</style>

