<script lang="ts">
  import { Download, Plus, Search, Server } from 'lucide-svelte'
  import type { ProviderConnection, ProviderNode } from '../../api/client'
  import Button from '../../lib/ui/Button.svelte'
  import { PROVIDER_CATALOG, isChatProvider } from '../../lib/providers'
  import ProviderCard from './ProviderCard.svelte'
  import { getProviderStats, matchesFilter, matchesSearch, providerAuthTypes } from './types'

  interface Props {
    connections: ProviderConnection[]
    providerNodes: ProviderNode[]
    onSelectProvider: (id: string) => void
    onToggleAll: (id: string, active: boolean) => void
    /** Opens the unified add-custom-provider dialog; the modal owns the type. */
    onAddCustom: () => void
    /** Opens the import surface. Optional so existing callers keep working. */
    onImport?: () => void
  }

  let {
    connections = [],
    providerNodes = [],
    onSelectProvider,
    onToggleAll,
    onAddCustom,
    onImport,
  }: Props = $props()

  let searchQuery = $state('')
  let statusFilter = $state<'all' | 'connected' | 'error' | 'disabled' | 'not_connected'>('all')
  let showAllApikey = $state(false)

  const APIKEY_INITIAL_VISIBLE = 20

  // 1. Custom Providers
  let customNodes = $derived(
    providerNodes
      .filter((n) => matchesSearch(n.name, searchQuery, n.id) && matchesFilter(getProviderStats(connections, n.id), statusFilter))
      .map((n) => ({ ...n, stats: getProviderStats(connections, n.id) }))
  )

  // 2. OAuth Providers
  let oauthProviders = $derived(
    PROVIDER_CATALOG
      .filter((p) => isChatProvider(p) && !p.hidden && p.category === 'oauth' && matchesSearch(p.name, searchQuery, p.id, p.alias) && matchesFilter(getProviderStats(connections, p.id, providerAuthTypes(p, ['oauth'])), statusFilter))
      .map((p) => ({ ...p, stats: getProviderStats(connections, p.id, providerAuthTypes(p, ['oauth'])) }))
      .sort((a, b) => (b.stats.connected > 0 ? 1 : 0) - (a.stats.connected > 0 ? 1 : 0) || a.name.localeCompare(b.name))
  )

  // 3. Free Tier Providers
  let freeTierProviders = $derived(
    PROVIDER_CATALOG
      .filter((p) => isChatProvider(p) && !p.hidden && (p.category === 'free' || p.category === 'freeTier') && matchesSearch(p.name, searchQuery, p.id, p.alias) && matchesFilter(getProviderStats(connections, p.id), statusFilter, p.noAuth))
      .map((p) => ({ ...p, stats: getProviderStats(connections, p.id) }))
      .sort((a, b) => (b.stats.connected > 0 || b.noAuth ? 1 : 0) - (a.stats.connected > 0 || a.noAuth ? 1 : 0) || a.name.localeCompare(b.name))
  )

  // 4. API Key Providers
  let apikeyProviders = $derived(
    PROVIDER_CATALOG
      .filter((p) => isChatProvider(p) && !p.hidden && (p.category === 'apikey' || p.category === 'webCookie') && matchesSearch(p.name, searchQuery, p.id, p.alias) && matchesFilter(getProviderStats(connections, p.id, providerAuthTypes(p, ['apikey', 'api_key'])), statusFilter))
      .map((p) => ({ ...p, stats: getProviderStats(connections, p.id, providerAuthTypes(p, ['apikey', 'api_key'])) }))
      .sort((a, b) => (b.stats.connected > 0 ? 1 : 0) - (a.stats.connected > 0 ? 1 : 0) || a.name.localeCompare(b.name))
  )

  let visibleApikeyProviders = $derived(
    showAllApikey || searchQuery.trim() !== '' || statusFilter !== 'all'
      ? apikeyProviders
      : apikeyProviders.slice(0, APIKEY_INITIAL_VISIBLE)
  )
</script>

