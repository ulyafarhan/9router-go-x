<script lang="ts">
  import { onMount } from 'svelte'
  import {
    AlertCircle,
    Check,
    Copy,
    Cpu,
    FileText,
    Loader2,
    Save,
    Sparkles,
    Terminal,
    Trash2,
    Wrench,
  } from 'lucide-svelte'
  import { copyToClipboard } from '../../lib/clipboard'
  import {
    api,
    type APIKey,
    type CliToolSettingsApplyPayload,
  } from '../../api/client'

  interface Props {
    toolId: string
    toolName: string
    apiKeys?: APIKey[]
    installed?: boolean
    has9Router?: boolean
    envVars?: Record<string, string>
    instructions?: string[]
    onStatusChange?: () => void
  }

  let {
    toolId,
    toolName,
    apiKeys = [],
    installed = false,
    has9Router = false,
    envVars,
    instructions = [],
    onStatusChange,
  }: Props = $props()

  // State
  let loading = $state(true)
  let applying = $state(false)
  let resetting = $state(false)
  let message = $state<{ type: 'success' | 'error'; text: string } | null>(null)
  let activeTab = $state<'form' | 'manual'>('form')
  let copiedSnippetId = $state<string | null>(null)

  // Form values
  let baseUrl = $state('')
  let selectedApiKey = $state('')
  let customApiKey = $state('')
  let hasStoredApiKey = $state(false)
  let model = $state('')

  // Claude Code specific
  let tierSonnet = $state('')
  let tierOpus = $state('')
  let tierHaiku = $state('')
  let tierFable = $state('')
  let autoCompactWindow = $state('default')
  let customCompactWindow = $state('')

  // Files info & available models
  let targetFiles = $state<Array<{ path: string; format: string; purpose: string }>>([])
  let availableModels = $state<string[]>([])

  // SSR origin fallback
  let localOrigin = $state(typeof window !== 'undefined' ? window.location.origin : 'http://localhost:20130')

  onMount(() => {
    if (typeof window !== 'undefined') {
      localOrigin = window.location.origin
    }
    loadData()
  })

  let effectiveApiKey = $derived(
    selectedApiKey === '__custom__'
      ? customApiKey.trim()
      : selectedApiKey.trim()
  )

  let manualApiKey = $derived(
    effectiveApiKey || (hasStoredApiKey ? 'stored-token' : apiKeys[0]?.key || 'your-9router-api-key')
  )

  async function loadData() {
    loading = true
    message = null
    try {
      // 1. Fetch available combos and custom models for auto-suggest
      const [combosRes, modelsRes, settingsRes] = await Promise.allSettled([
        api.getCombos(),
        api.getCustomModels(),
        api.getCliToolSettings(toolId),
      ])

      const modelList = new Set<string>()
      if (combosRes.status === 'fulfilled' && Array.isArray(combosRes.value)) {
        for (const c of combosRes.value) {
          if (c.name) modelList.add(c.name)
          if (c.id && c.id !== c.name) modelList.add(c.id)
        }
      }
      if (modelsRes.status === 'fulfilled' && modelsRes.value?.models) {
        for (const m of modelsRes.value.models) {
          if (m.id) modelList.add(m.id)
        }
      }
      // Add common default aliases
      if (toolId === 'claude') {
        modelList.add('sonnet')
        modelList.add('opus')
        modelList.add('haiku')
        modelList.add('claude-sonnet-5-5[1m]')
        modelList.add('claude-opus-5-5[1m]')
        modelList.add('claude-haiku-5-5')
      }
      availableModels = Array.from(modelList)

      // 2. Populate from current settings
      if (settingsRes.status === 'fulfilled') {
        const data = settingsRes.value
        targetFiles = data.files || []

        const cfg = data.currentConfig || {}
        baseUrl = cfg.baseUrl || localOrigin
        hasStoredApiKey = !!cfg.hasApiKey
        if (cfg.hasApiKey) {
          selectedApiKey = ''
          customApiKey = ''
        } else if (cfg.apiKey) {
          // If the key matches an existing api key, select it, otherwise set custom
          const match = apiKeys.find((k) => k.key === cfg.apiKey)
          if (match) {
            selectedApiKey = match.key
          } else {
            selectedApiKey = '__custom__'
            customApiKey = cfg.apiKey
          }
        } else if (apiKeys[0]?.key) {
          selectedApiKey = apiKeys[0].key
        }

        model = cfg.model || ''
        if (cfg.tiers) {
          tierSonnet = cfg.tiers.sonnet || ''
          tierOpus = cfg.tiers.opus || ''
          tierHaiku = cfg.tiers.haiku || ''
          tierFable = cfg.tiers.fable || ''
        }

        if (cfg.autoCompactWindow) {
          const raw = String(cfg.autoCompactWindow).trim()
          if (['100000', '150000', '180000', '200000', '330000'].includes(raw)) {
            autoCompactWindow = raw
          } else {
            autoCompactWindow = 'custom'
            customCompactWindow = raw
          }
        } else {
          autoCompactWindow = 'default'
        }
      } else {
        baseUrl = localOrigin
        if (apiKeys[0]?.key) {
          selectedApiKey = apiKeys[0].key
        }
      }
    } catch (err: any) {
      console.warn('Failed to load tool settings:', err)
    } finally {
      loading = false
    }
  }

  async function handleApply() {
    message = null
    const url = baseUrl.trim() || localOrigin
    if (!url) {
      message = { type: 'error', text: 'Base URL is required' }
      return
    }

    applying = true
    try {
      const payload: CliToolSettingsApplyPayload = {
        baseUrl: url,
        model: model.trim(),
      }
      if (effectiveApiKey) {
        payload.apiKey = effectiveApiKey
      }

      if (toolId === 'claude') {
        const tiers: Record<string, string> = {}
        if (tierSonnet.trim()) tiers.sonnet = tierSonnet.trim()
        if (tierOpus.trim()) tiers.opus = tierOpus.trim()
        if (tierHaiku.trim()) tiers.haiku = tierHaiku.trim()
        if (tierFable.trim()) tiers.fable = tierFable.trim()
        if (Object.keys(tiers).length > 0) {
          payload.tiers = tiers
        }

        if (autoCompactWindow !== 'default') {
          payload.autoCompactWindow =
            autoCompactWindow === 'custom' ? customCompactWindow.trim() : autoCompactWindow
        }
      }

      const res = await api.applyCliToolSettings(toolId, payload)
      if (res.success) {
        if (payload.apiKey) {
          hasStoredApiKey = true
          selectedApiKey = ''
          customApiKey = ''
        }
        const backupNote =
          res.backups && res.backups.length > 0
            ? ` (Backup saved: ${res.backups[0].split('/').pop()})`
            : ''
        message = {
          type: 'success',
          text: `Settings successfully applied to ${toolName} config!${backupNote}`,
        }
        if (onStatusChange) onStatusChange()
      } else {
        message = { type: 'error', text: res.message || 'Failed to apply settings' }
      }
    } catch (err: any) {
      message = { type: 'error', text: err?.message || 'Error applying settings' }
    } finally {
      applying = false
    }
  }

  async function handleReset() {
    if (!confirm(`Are you sure you want to disconnect 9Router and reset ${toolName} settings?`)) {
      return
    }

    resetting = true
    message = null
    try {
      const url = baseUrl.trim() || localOrigin
      const res = await api.resetCliToolSettings(toolId, url)
      if (res.success) {
        message = {
          type: 'success',
          text: `${toolName} configuration has been reset.`,
        }
        hasStoredApiKey = false
        selectedApiKey = apiKeys[0]?.key || ''
        customApiKey = ''
        model = ''
        tierSonnet = ''
        tierOpus = ''
        tierHaiku = ''
        tierFable = ''
        autoCompactWindow = 'default'
        customCompactWindow = ''
        if (onStatusChange) onStatusChange()
      } else {
        message = { type: 'error', text: res.message || 'Failed to reset settings' }
      }
    } catch (err: any) {
      message = { type: 'error', text: err?.message || 'Error resetting settings' }
    } finally {
      resetting = false
    }
  }

  function copyText(text: string, id: string) {
    copyToClipboard(text)
    copiedSnippetId = id
    setTimeout(() => {
      if (copiedSnippetId === id) copiedSnippetId = null
    }, 2000)
  }
