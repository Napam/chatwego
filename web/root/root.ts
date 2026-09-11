// Page script for the root chat view. Four subsystems -- thoughts sync,
// follow-mode scroll, frame batching, stream lifecycle -- are wired up below;
// each init owns its listeners and state.
//
// Datastar owns the network: the composer's data-on:submit action POSTs to
// /send-prompt and Datastar streams the reply into the DOM. This script only
// reacts to the DOM and to Datastar's lifecycle event.
type StreamWindow = Window & { answering: boolean }
type ThoughtsSync = { sync: () => void; finalize: () => void }
type FollowMode = { isFollowing: () => boolean; scrollToBottom: () => void; scrollThoughts: () => void }

// Element ids rendered by the RootPage template; named here so the page
// script's hardcoded lookups can't drift from the template's const block.
const CHAT_MESSAGES_ID = 'chat-messages'
const STOP_BUTTON_ID = 'stop-button'
const CHAT_TEXTAREA_ID = 'chat-textarea'
const CHAT_FORM_ID = 'chat-form'
// window.answering is the stream-liveness flag: set by the composer's submit
// action before a prompt, cleared by finalizeStream.
const w = window as unknown as StreamWindow
w.answering = false

const container = document.getElementById(CHAT_MESSAGES_ID)
if (container) {
  const thoughts = initThoughtsSync(container) // returns { sync, finalize }
  const follow = initFollowMode(container) // returns { isFollowing, scrollToBottom, scrollThoughts }
  initFrameBatcher(container, { thoughts, follow })
  initStreamLifecycle(container, thoughts)
}

// Owns live thought disclosures: summary label toggling, user-pin
// tracking, and stream-end finalize.
function initThoughtsSync(container: HTMLElement): ThoughtsSync {
  // A live disclosure doubles as the loading indicator: pulsing
  // "Thinking ..." until reply text or tool calls arrive, then
  // "Thoughts". Toggle the native open attribute -- the only
  // reliable way to force a <details> open.
  const setSummary = (details: HTMLDetailsElement, thinking: boolean) => {
    for (const label of details.querySelectorAll('[data-thinking-label]')) {
      label.classList.toggle('hidden', !thinking)
      label.classList.toggle('animate-pulse', thinking)
    }
    for (const label of details.querySelectorAll('[data-thoughts-label]')) {
      label.classList.toggle('hidden', thinking)
    }
  }
  // A summary click (incl. keyboard) pins the disclosure for
  // the rest of the stream; the toggle event can't tell manual
  // from syncThoughts' own programmatic open/close. WeakSet:
  // GC'd with the node, fresh details per round.
  const userPinned = new WeakSet<Element>()
  container.addEventListener('click', (e) => {
    const details = (e.target as Element).closest?.('summary')?.closest?.('[data-live-thoughts]')
    if (details) userPinned.add(details)
  })
  const sync = () => {
    for (const src of container.querySelectorAll('[data-markdown-source]')) {
      const body = src.parentElement!.querySelector('[data-reasoning-body]')
      if (!body) continue
      const details = body.closest('details') as HTMLDetailsElement
      const acc = details.querySelector('[data-reasoning-accumulator]')
      const toolCalls = src.parentElement!.querySelector('[data-tool-calls]')
      const replyEmpty = src.textContent.trim() === ''
      const toolsEmpty = !toolCalls || toolCalls.textContent.trim() === ''
      const reasoningEmpty = !acc || acc.textContent.trim() === ''
      // Auto until the user toggles by hand; their choice wins.
      if (!userPinned.has(details)) details.open = replyEmpty
      // "Thinking ..." only while a stream is live: answering is set by
      // the composer and cleared by finalizeStream, so a settled,
      // stopped, or errored round can never re-enter the thinking state.
      const thinking = w.answering && replyEmpty && toolsEmpty
      setSummary(details, thinking)
      details.classList.toggle('hidden', !replyEmpty && reasoningEmpty)
    }
  }
  // Stream end: flip live disclosures back to "Thoughts", hide ones
  // with nothing to show (e.g. stream error mid-reasoning). Called
  // from finalizeStream, which is itself idempotent.
  const finalize = () => {
    for (const details of container.querySelectorAll(
      '[data-live-thoughts]'
    ) as NodeListOf<HTMLDetailsElement>) {
      const acc = details.querySelector('[data-reasoning-accumulator]')
      const reply = details.parentElement!.querySelector('[data-markdown-source]')
      const reasoningEmpty = !acc || acc.textContent.trim() === ''
      const replyEmpty = !reply || reply.textContent.trim() === ''
      setSummary(details, false)
      if (reasoningEmpty && replyEmpty) details.classList.add('hidden')
    }
  }
  return { sync, finalize }
}

