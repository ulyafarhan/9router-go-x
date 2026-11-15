<script lang="ts">
  // Port of decolua/9router src/shared/components/Button.js
  import type { Snippet } from 'svelte'

  type Variant = 'primary' | 'secondary' | 'outline' | 'ghost' | 'danger' | 'success'
  type Size = 'sm' | 'md' | 'lg'

  let {
    variant = 'primary',
    size = 'md',
    type = 'button',
    icon = '',
    iconRight = '',
    disabled = false,
    loading = false,
    fullWidth = false,
    title = '',
    class: klass = '',
    onclick,
    children
  }: {
    title?: string
    variant?: Variant
    size?: Size
    type?: 'button' | 'submit' | 'reset'
    icon?: string
    iconRight?: string
    disabled?: boolean
    loading?: boolean
    fullWidth?: boolean
    class?: string
    onclick?: (e: MouseEvent) => void
    children?: Snippet
  } = $props()
  const variants: Record<Variant, string> = {
    primary:
      'bg-brand-500 hover:bg-brand-600 text-white shadow-sm disabled:bg-surface-3 disabled:text-text-muted',
    secondary:
      'bg-surface-2 hover:bg-surface-3 text-text-main border border-border disabled:opacity-50',
    outline:
      'border border-border text-text-main hover:bg-surface-2 hover:border-brand-500/40',
    ghost: 'text-text-muted hover:bg-surface-2 hover:text-text-main',
    danger: 'bg-red-600 hover:bg-red-700 text-white shadow-sm disabled:bg-surface-3 disabled:text-text-muted',
    success: 'bg-emerald-700 hover:bg-emerald-800 text-white shadow-sm disabled:bg-surface-3 disabled:text-text-muted',
  }

  const sizes: Record<Size, string> = {
    sm: 'min-h-8 h-auto px-3 py-1 text-xs rounded-[8px]',
    md: 'min-h-9 h-auto px-4 py-1.5 text-sm rounded-[10px]',
    lg: 'min-h-11 h-auto px-6 py-2 text-sm rounded-[10px]',
  }
</script>

<button
  {type}
  {title}
  class="inline-flex items-center justify-center gap-2 whitespace-nowrap shrink-0 font-semibold transition-all duration-150 ease-out cursor-pointer active:scale-[0.97] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/60 focus-visible:ring-offset-1 disabled:opacity-50 disabled:cursor-not-allowed disabled:active:scale-100 {variants[variant]} {sizes[size]} {fullWidth ? 'w-full' : ''} {klass}"
  disabled={disabled || loading}
  {onclick}
>
  {#if loading}
    <span class="material-symbols-outlined animate-spin text-[18px]">progress_activity</span>
  {:else if icon}
    <span class="material-symbols-outlined text-[18px]">{icon}</span>
  {/if}
  {@render children?.()}
  {#if iconRight && !loading}
    <span class="material-symbols-outlined text-[18px]">{iconRight}</span>
  {/if}
</button>
