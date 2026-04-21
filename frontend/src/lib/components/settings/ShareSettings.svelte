<script lang="ts">
  import SettingsSection from "./SettingsSection.svelte";
  import { getShareConfig, setShareConfig } from "../../api/client.js";
  import type { ShareConfig } from "../../api/types/share.js";

  let config: ShareConfig | null = $state(null);
  let loading: boolean = $state(true);
  let saving: boolean = $state(false);
  let error: string | null = $state(null);
  let success: string | null = $state(null);

  let localUrl: string = $state("");
  let localToken: string = $state("");

  async function loadConfig() {
    loading = true;
    error = null;
    try {
      config = await getShareConfig();
      localUrl = config.url;
      localToken = "";
    } catch (e) {
      error = e instanceof Error ? e.message : "Failed to load share config";
    } finally {
      loading = false;
    }
  }

  loadConfig();

  async function handleSave() {
    saving = true;
    error = null;
    success = null;
    try {
      config = await setShareConfig({
        url: localUrl.trim() || undefined,
        token: localToken.trim() || undefined,
      });
      localUrl = config.url;
      localToken = "";
      success = "Share configuration saved.";
    } catch (e) {
      error = e instanceof Error ? e.message : "Failed to save";
    } finally {
      saving = false;
    }
  }

  let dirty = $derived.by(() => {
    const savedUrl = config?.url ?? "";
    return localUrl !== savedUrl || localToken !== "";
  });
</script>

<SettingsSection
  title="Share Publishing"
  description="Server URL and auth token for session sharing."
>
  {#if loading}
    <span class="status-loading">Loading...</span>
  {:else}
    <div class="status-row">
      <span class="status-label">Status</span>
      <span class="status-value" class:configured={config?.configured}>
        {config?.configured ? "Configured" : "Not configured"}
      </span>
    </div>

    <div class="field-col">
      <label class="field-label" for="share-url">Server URL</label>
      <input
        id="share-url"
        class="setting-input"
        type="text"
        placeholder="https://share.example.com"
        bind:value={localUrl}
      />
    </div>

    <div class="field-col">
      <label class="field-label" for="share-token">Auth Token</label>
      <input
        id="share-token"
        class="setting-input"
        type="password"
        placeholder={config?.has_token
          ? "Token is set -- enter new value to replace"
          : "Enter bearer token"}
        bind:value={localToken}
      />
    </div>

    <button
      class="save-btn"
      disabled={saving || !dirty}
      onclick={handleSave}
    >
      {saving ? "Saving..." : "Save"}
    </button>

    {#if error}
      <p class="msg error">{error}</p>
    {/if}
    {#if success}
      <p class="msg success">{success}</p>
    {/if}
  {/if}
</SettingsSection>

<style>
  .status-row {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .status-label {
    font-size: 12px;
    font-weight: 500;
    color: var(--text-secondary);
  }

  .status-value {
    font-size: 12px;
    color: var(--text-muted);
  }

  .status-value.configured {
    color: var(--accent-green);
  }

  .status-loading {
    font-size: 12px;
    color: var(--text-muted);
  }

  .field-col {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .field-label {
    font-size: 11px;
    font-weight: 500;
    color: var(--text-secondary);
  }

  .setting-input {
    height: 30px;
    padding: 0 10px;
    border-radius: var(--radius-sm);
    font-size: 12px;
    font-family: var(--font-mono, monospace);
    color: var(--text-primary);
    background: var(--bg-inset);
    border: 1px solid var(--border-muted);
    transition: border-color 0.15s;
  }

  .setting-input:focus {
    outline: none;
    border-color: var(--accent-blue);
  }

  .save-btn {
    align-self: flex-start;
    height: 30px;
    padding: 0 14px;
    border-radius: var(--radius-sm);
    font-size: 12px;
    font-weight: 500;
    color: white;
    background: var(--accent-blue);
    border: none;
    cursor: pointer;
    white-space: nowrap;
    transition: opacity 0.12s;
  }

  .save-btn:hover:not(:disabled) {
    opacity: 0.9;
  }

  .save-btn:disabled {
    opacity: 0.6;
    cursor: default;
  }

  .msg {
    font-size: 11px;
    margin: 0;
  }

  .msg.error {
    color: var(--accent-red, #ef4444);
  }

  .msg.success {
    color: var(--accent-green, #22c55e);
  }
</style>
