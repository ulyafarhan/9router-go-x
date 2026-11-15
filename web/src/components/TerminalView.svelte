<script lang="ts">
  // Console Log page. Owns the authenticated SSE transport, the server log
  // level, and the buffer window; ConsoleLogStream owns how rows are read.
  import { RefreshCw, Trash2 } from 'lucide-svelte'
  import ConsoleLogStream from './ConsoleLogStream.svelte'
  import Button from '../lib/ui/Button.svelte'
  import Card from '../lib/ui/Card.svelte'
  import { getAuthHeaders, responseErrorMessage } from '../api/client'
  import { notifications } from '../lib/notifications'
  import { toConsoleEntry, type ConsoleEntry } from '../lib/consoleLog'

  const MAX_LINES = 200
  const LOG_LEVELS = ['debug', 'info', 'warn', 'error'] as const

  type StreamStatus = 'connecting' | 'live' | 'reconnecting' | 'error'

  let entries = $state<ConsoleEntry[]>([])
  let logLevel = $state<string>('info')
  let levelBusy = $state(false)
  let streamStatus = $state<StreamStatus>('connecting')
  let streamError = $state('')
  let reconnectNonce = $state(0)
  let seq = 0

  // A reconnect replays the whole buffer as "init", so the old rows have to go
  // or the same line would appear twice.
  function adoptBuffer(raw: unknown[]): void {
    seq = 0
    entries = raw.map((item) => toConsoleEntry(item as Record<string, unknown>, seq++)).slice(-MAX_LINES)
  }

  async function fetchInitialLogs() {
    try {
      const res = await fetch('/api/translator/console-logs', {
        headers: getAuthHeaders(),
        credentials: 'same-origin',
      })
      if (!res.ok) {
        streamError = await responseErrorMessage(res, `HTTP ${res.status}`)
        return
      }
      const data = await res.json()
      if (Array.isArray(data?.logs)) {
        adoptBuffer(data.logs)
        streamError = ''
      }
    } catch (err) {
      streamError = err instanceof Error ? err.message : 'Failed to load console logs'
    }
  }

  async function fetchLogLevel() {
    try {
      const res = await fetch('/api/translator/console-logs/level', {
        headers: getAuthHeaders(),
        credentials: 'same-origin',
      })
      if (!res.ok) return
      const data = await res.json()
      if (typeof data.level === 'string') logLevel = data.level
    } catch {
      // keep default; the selector still writes a level
    }
  }

  async function handleLevelChange(e: Event) {
    const next = (e.target as HTMLSelectElement).value
    const prev = logLevel
    logLevel = next
    levelBusy = true
    try {
      const res = await fetch('/api/translator/console-logs/level', {
        method: 'PUT',
        headers: getAuthHeaders(),
        credentials: 'same-origin',
        body: JSON.stringify({ level: next }),
      })
      if (!res.ok) throw new Error(await responseErrorMessage(res, `HTTP ${res.status}`))
      const data = await res.json()
      if (!data?.success) throw new Error('Log level update was rejected')
      logLevel = data.level || next
      notifications.success(`Log level set to ${logLevel} (no restart needed)`)
    } catch (err) {
      logLevel = prev
      notifications.error(`Failed to set log level: ${err instanceof Error ? err.message : err}`)
    } finally {
      levelBusy = false
    }
  }

  async function handleClear() {
    try {
      const res = await fetch('/api/translator/console-logs', {
        method: 'DELETE',
        headers: getAuthHeaders(),
        credentials: 'same-origin',
      })
      if (!res.ok) throw new Error(await responseErrorMessage(res, `HTTP ${res.status}`))
      entries = []
      // The view also clears via the SSE "clear" event, so the server stays the
      // single owner of the buffer.
    } catch (err) {
      notifications.error(`Failed to clear console logs: ${err instanceof Error ? err.message : err}`)
    }
  }

  function retryStream() {
    streamError = ''
    streamStatus = 'connecting'
    reconnectNonce += 1
    void fetchInitialLogs()
  }

  fetchLogLevel()
  void fetchInitialLogs()

  $effect(() => {
    void reconnectNonce
    let cancelled = false
    let controller: AbortController | null = null

    // Read through fetch so the stream rides the same authenticated transport
    // and error handling as the rest of the dashboard.
    const connect = async () => {
      if (!cancelled && streamStatus !== 'live') {
        streamStatus = streamError ? 'reconnecting' : 'connecting'
      }
      try {
        controller = new AbortController()
        const res = await fetch('/api/translator/console-logs/stream', {
          headers: getAuthHeaders(),
          credentials: 'same-origin',
          signal: controller.signal,
        })
        if (!res.ok) throw new Error(await responseErrorMessage(res, `HTTP ${res.status}`))

        const reader = res.body?.getReader()
        if (!reader) throw new Error('Streaming response body is unavailable')

        streamStatus = 'live'
        streamError = ''

        const decoder = new TextDecoder()
        let buffer = ''
        while (!cancelled) {
          const { done, value } = await reader.read()
          if (done) break

          buffer += decoder.decode(value, { stream: true })
          const frames = buffer.split('\n')
          buffer = frames.pop() || ''

          for (const frame of frames) {
            const trimmed = frame.trim()
            if (!trimmed.startsWith('data: ')) continue

            let msg: Record<string, unknown>
            try {
              msg = JSON.parse(trimmed.slice(6))
            } catch {
              continue
            }

            if (msg.type === 'init') {
              adoptBuffer(Array.isArray(msg.entries) ? msg.entries : [])
            } else if (msg.type === 'line') {
              entries = [...entries, toConsoleEntry(msg.entry as Record<string, unknown>, seq++)].slice(
                -MAX_LINES
              )
            } else if (msg.type === 'clear') {
              entries = []
            }
          }
        }
        if (!cancelled) streamStatus = 'reconnecting'
      } catch (err) {
        if (!cancelled) {
          streamStatus = 'error'
          streamError = err instanceof Error ? err.message : 'Console stream disconnected'
        }
      }

      if (!cancelled) setTimeout(connect, 5000)
    }

    connect()

    return () => {
      cancelled = true
      controller?.abort()
    }
  })
