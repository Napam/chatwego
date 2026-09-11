import hljs from 'highlight.js/lib/common'
import hljsDarkCss from 'highlight.js/styles/github-dark.css' with { type: 'text' }
import hljsLightCss from 'highlight.js/styles/github.css' with { type: 'text' }
import { html, type PropertyValues } from 'lit'
import { customElement, property, state } from 'lit/decorators.js'
import { unsafeHTML } from 'lit/directives/unsafe-html.js'
import MarkdownIt from 'markdown-it'
import taskLists from 'markdown-it-task-lists'
import { twJoin } from 'tailwind-merge'
import { onFadeAnimationEnd, reconcileMarkdown } from './markdown-reconcile'
import { StyledLitElement } from './styled-lit-element'

const md = new MarkdownIt({
  html: false,
  linkify: true,
  breaks: true,
  highlight(code, lang) {
    const language = lang && hljs.getLanguage(lang) ? lang : undefined
    const { value } = language ? hljs.highlight(code, { language }) : hljs.highlightAuto(code)
    // Not Tailwind classes -- "hljs"/"language-x" are highlight.js's own markers.
    // eslint-disable-next-line better-tailwindcss/no-concatenated-classes, better-tailwindcss/no-unknown-classes
    const codeClass = language ? `hljs language-${language}` : 'hljs'
    return `<pre><code class="${codeClass}">${value}</code></pre>`
  },
}).use(taskLists)

// Tailwind's preflight reset (adopted into this component's shadow root
// along with everything else) strips default heading/code/list styling, so
// without `prose` the parsed markdown looks like plain unstyled paragraphs.
// max-w-none overrides prose's own 65ch cap -- we want it to fill the bubble.
// prose-pre: zeroed out since our own <pre> (built by `highlight` above,
// styled by the adopted hljs theme sheet) already supplies its own box.
const proseClass = twJoin('prose max-w-none dark:prose-invert', 'prose-pre:bg-transparent prose-pre:p-0')

// Reasoning renders through the `muted` variant (see ReasoningBlock in
// root.templ): the same markdown pipeline, but smaller and in subdued grays
// so it reads as a secondary detail. The typography plugin drives the prose
// palette through --tw-prose-* custom properties, emitted under :where()/
// @layer, so this unlayered sheet just re-points those variables at the
// app's gray/neutral scales, scoped to muted hosts -- non-muted messages
// never match it. Code blocks keep their own colors: the hljs sheets and
// their token rules win on the code element itself, and --tw-prose-pre-bg
// stays transparent via the existing prose-pre:bg-transparent utility.
const MUTED_PROSE_CSS = `
:host([muted]) .prose {
  --tw-prose-body: var(--color-gray-400);
  --tw-prose-headings: var(--color-gray-400);
  --tw-prose-lead: var(--color-gray-400);
  --tw-prose-links: var(--color-gray-400);
  --tw-prose-bold: var(--color-gray-400);
  --tw-prose-counters: var(--color-gray-400);
  --tw-prose-bullets: var(--color-gray-300);
  --tw-prose-hr: var(--color-gray-300);
  --tw-prose-quotes: var(--color-gray-400);
  --tw-prose-quote-borders: var(--color-gray-300);
  --tw-prose-captions: var(--color-gray-400);
  --tw-prose-kbd: var(--color-gray-400);
  --tw-prose-kbd-shadows: color-mix(in oklab, var(--color-gray-400) 10%, transparent);
  --tw-prose-code: var(--color-gray-400);
  --tw-prose-pre-code: var(--color-gray-400);
  --tw-prose-pre-bg: transparent;
  --tw-prose-th-borders: var(--color-gray-300);
  --tw-prose-td-borders: var(--color-gray-300);
}
:host([muted]) .dark .prose {
  --tw-prose-body: var(--color-neutral-500);
  --tw-prose-headings: var(--color-neutral-500);
  --tw-prose-lead: var(--color-neutral-500);
  --tw-prose-links: var(--color-neutral-500);
  --tw-prose-bold: var(--color-neutral-500);
  --tw-prose-counters: var(--color-neutral-500);
  --tw-prose-bullets: var(--color-neutral-600);
  --tw-prose-hr: var(--color-neutral-600);
  --tw-prose-quotes: var(--color-neutral-500);
  --tw-prose-quote-borders: var(--color-neutral-600);
  --tw-prose-captions: var(--color-neutral-500);
  --tw-prose-kbd: var(--color-neutral-500);
  --tw-prose-kbd-shadows: color-mix(in oklab, var(--color-neutral-500) 10%, transparent);
  --tw-prose-code: var(--color-neutral-500);
  --tw-prose-pre-code: var(--color-neutral-500);
  --tw-prose-pre-bg: transparent;
  --tw-prose-th-borders: var(--color-neutral-700);
  --tw-prose-td-borders: var(--color-neutral-600);
}
`

