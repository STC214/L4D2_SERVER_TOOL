# Go + Win32 Stability Checklist

This checklist is used for `L4D2ModuleWatchGUI.exe`.

## 1. UI Thread Must Stay Responsive

Risk:

- Long work in `WM_COMMAND`, `WM_CREATE`, `WM_PAINT`, or other window handlers blocks the message loop.
- Blocking the message loop makes the window look frozen.

Current status:

- Fixed. Process/module polling runs in `captureWorker`.
- Report writing runs in the worker after capture ends.
- Button handlers only start/cancel work and update small UI state.

## 2. Lock The UI Thread

Risk:

- Go goroutines can move between OS threads.
- Win32 windows and their message loop should stay on one OS thread.

Current status:

- Fixed. `runGUI` calls `runtime.LockOSThread()`.

## 3. Do Not Pass Go Pointers Through Asynchronous Window Messages

Risk:

- `PostMessage` is asynchronous.
- Passing `unsafe.Pointer` to Go heap objects through `lParam` can become invalid before the UI thread processes the message.
- This can cause random crashes or corrupted UI state.

Current status:

- Fixed. Worker events and completion records are stored in mutex-protected queues.
- `PostMessage` only sends a notification with zero `lParam`.
- UI drains queued Go values on the UI thread.

## 4. Worker Must Not Touch Win32 Controls

Risk:

- Updating controls from non-UI goroutines can race with the window thread.

Current status:

- Fixed. The worker uses `postEvent` and `postDone`.
- `setText`, `appendLog`, and button state changes run on the UI thread.

## 5. Cancellation Must Be Cooperative

Risk:

- A stop button that waits on blocking work can freeze the UI.

Current status:

- Fixed. `Stop And Save` calls `context.CancelFunc`.
- Worker checks `ctx.Done()` during each capture loop and during the wait interval.

## 6. Do Not Silently Ignore Toolhelp Failures

Risk:

- Module enumeration can fail because of bitness, permissions, or process exit.
- Silent failure produces misleading empty reports.

Current status:

- Fixed. Module enumeration errors are logged.
- The watcher uses `TH32CS_SNAPMODULE | TH32CS_SNAPMODULE32`.
- The status line shows total and relevant module counts.

## 6.1 Do Not Silently Ignore Network Table Failures

Risk:

- `GetExtendedTcpTable` / `GetExtendedUdpTable` can fail due to transient process exit, buffer sizing, or platform behavior.
- If ignored, the report can look like the game had no network activity.

Current status:

- Fixed. Endpoint enumeration errors are logged in the live log and report.
- TCP remote endpoints and UDP local sockets are collected once per second in the worker goroutine.
- UDP remote endpoints are explicitly marked as not exposed by the owner table.

## 6.2 WinDivert Capture Must Not Block Shutdown

Risk:

- `Recv` on WinDivert handles blocks until a packet/event arrives.
- If shutdown only cancels a Go context, the worker can hang waiting in `Recv`.

Current status:

- Fixed. The packet probe owns Flow and Network handles.
- On context cancellation, both handles receive `Shutdown(ShutdownRecv)`.
- Packet capture runs in worker goroutines, never on the UI thread.
- Open/recv errors are posted to the report instead of freezing the UI.

## 7. Avoid Unbounded UI Text Growth

Risk:

- Rewriting a huge edit control on every event can slow or hang the UI.
- Packet capture can generate many events per second; appending all of them to an edit control can freeze the window.

Current status:

- Fixed for packet capture. High-frequency packet observations are aggregated in memory and written to the final report.
- The UI receives low-frequency status summaries instead of per-packet messages.
- A UI-thread `SetTimer` refreshes counters once per second, so progress display does not depend on backend log messages.
- Live log keeps the last 500 lines for phase, module, endpoint, and error messages.
- If capture becomes much noisier, switch to `EM_REPLACESEL` append instead of rewriting all log text.

## 8. Window Creation Must Use Stable Timing

Risk:

- Creating controls before/after the wrong window messages can lead to missing or malformed controls.

Current status:

- Fixed. Controls are created in `WM_CREATE`.
- Layout uses `GetClientRect`, not hard-coded outer window size.

## 9. Closing While Worker Is Running

Risk:

- Worker can post to a destroyed window.

Current status:

- Mostly fixed. `WM_DESTROY` marks `closing=true` and cancels capture.
- `postEvent` and `postDone` drop messages after closing starts.