</script>

<Card padding="none" class="flex flex-col overflow-hidden rounded-[14px]">
  <div class="flex flex-wrap items-center gap-3 border-b border-border-subtle px-5 py-3.5">
    <div class="flex items-center gap-2">
      <label class="text-xs font-medium text-text-muted" for="console-log-level">Level</label>
      <select
        id="console-log-level"
        class="h-9 sm:h-8 rounded-[8px] border border-border bg-surface px-2.5 text-[16px] sm:text-xs text-text-main focus:outline-none focus:border-brand-500 focus-visible:ring-2 focus-visible:ring-brand-500/50 disabled:opacity-50"
        bind:value={logLevel}
        disabled={levelBusy}
        onchange={handleLevelChange}
      >
        {#each LOG_LEVELS as level (level)}
          <option value={level}>{level}</option>
        {/each}
      </select>
    </div>

    <span class="text-xs text-text-muted">
      Applies immediately: lines below this level are not buffered.
    </span>

    <div class="ml-auto flex flex-wrap items-center gap-2">
      <span
        class="inline-flex h-7 items-center gap-1.5 rounded-full border px-2.5 text-[11px] font-medium {streamStatus === 'live'
          ? 'border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-400'
          : streamStatus === 'error'
            ? 'border-red-500/30 bg-red-500/10 text-red-700 dark:text-red-400'
            : 'border-border bg-surface-2 text-text-muted'}"
      >
        <span
          class="h-1.5 w-1.5 rounded-full {streamStatus === 'live'
            ? 'bg-emerald-500'
            : streamStatus === 'error'
              ? 'bg-red-500'
              : 'animate-pulse bg-amber-500'}"
        ></span>
        {streamStatus === 'live'
          ? 'Live'
          : streamStatus === 'error'
            ? 'Disconnected'
            : streamStatus === 'reconnecting'
              ? 'Reconnecting…'
              : 'Connecting…'}
      </span>

      <Button size="sm" variant="outline" onclick={retryStream} title="Reconnect and reload logs">
        <RefreshCw class="h-3.5 w-3.5" />
        Refresh
      </Button>

      <Button size="sm" variant="outline" onclick={handleClear}>
        <Trash2 class="h-3.5 w-3.5" />
        Clear
      </Button>
    </div>
  </div>

  {#if streamError}
    <div
      role="alert"
      class="flex flex-wrap items-center justify-between gap-2 border-b border-red-500/20 bg-red-500/5 px-5 py-2.5 text-xs text-red-700 dark:text-red-300"
    >
      <span>Console log stream error: {streamError}</span>
      <button
        type="button"
        onclick={retryStream}
        class="min-h-7 cursor-pointer rounded-md border border-red-500/30 px-2.5 py-1 text-[11px] font-medium hover:bg-red-500/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red-500/50"
      >
        Retry
      </button>
    </div>
  {/if}

  <ConsoleLogStream {entries} />
</Card>