# Maintenance

A maintenance window marks a planned period of work on one or more monitors. While a window is active, Pulse suppresses alerts and auto-incidents for the affected monitors and shows the window on the public page.

## Scheduling

A maintenance window is created in one of two ways:

- **Scheduled.** You set a future start time and an end time.
- **Start now.** The window begins immediately.

A window applies to the monitors you select.

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
