# Maintenance

A maintenance window marks a planned period of work on one or more monitors. While a window is active, Pulse suppresses alerts and auto-incidents for the affected monitors and shows the window on the public page.

## Scheduling

A maintenance window is created in one of two ways:

- **Scheduled.** You set a future start time and an end time.
- **Start now.** The window begins immediately.

A window applies to the monitors you select.

The end time must be after the start time. With **Start now**, the end time must be in the future. The API rejects any other window with a 400 error.

## Time zones

Pulse stores every maintenance time in UTC. The API accepts RFC 3339 times with any offset, such as `2026-10-23T10:00:00+05:30` or `2026-10-23T04:30:00Z`. Pulse converts them to UTC before saving them, and the API returns them in UTC. The admin UI sends UTC already. The host's time zone does not change when a window starts or ends.

Upgrading to this version runs a database migration. It rewrites older timestamps that were saved with a host offset into UTC. The migration runs once and is safe to repeat.

## Automatic transitions

The worker transitions a window through three states at its boundaries:

1. `scheduled` before the start time.
2. `in progress` between the start and end times.
3. `completed` after the end time.

You do not move a window between these states manually. The worker does it at the configured boundaries.

## Alert suppression

While a window is `in progress`, alerts and auto-incidents are suppressed for the affected monitors. A failing check during a maintenance window does not open an auto-incident for a monitor covered by that window.

## Public display

The public status page shows maintenance in three sections:

- An **in-progress banner** while a window is active.
- An **upcoming** section for scheduled windows.
- A **history** of completed windows.

Times render in the visitor's local timezone.
