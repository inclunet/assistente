<script>
  import Topbar from './Topbar.svelte';
  
  export let currentPage = 'chat';
  export let hasApiKey = false;
  
  import { createEventDispatcher } from 'svelte';
  
  const dispatch = createEventDispatcher();
  
  function handleNavigate(event) {
    dispatch('navigate', event.detail);
  }
</script>

<div class="layout">
  <Topbar 
    {currentPage} 
    {hasApiKey} 
    on:navigate={handleNavigate}
  />
  
  <main class="main-content" id="main-content">
    <slot />
  </main>
</div>

<style>
  .layout {
    display: flex;
    flex-direction: column;
    height: 100vh;
    width: 100%;
    overflow: hidden;
  }
  
  .main-content {
    flex: 1;
    overflow: hidden;
    display: flex;
    flex-direction: column;
    background: var(--color-bg-primary, #121212);
  }
</style>
