<script lang="ts">
  import { ClerkProvider, ClerkLoaded, ClerkLoading, Show, SignIn } from "svelte-clerk/client";
  import App from "./App.svelte";
  import { getClerkPublishableKey } from "./lib/runtime-config.js";
  import { isRemoteConnection } from "./lib/api/client.js";

  const clerkKey = getClerkPublishableKey();
  const clerkMode = clerkKey !== "" && !isRemoteConnection();
</script>

{#if clerkMode}
  <ClerkProvider publishableKey={clerkKey}>
    <ClerkLoading>
      <div class="auth-overlay">
        <div class="auth-card">
          <p class="auth-card-desc">Loading sign-in form...</p>
        </div>
      </div>
    </ClerkLoading>
    <ClerkLoaded>
      <Show when="signed-out">
        <div class="auth-overlay">
          <SignIn />
        </div>
      </Show>
      <Show when="signed-in">
        <App />
      </Show>
    </ClerkLoaded>
  </ClerkProvider>
{:else}
  <App />
{/if}

<style>
  .auth-overlay {
    display: flex;
    align-items: center;
    justify-content: center;
    height: 100vh;
    background: var(--bg-default);
  }

  .auth-card {
    text-align: center;
    max-width: 420px;
    padding: 32px 24px;
    background: var(--bg-surface);
    border: 1px solid var(--border-default);
    border-radius: 12px;
    box-shadow: var(--shadow-lg);
  }

  .auth-card-desc {
    font-size: 13px;
    color: var(--text-muted);
    margin: 0;
  }
</style>
