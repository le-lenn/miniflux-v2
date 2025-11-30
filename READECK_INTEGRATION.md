Readeck Integration Behavior and Auto‑Send Option

Summary
- Current behavior: Readeck is a “save‑style” integration. Miniflux sends an entry to Readeck only when you explicitly save an item (UI/API). New entries discovered during feed refresh are not pushed automatically.
- Proposed change: Add a user setting `ReadeckAutoPush` to automatically send new entries to Readeck during feed refresh, in addition to the manual Save.

Current Flow (code references)
- Save dispatch:
  - UI save handler calls `integration.SendEntry(entry, settings)`: internal/ui/entry_save.go:38
  - API save handler calls `integration.SendEntry(entry, settings)`: internal/api/entry.go:231
  - `SendEntry` implements save‑style targets (including Readeck): internal/integration/integration.go:41
  - Readeck block in `SendEntry`: internal/integration/integration.go:294
  - Readeck client details: internal/integration/readeck/readeck.go
- Refresh dispatch:
  - Feed refresh invokes `integration.PushEntries(feed, newEntries, settings)`: internal/reader/handler/handler.go:363
  - `PushEntries` handles push/notifications (Webhook, Ntfy, Apprise, Matrix, Slack, Telegram, Pushover): internal/integration/integration.go:501
  - Readeck is not handled here today.

Design: Auto‑Send New Entries to Readeck
- Setting: Add `ReadeckAutoPush` (bool) at the user integration level.
- Behavior: When `ReadeckEnabled && ReadeckAutoPush`, `PushEntries` will send each new entry to Readeck. Manual Save continues to work.
- Content vs URL‑only: Use existing `ReadeckOnlyURL` to decide whether to upload full content (multipart) or just URL/title/labels (JSON).
- Duplication: Saving manually after auto‑push may create duplicates. Two options:
  - Keep both behaviors; rely on Readeck’s deduplication by URL if any, or accept duplicates.
  - Alternative: Skip Readeck in `SendEntry` when `ReadeckAutoPush` is enabled. This avoids duplication but changes Save semantics.

Implementation Status
- Implemented in this repo. Enable it in Settings → Integrations → Readeck by checking “Automatically send new entries”.
- Code references:
  - Model field: internal/model/integration.go:83
  - DB migration: internal/database/migrations.go: add `readeck_auto_push` at the end
  - Storage load/save: internal/storage/integration.go:232, 494–497, 618–621
  - UI form parse/struct: internal/ui/form/integration.go: add `ReadeckAutoPush`
  - Template checkbox: internal/template/templates/views/integrations.html:513
  - Push dispatcher: internal/integration/integration.go:686

Required Changes
- Schema (DB migration)
  - Add a new column on `integrations` table:
    - internal/database/migrations.go: add a migration function near the end:
      `ALTER TABLE integrations ADD COLUMN readeck_auto_push bool default 'f';`

- Model
  - Extend `internal/model/integration.go`:
    - Add `ReadeckAutoPush bool` next to other Readeck fields: internal/model/integration.go:83

- Storage (load/save)
  - Extend `Integration(userID)` SELECT to include the new column and scan into `integration.ReadeckAutoPush`: internal/storage/integration.go:116–520
  - Extend `UpdateIntegration(integration)` UPDATE to set `readeck_auto_push=$[n]` and pass `integration.ReadeckAutoPush`: internal/storage/integration.go:520–720

- UI: Form parsing and template
  - Parse form value into the model:
    - Add `ReadeckAutoPush: r.FormValue("readeck_auto_push") == "1",` in `internal/ui/form/integration.go:280–520` alongside other Readeck fields.
  - Template checkbox in Readeck panel:
    - internal/template/templates/views/integrations.html:524
    - Add a checkbox:
      `name="readeck_auto_push"` with label “Automatically send new entries”.
    - Keep existing fields: enable, only‑URL, URL, API key, labels, and the Test button.

- Dispatcher: Push during refresh
  - In `integration.PushEntries(...)`, add Readeck handling similar to Telegram’s per‑entry loop:
    - internal/integration/integration.go:501
    - Pseudocode:
      `if userIntegrations.ReadeckEnabled && userIntegrations.ReadeckAutoPush { for _, entry := range entries { client := readeck.NewClient(userIntegrations.ReadeckURL, userIntegrations.ReadeckAPIKey, userIntegrations.ReadeckLabels, userIntegrations.ReadeckOnlyURL); _ = client.CreateBookmark(entry.URL, entry.Title, entry.Content) } }`
    - Log success/failures similarly to the `SendEntry` block.

- Optional: Avoid duplicates on Save
  - If desired, wrap the Readeck block in `SendEntry` with `!userIntegrations.ReadeckAutoPush`:
    - internal/integration/integration.go:294
    - This makes Save no‑op for Readeck when auto‑push is enabled.

Testing & Validation
- Use the built‑in Test button in Integrations → Readeck to verify credentials: internal/template/templates/views/integrations.html:546, internal/ui/integration_test_action.go:110–116.
- To exercise auto‑push, enable “Automatically send new entries” and refresh any feed that produces new items; monitor logs for “Sending new entries to Readeck”.

Notes
- Timeouts and rate‑limits: Readeck client uses a 10s HTTP timeout; heavy feeds could generate many requests. Consider batching later if Readeck adds a bulk API.
- Security: API key is sent as Bearer; ensure HTTPS in `ReadeckURL`.
