<script lang="ts">
  /**
   * Hermes per-profile configuration card.
   *
   * Port of upstream decolua/9router
   * `src/app/(dashboard)/dashboard/cli-tools/components/HermesToolCard.js`
   * (PR #4660), re-expressed with Svelte 5 runes and this repo's design
   * tokens. Upstream picks models through a `ModelSelectModal` driven by the
   * gateway's live model list; this repo has no such modal, so the model
   * fields are plain text inputs, matching the guide cards in CliToolsView.
   */
  import { untrack } from 'svelte'
  import {
    AlertTriangle,
    Check,
    ChevronRight,
    Copy,
    Layers,
    Loader2,
    RotateCcw,
    Save,
    Terminal,
    X,
  } from 'lucide-svelte'
  import Card from '../../lib/ui/Card.svelte'
  import { copyToClipboard } from '../../lib/clipboard'
  import { HERMES_ROLES, hermesRoleLabel } from '../../lib/hermesRoles'
  import {
    activeRoleModels,
    hermesConfigYaml,
    hermesEnvContent,
    hermesHomeDir,
    isKnown9RouterEndpoint,
    normalizeHermesEndpoint,
    profileStatus,
    type EndpointContext,
  } from '../../lib/hermesConfig'
  import { ApiError, api, type APIKey, type HermesProfile, type HermesSettingsResponse } from '../../api/client'

  interface Props {
    /** Whether the status scan found a local Hermes install. */
    installed?: boolean
    apiKeys?: APIKey[]
  }

  let { installed = false, apiKeys = [] }: Props = $props()

  let settings = $state<HermesSettingsResponse | null>(null)
  let profiles = $state<HermesProfile[]>([])
  let activeProfile = $state('default')
  let checking = $state(false)
  let applying = $state(false)
  let applyingAll = $state(false)
  let resetting = $state(false)
  let message = $state<{ type: 'success' | 'error'; text: string } | null>(null)
  let endpoint = $state('')
  let selectedApiKey = $state('')
  let selectedModel = $state('')
  let roleModels = $state<Record<string, string>>({})
  let showRoles = $state(false)
  let showManualConfig = $state(false)
  let copiedCommand = $state('')
  let tunnelUrl = $state('')
  let tailscaleUrl = $state('')

  // SSR fallback uses the Go default port; the live origin wins on mount.
  let localOrigin = $state(typeof window !== 'undefined' ? window.location.origin : 'http://127.0.0.1:20130')
  $effect(() => {
    if (typeof window !== 'undefined') localOrigin = window.location.origin
  })

  const endpointContext = $derived<EndpointContext>({ localOrigin, tunnelUrl, tailscaleUrl })
  // The server appends /v1, so what we display and what we send both omit it.
  let effectiveEndpoint = $derived(normalizeHermesEndpoint(endpoint, localOrigin))
  let activeProfileData = $derived(profiles.find((p) => p.name === activeProfile) ?? null)
  let runCommand = $derived(activeProfileData?.command || (activeProfile === 'default' ? 'hermes' : `hermes -p ${activeProfile}`))
  let keyToUse = $derived(selectedApiKey.trim() || apiKeys[0]?.key || '')
  let currentBaseUrl = $derived(settings?.settings?.model?.base_url ?? '')

  function selections() {
    return [
      { role: 'default', model: selectedModel.trim() },
      ...activeRoleModels(roleModels),
    ]
  }

  async function loadProfiles() {
    const res = await api.getHermesProfiles()
    profiles = res.profiles ?? []
    if (!profiles.some((p) => p.name === activeProfile)) activeProfile = 'default'
  }

  // Mirrors the settings payload into the form. Both fields are always reset:
  // values from the previously selected profile must not leak into the next.
  function hydrate(data: HermesSettingsResponse) {
    const model = data.settings?.model
    selectedModel = model?.default ?? ''
    roleModels = {}
    if (data.settings?.delegation?.model) roleModels.delegation = data.settings.delegation.model
    for (const [role, cfg] of Object.entries(data.settings?.auxiliary ?? {})) {
      if (cfg?.model) roleModels[role] = cfg.model
    }
    endpoint = data.settings?.model?.base_url
      ? normalizeHermesEndpoint(data.settings.model.base_url, localOrigin)
      : localOrigin
  }

  async function loadSettings(profile = activeProfile) {
    checking = true
    try {
      const data = await api.getHermesSettings(profile)
      settings = data
      hydrate(data)
    } catch (err) {
      const text = err instanceof Error ? err.message : 'Failed to load Hermes settings'
      // A 404 means the profile vanished (deleted in another tab): fall back
      // to default and re-read the profile list rather than leaving a dead
      // selection on screen. The status code is the signal — the message text
      // is written for humans and can change.
      if (err instanceof ApiError && err.status === 404) {
        message = { type: 'error', text }
        activeProfile = 'default'
        await loadProfiles()
        await loadSettings('default')
        return
      }
      settings = { installed: false, settings: null, message: text }
      message = { type: 'error', text }
    } finally {
      checking = false
    }
  }

  // Fire once on mount. `untrack` keeps the state this writes (selectedApiKey,
  // profiles, settings) out of the dependency set — re-running on those would
  // re-hydrate the form and wipe what the user is typing.
  $effect(() => {
    if (typeof window === 'undefined') return
    untrack(() => {
      void (async () => {
        if (apiKeys.length > 0 && !selectedApiKey) selectedApiKey = apiKeys[0].key
        await loadProfiles()
        await loadSettings('default')
        const tunnel = await api.getTunnelStatus().catch(() => null)
        tunnelUrl = tunnel?.tunnel?.tunnelUrl ?? tunnel?.tunnel?.publicUrl ?? ''
        tailscaleUrl = tunnel?.tailscale?.tunnelUrl ?? ''
      })()
    })
  })

  async function selectProfile(name: string) {
    if (name === activeProfile) return
    activeProfile = name
    message = null
    roleModels = {}
    selectedModel = ''
    await loadSettings(name)
  }

  async function copyCommand() {
    if (await copyToClipboard(runCommand)) {
      copiedCommand = runCommand
      setTimeout(() => (copiedCommand = ''), 2000)
    }
  }

  async function afterChange() {
    await loadProfiles()
    await loadSettings(activeProfile)
  }

  async function handleApply() {
    applying = true
    message = null
    try {
      const res = await api.applyHermesSettings({
        profile: activeProfile,
        baseUrl: effectiveEndpoint,
        apiKey: keyToUse || null,
        selections: selections(),
      })
      message = { type: 'success', text: res.message || `Settings applied to ${activeProfile === 'default' ? 'the default profile' : `profile "${activeProfile}"`}.` }
      await afterChange()
    } catch (err) {
      message = { type: 'error', text: err instanceof Error ? err.message : 'Failed to apply settings' }
    } finally {
      applying = false
    }
  }

  // The endpoint field mirrors whatever the active profile currently points
  // at, so it can hold a foreign provider URL; propagating that (and its key)
  // to every profile would be a mistake. Hence the known-endpoint guard.
  async function handleApplyAll() {
    if (!isKnown9RouterEndpoint(effectiveEndpoint, endpointContext)) {
      message = {
        type: 'error',
        text: `"${effectiveEndpoint}" is not a 9router endpoint. Select the local, tunnel, Tailscale or a saved 9router endpoint before applying to all profiles.`,
      }
      return
    }
    applyingAll = true
    message = null
    try {
      const res = await api.applyHermesToAll({
        baseUrl: effectiveEndpoint,
        apiKey: keyToUse || null,
        model: selectedModel.trim(),
      })
      const results = res.results ?? []
      const notUpdated = results.filter((r) => r.status !== 'updated')
      if (notUpdated.length === 0) {
        message = { type: 'success', text: res.message || `Endpoint and API key applied to ${res.updated ?? 0} profile(s).` }
      } else {
        const detail = notUpdated.map((r) => `${r.profile}: ${r.reason || r.status}`).join('; ')
        message = {
          type: (res.updated ?? 0) > 0 ? 'success' : 'error',
          text: `Updated ${res.updated ?? 0} profile(s), skipped ${notUpdated.length}: ${detail}`,
        }
      }
      await afterChange()
    } catch (err) {
      message = { type: 'error', text: err instanceof Error ? err.message : 'Failed to apply settings' }
    } finally {
      applyingAll = false
    }
  }

  async function handleReset() {
    resetting = true
    message = null
    try {
      const res = await api.resetHermesSettings(activeProfile)
      message = { type: 'success', text: res.message || `Settings reset for ${activeProfile}.` }
      await afterChange()
    } catch (err) {
      message = { type: 'error', text: err instanceof Error ? err.message : 'Failed to reset settings' }
    } finally {
      resetting = false
    }
  }

  let manualConfigs = $derived([
    { filename: `${hermesHomeDir(activeProfile)}/config.yaml`, content: hermesConfigYaml({ default: selectedModel, roles: roleModels }, effectiveEndpoint, runCommand) },
    { filename: `${hermesHomeDir(activeProfile)}/.env`, content: hermesEnvContent(keyToUse) },
  ])

  const STATUS_DOT = {
    configured: 'bg-green-500',
    other: 'bg-blue-500',
    not_configured: 'bg-yellow-500',
  } as const
  const STATUS_LABEL = {
    configured: 'Connected',
    other: 'Other endpoint',
    not_configured: 'Not configured',
  } as const
