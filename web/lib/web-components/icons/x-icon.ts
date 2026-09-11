import { html } from 'lit'
import { customElement, property } from 'lit/decorators.js'
import { twMerge } from 'tailwind-merge'
import { LightLitElement } from '../light-lit-element'

@customElement('x-icon')
export class XIcon extends LightLitElement {
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
          d="M6 6l12 12M18 6L6 18"
        />
      </svg>
    `
  }
}