// Streamed-in content is flagged with .md-fade by the reconciler (see
// markdown-reconcile.ts): spans wrapping appended text, or the class on
// newly appeared elements. The reconciler preserves those nodes across
// frames, so the animation always runs to completion; animationend then
// unwraps/strips them (see onFadeAnimationEnd).
const FADE_CHUNK_CSS = `
.md-fade {
  animation: md-fade-in 0.2s ease-out;
}
@keyframes md-fade-in {
  from { opacity: 0; }
  to { opacity: 1; }
}
`

let hljsLightSheet: CSSStyleSheet | undefined
let hljsDarkSheet: CSSStyleSheet | undefined
let hljsLightOverrideSheet: CSSStyleSheet | undefined
let hljsDarkOverrideSheet: CSSStyleSheet | undefined

function getHljsLightSheet(): CSSStyleSheet {
  if (!hljsLightSheet) {
    hljsLightSheet = new CSSStyleSheet()
    hljsLightSheet.replaceSync(hljsLightCss)
  }
  return hljsLightSheet
}

function getHljsDarkSheet(): CSSStyleSheet {
  if (!hljsDarkSheet) {
    hljsDarkSheet = new CSSStyleSheet()
    hljsDarkSheet.replaceSync(hljsDarkCss)
  }
  return hljsDarkSheet
}

// Both hljs themes ship colors that don't quite fit this app: github.css's
// background is pure white (invisible against the page), and github-dark's
// near-black background reads flatter than this app's own dark surfaces.
// These overrides are adopted after their theme sheet so they win the
// cascade -- add more rules here (e.g. ".hljs-keyword { color: ... }") to
// tune anything else about either theme's palette.
const HLJS_LIGHT_OVERRIDE_CSS = `.hljs { background-color: var(--color-gray-50); }`
const HLJS_DARK_OVERRIDE_CSS = `.hljs { background-color: var(--color-neutral-850); }`

function getHljsLightOverrideSheet(): CSSStyleSheet {
  if (!hljsLightOverrideSheet) {
    hljsLightOverrideSheet = new CSSStyleSheet()
    hljsLightOverrideSheet.replaceSync(HLJS_LIGHT_OVERRIDE_CSS)
  }
  return hljsLightOverrideSheet
}

function getHljsDarkOverrideSheet(): CSSStyleSheet {
  if (!hljsDarkOverrideSheet) {
    hljsDarkOverrideSheet = new CSSStyleSheet()
    hljsDarkOverrideSheet.replaceSync(HLJS_DARK_OVERRIDE_CSS)
  }
  return hljsDarkOverrideSheet
}

let mutedProseSheet: CSSStyleSheet | undefined

function getMutedProseSheet(): CSSStyleSheet {
  if (!mutedProseSheet) {
    mutedProseSheet = new CSSStyleSheet()
    mutedProseSheet.replaceSync(MUTED_PROSE_CSS)
  }
  return mutedProseSheet
}

let fadeChunkSheet: CSSStyleSheet | undefined

function getFadeChunkSheet(): CSSStyleSheet {
  if (!fadeChunkSheet) {
    fadeChunkSheet = new CSSStyleSheet()
    fadeChunkSheet.replaceSync(FADE_CHUNK_CSS)
  }
  return fadeChunkSheet
}

/**
 * Renders markdown as sanitized-by-construction HTML, with highlight.js
 * syntax highlighting for fenced code blocks.
 *
 * Two modes, both driven by a MutationObserver rather than a one-shot read
 * at some lifecycle hook: the bundle loads synchronously in <head>, so this
 * element upgrades (and connectedCallback fires) as soon as the parser sees
 * the opening tag -- before it has necessarily inserted the raw text between
 * the tags. Observing sidesteps needing to know exactly when that happens.
 *
 * - Static: pass the full markdown as light-dom text content, e.g.
 *   `<markdown-message>${message}</markdown-message>`. Shadow DOM (no slot)
 *   keeps that raw text out of the rendered output; we only ever read it.
 * - Streaming: set `source` to the id of a sibling element that the server
 *   appends raw text deltas into (Datastar element patches). That sibling is never
 *   touched by this component's render, so it stays a clean accumulator of
 *   the raw markdown source. Markdown renders live on every frame; the
 *   reconciler (markdown-reconcile.ts) diffs it into the .prose div so
 *   appended text fades in via .md-fade while unchanged nodes persist.
 */
@customElement('markdown-message')
export class MarkdownMessage extends StyledLitElement {
  @property({ type: String }) source = ''

  // Muted rendering (smaller, subdued prose palette) for secondary content
  // like reasoning traces. Set it as a bare attribute: <markdown-message muted>.
  @property({ type: Boolean, reflect: true }) muted = false

  // Set (by root.ts when the Datastar stream finishes) when the stream is
  // done: forces one final synchronous render so the last delta can't be
  // left behind in the rAF-batched pipeline (see updated()).
  @property({ type: Boolean, reflect: true }) settled = false

  @state() private rendered = ''

  private renderScheduled = false

