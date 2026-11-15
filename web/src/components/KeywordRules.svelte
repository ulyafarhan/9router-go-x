<script lang="ts">
  import {
    JAILBREAK_KEYWORD_PARTIAL,
    JAILBREAK_KEYWORD_REGEX,
    JAILBREAK_KEYWORD_WORD,
    MAX_JAILBREAK_KEYWORDS,
    normalizeJailbreakKeywords,
  } from '../lib/jailbreakKeywords'
  import type { JailbreakKeyword } from '../api/client'

  interface Props {
    label: string
    hint?: string
    rules: JailbreakKeyword[]
    onchange: (rules: JailbreakKeyword[]) => void
  }

  let { label, hint = '', rules, onchange }: Props = $props()

  let draft = $state('')
  let mode = $state<string>(JAILBREAK_KEYWORD_WORD)
  let error = $state('')

  function modeLabel(m: string | undefined): string {
    if (m === JAILBREAK_KEYWORD_PARTIAL) return 'partial'
    if (m === JAILBREAK_KEYWORD_REGEX) return 'regex'
    return 'word'
  }

  function add() {
    const text = draft.trim()
    if (!text) return
    // Validate the whole list, not just the new row: the gateway rejects the
    // list as a unit, so the dashboard must show the same verdict up front.
    const result = normalizeJailbreakKeywords([...rules, { text, mode }])
    if (!result.ok) {
      error = result.error
      return
    }
    error = ''
    onchange(result.rules)
    draft = ''
  }

  function remove(text: string) {
    onchange(rules.filter((rule) => rule.text !== text))
  }
</script>

<div class="space-y-2">
  <div class="flex items-baseline justify-between gap-2">
    <span class="text-xs text-text-muted">{label}</span>
    <span class="text-[11px] text-text-subtle">{rules.length}/{MAX_JAILBREAK_KEYWORDS}</span>
  </div>
  {#if hint}<p class="text-[11px] text-text-subtle">{hint}</p>{/if}
  {#if rules.length > 0}
    <ul class="flex flex-wrap gap-1.5">
      {#each rules as rule (rule.text + '|' + (rule.mode ?? ''))}
        <li
          class="inline-flex items-center gap-1.5 px-2 py-1 rounded-[8px] bg-surface-2 border border-border text-xs text-text-main font-code"
        >
          <span class="break-all">{rule.text}</span>
          <span class="text-[10px] text-text-subtle">{modeLabel(rule.mode)}</span>
          <button
            type="button"
            onclick={() => remove(rule.text)}
            aria-label={`Remove ${rule.text}`}
            class="min-h-6 min-w-6 inline-flex items-center justify-center rounded text-text-subtle hover:text-error focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/50 leading-none cursor-pointer"
          >
            ×
          </button>
        </li>
      {/each}
    </ul>
  {/if}
  <div class="flex flex-col sm:flex-row gap-2">
    <input
      type="text"
      bind:value={draft}
      placeholder="deepseek"
      onkeydown={(e) => {
        if (e.key === 'Enter') add()
      }}
      class="flex-1 min-h-9 px-3 py-2 rounded-[10px] bg-surface-2 border border-border text-text-main text-[16px] sm:text-sm outline-none focus:border-brand-500 focus-visible:ring-2 focus-visible:ring-brand-500/50"
    />
    <select
      bind:value={mode}
      class="min-h-9 px-3 py-2 rounded-[10px] bg-surface-2 border border-border text-text-main text-[16px] sm:text-sm outline-none focus:border-brand-500 focus-visible:ring-2 focus-visible:ring-brand-500/50"
    >
      <option value={JAILBREAK_KEYWORD_WORD}>word</option>
      <option value={JAILBREAK_KEYWORD_PARTIAL}>partial</option>
      <option value={JAILBREAK_KEYWORD_REGEX}>regex</option>
    </select>
    <button
      type="button"
      onclick={add}
      class="min-h-9 px-3 py-2 rounded-[10px] border border-border text-text-main text-sm font-medium hover:bg-surface-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/50 cursor-pointer"
    >
      Add
    </button>
  </div>
  {#if error}<p class="text-xs text-error">{error}</p>{/if}
</div>