<div class="flex flex-col gap-8 animate-fade-in">
  <!-- Search / Filter Bar -->
  <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
    <div class="relative flex-1 max-w-md">
      <Search class="w-4 h-4 text-text-muted absolute left-3 top-1/2 -translate-y-1/2 pointer-events-none" />
      <input
        type="text"
        bind:value={searchQuery}
        placeholder="Search providers..."
        class="w-full min-h-9 pl-9 pr-4 py-2 text-[16px] sm:text-xs rounded-xl bg-surface border border-border text-text-main placeholder:text-text-muted focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/50 focus:border-brand-500 transition-colors"
      />
    </div>

    <select
      bind:value={statusFilter}
      aria-label="Filter providers by status"
      class="min-h-9 rounded-xl border border-border bg-surface px-3 py-1.5 text-xs text-text-main focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/50 transition-colors hover:border-brand-500/40 cursor-pointer"
    >
      <option value="all">All</option>
      <option value="connected">Connected</option>
      <option value="error">Error</option>
      <option value="disabled">Disabled</option>
      <option value="not_connected">Not Connected</option>
    </select>
  </div>

  <!-- 1. Custom Providers -->
  <div class="flex flex-col gap-4">
    <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
      <h2 class="text-lg sm:text-xl font-semibold leading-tight text-text-main">
        Custom Providers (OpenAI, Anthropic, Embedding)
      </h2>
      <div class="flex flex-wrap gap-2 sm:flex-row sm:w-auto">
        <Button size="sm" onclick={onAddCustom} class="w-full sm:w-auto">
          <Plus class="w-4 h-4" /> Add Custom Provider
        </Button>
        {#if onImport}
          <Button size="sm" variant="secondary" onclick={onImport} class="w-full sm:w-auto">
            <Download class="w-4 h-4" /> Import
          </Button>
        {/if}
      </div>
    </div>

    {#if customNodes.length === 0}
      <div class="flex flex-wrap items-center justify-center gap-2 px-4 py-5 text-center border border-dashed border-border rounded-xl text-text-muted text-xs leading-relaxed">
        <Server class="w-4 h-4 shrink-0" />
        <span>No custom providers — use “Add Custom Provider” to register an OpenAI, Anthropic, or embedding endpoint</span>
      </div>
    {:else}
      <div class="grid grid-cols-1 gap-3 sm:grid-cols-2 sm:gap-4 lg:grid-cols-3 xl:grid-cols-4">
        {#each customNodes as node (node.id)}
          <ProviderCard
            id={node.id}
            name={node.name}
            apiType={node.apiType}
            stats={node.stats}
            onClick={() => onSelectProvider(node.id)}
            onToggleAll={(active) => onToggleAll(node.id, active)}
          />
        {/each}
      </div>
    {/if}
  </div>

  <!-- 2. OAuth Providers -->
  {#if oauthProviders.length > 0}
    <div class="flex flex-col gap-4">
      <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <h2 class="text-lg sm:text-xl font-semibold leading-tight text-text-main">OAuth Providers</h2>
      </div>
      <div class="grid grid-cols-1 gap-3 sm:grid-cols-2 sm:gap-4 lg:grid-cols-3 xl:grid-cols-4">
        {#each oauthProviders as p (p.id)}
          <ProviderCard
            id={p.id}
            name={p.name}
            stats={p.stats}
            onClick={() => onSelectProvider(p.id)}
            onToggleAll={(active) => onToggleAll(p.id, active)}
          />
        {/each}
      </div>
    </div>
  {/if}

  <!-- 3. Free Tier Providers -->
  {#if freeTierProviders.length > 0}
    <div class="flex flex-col gap-4">
      <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <h2 class="text-lg sm:text-xl font-semibold leading-tight text-text-main">Free Tier Providers</h2>
      </div>
      <div class="grid grid-cols-1 gap-3 sm:grid-cols-2 sm:gap-4 lg:grid-cols-3 xl:grid-cols-4">
        {#each freeTierProviders as p (p.id)}
          <ProviderCard
            id={p.id}
            name={p.name}
            stats={p.stats}
            noAuth={p.noAuth}
            onClick={() => onSelectProvider(p.id)}
            onToggleAll={(active) => onToggleAll(p.id, active)}
          />
        {/each}
      </div>
    </div>
  {/if}

  <!-- 4. API Key Providers -->
  {#if apikeyProviders.length > 0}
    <div class="flex flex-col gap-4">
      <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <h2 class="text-lg sm:text-xl font-semibold leading-tight text-text-main">API Key Providers</h2>
      </div>
      <div class="grid grid-cols-1 gap-3 sm:grid-cols-2 sm:gap-4 lg:grid-cols-3 xl:grid-cols-4">
        {#each visibleApikeyProviders as p (p.id)}
          <ProviderCard
            id={p.id}
            name={p.name}
            stats={p.stats}
            onClick={() => onSelectProvider(p.id)}
            onToggleAll={(active) => onToggleAll(p.id, active)}
          />
        {/each}
      </div>
      {#if !showAllApikey && !searchQuery.trim() && statusFilter === 'all' && apikeyProviders.length > visibleApikeyProviders.length}
        <button
          type="button"
          onclick={() => (showAllApikey = true)}
          class="flex w-full min-h-10 items-center justify-center gap-1.5 rounded-xl border border-dashed border-border py-2.5 text-xs font-medium text-text-muted hover:text-brand-500 hover:border-brand-500/40 transition-colors cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/60"
        >
          Show all {apikeyProviders.length} providers
        </button>
      {/if}
    </div>
  {/if}
</div>