  // Streaming state: raw is the full accumulated source; pendingHtml is
  // rendered markdown waiting for the .prose div to exist (first lit render
  // hasn't happened yet when the first delta arrives).
  private raw = ''
  private pendingHtml: string | null = null
  private target?: HTMLElement
  private fadeListenerAttached = false

  private observer?: MutationObserver

  // hljs themes are plain CSS files with no built-in dark-mode variant, so
  // (unlike the shared Tailwind sheet) we swap the whole adopted sheet on
  // theme change rather than relying on a `dark:` selector. The muted prose
  // and fade-chunk sheets are managed here too: they have their own
  // :host([muted])/animation scoping, so they just need to survive the swap.
  private syncHljsTheme = () => {
    const root = this.shadowRoot
    if (!root) return
    const dark = document.documentElement.classList.contains('dark')
    const knownHljsSheets = [hljsLightSheet, hljsDarkSheet, hljsLightOverrideSheet, hljsDarkOverrideSheet]
    const extraSheets = [getMutedProseSheet(), getFadeChunkSheet()]
    const withoutHljsSheets = root.adoptedStyleSheets.filter(
      (sheet) => !knownHljsSheets.includes(sheet) && !extraSheets.includes(sheet)
    )
    const themeSheets = dark
      ? [getHljsDarkSheet(), getHljsDarkOverrideSheet()]
      : [getHljsLightSheet(), getHljsLightOverrideSheet()]
    root.adoptedStyleSheets = [...withoutHljsSheets, ...themeSheets, ...extraSheets]
  }

  connectedCallback() {
    super.connectedCallback()
    this.syncHljsTheme()
    window.addEventListener('theme-change', this.syncHljsTheme)

    const target = this.source ? document.getElementById(this.source) : this
    if (!target) {
      console.warn(`markdown-message: no element with id "${this.source}" to source markdown from`)
      return
    }

    this.target = target
    this.renderFrom(target.textContent ?? '')
    this.observer = new MutationObserver(() => this.scheduleRender(target))
    this.observer.observe(target, { childList: true, characterData: true, subtree: true })
  }

  disconnectedCallback() {
    super.disconnectedCallback()
    window.removeEventListener('theme-change', this.syncHljsTheme)
    this.observer?.disconnect()
  }

  private renderFrom(raw: string) {
    if (raw === this.raw) return
    this.raw = raw
    const html = md.render(raw)
    if (!this.source) {
      this.rendered = html
      return
    }
    this.reconcileInto(html)
  }

  // Streaming renders imperatively into the .prose div rather than through
  // lit's template: lit would replace the whole rendered subtree on every
  // frame, and a per-chunk fade needs the new text to live in a persistent
  // node for the animation's duration. The reconciler diffs the freshly
  // rendered markdown against the live DOM, keeping unchanged nodes and
  // flagging inserted content with .md-fade.
  private reconcileInto(html: string) {
    const prose = this.shadowRoot?.querySelector('.prose')
    if (!prose) {
      this.pendingHtml = html // first update hasn't rendered yet; updated() retries
      return
    }
    this.pendingHtml = null
    // Listener goes on the shadow root, not the .prose div: animationend
    // bubbles, and the root is stable for the element's lifetime while lit
    // could replace the div (template shape change) -- a dead div would
    // silently keep fade spans wrapped forever.
    if (!this.fadeListenerAttached) {
      this.fadeListenerAttached = true
      this.shadowRoot!.addEventListener('animationend', onFadeAnimationEnd)
    }
    reconcileMarkdown(prose, html)
  }

  protected updated(changedProperties: PropertyValues) {
    super.updated(changedProperties)
    if (this.pendingHtml !== null) this.reconcileInto(this.pendingHtml)
    // sse:close can beat the rAF that would render the final delta; settle
    // forces one synchronous render of whatever the accumulator holds.
    if (changedProperties.has('settled') && this.settled && this.target) {
      this.renderFrom(this.target.textContent ?? '')
    }
  }

  // SSE deltas fire one MutationObserver callback per chunk; each full
  // md.render + reconcile is wasted work per chunk, so coalesce bursts to
  // one render per animation frame (latest content wins).
  private scheduleRender(target: HTMLElement) {
    if (this.renderScheduled) return
    this.renderScheduled = true
    requestAnimationFrame(() => {
      this.renderScheduled = false
      if (this.isConnected) this.renderFrom(target.textContent ?? '')
    })
  }

  render() {
    // prose-sm keeps the muted variant at roughly the old plain-text size
    // (reasoning used to render as text-sm). In streaming mode the div stays
    // empty in the template -- its children are reconciled imperatively (see
    // reconcileInto), so lit owns only the shell and never replaces the
    // nodes the fade animations depend on.
    const classes = twJoin(proseClass, this.muted && 'prose-sm')
    if (this.source) return html`<div class=${classes}></div>`
    return html`<div class=${classes}>${unsafeHTML(this.rendered)}</div>`
  }
}
