<script lang="ts">
  import {
    api,
    type PersonaCreatePayload,
    type PersonaInfo,
    type Settings,
  } from '../api/client'
  import Toggle from '../lib/ui/Toggle.svelte'

  interface Props {
    settings?: Settings | Record<string, unknown>
    onRefresh?: () => void
  }
  let { settings, onRefresh }: Props = $props()

  const PERSONA_ID_RE = /^[a-z0-9][a-z0-9_-]{0,47}$/

  let personaEnabled = $state(false)
  let personaActive = $state('')
  let personaAutoDetect = $state(false)
  let personas = $state<PersonaInfo[]>([])
  let loadState = $state<'loading' | 'ready' | 'error'>('loading')
  let loadError = $state('')
  let saveError = $state('')
  let personasFetched = $state(false)

  // Editor state for creating/updating custom persona packs.
  let editorMode = $state<'closed' | 'create' | 'edit'>('closed')
  let draftId = $state('')
  let draftName = $state('')
  let draftDescription = $state('')
  let draftPrefix = $state('')
  let draftSuffix = $state('')
  let editorError = $state('')
  let savingCustom = $state(false)
  let confirmDeleteId = $state('')
  let deletingId = $state('')

  $effect(() => {
    if (!settings) return
    if (typeof settings.personaEnabled === 'boolean') personaEnabled = settings.personaEnabled
    if (typeof settings.personaActive === 'string') personaActive = settings.personaActive
    if (typeof settings.personaAutoDetect === 'boolean') personaAutoDetect = settings.personaAutoDetect
  })

  async function loadPersonas() {
    loadState = 'loading'
    loadError = ''
    try {
      const res = await api.listPersonas()
      personas = res.personas ?? []
      personaEnabled = Boolean(res.enabled)
      personaActive = res.active ?? ''
      personaAutoDetect = Boolean(res.autoDetect)
      loadState = 'ready'
    } catch (error) {
      loadError = error instanceof Error ? error.message : 'Failed to load personas'
      loadState = 'error'
    }
  }

  $effect(() => {
    if (personasFetched) return
    personasFetched = true
    void loadPersonas()
  })

  async function patchSetting(patch: Partial<Settings>, revert: () => void) {
    saveError = ''
    try {
      await api.updateSettings(patch)
      onRefresh?.()
    } catch (error) {
      revert()
      saveError = error instanceof Error ? error.message : 'Failed to save persona settings'
    }
  }

  function activePersona(): PersonaInfo | undefined {
    return personas.find((p) => p.id === personaActive)
  }

  function customPersonaCount(): number {
    return personas.filter((p) => !p.builtIn).length
  }

  async function handleTogglePersona(value: boolean) {
    const prev = personaEnabled
    personaEnabled = value
    await patchSetting({ personaEnabled: value }, () => {
      personaEnabled = prev
    })
  }

  async function handleToggleAutoDetect(value: boolean) {
    const prev = personaAutoDetect
    personaAutoDetect = value
    await patchSetting({ personaAutoDetect: value }, () => {
      personaAutoDetect = prev
    })
  }

  async function handleSelectPersona(id: string) {
    const prev = personaActive
    personaActive = id
    await patchSetting({ personaActive: id }, () => {
      personaActive = prev
    })
  }

  function openCreateEditor() {
    editorMode = 'create'
    draftId = ''
    draftName = ''
    draftDescription = ''
    draftPrefix = ''
    draftSuffix = ''
    editorError = ''
    confirmDeleteId = ''
  }

  function openEditEditor(p: PersonaInfo) {
    editorMode = 'edit'
    draftId = p.id
    draftName = p.name
    draftDescription = p.description ?? ''
    draftPrefix = p.prefix
    draftSuffix = p.suffix ?? ''
    editorError = ''
    confirmDeleteId = ''
  }

  function closeEditor() {
    editorMode = 'closed'
    editorError = ''
  }

  function handleWindowKeydown(e: KeyboardEvent) {
    if (e.key !== 'Escape') return
    if (confirmDeleteId) {
      confirmDeleteId = ''
      return
    }
    if (editorMode !== 'closed') {
      closeEditor()
    }
  }

  async function handleSaveCustom(e: SubmitEvent) {
    e.preventDefault()
    if (savingCustom) return
    const normalizedId = draftId.trim().toLowerCase()
    const trimmedName = draftName.trim()
    const trimmedDesc = draftDescription.trim()
    const trimmedPrefix = draftPrefix.trim()
    const trimmedSuffix = draftSuffix.trim()

    if (editorMode === 'create' && !PERSONA_ID_RE.test(normalizedId)) {
      editorError = 'ID must start with a-z or 0-9 and use only lowercase letters, numbers, hyphens, or underscores (max 48 chars).'
      return
    }
    if (!trimmedName || trimmedName.length > 80) {
      editorError = 'Name is required (1 to 80 characters).'
      return
    }
    if (trimmedDesc.length > 240) {
      editorError = 'Description must be at most 240 characters.'
      return
    }
    if (!trimmedPrefix) {
      editorError = 'Prefix instructions are required.'
      return
    }

    savingCustom = true
    editorError = ''
    saveError = ''
    try {
      let saved: PersonaInfo
      if (editorMode === 'create') {
        const payload: PersonaCreatePayload = {
          id: normalizedId,
          name: trimmedName,
          description: trimmedDesc || undefined,
          prefix: trimmedPrefix,
          suffix: trimmedSuffix || undefined,
        }
        saved = await api.createPersona(payload)
      } else {
        saved = await api.updatePersona(normalizedId, {
          name: trimmedName,
          description: trimmedDesc || undefined,
          prefix: trimmedPrefix,
          suffix: trimmedSuffix || undefined,
        })
      }
      await loadPersonas()
      if (editorMode === 'create' && personaEnabled) {
        await handleSelectPersona(saved.id)
      } else {
        onRefresh?.()
      }
      closeEditor()
    } catch (error) {
      editorError = error instanceof Error ? error.message : 'Failed to save custom persona'
    } finally {
      savingCustom = false
    }
  }

  async function handleDeleteCustom(id: string) {
    if (deletingId) return
    deletingId = id
    saveError = ''
    try {
      await api.deletePersona(id)
      confirmDeleteId = ''
      if (editorMode === 'edit' && draftId === id) {
        closeEditor()
      }
      await loadPersonas()
      onRefresh?.()
    } catch (error) {
      saveError = error instanceof Error ? error.message : 'Failed to delete custom persona'
    } finally {
      deletingId = ''
    }
  }
