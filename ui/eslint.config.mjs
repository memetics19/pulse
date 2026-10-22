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
      // Native dialogs are inaccessible and block the event loop. Use the
      // shared useConfirm() dialog (components/ConfirmDialog) instead.
      'no-alert': 'error',
      'no-restricted-globals': [
        'error',
        { name: 'confirm', message: 'Use the shared useConfirm() dialog instead of the native confirm().' },
        { name: 'prompt', message: 'Use an in-app form/dialog instead of the native prompt().' },
      ],
    },
  },
]

export default config
