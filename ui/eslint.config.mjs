import coreWebVitals from 'eslint-config-next/core-web-vitals'
import typescript from 'eslint-config-next/typescript'

/** @type {import('eslint').Linter.Config[]} */
const config = [
  {
    ignores: ['.next/**', 'out/**', 'next-env.d.ts'],
  },
  ...coreWebVitals,
  ...typescript,
  {
    rules: {
      // Pre-existing mount-time patterns (reading localStorage for the theme,
      // fetching a list on mount) trip this rule. They are hydration-safe as
      // written — a lazy initialiser would read localStorage during render and
      // desync from the prerendered HTML. Warn rather than block; revisiting
      // these belongs in its own change, not a dependency bump.
      'react-hooks/set-state-in-effect': 'warn',
    },
  },
]

export default config