// Owns follow-mode scrolling: container + per-body pin state, and
// the scroll actions (container pin and per-body thoughts scroll).
function initFollowMode(container: HTMLElement): FollowMode {
  const scrollToBottom = () => {
    container.scrollTop = container.scrollHeight
  }
  // Follow-mode: pin to bottom while streaming; stop once the
  // user scrolls away. Wheel-up is the unambiguous gesture --
  // flip off instantly, no threshold. Scroll events can't
  // distinguish user input from layout/anchoring, so they only
  // flip off on an upward move of >=24px; the pin's own async
  // scroll always moves scrollTop DOWN, never a scroll-away.
  const gap = (el: Element) => el.scrollHeight - el.scrollTop - el.clientHeight
  const isNearBottom = (el: Element) => gap(el) < 4
  const scrolledAway = (el: Element, prev: number) => el.scrollTop < prev && gap(el) >= 24
  let following = true
  const bodyFollow = new WeakMap<Element, boolean>()
  // Wheel over a Thoughts body flips only that body off.
  container.addEventListener(
    'wheel',
    (e) => {
      if (e.deltaY >= 0) return
      const body = (e.target as Element).closest?.('[data-reasoning-body]')
      if (body) bodyFollow.set(body, false)
      else following = false
    },
    { passive: true }
  )
  // Scroll doesn't bubble; capture also sees inner Thoughts
  // bodies, tracking each scrollable's follow state separately.
  const lastTop = new WeakMap<Element, number>()
  container.addEventListener(
    'scroll',
    (e) => {
      const el = e.target as Element
      const prev = lastTop.get(el) ?? el.scrollTop
      lastTop.set(el, el.scrollTop)
      if (el === container) {
        if (isNearBottom(el)) {
          following = true
          return
        }
        if (scrolledAway(el, prev)) following = false
      } else if (el instanceof HTMLElement && el.hasAttribute('data-reasoning-body')) {
        if (isNearBottom(el)) {
          bodyFollow.set(el, true)
          return
        }
        if (scrolledAway(el, prev)) bodyFollow.set(el, false)
      }
    },
    true
  )
  // Each Thoughts body scrolls its own overflow, following
  // independently -- a body scrolled up stays put.
  const scrollThoughts = () => {
    for (const body of container.querySelectorAll('[data-reasoning-body]')) {
      if (bodyFollow.get(body) !== false) body.scrollTop = body.scrollHeight
    }
  }
  const isFollowing = () => following
  return { isFollowing, scrollToBottom, scrollThoughts }
}

// Owns frame batching: coalesces observer/load triggers into one rAF
// pass that syncs thoughts and pins the scroll while following.
function initFrameBatcher(container: HTMLElement, deps: { thoughts: ThoughtsSync; follow: FollowMode }) {
  const { thoughts, follow } = deps
  // Coalesce the per-delta observer to one pass per frame.
  // While following, pin unconditionally -- layout adjustments
  // can't flip it off.
  let scheduled = false
  const update = () => {
    if (scheduled) return
    scheduled = true
    requestAnimationFrame(() => {
      scheduled = false
      thoughts.sync()
      if (follow.isFollowing()) {
        follow.scrollThoughts()
        follow.scrollToBottom()
      }
    })
  }
  // Datastar patches are DOM mutations (appended bubbles, accumulator
  // text), so the observer sees every delta without a separate hook.
  new MutationObserver(update).observe(container, {
    childList: true,
    subtree: true,
    characterData: true,
  })
  requestAnimationFrame(update)
  window.addEventListener('load', update)
}

// Owns stream lifecycle: stop button visibility and cleanup on natural
// end, error, and stop, driven by Datastar's fetch lifecycle event.
function initStreamLifecycle(container: HTMLElement, thoughts: ThoughtsSync) {
  const stopButton = document.getElementById(STOP_BUTTON_ID)
  const showStop = (show: boolean) => {
    stopButton?.classList.toggle('hidden', !show)
    stopButton?.classList.toggle('inline-flex', show)
  }
  // Stream end: finalize disclosures to "Thoughts", hide ones with nothing
  // to show (e.g. stream error mid-reasoning), settle markdown -- matching
  // reloaded rendering. Idempotent: re-runs harmlessly on natural end,
  // abort, and the stop handler's immediate call.
  const finalizeStream = () => {
    w.answering = false
    thoughts.finalize()
    for (const mm of container.querySelectorAll('markdown-message[source]')) {
      mm.setAttribute('settled', '')
    }
  }
  // datastar-fetch fires on document for every Datastar action:
  // 'started' when the request begins, 'finished'/'error' when it ends.
  // Aborting counts as finished, so stop runs through the same cleanup.
  document.addEventListener('datastar-fetch', (e) => {
    const detail = (e as CustomEvent).detail
    // Only the composer's fetch drives the chat UI. Datastar fires this for
    // every action on the page, and another action finishing must not clear
    // the stop button or settle the live reply.
    if (detail?.el?.id !== CHAT_FORM_ID) return
    const type = detail.type
    if (type === 'started') {
      w.answering = true
      showStop(true)
    } else if (type === 'finished' || type === 'error') {
      finalizeStream()
      showStop(false)
    }
  })
  stopButton?.addEventListener('click', () => {
    if (!w.answering) return // stale click after natural end / double-click
    w.answering = false // textarea accepts a new prompt immediately
    finalizeStream() // immediate UI settle; 'finished' will repeat it (idempotent)
    showStop(false)
    document.getElementById(CHAT_TEXTAREA_ID)?.focus()
  })
}