</script>

<Card padding="xs">
  {#if checking}
    <div class="flex items-center gap-2 text-sm text-text-muted py-2">
      <Loader2 class="w-4 h-4 animate-spin" />
      <span>Checking Hermes Agent…</span>
    </div>
  {:else if settings && !settings.installed}
    <div class="flex flex-col gap-3 p-4 rounded-[10px] bg-amber-500/10 border border-amber-500/30">
      <div class="flex items-start gap-3">
        <AlertTriangle class="w-5 h-5 text-amber-700 dark:text-amber-400 shrink-0" />
        <div class="flex-1 min-w-0">
          <p class="font-semibold text-amber-800 dark:text-amber-300">Hermes Agent not detected locally</p>
          <p class="text-sm text-text-muted break-words">
            Install: curl -fsSL https://raw.githubusercontent.com/NousResearch/hermes-agent/main/scripts/install.sh | bash
          </p>
        </div>
      </div>
      <button
        type="button"
        onclick={() => (showManualConfig = true)}
        class="self-start min-h-8 flex items-center gap-1.5 px-3 py-1.5 rounded-[10px] text-xs font-semibold bg-amber-500/15 border border-amber-500/40 text-amber-800 dark:text-amber-300 hover:bg-amber-500/25 cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/50"
      >
        <Copy class="w-3.5 h-3.5" />
        Manual Config
      </button>
    </div>
  {:else if settings?.installed}
    <div class="flex flex-col gap-4">
      {#if profiles.length > 1}
        <div class="flex flex-col gap-1.5">
          <div class="flex flex-wrap items-center gap-1.5">
            <span class="text-xs font-semibold text-text-main mr-1">Profile</span>
            {#each profiles as p (p.name)}
              {@const status = profileStatus(p, endpointContext)}
              <button
                type="button"
                onclick={() => selectProfile(p.name)}
                title="{p.isDefault ? 'Default profile' : p.command} — {STATUS_LABEL[status]}"
                class="flex items-center gap-1.5 px-2 py-1 rounded border text-xs cursor-pointer transition-colors {p.name === activeProfile
                  ? 'border-brand-500 bg-brand-500/10 text-brand-500'
                  : 'bg-surface border-border text-text-muted hover:text-text-main'}"
              >
                <span class="size-1.5 rounded-full shrink-0 {STATUS_DOT[status]}"></span>
                <span class="font-medium">{p.displayName || p.name}</span>
                {#if p.displayName && !p.isDefault}
                  <span class="font-mono text-[10px] opacity-60">{p.name}</span>
                {/if}
              </button>
            {/each}
          </div>
          <div class="flex flex-wrap items-center gap-2 text-xs text-text-muted">
            <Terminal class="w-3.5 h-3.5 shrink-0" />
            <code class="px-1.5 py-0.5 rounded bg-surface-2 font-mono text-[11px]">{runCommand}</code>
            <button type="button" onclick={copyCommand} class="p-1 rounded cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/50 {copiedCommand === runCommand ? 'text-emerald-700 dark:text-emerald-400' : 'text-text-muted hover:text-text-main'}" title="Copy command" aria-label="Copy command">
              {#if copiedCommand === runCommand}
                <Check class="w-3.5 h-3.5" />
              {:else}
                <Copy class="w-3.5 h-3.5" />
              {/if}
            </button>
            <span class="hidden sm:inline">Every profile runs as its own agent with its own model.</span>
          </div>
        </div>
      {/if}

      <div class="flex flex-col gap-2.5">
        <div class="flex flex-col sm:flex-row sm:items-center gap-1.5 sm:gap-2">
          <span class="text-xs font-semibold text-text-main sm:w-32 sm:text-right sm:text-sm">Endpoint</span>
          <input
            type="text"
            bind:value={endpoint}
            placeholder="{localOrigin} (the server appends /v1)"
            class="flex-1 min-w-0 min-h-8 px-2.5 py-1.5 rounded-lg bg-surface border border-border text-[16px] sm:text-xs text-text-main focus:outline-none focus:border-brand-500 focus-visible:ring-2 focus-visible:ring-brand-500/50"
          />
        </div>

        {#if currentBaseUrl}
          <div class="flex flex-col sm:flex-row sm:items-center gap-1.5 sm:gap-2">
            <span class="text-xs font-semibold text-text-main sm:w-32 sm:text-right sm:text-sm">Current</span>
            <span class="flex-1 min-w-0 truncate rounded-lg bg-surface-2 px-2.5 py-1.5 text-xs text-text-muted">{currentBaseUrl}</span>
          </div>
        {/if}

        <div class="flex flex-col sm:flex-row sm:items-center gap-1.5 sm:gap-2">
          <span class="text-xs font-semibold text-text-main sm:w-32 sm:text-right sm:text-sm">API Key</span>
          <select
            bind:value={selectedApiKey}
            class="flex-1 min-w-0 min-h-8 px-2.5 py-1.5 rounded-lg bg-surface border border-border text-[16px] sm:text-xs text-text-main focus:outline-none focus:border-brand-500 focus-visible:ring-2 focus-visible:ring-brand-500/50"
          >
            {#if apiKeys.length === 0}
              <option value="">No key created</option>
            {/if}
            {#each apiKeys as key (key.id)}
              <option value={key.key}>{key.name || key.keyDisplay || key.key}</option>
            {/each}
          </select>
        </div>

        <div class="flex flex-col sm:flex-row sm:items-center gap-1.5 sm:gap-2">
          <span class="text-xs font-semibold text-text-main sm:w-32 sm:text-right sm:text-sm">Default Model</span>
          <input
            type="text"
            bind:value={selectedModel}
            placeholder="provider/model-id"
            class="flex-1 min-w-0 min-h-8 px-2.5 py-1.5 rounded-lg bg-surface border border-border text-[16px] sm:text-xs text-text-main focus:outline-none focus:border-brand-500 focus-visible:ring-2 focus-visible:ring-brand-500/50"
          />
        </div>

        <div>
          <button
            type="button"
            onclick={() => (showRoles = !showRoles)}
            class="min-h-7 flex items-center gap-1 rounded px-1 text-xs font-semibold text-text-main hover:text-brand-600 dark:hover:text-brand-400 cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/50"
          >
            <ChevronRight class="w-4 h-4 text-text-muted transition-transform {showRoles ? 'rotate-90' : ''}" />
            Model Roles (optional)
          </button>
          {#if showRoles}
            <div class="mt-2 flex flex-col gap-1.5">
              {#each HERMES_ROLES as role (role.id)}
                <div class="flex flex-col sm:flex-row sm:items-center gap-1.5 sm:gap-2">
                  <span class="text-xs font-semibold text-text-main sm:w-32 sm:text-right sm:text-sm truncate" title={role.label}>
                    {hermesRoleLabel(role.id)}
                  </span>
                    <input
                      type="text"
                      value={roleModels[role.id] ?? ''}
                      oninput={(e) => (roleModels = { ...roleModels, [role.id]: e.currentTarget.value })}
                      placeholder="inherit default"
                      class="flex-1 min-w-0 min-h-8 px-2.5 py-1.5 rounded-lg bg-surface border border-border text-[16px] sm:text-xs text-text-main focus:outline-none focus:border-brand-500 focus-visible:ring-2 focus-visible:ring-brand-500/50"
                    />
                </div>
              {/each}
              <p class="text-xs text-text-muted">Empty roles inherit the default model.</p>
            </div>
          {/if}
        </div>
      </div>

      {#if message}
        <div class="flex items-start gap-2 px-3 py-2 rounded-lg text-xs {message.type === 'success' ? 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-400' : 'bg-danger/10 text-danger'}">
          {#if message.type === 'success'}
            <Check class="w-3.5 h-3.5 shrink-0 mt-0.5" />
          {:else}
            <AlertTriangle class="w-3.5 h-3.5 shrink-0 mt-0.5" />
          {/if}
          <span class="break-words">{message.text}</span>
        </div>
      {/if}

      <div class="flex flex-wrap items-center gap-2 pt-1">
        <button
          type="button"
          onclick={handleApply}
          disabled={!selectedModel.trim() || applying}
          class="min-h-9 flex items-center justify-center gap-1.5 px-3.5 py-1.5 rounded-[10px] bg-brand-500 hover:bg-brand-600 text-white font-semibold text-xs whitespace-nowrap shrink-0 disabled:opacity-50 cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/50"
        >
          <Save class="w-3.5 h-3.5" />Apply
        </button>
        {#if profiles.length > 1}
          <button
            type="button"
            onclick={handleApplyAll}
            disabled={applyingAll}
            title="Set the endpoint and API key on every profile that routes through 9router. Each profile keeps its own model."
            class="min-h-9 flex items-center justify-center gap-1.5 px-3.5 py-1.5 rounded-[10px] bg-surface-2 hover:bg-surface-3 text-text-main border border-border font-semibold text-xs whitespace-nowrap shrink-0 disabled:opacity-50 cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/50"
          >
            <Layers class="w-3.5 h-3.5" />Apply to All Profiles
          </button>
        {/if}
        <button
          type="button"
          onclick={handleReset}
          disabled={!settings.has9Router || resetting}
          class="min-h-9 flex items-center justify-center gap-1.5 px-3.5 py-1.5 rounded-[10px] border border-border text-text-main hover:bg-surface-2 font-semibold text-xs whitespace-nowrap shrink-0 disabled:opacity-50 cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/50"
        >
          <RotateCcw class="w-3.5 h-3.5" />Reset
        </button>
        <button
          type="button"
          onclick={() => (showManualConfig = true)}
          class="min-h-9 flex items-center justify-center gap-1.5 px-3 py-1.5 rounded-[10px] text-text-muted font-semibold text-xs whitespace-nowrap shrink-0 hover:text-text-main hover:bg-surface-2 cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/50"
        >
          <Copy class="w-3.5 h-3.5" />Manual Config
        </button>
      </div>
    </div>
  {:else}
    <div class="py-2 text-sm text-text-muted">
      {installed ? 'Loading Hermes settings…' : 'Checking Hermes Agent…'}
    </div>
  {/if}
</Card>

{#if showManualConfig}
  <div
    class="fixed inset-0 z-50 flex items-center justify-center bg-black/75 backdrop-blur-sm p-4"
    onclick={() => (showManualConfig = false)}
    onkeydown={(e) => { if (e.key === 'Escape') showManualConfig = false }}
    role="presentation"
  >
    <div
      role="dialog"
      aria-modal="true"
      aria-label="Hermes Agent Manual Configuration"
      class="w-full max-w-2xl max-h-[85vh] overflow-y-auto custom-scrollbar p-6 rounded-2xl bg-surface border border-border shadow-[var(--shadow-elevated)] space-y-4"
      onclick={(e) => e.stopPropagation()}
      onkeydown={(e) => e.stopPropagation()}
    >
      <div class="flex items-start justify-between gap-3 pb-3 border-b border-border">
        <div>
          <h3 class="text-base font-bold text-text-main">Hermes Agent Manual Configuration</h3>
          <p class="text-xs text-text-muted">Write these into {hermesHomeDir(activeProfile)} when you prefer to edit the files yourself.</p>
        </div>
        <button
          type="button"
          onclick={() => (showManualConfig = false)}
          class="min-h-8 min-w-8 flex items-center justify-center rounded-lg text-text-muted hover:text-text-main hover:bg-surface-2 transition-colors cursor-pointer shrink-0 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/50"
          title="Close"
          aria-label="Close"
        >
          <X class="w-4 h-4" />
        </button>
      </div>
      {#each manualConfigs as cfg (cfg.filename)}
        <div class="space-y-1.5">
          <div class="flex items-center justify-between gap-2">
            <span class="text-xs font-bold text-text-main font-mono break-all">{cfg.filename}</span>
            <button
              type="button"
              onclick={() => copyToClipboard(cfg.content)}
              class="min-h-7 px-2 rounded flex items-center gap-1 text-xs font-semibold text-brand-600 dark:text-brand-400 hover:bg-surface-2 cursor-pointer shrink-0 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/50"
            >
              <Copy class="w-3.5 h-3.5" />Copy
            </button>
          </div>
          <pre class="p-3 rounded-xl bg-bg border border-border font-mono text-xs text-text-main whitespace-pre-wrap break-all overflow-x-auto select-all">{cfg.content}</pre>
        </div>
      {/each}
    </div>
  </div>
{/if}