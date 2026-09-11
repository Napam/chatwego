import { html } from 'lit'
import { customElement, property } from 'lit/decorators.js'
import { twMerge } from 'tailwind-merge'
import { LightLitElement } from '../light-lit-element'

@customElement('menu-icon')
export class MenuIcon extends LightLitElement {
  @property({ type: String }) svgClass = ''

  render() {
    return html`
      <svg
        class=${twMerge('size-6 text-gray-600 dark:text-gray-200', this.svgClass)}
        aria-hidden="true"
        xmlns="http://www.w3.org/2000/svg"
        fill="none"
        viewBox="0 0 24 24"
      >
        <path
          stroke="currentColor"
          stroke-linecap="round"
          stroke-linejoin="round"
          stroke-width="2"
          d="M4 6h16M4 12h16M4 18h16"
        />
      </svg>
    `
  }
}