</script>

<datalist id="cli-tool-models-datalist">
  {#each availableModels as m}
    <option value={m}></option>
  {/each}
</datalist>

<div class="space-y-4">
  <!-- Tabs: Direct Config vs Manual -->
  <div class="flex items-center justify-between border-b border-border/80 pb-2">
    <div class="flex items-center gap-1.5 bg-surface-2 p-1 rounded-lg border border-border">
      <button
        type="button"
        onclick={() => (activeTab = 'form')}
        class="px-3 py-1 rounded-md text-xs font-semibold transition cursor-pointer {activeTab === 'form'
          ? 'bg-surface text-brand-500 shadow-sm'
          : 'text-text-muted hover:text-text-main'}"
      >
        <span class="flex items-center gap-1.5">
          <Wrench class="w-3.5 h-3.5" />
          <span>Configuration Editor</span>
        </span>
      </button>
      <button
        type="button"
        onclick={() => (activeTab = 'manual')}
        class="px-3 py-1 rounded-md text-xs font-semibold transition cursor-pointer {activeTab === 'manual'
          ? 'bg-surface text-brand-500 shadow-sm'
          : 'text-text-muted hover:text-text-main'}"
      >
        <span class="flex items-center gap-1.5">
          <Terminal class="w-3.5 h-3.5" />
          <span>Manual Shell Setup</span>
        </span>
      </button>
    </div>

    {#if targetFiles.length > 0}
      <span class="hidden sm:inline-flex items-center gap-1 text-[11px] font-mono text-text-subtle truncate max-w-[220px]" title={targetFiles[0].path}>
        <FileText class="w-3.5 h-3.5 shrink-0" />
        <span class="truncate">{targetFiles[0].path.split('/').slice(-2).join('/')}</span>
      </span>
    {/if}
  </div>

  <!-- Notification Banner -->
  {#if message}
    <div
      class="p-3 rounded-xl border text-xs flex items-center justify-between gap-2 {message.type === 'success'
        ? 'bg-success/10 border-success/30 text-success'
        : 'bg-error/10 border-error/30 text-error'}"
    >
      <div class="flex items-center gap-2">
        {#if message.type === 'success'}
          <Check class="w-4 h-4 shrink-0" />
        {:else}
          <AlertCircle class="w-4 h-4 shrink-0" />
        {/if}
        <span>{message.text}</span>
      </div>
      <button
        type="button"
        onclick={() => (message = null)}
        class="text-xs opacity-70 hover:opacity-100 cursor-pointer font-bold px-1"
      >
        ✕
      </button>
    </div>
  {/if}

  {#if loading}
    <div class="flex flex-col items-center justify-center py-10 space-y-2 text-text-muted text-xs">
      <Loader2 class="w-6 h-6 animate-spin text-brand-500" />
      <span>Loading local client configuration...</span>
    </div>
  {:else if activeTab === 'form'}
    <!-- CONFIGURATION FORM -->
    <div class="space-y-4 max-h-[60vh] overflow-y-auto pr-1">
      <!-- Target Config File Indicator -->
      {#if targetFiles.length > 0}
        <div class="p-2.5 rounded-lg bg-surface-2 border border-border/70 flex flex-wrap items-center justify-between gap-2 text-xs">
          <div class="flex items-center gap-2 text-text-muted min-w-0 flex-1">
            <FileText class="w-3.5 h-3.5 text-brand-500 shrink-0" />
            <span class="font-medium text-text-main shrink-0">Config File:</span>
            <span class="font-mono text-[11px] truncate text-text-muted" title={targetFiles[0].path}>
              {targetFiles[0].path}
            </span>
          </div>
          <div class="flex items-center gap-1.5 shrink-0">
            <span class="text-[10px] font-semibold px-2 py-0.5 rounded border whitespace-nowrap {has9Router ? 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-400 border-emerald-500/25' : installed ? 'bg-amber-500/10 text-amber-800 dark:text-amber-300 border-amber-500/25' : 'bg-surface-2 text-text-muted border-border'}">
              {has9Router ? 'Connected' : installed ? 'Not configured' : 'Not installed'}
            </span>
            <span class="text-[10px] font-mono uppercase px-1.5 py-0.5 rounded bg-surface border border-border text-text-muted">
              {targetFiles[0].format}
            </span>
          </div>
        </div>
      {/if}

      <!-- Base URL & API Key -->
      <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
        <div class="space-y-1">
          <div class="text-xs font-bold text-text-main flex items-center justify-between">
            <span>Gateway Base URL</span>
          </div>
          <input
            type="text"
            bind:value={baseUrl}
            placeholder={localOrigin}
            class="w-full min-h-9 px-3 py-1.5 rounded-lg bg-surface-2 border border-border text-[16px] sm:text-xs text-text-main font-mono focus:outline-none focus:border-brand-500 focus-visible:ring-2 focus-visible:ring-brand-500/50"
          />
        </div>

        <div class="space-y-1">
          <div class="text-xs font-bold text-text-main flex items-center justify-between">
            <span>9Router API Key</span>
          </div>
          {#if apiKeys.length > 0}
            <div class="flex gap-1.5">
              <select
                bind:value={selectedApiKey}
                class="w-full min-h-9 px-2.5 py-1.5 rounded-lg bg-surface-2 border border-border text-[16px] sm:text-xs text-text-main font-mono focus:outline-none focus:border-brand-500 focus-visible:ring-2 focus-visible:ring-brand-500/50"
              >
                {#if hasStoredApiKey}
                  <option value="">(Keep saved token)</option>
                {/if}
                {#each apiKeys as k}
                  <option value={k.key}>
                    {k.name || 'Key'} ({k.key.slice(0, 10)}...)
                  </option>
                {/each}
                <option value="__custom__">Custom key...</option>
              </select>
            </div>
            {#if selectedApiKey === '__custom__' || (!hasStoredApiKey && !selectedApiKey)}
              <input
                type="password"
                bind:value={customApiKey}
                placeholder="sk-9router-..."
                class="w-full mt-1.5 min-h-9 px-3 py-1.5 rounded-lg bg-surface-2 border border-border text-[16px] sm:text-xs text-text-main font-mono focus:outline-none focus:border-brand-500 focus-visible:ring-2 focus-visible:ring-brand-500/50"
              />
            {/if}
          {:else}
            {#if hasStoredApiKey}
              <div class="text-[11px] text-text-muted mb-1">Current token is saved in client config. Enter a new key to replace:</div>
            {/if}
            <input
              type="password"
              bind:value={customApiKey}
              placeholder={hasStoredApiKey ? "Leave empty to keep saved token..." : "sk-9router-..."}
              class="w-full min-h-9 px-3 py-1.5 rounded-lg bg-surface-2 border border-border text-[16px] sm:text-xs text-text-main font-mono focus:outline-none focus:border-brand-500 focus-visible:ring-2 focus-visible:ring-brand-500/50"
            />
          {/if}
        </div>
      </div>

      <!-- CLAUDE CODE SPECIFIC SECTION -->
      {#if toolId === 'claude'}
        <div class="p-3.5 rounded-xl bg-surface-2/70 border border-border/80 space-y-3">
          <div class="flex items-center justify-between pb-2 border-b border-border/60">
            <div class="flex items-center gap-2">
              <Sparkles class="w-4 h-4 text-brand-500" />
              <span class="text-xs font-bold text-text-main">Claude Code Model Tiers</span>
            </div>
            <span class="text-[10px] text-text-muted">Managed via ~/.claude/settings.json</span>
          </div>

          <!-- Primary Default Model -->
          <div class="space-y-1">
            <div class="text-xs font-medium text-text-main flex flex-wrap items-center justify-between gap-1">
              <span>Primary / Default Model</span>
              <span class="text-[10px] font-mono text-text-muted">"model" key</span>
            </div>
            <input
              type="text"
              list="cli-tool-models-datalist"
              bind:value={model}
              placeholder="e.g. sonnet, opus, or custom combo/model ID"
              class="w-full min-h-9 px-3 py-1.5 rounded-lg bg-surface border border-border text-[16px] sm:text-xs text-text-main font-mono focus:outline-none focus:border-brand-500 focus-visible:ring-2 focus-visible:ring-brand-500/50"
            />
          </div>

          <!-- Sonnet Tier -->
          <div class="space-y-1">
            <div class="text-xs font-medium text-text-main flex flex-wrap items-center justify-between gap-1">
              <span class="flex items-center gap-1.5">
                <span class="size-2 rounded-full bg-amber-500"></span>
                <span>Sonnet Tier Override</span>
              </span>
              <span class="text-[10px] font-mono text-text-muted">ANTHROPIC_DEFAULT_SONNET_MODEL</span>
            </div>
            <input
              type="text"
              list="cli-tool-models-datalist"
              bind:value={tierSonnet}
              placeholder="e.g. claude-sonnet-5-5[1m], sonnet, or oc/deepseek-v4.1-flash-free"
              class="w-full min-h-9 px-3 py-1.5 rounded-lg bg-surface border border-border text-[16px] sm:text-xs text-text-main font-mono focus:outline-none focus:border-brand-500 focus-visible:ring-2 focus-visible:ring-brand-500/50"
            />
          </div>

          <!-- Opus Tier -->
          <div class="space-y-1">
            <div class="text-xs font-medium text-text-main flex flex-wrap items-center justify-between gap-1">
              <span class="flex items-center gap-1.5">
                <span class="size-2 rounded-full bg-purple-500"></span>
                <span>Opus Tier Override</span>
              </span>
              <span class="text-[10px] font-mono text-text-muted">ANTHROPIC_DEFAULT_OPUS_MODEL</span>
            </div>
            <input
              type="text"
              list="cli-tool-models-datalist"
              bind:value={tierOpus}
              placeholder="e.g. claude-opus-5-5[1m], opus, or combo-opus"
              class="w-full min-h-9 px-3 py-1.5 rounded-lg bg-surface border border-border text-[16px] sm:text-xs text-text-main font-mono focus:outline-none focus:border-brand-500 focus-visible:ring-2 focus-visible:ring-brand-500/50"
            />
          </div>

          <!-- Haiku Tier -->
          <div class="space-y-1">
            <div class="text-xs font-medium text-text-main flex flex-wrap items-center justify-between gap-1">
              <span class="flex items-center gap-1.5">
                <span class="size-2 rounded-full bg-teal-500"></span>
                <span>Haiku Tier Override</span>
              </span>
              <span class="text-[10px] font-mono text-text-muted">ANTHROPIC_DEFAULT_HAIKU_MODEL</span>
            </div>
            <input
              type="text"
              list="cli-tool-models-datalist"
              bind:value={tierHaiku}
              placeholder="e.g. claude-haiku-5-5, haiku, or oc/deepseek-v4.1-flash-free"
              class="w-full min-h-9 px-3 py-1.5 rounded-lg bg-surface border border-border text-[16px] sm:text-xs text-text-main font-mono focus:outline-none focus:border-brand-500 focus-visible:ring-2 focus-visible:ring-brand-500/50"
            />
          </div>

          <!-- Auto-Compact Window Setting -->
          <div class="pt-2 border-t border-border/60 space-y-2">
            <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-2">
              <div>
                <span class="text-xs font-medium text-text-main">Auto-Compact Window Threshold</span>
                <p class="text-[11px] text-text-muted">Compact context before token limit (e.g. 330k for DeepSeek, 200k for Claude)</p>
              </div>
              <div class="flex flex-wrap items-center gap-1 bg-surface p-1 rounded-lg border border-border">
                {#each ['default', '100000', '180000', '200000', '330000', 'custom'] as opt}
                  <button
                    type="button"
                    class="min-h-7 px-2.5 py-1 text-[11px] font-medium rounded whitespace-nowrap transition-colors cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/50 {autoCompactWindow === opt
                      ? 'bg-brand-500 text-white'
                      : 'text-text-muted hover:text-text-main'}"
                    onclick={() => (autoCompactWindow = opt)}
                  >
                    {opt === 'default'
                      ? 'Default'
                      : opt === '100000'
                        ? '100k'
                        : opt === '180000'
                          ? '180k'
                          : opt === '200000'
                            ? '200k'
                            : opt === '330000'
                              ? '330k'
                              : 'Custom'}
                  </button>
                {/each}
              </div>
            </div>

            {#if autoCompactWindow === 'custom'}
              <div class="flex items-center gap-2 mt-1">
                <input
                  type="number"
                  bind:value={customCompactWindow}
                  placeholder="330000"
                  class="w-48 min-h-9 px-3 py-1.5 rounded-lg bg-surface border border-border text-[16px] sm:text-xs text-text-main font-mono focus:outline-none focus:border-brand-500 focus-visible:ring-2 focus-visible:ring-brand-500/50"
                />
                <span class="text-xs text-text-muted">tokens</span>
              </div>
            {/if}
          </div>
        </div>
      {:else}
        <!-- GENERIC CLI TOOLS MODEL SELECTOR (Codex, OpenCode, Droid, Cline, Pi, OMP...) -->
        <div class="p-3.5 rounded-xl bg-surface-2/70 border border-border/80 space-y-2">
          <div class="text-xs font-bold text-text-main flex flex-wrap items-center justify-between gap-2">
            <span class="flex items-center gap-1.5">
              <Cpu class="w-4 h-4 text-brand-500" />
              <span>Configured Default Model</span>
            </span>
            <span class="text-[10px] font-mono text-text-muted">
              {toolId === 'codex' ? 'config.toml' : 'settings file'}
            </span>
          </div>
          <input
            type="text"
            list="cli-tool-models-datalist"
            bind:value={model}
            placeholder="e.g. codex-combo, gemini-2.5-pro, or gpt-4.5"
            class="w-full min-h-9 px-3 py-1.5 rounded-lg bg-surface border border-border text-[16px] sm:text-xs text-text-main font-mono focus:outline-none focus:border-brand-500 focus-visible:ring-2 focus-visible:ring-brand-500/50"
          />
          <p class="text-[11px] text-text-muted">
            Select a combo or model ID configured in 9Router. Writes to the client's official config file.
          </p>
        </div>
      {/if}

      <!-- Action Buttons -->
      <div class="flex flex-col sm:flex-row items-center justify-between gap-3 pt-2">
        <button
          type="button"
          onclick={handleReset}
          disabled={resetting || applying}
          class="w-full sm:w-auto min-h-9 flex items-center justify-center gap-1.5 px-3.5 py-2 rounded-lg bg-error/10 hover:bg-error/20 text-error border border-error/20 text-xs font-semibold whitespace-nowrap transition cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-error/50 disabled:opacity-50"
        >
          {#if resetting}
            <Loader2 class="w-3.5 h-3.5 animate-spin" />
            <span>Resetting...</span>
          {:else}
            <Trash2 class="w-3.5 h-3.5" />
            <span>Disconnect / Reset</span>
          {/if}
        </button>

        <button
          type="button"
          onclick={handleApply}
          disabled={applying || resetting}
          class="w-full sm:w-auto min-h-9 flex items-center justify-center gap-1.5 px-5 py-2 rounded-lg bg-brand-500 hover:bg-brand-600 text-white text-xs font-bold whitespace-nowrap shadow-md transition cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/50 disabled:opacity-50"
        >
          {#if applying}
            <Loader2 class="w-3.5 h-3.5 animate-spin" />
            <span>Writing config...</span>
          {:else}
            <Save class="w-3.5 h-3.5" />
            <span>Save & Apply Settings</span>
          {/if}
        </button>
      </div>
    </div>
  {:else}
    <!-- MANUAL SHELL SETUP TAB -->
    <div class="space-y-3 max-h-[60vh] overflow-y-auto pr-1">
      {#if envVars}
        <div class="space-y-2">
          <div class="flex flex-wrap items-center justify-between gap-2">
            <span class="text-xs font-bold text-text-main">One-Click Environment Setup</span>
            <button
              type="button"
              onclick={() => {
                const lines = Object.entries(envVars || {})
                  .map(([k, v]) => `export ${k}="${k.includes('KEY') || k.includes('TOKEN') ? manualApiKey : v}"`)
                  .join('\n')
                copyText(lines, 'all-env')
              }}
              class="min-h-7 px-2 rounded flex items-center gap-1 text-xs font-semibold text-brand-600 dark:text-brand-400 hover:bg-surface-2 cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/50"
            >
              {#if copiedSnippetId === 'all-env'}
                <Check class="w-3.5 h-3.5 text-emerald-700 dark:text-emerald-400" />
                <span>Copied!</span>
              {:else}
                <Copy class="w-3.5 h-3.5" />
                <span>Copy Export Commands</span>
              {/if}
            </button>
          </div>

          <div class="p-3 rounded-xl bg-bg border border-border font-mono text-xs text-text-main space-y-1.5 select-all overflow-x-auto">
            {#each Object.entries(envVars) as [key, val]}
              <div class="flex items-center justify-between gap-2 min-w-0">
                <span class="text-text-muted break-all">export {key}="<span class="text-brand-600 dark:text-brand-400">{key.includes('KEY') || key.includes('TOKEN') ? manualApiKey : val}</span>"</span>
                <button
                  type="button"
                  onclick={() => copyText(`export ${key}="${key.includes('KEY') || key.includes('TOKEN') ? manualApiKey : val}"`, key)}
                  class="min-h-7 min-w-7 flex items-center justify-center rounded text-text-subtle hover:text-text-main hover:bg-surface-2 cursor-pointer shrink-0 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/50"
                  title="Copy single variable"
                  aria-label={`Copy ${key}`}
                >
                  {#if copiedSnippetId === key}
                    <Check class="w-3 h-3 text-emerald-700 dark:text-emerald-400" />
                  {:else}
                    <Copy class="w-3 h-3" />
                  {/if}
                </button>
              </div>
            {/each}
          </div>
        </div>
      {/if}

      {#if instructions && instructions.length > 0}
        <div class="space-y-2">
          <span class="text-xs font-bold text-text-main">Manual Setup Instructions</span>
          <div class="space-y-2 text-xs">
            {#each instructions as step, idx}
              <div class="flex items-start gap-2.5 p-2.5 rounded-lg bg-surface-2 border border-border/60">
                <span class="w-5 h-5 rounded-full bg-brand-500/10 text-brand-500 font-bold flex items-center justify-center text-[10px] shrink-0 mt-0.5">
                  {idx + 1}
                </span>
                <span class="text-text-muted leading-relaxed select-text flex-1">
                  {step}
                </span>
              </div>
            {/each}
          </div>
        </div>
      {/if}
    </div>
  {/if}
</div>
