// Incremental markdown DOM reconciliation for streaming.
//
// Streaming re-renders the full message with md.render() every animation
// frame. Assigning innerHTML would replace every node each frame, killing
// the per-chunk fade-in: a node must survive ~200ms for its animation to
// complete. Instead, the freshly rendered tree is walked against the live
// one and only the differences are applied:
//
// - nodes that still match keep their identity (markdown-it's output for
//   completed blocks is append-stable, so in the common case only the last
//   block's tail is touched per frame),
// - text appended to an existing text run is wrapped in a persistent
//   .md-fade span,
// - elements with no counterpart yet are inserted with .md-fade on them.
//
// Fade spans count as part of their surrounding text run when matching, so
// they survive later frames until their animationend handler unwraps them.

export const FADE_CLASS = 'md-fade'

// Attributes allowed to change in place on a matched element: a growing
// autolinked URL must not make the whole link re-fade (or an <img> reload).
const MUTABLE_ATTRS: Record<string, readonly string[]> = {
  A: ['href', 'title'],
  IMG: ['src', 'alt', 'title'],
}

// Parents whose whitespace-only text children are markdown-it's inter-block
// formatting, not content. Real content whitespace (P, CODE, PRE, inline
// elements) is never skipped.
const STRUCTURAL_TAGS = new Set(['UL', 'OL', 'LI', 'BLOCKQUOTE'])

// A matchable unit of the live child list: either a run of adjacent
// text-like nodes (text nodes plus the fade spans this module inserted,
// which logically form one string) or a single element.
type Unit = { kind: 'run'; nodes: ChildNode[]; text: string } | { kind: 'el'; el: Element }

// Text-run fade spans carry a marker attribute distinguishing them from
// real rendered <span>s (e.g. hljs tokens) that were merely flagged with
// FADE_CLASS on insertion: only the former may be unwrapped on
// animationend or grouped into a text run.
const FADE_SPAN_ATTR = 'data-md-fade'

function isFadeSpan(node: ChildNode): boolean {
  return (
    node.nodeType === Node.ELEMENT_NODE &&
    (node as Element).tagName === 'SPAN' &&
    (node as Element).hasAttribute(FADE_SPAN_ATTR) &&
    node.childNodes.length === 1 &&
    node.firstChild?.nodeType === Node.TEXT_NODE
  )
}

function groupUnits(parent: Element, structural: boolean): Unit[] {
  const units: Unit[] = []
  let run: ChildNode[] = []
  let text = ''
  const flush = () => {
    if (run.length > 0) units.push({ kind: 'run', nodes: run, text })
    run = []
    text = ''
  }
  for (const node of parent.childNodes) {
    if (node.nodeType === Node.TEXT_NODE || isFadeSpan(node)) {
      // Inter-block formatting whitespace isn't content; ignore it so it
      // can't form a phantom run (importNew strips it from fresh subtrees).
      if (structural && !(node.textContent ?? '').trim()) continue
      run.push(node)
      text += node.textContent
    } else if (node.nodeType === Node.ELEMENT_NODE) {
      flush()
      units.push({ kind: 'el', el: node as Element })
    }
  }
  flush()
  return units
}

// Class comparison ignores FADE_CLASS on the live side: it's this module's
// own annotation, not part of the rendered markdown.
function classKey(el: Element): string {
  return (el.getAttribute('class') ?? '')
    .split(/\s+/)
    .filter((c) => c !== '' && c !== FADE_CLASS)
    .sort()
    .join(' ')
}

function elementsMatch(oldEl: Element, newEl: Element): boolean {
  if (oldEl.tagName !== newEl.tagName || classKey(oldEl) !== classKey(newEl)) return false
  const mutable = MUTABLE_ATTRS[oldEl.tagName] ?? []
  const oldNames = oldEl.getAttributeNames().filter((n) => n !== 'class')
  const newNames = newEl.getAttributeNames().filter((n) => n !== 'class')
  if (oldNames.length !== newNames.length) return false
  return newNames.every((name) => {
    if (oldEl.getAttribute(name) === newEl.getAttribute(name)) return true
    // A changed value is only acceptable in-place-updatable attributes
    // present on both sides.
    return mutable.includes(name) && oldEl.hasAttribute(name)
  })
}

function matches(unit: Unit, node: ChildNode): boolean {
  if (node.nodeType === Node.TEXT_NODE) return unit.kind === 'run'
  return unit.kind === 'el' && node.nodeType === Node.ELEMENT_NODE && elementsMatch(unit.el, node as Element)
}

