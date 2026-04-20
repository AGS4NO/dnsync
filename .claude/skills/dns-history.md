---
name: dns-history
description: Query DNS change history, restore zones to previous states, and investigate record changes using the dnsync audit log.
---

# DNS History & Zone Restoration

You have access to a DNS audit log at `.dnsync.audit.json` that records every DNS change made by dnsync. Use this skill when the user asks about DNS history, wants to restore a zone, or needs to investigate record changes.

## Audit Log Structure

The audit log is a JSON file with this structure:

```json
{
  "_description": "...",
  "entries": [
    {
      "timestamp": "2026-04-19T15:30:00Z",
      "action": "apply",
      "zones": {
        "zone.name": {
          "manage": "partial",
          "changes": [
            {
              "action": "create|update|delete",
              "name": "record-name",
              "type": "A",
              "content": "new-value",
              "ttl": 3600,
              "old_content": "previous-value",
              "old_ttl": 3600
            }
          ],
          "snapshot": [
            {"name": "...", "type": "...", "content": "...", "ttl": 3600}
          ]
        }
      }
    }
  ]
}
```

- **entries** are ordered chronologically (oldest first)
- **changes** lists what was created, updated, or deleted in that apply
- **snapshot** is the complete zone state AFTER changes were applied
- **old_content** / **old_ttl** are only present on update actions
- Record names use `@` for the zone apex

## How to Handle User Requests

### "Restore my zone to {DATE/TIMESTAMP}"

1. Read `.dnsync.audit.json`
2. Find the last entry with a timestamp at or before the requested date
3. Extract the `snapshot` for the requested zone
4. Read the current `dns.yaml`
5. Replace the zone's records with the snapshot records, converting:
   - Empty `name` → `"@"` 
   - Skip `SOA` records and apex `NS` records (these are managed by the DNS provider)
   - Preserve the zone's `manage` mode
6. Write the updated `dns.yaml`
7. Tell the user to review and commit the changes — the GitHub Action will apply them on merge

Example:
```
User: "Restore dnsync.net to April 15, 2026"
→ Find snapshot at/before 2026-04-15
→ Edit dns.yaml with those records
→ "I've updated dns.yaml to match the zone state from 2026-04-15T10:00:00Z. Review the changes and commit to apply."
```

### "When was {RECORD} last updated/changed?"

1. Read `.dnsync.audit.json`
2. Search all entries for changes matching the record name and type
3. Report the timestamps and details of each change, most recent first

Example:
```
User: "When was the www record last changed?"
→ Search changes for name="www" across all entries
→ "The www CNAME record was last changed on 2026-04-18 at 14:00 UTC — it was updated from test.example.com to test.dnsync.net."
```

### "Show me all changes in the last {PERIOD}"

1. Read `.dnsync.audit.json`
2. Filter entries by the date range
3. Summarize changes across all matching entries

### "What did {RECORD/ZONE} look like on {DATE}?"

1. Read `.dnsync.audit.json`
2. Find the snapshot at or before the requested date
3. Filter for the specific record type if requested, or show the full zone

### "Show me the history of {RECORD}"

1. Read `.dnsync.audit.json`
2. Collect all changes for that record across all entries
3. Present as a timeline with timestamps, actions, and values

## Important Notes

- The audit log only contains changes made through dnsync. Manual changes in DNSimple are not tracked.
- If no snapshot exists before the requested date, tell the user that the audit log doesn't go back that far.
- When restoring, always preserve the `manage` mode (`full` or `partial`) from the current `dns.yaml` unless the user explicitly asks to change it.
- After editing `dns.yaml`, remind the user to commit and merge — changes are not applied until the GitHub Action runs.
- The snapshot reflects the zone state after DNSimple's processing, so content may differ slightly from what was in `dns.yaml` (e.g., DNSimple normalizes certain record formats).