</script>

<svelte:window onkeydown={handleWindowKeydown} />

<div class="max-w-5xl space-y-8">
  {#if saveError}
    <div
      role="alert"
      class="px-4 py-3 rounded-lg border border-error/40 bg-error/10 text-sm text-error"
    >
      {saveError}
    </div>
  {/if}

  <section class="p-6 sm:p-7 rounded-xl bg-surface-2/40 border border-border space-y-6" aria-labelledby="persona-heading">
    <div class="flex items-start justify-between gap-6 flex-wrap">
      <div class="min-w-0 flex-1 space-y-2">
        <div class="flex items-center gap-3 flex-wrap">
          <h2 id="persona-heading" class="text-base font-semibold">Persona routing</h2>
          <span
            class="text-xs px-2.5 py-1 rounded font-medium {personaEnabled
              ? 'bg-success/15 text-success'
              : 'bg-surface-3 text-text-muted'}"
          >
            {personaEnabled ? personaActive || 'No selection' : 'Disabled'}
          </span>
        </div>
        <p class="text-sm text-text-muted leading-relaxed max-w-2xl">
          Attach a built-in or custom task rule pack at the gateway so every upstream model receives the same
          working conventions without repeating them per prompt.
        </p>
      </div>
      <div class="flex items-center shrink-0 pt-1">
        <Toggle
          checked={personaEnabled}
          label="Enable persona routing"
          onChange={() => handleTogglePersona(!personaEnabled)}
        />
      </div>
    </div>

    <div class="pt-5 border-t border-border flex items-start justify-between gap-6 flex-wrap">
      <div class="min-w-0 flex-1 space-y-1.5">
        <div class="flex items-center gap-2.5 flex-wrap">
          <h3 id="persona-autodetect-heading" class="text-sm font-medium">Auto-detect coding on tool requests (opt-in)</h3>
          <span
            class="text-xs px-2 py-0.5 rounded font-medium {personaAutoDetect
              ? 'bg-primary/15 text-primary'
              : 'bg-surface-3 text-text-muted'}"
          >
            {personaAutoDetect ? 'On' : 'Off'}
          </span>
        </div>
        <p class="text-sm text-text-muted leading-relaxed max-w-2xl">
          Off by default. When enabled, requests that declare <code class="font-mono text-xs bg-surface-3 px-1.5 py-0.5 rounded">tools</code>
          adopt the <code class="font-mono text-xs bg-surface-3 px-1.5 py-0.5 rounded">coding</code> persona only if neither
          a request header nor an active dashboard selection is present.
        </p>
      </div>
      <div class="flex items-center shrink-0 pt-1">
        <Toggle
          checked={personaAutoDetect}
          label="Auto-detect coding persona on tool requests"
          onChange={() => handleToggleAutoDetect(!personaAutoDetect)}
        />
      </div>
    </div>
  </section>

  <section class="p-6 sm:p-7 rounded-xl bg-surface-2/40 border border-border space-y-5" aria-labelledby="persona-pick-heading">
    <div class="flex items-start justify-between gap-4 flex-wrap">
      <div class="space-y-1">
        <h2 id="persona-pick-heading" class="text-sm font-semibold">
          {personaEnabled ? 'Active persona' : 'Persona library'}
        </h2>
        <p class="text-sm text-text-muted leading-relaxed">
          {#if personaEnabled}
            Applied across all model lanes unless overridden by a request header or Token Saver bypass.
          {:else}
            Create or edit custom packs now, then enable Persona routing above to activate one globally.
          {/if}
        </p>
      </div>
      <button
        type="button"
        onclick={openCreateEditor}
        disabled={customPersonaCount() >= 64}
        class="px-4 py-2.5 min-h-[44px] rounded-lg border border-primary/40 bg-primary/10 text-primary text-xs font-medium hover:bg-primary/15 disabled:opacity-50 disabled:cursor-not-allowed focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary cursor-pointer"
      >
        + New custom persona
      </button>
    </div>

    {#if editorMode !== 'closed'}
      <form
        onsubmit={handleSaveCustom}
        class="p-5 rounded-xl bg-surface-1 border border-primary/30 space-y-4"
        aria-labelledby="persona-editor-heading"
      >
        <div class="flex items-center justify-between gap-4">
          <h3 id="persona-editor-heading" class="text-sm font-semibold">
            {editorMode === 'create' ? 'Create custom persona' : `Edit custom persona: ${draftId}`}
          </h3>
          <button
            type="button"
            onclick={closeEditor}
            class="px-3 py-2 min-h-[44px] rounded-lg border border-border text-xs text-text-muted hover:text-text-main hover:border-primary/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary cursor-pointer"
          >
            Cancel
          </button>
        </div>

        {#if editorError}
          <div role="alert" class="px-3 py-2.5 rounded-lg border border-error/40 bg-error/10 text-xs text-error">
            {editorError}
          </div>
        {/if}

        <div class="grid gap-4 sm:grid-cols-2">
          <div class="space-y-1.5">
            <label for="custom-persona-id" class="block text-xs font-medium text-text-muted">
              Persona ID (header slug)
            </label>
            <input
              id="custom-persona-id"
              type="text"
              bind:value={draftId}
              disabled={editorMode === 'edit'}
              placeholder="sql-auditor"
              autocomplete="off"
              class="w-full min-h-[44px] px-3 py-2 rounded-lg bg-surface-2 border border-border text-[16px] sm:text-sm font-mono focus:outline-none focus-visible:ring-2 focus-visible:ring-primary disabled:opacity-60"
            />
          </div>
          <div class="space-y-1.5">
            <label for="custom-persona-name" class="block text-xs font-medium text-text-muted">
              Display name
            </label>
            <input
              id="custom-persona-name"
              type="text"
              bind:value={draftName}
              placeholder="SQL Auditor"
              maxlength="80"
              class="w-full min-h-[44px] px-3 py-2 rounded-lg bg-surface-2 border border-border text-[16px] sm:text-sm focus:outline-none focus-visible:ring-2 focus-visible:ring-primary"
            />
          </div>
        </div>

        <div class="space-y-1.5">
          <label for="custom-persona-description" class="block text-xs font-medium text-text-muted">
            Description (optional)
          </label>
          <input
            id="custom-persona-description"
            type="text"
            bind:value={draftDescription}
            placeholder="Checks SQLite query plans and enforces index coverage."
            maxlength="240"
            class="w-full min-h-[44px] px-3 py-2 rounded-lg bg-surface-2 border border-border text-[16px] sm:text-sm focus:outline-none focus-visible:ring-2 focus-visible:ring-primary"
          />
        </div>

        <div class="space-y-1.5">
          <label for="custom-persona-prefix" class="block text-xs font-medium text-text-muted">
            Prefix instructions (placed before user system prompt)
          </label>
          <textarea
            id="custom-persona-prefix"
            bind:value={draftPrefix}
            rows="5"
            placeholder="Enter the primary instruction envelope for this task..."
            class="w-full px-3 py-2.5 rounded-lg bg-surface-2 border border-border text-[16px] sm:text-xs font-mono leading-relaxed resize-y focus:outline-none focus-visible:ring-2 focus-visible:ring-primary"
          ></textarea>
        </div>

        <div class="space-y-1.5">
          <label for="custom-persona-suffix" class="block text-xs font-medium text-text-muted">
            Suffix instructions (optional, reinforced at end of system prompt)
          </label>
          <textarea
            id="custom-persona-suffix"
            bind:value={draftSuffix}
            rows="2"
            placeholder="Optional closing reminder..."
            class="w-full px-3 py-2.5 rounded-lg bg-surface-2 border border-border text-[16px] sm:text-xs font-mono leading-relaxed resize-y focus:outline-none focus-visible:ring-2 focus-visible:ring-primary"
          ></textarea>
        </div>

        <div class="flex items-center justify-end gap-3 pt-2">
          <button
            type="button"
            onclick={closeEditor}
            class="px-4 py-2.5 min-h-[44px] rounded-lg border border-border text-xs font-medium text-text-muted hover:text-text-main focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary cursor-pointer"
          >
            Cancel
          </button>
          <button
            type="submit"
            disabled={savingCustom}
            class="px-5 py-2.5 min-h-[44px] rounded-lg bg-primary text-surface-1 text-xs font-semibold hover:opacity-90 disabled:opacity-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary cursor-pointer"
          >
            {savingCustom ? 'Saving...' : editorMode === 'create' ? 'Save persona' : 'Update persona'}
          </button>
        </div>
      </form>
    {/if}

    {#if loadState === 'loading'}
      <p class="text-sm text-text-muted" role="status">Loading persona packs...</p>
    {:else if loadState === 'error'}
      <div role="alert" class="flex items-center justify-between gap-4 flex-wrap p-4 rounded-lg border border-error/40 bg-error/10">
        <p class="text-sm text-error">{loadError}</p>
        <button
          type="button"
          onclick={() => loadPersonas()}
          class="px-3 py-2 min-h-[44px] rounded-lg border border-border bg-surface-1 text-xs font-medium hover:border-primary/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary cursor-pointer"
        >
          Retry
        </button>
      </div>
    {:else if personas.length === 0}
      <p class="text-sm text-text-muted">No persona packs available yet.</p>
    {:else}
      <div class="grid gap-3 grid-cols-1 sm:grid-cols-2 lg:grid-cols-3">
        {#each personas as p (p.id)}
          <div
            class="flex flex-col justify-between rounded-lg border transition-colors
              {personaActive === p.id
                ? 'border-primary bg-primary/10'
                : 'border-border bg-surface-1 hover:border-primary/50'}"
          >
            <button
              type="button"
              aria-pressed={personaActive === p.id}
              onclick={() => handleSelectPersona(p.id)}
              class="w-full text-left px-4 py-4 min-h-[44px] rounded-lg cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
            >
              <div class="flex items-start justify-between gap-2">
                <p class="text-sm font-medium text-text-main">{p.name}</p>
                <span
                  class="shrink-0 text-[10px] px-1.5 py-0.5 rounded font-medium
                    {p.builtIn === false
                      ? 'bg-primary/15 text-primary'
                      : 'bg-surface-3 text-text-muted'}"
                >
                  {p.builtIn === false ? 'Custom' : 'Built-in'}
                </span>
              </div>
              {#if p.description}
                <p class="text-xs text-text-muted leading-relaxed mt-1.5">{p.description}</p>
              {/if}
              <p class="text-[11px] font-mono text-text-muted mt-3">{p.id}</p>
            </button>

            {#if p.builtIn === false}
              <div class="px-3 py-2 border-t border-border/60 flex items-center justify-end gap-2">
                {#if confirmDeleteId === p.id}
                  <button
                    type="button"
                    disabled={deletingId === p.id}
                    onclick={() => handleDeleteCustom(p.id)}
                    class="px-3 py-2 min-h-[44px] rounded-md bg-error/15 text-error text-xs font-medium hover:bg-error/25 disabled:opacity-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-error cursor-pointer"
                  >
                    {deletingId === p.id ? 'Deleting...' : 'Confirm delete'}
                  </button>
                  <button
                    type="button"
                    onclick={() => (confirmDeleteId = '')}
                    class="px-3 py-2 min-h-[44px] rounded-md text-xs text-text-muted hover:text-text-main focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary cursor-pointer"
                  >
                    Cancel
                  </button>
                {:else}
                  <button
                    type="button"
                    onclick={() => openEditEditor(p)}
                    class="px-3 py-2 min-h-[44px] rounded-md text-xs font-medium text-text-muted hover:text-text-main focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary cursor-pointer"
                  >
                    Edit
                  </button>
                  <button
                    type="button"
                    onclick={() => (confirmDeleteId = p.id)}
                    class="px-3 py-2 min-h-[44px] rounded-md text-xs font-medium text-error/80 hover:text-error focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-error cursor-pointer"
                  >
                    Delete
                  </button>
                {/if}
              </div>
            {/if}
          </div>
        {/each}
      </div>
    {/if}
  </section>

  {#if activePersona()}
    {@const ap = activePersona()!}
    <section class="p-6 sm:p-7 rounded-xl bg-surface-2/40 border border-border" aria-labelledby="persona-preview-heading">
      <h2 id="persona-preview-heading" class="text-sm font-semibold mb-1">Injected instructions</h2>
      <p class="text-sm text-text-muted leading-relaxed mb-5">
        Read-only preview of <span class="font-medium text-text-main">{ap.name}</span>.
      </p>
      <div class="space-y-6">
        <div class="space-y-2">
          <label class="text-xs font-medium text-text-muted" for="persona-prefix-preview">
            Prefix (placed before your system prompt)
          </label>
          <textarea
            id="persona-prefix-preview"
            value={ap.prefix}
            rows="8"
            readonly
            class="w-full px-4 py-3 rounded-lg bg-surface-1 border border-border text-xs font-mono leading-relaxed resize-y text-text-muted focus:outline-none focus-visible:ring-2 focus-visible:ring-primary"
          ></textarea>
        </div>
        {#if ap.suffix}
          <div class="space-y-2">
            <label class="text-xs font-medium text-text-muted" for="persona-suffix-preview">
              Suffix (placed after your system prompt)
            </label>
            <textarea
              id="persona-suffix-preview"
              value={ap.suffix}
              rows="3"
              readonly
              class="w-full px-4 py-3 rounded-lg bg-surface-1 border border-border text-xs font-mono leading-relaxed resize-y text-text-muted focus:outline-none focus-visible:ring-2 focus-visible:ring-primary"
            ></textarea>
          </div>
        {/if}
      </div>
    </section>
  {:else if personaEnabled && loadState === 'ready'}
    <section class="p-6 sm:p-7 rounded-xl border border-dashed border-border" role="status">
      <p class="text-sm text-text-muted leading-relaxed">
        No persona selected yet. Pick one above; until then requests keep only the client system prompt unless auto-detect applies.
      </p>
    </section>
  {/if}

  <section class="p-6 sm:p-7 rounded-xl bg-surface-2/20 border border-border" aria-labelledby="persona-order-heading">
    <h2 id="persona-order-heading" class="text-sm font-semibold mb-4">Resolution order per request</h2>
    <ol class="space-y-3 text-sm text-text-muted leading-relaxed">
      <li class="flex gap-3">
        <span class="shrink-0 w-6 h-6 rounded-full bg-surface-3 text-text-main text-xs font-medium flex items-center justify-center" aria-hidden="true">1</span>
        <span>
          Request header <code class="font-mono text-xs bg-surface-3 px-1.5 py-0.5 rounded">X-Persona</code> or
          <code class="font-mono text-xs bg-surface-3 px-1.5 py-0.5 rounded">X-9Router-Persona</code> wins when valid.
          Unknown IDs leave the request unmodified instead of falling through to the dashboard selection.
        </span>
      </li>
      <li class="flex gap-3">
        <span class="shrink-0 w-6 h-6 rounded-full bg-surface-3 text-text-main text-xs font-medium flex items-center justify-center" aria-hidden="true">2</span>
        <span>The active persona selected on this page when persona routing is enabled.</span>
      </li>
      <li class="flex gap-3">
        <span class="shrink-0 w-6 h-6 rounded-full bg-surface-3 text-text-main text-xs font-medium flex items-center justify-center" aria-hidden="true">3</span>
        <span>Opt-in auto-detect: applies <code class="font-mono text-xs bg-surface-3 px-1.5 py-0.5 rounded">coding</code> on tool-bearing requests when enabled above.</span>
      </li>
      <li class="flex gap-3">
        <span class="shrink-0 w-6 h-6 rounded-full bg-surface-3 text-text-main text-xs font-medium flex items-center justify-center" aria-hidden="true">4</span>
        <span>Passthrough: the request goes out with the client system prompt unmodified.</span>
      </li>
    </ol>
    <p class="text-xs text-text-muted leading-relaxed mt-6 pt-4 border-t border-border">
      Task personas compose cleanly with Instruction Presets inside the Prompt Envelope, never weaken upstream safety
      thresholds, and are skipped when <code class="font-mono text-[11px] bg-surface-3 px-1.5 py-0.5 rounded">X-9Router-Token-Saver: off</code> is sent.
    </p>
  </section>
</div>
