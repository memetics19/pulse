# Docs assets

## `status-page.gif`

The home page and the project README embed `status-page.gif` — a short screen
recording of the live status page. **Drop the recorded file here as
`status-page.gif`** (this directory). It is referenced by `docs/index.md` and
`README.md`.

### Recording it

Record `https://status.shreeda.xyz` (a ~5–8s loop: page load, an incident, the
range selector). Any of:

- **macOS:** [Kap](https://getkap.co) — record a region, export as GIF.
- **Linux:** [Peek](https://github.com/phw/peek).
- **Cross-platform:** [LICEcap](https://www.cockos.com/licecap/).
- **Scripted (Playwright):**
  ```sh
  npx playwright screenshot --wait-for-timeout=2000 https://status.shreeda.xyz status.png
  # or record a video via a short Playwright script, then convert with gifski:
  #   gifski --fps 15 -o status-page.gif frames/*.png
  ```

Keep it under ~3 MB so the docs site and README stay light.
