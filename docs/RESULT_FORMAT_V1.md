# Result JSON v1

**Status:** implemented in PR #2, *not* a benchmark protocol version. See [JSON Schema](schema/result-v1.schema.json).

The storage format is a UTF-8 JSON object with exactly two top-level properties, `schema_version` and `run`. Version 1 is strict: unknown fields or versions are rejected. Max file size: 4 MiB. A valid run contains `id`, `label`, `complete`, and 1 to 64 positive, finite metrics. Metric keys are `cpu.N`, `ram.N`, `ffmpeg.N`, or `go.N`, with a positive integer `N` (worker/thread count). Metric keys must be unique; units and `higher_is_better` must match the metric family. Metadata is optional; checksums, when present, are lowercase SHA-256 hexadecimal.

```json
{
  "schema_version": 1,
  "run": {
    "id": "rapport_example.json",
    "complete": true,
    "label": "example",
    "timestamp": "2026-11-15T14:23:31+01:00",
    "cpu": "Example CPU",
    "ram": "11Gi",
    "sysbench_version": "1.0.20",
    "metrics": [
      {
        "key": "cpu.1",
        "label": "CPU · 1 thread",
        "value": 1119.09,
        "unit": "événements/s",
        "higher_is_better": true
      }
    ]
  }
}
```

`run.id` **must equal the JSON filename** (a `rapport_*.json` basename). This prevents an imported file from silently impersonating a different campaign. Do not embed host machine IDs, boot IDs, home paths, user identities or raw logs. `export` emits only whitelisted fields parsed by the historical importer. Report files are private local files, not intended for publication.

### CLI migration (no source deletion)

```bash
pc-bench export --reports "$HOME/Téléchargements" --from rapport_avant_upgrade_20261115_142331.txt
pc-bench list --reports "$HOME/Téléchargements"
```

The command creates the same-basename `.json` sibling with `0600` permissions; it **does not overwrite** existing files and does not follow source symlinks. Existing `.txt` files remain unchanged. When both are valid, `list`, `compare` and the HTTP UI select the JSON version, not both. If JSON is corrupt, they fall back to the legacy text file. Unknown or malformed JSON is not listed; validation errors are surfaced when importing directly through the library. No browser-side or benchmark-script changes in this PR.

### Comparability limitations

The JSON schema assures structural validity, **not** performance-measurement equivalence. Historical results lack a complete `protocol_id`, tool build metadata, microcode, cooling and runtime environment controls. The existing version/checksum comparisons still apply. In addition, comparison now rejects mismatched metric units or `higher_is_better` directions. Incomplete runs remain visible but cannot certify improvements. Cross-OS support of the dashboard is distinct from OS-specific benchmark collection.

### Security and compatibility notes

- Strict decoding rejects unknown fields, missing required fields, trailing values, invalid metrics and unsupported schema versions.
- `Load` still limits reports to regular files from one directory and keeps the 500-item cap; a corrupt JSON does not suppress its `.txt` twin.
- The parser is bounded; the resulting schema is not a permission boundary for users who already have filesystem access.
- JSON uses the Go standard library only; no new runtime service or dependency.
- Valid Duplicate JSON object-member names are rejected, including inside metric arrays.
