import { twJoin } from 'tailwind-merge'

const mdQuery = '(min-width: 768px)'

// State classes toggled on the drawer. Kept as strings (and run through
// twJoin) so both the Tailwind scanner and the eslint plugin see them.
// classList needs individual names, hence the split before each toggle.
const drawerOpenClasses = twJoin('translate-x-0')
const drawerCollapsedClasses = twJoin('md:w-0', 'md:invisible')
const drawerTransitionClasses = twJoin('transition-transform', 'duration-200', 'ease-in-out')

function toggleClasses(el: Element, classes: string, on: boolean) {
  for (const name of classes.split(' ')) el.classList.toggle(name, on)
}

/**
 * Off-canvas drawer below md, collapsible in-flow column at md and up.
 *
 * Behavior only: it never renders, so its light-DOM children -- including the
 * server-owned session list that Datastar patches by id -- stay where they
 * are. The markup and base classes live in root.templ; this element only
 * toggles state classes. The hamburger sits outside the element, so toggles
 * and closes are delegated from the document via data-sidebar-toggle and
 * data-sidebar-close.
 *
 * Not a Lit element: rendering would replace the server-owned children.
 */
export class ChatSidebar extends HTMLElement {
  private readonly media = window.matchMedia(mdQuery)
  private wide = this.media.matches
  private open = false
  private collapsed = false

  connectedCallback() {
    this.media.addEventListener('change', this.onBreakpointChange)
    this.addEventListener('transitionend', this.onTransitionEnd)
    document.addEventListener('click', this.onClick)
  }

  disconnectedCallback() {
    this.media.removeEventListener('change', this.onBreakpointChange)
    this.removeEventListener('transitionend', this.onTransitionEnd)
    document.removeEventListener('click', this.onClick)
  }

  // Public so the composer can close the drawer after sending on narrow.
  close() {
    if (!this.wide && this.open) this.setOpen(false, true)
  }

  private get drawer() {
    return this.querySelector<HTMLElement>('[data-sidebar-drawer]')
  }

  private get scrim() {
    return this.querySelector<HTMLElement>('[data-sidebar-scrim]')
  }

  private readonly onBreakpointChange = (event: MediaQueryListEvent) => {
    this.wide = event.matches
    // A breakpoint change is always instant. Cut short any in-flight slide so
    // it cannot replay as the media state flips.
    this.clearTransition()
    // The drawer is mobile-only, so forget it when the column takes over;
    // re-entering narrow then starts closed.
    if (this.wide) this.open = false
    this.apply()
  }

  private readonly onClick = (event: MouseEvent) => {
    const target = event.target as Element | null
    if (target?.closest('[data-sidebar-toggle]')) {
      this.toggle()
    } else if (target?.closest('[data-sidebar-close]')) {
      this.close()
    }
  }

  private readonly onTransitionEnd = (event: TransitionEvent) => {
    if (event.target === this.drawer) this.clearTransition()
  }

  private toggle() {
    if (this.wide) {
      this.collapsed = !this.collapsed
      this.apply()
    } else {
      this.setOpen(!this.open, true)
    }
  }

  private setOpen(open: boolean, animate: boolean) {
    this.open = open
    if (animate) {
      this.withTransition(() => this.apply())
    } else {
      this.apply()
    }
  }

  private apply() {
    const { drawer, scrim } = this
    if (drawer) {
      toggleClasses(drawer, drawerOpenClasses, this.open)
      toggleClasses(drawer, drawerCollapsedClasses, this.collapsed)
    }
    if (scrim) scrim.hidden = !this.open
  }

  // Add the transition properties and change state in the same task, which
  // starts the slide. onTransitionEnd drops the classes again, so they are
  // only present while a user toggle is animating -- a breakpoint change has
  // nothing to transition. clearTransition is the escape hatch for when the
  // slide is interrupted (see onBreakpointChange).
  private withTransition(mutate: () => void) {
    const drawer = this.drawer
    if (!drawer) {
      mutate()
      return
    }
    toggleClasses(drawer, drawerTransitionClasses, true)
    mutate()
  }

  private clearTransition() {
    const drawer = this.drawer
    if (drawer) toggleClasses(drawer, drawerTransitionClasses, false)
  }
}

customElements.define('chat-sidebar', ChatSidebar)
