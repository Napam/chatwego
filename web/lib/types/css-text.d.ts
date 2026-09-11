// Bun's `with { type: 'text' }` import attribute inlines a file's contents
// as a plain string at bundle time -- used to load a CSS theme's raw text so
// it can be built into a CSSStyleSheet and adopted into a shadow root.
declare module '*.css' {
  const text: string
  export default text
}