function fadeSpan(text: string): HTMLSpanElement {
  const span = document.createElement('span')
  span.className = FADE_CLASS
  span.setAttribute(FADE_SPAN_ATTR, '')
  span.textContent = text
  return span
}

function morphRun(unit: Extract<Unit, { kind: 'run' }>, newText: string): void {
  if (newText === unit.text) return
  const last = unit.nodes[unit.nodes.length - 1]
  if (newText.startsWith(unit.text)) {
    last.after(fadeSpan(newText.slice(unit.text.length)))
    return
  }
  // Mid-text rewrite (e.g. linkify changing its mind about a partial URL):
  // rare, and a fading partial rewrite would look odd -- replace outright.
  last.after(document.createTextNode(newText))
  for (const node of unit.nodes) node.remove()
}

function morphElement(oldEl: Element, newEl: Element): void {
  for (const name of MUTABLE_ATTRS[oldEl.tagName] ?? []) {
    const value = newEl.getAttribute(name)
    if (value !== null && oldEl.getAttribute(name) !== value) oldEl.setAttribute(name, value)
  }
  reconcileChildren(oldEl, newEl.childNodes)
}

// markdown-it puts "\n" between block elements; those would be mistaken for
// text runs on later frames, so freshly inserted subtrees are stripped of
// whitespace-only text children at structural levels. Content whitespace
// (inside P/CODE/PRE/inline elements) is kept.
function stripStructuralWhitespace(el: Element): void {
  if (STRUCTURAL_TAGS.has(el.tagName)) {
    for (const node of [...el.childNodes]) {
      if (node.nodeType === Node.TEXT_NODE && !(node.textContent ?? '').trim()) node.remove()
    }
  }
  for (const child of el.children) stripStructuralWhitespace(child)
}

// Deep-clones a rendered node for insertion, flagging it to fade in. A text
// node becomes a fade span so it animates and stays matchable as part of
// its text run; an element fades as a whole (opacity covers its subtree).
function importNew(node: ChildNode): ChildNode {
  if (node.nodeType === Node.TEXT_NODE) return fadeSpan(node.textContent ?? '')
  const el = (node as Element).cloneNode(true) as Element
  el.classList.add(FADE_CLASS)
  stripStructuralWhitespace(el)
  return el
}

function reconcileChildren(parent: Element, newNodes: NodeListOf<ChildNode>, structural = false): void {
  structural = structural || STRUCTURAL_TAGS.has(parent.tagName)
  const units = groupUnits(parent, structural)
  let ui = 0
  const anchor = (): ChildNode | null => {
    const unit = units[ui]
    return unit ? (unit.kind === 'run' ? unit.nodes[0] : unit.el) : null
  }
  for (const node of newNodes) {
    if (structural && node.nodeType === Node.TEXT_NODE && !(node.textContent ?? '').trim()) continue
    const unit = units[ui]
    if (unit && matches(unit, node)) {
      if (unit.kind === 'run') morphRun(unit, node.textContent ?? '')
      else morphElement(unit.el, node as Element)
      ui++
    } else {
      // No counterpart: insert without consuming the live unit. Anything
      // still unconsumed after the loop is stale and removed below.
      parent.insertBefore(importNew(node), anchor())
    }
  }
  for (; ui < units.length; ui++) {
    const unit = units[ui]
    if (unit.kind === 'run') for (const node of unit.nodes) node.remove()
    else unit.el.remove()
  }
}

/**
 * Diffs freshly rendered markdown HTML into `container`, preserving the
 * identity of unchanged nodes and flagging inserted content with
 * FADE_CLASS. Stale children (from a previous stream) are diffed away.
 */
export function reconcileMarkdown(container: Element, html: string): void {
  const template = document.createElement('template')
  template.innerHTML = html
  reconcileChildren(container, template.content.childNodes, true)
}

/**
 * animationend handler for FADE_CLASS nodes: unwraps text-run fade spans
 * back to plain text and strips the class from anything else (e.g. hljs
 * token spans, which must keep their element), so a settled message
 * converges to clean markdown HTML.
 */
export function onFadeAnimationEnd(event: Event): void {
  const el = event.target
  if (!(el instanceof Element) || !el.classList.contains(FADE_CLASS)) return
  if (el.tagName === 'SPAN' && el.hasAttribute(FADE_SPAN_ATTR)) {
    el.replaceWith(document.createTextNode(el.textContent ?? ''))
  } else {
    el.classList.remove(FADE_CLASS)
  }
}
