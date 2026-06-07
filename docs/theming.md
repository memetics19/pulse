# Theming

Pulse styles public status pages with themes, uploaded assets, a configurable footer, and CSS variable overrides. Branding can be set globally or per page.

## Presets

Pulse ships three built-in theme presets:

- `default-light`
- `default-dark`
- `terminal-dark`

Select a preset in the admin under Theme.

## Logo and favicon

Upload a logo and a favicon. The logo appears on the public page header. The favicon is used by the browser tab.

## Footer fields

The footer is configurable with these fields:

- **Brand line.** The main footer brand text.
- **Tagline.** A short line under the brand.
- **Link list.** A set of footer links.
- **Copyright line.** The copyright text.
- **Powered by Pulse toggle.** A toggle to hide the "Powered by Pulse" line.

## CSS variable overrides

You can override the theme's CSS variables to adjust colors and other values without replacing the whole theme. Set the overrides in the admin under Theme.

## Per-page branding

Each [status page](status-pages.md) can have its own logo and theme. A page's branding overrides the global branding for that page, so different domains can present different looks.

## Dark mode

The public page includes a visitor dark-mode toggle. Visitors can switch between light and dark independently of the configured preset.

## Timezones

All times on the public page render in the visitor's local timezone. There is no server-side timezone to configure for display.
