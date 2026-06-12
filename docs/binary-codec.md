# Binary Codec — RTND v2

## File header (8 bytes)

```
Offset  Size  Field
0       4     Magic: "RTND" (0x52 0x54 0x4E 0x44)
4       1     Version: 2 (reader accepts v1 and v2)
5       3     Reserved: zero
```

## Per-node encoding

All multi-byte integers are **little-endian**. Strings are UTF-8.

```
[schema_version: 2B u16]
[id: 8B u64]
[title_len: 2B u16] [title: N UTF-8]
[status: 1B u8]
[claim_status: 1B u8]
[outcome: 1B u8]
[num_parents: 2B u16] [parents: 8B × N u64]
[agent_len: 2B u16] [agent: N UTF-8]
[num_tags: 2B u16]
  for each tag: [tag_len: 2B u16] [tag: N UTF-8]
[created: 8B i64 unix_nano]          (0 = zero time)
[modified: 8B i64 unix_nano]
[revision: 8B u64]
[num_commits: 2B u16]
  for each commit: [hash_len: 2B] [hash: N] [msg_len: 4B u32] [msg: N]
[num_artifacts: 2B u16]
  for each artifact:
    [mode: 1B u8]
    [host_len: 2B u16] [host: N]
    [path_len: 2B u16] [path: N]
    [desc_len: 2B u16] [desc: N]
    [size: 8B i64]
[num_invalidated_by: 2B u16] [invalidated_by: 8B × N u64]
[invalidation_reason_len: 2B u16] [invalidation_reason: N]
[body_len: 4B u32] [body: N]
[milestone_class: 1B u8]
[milestone_kind: 1B u8]
[milestone_reason_len: 4B u32] [milestone_reason: N]
```

## Enum mappings

### NodeStatus (1 byte)

| Value | Status |
|-------|--------|
| 0 | active |
| 1 | done |
| 2 | paused |

### ClaimStatus (1 byte)

| Value | Status |
|-------|--------|
| 0 | provisional |
| 1 | validated |
| 2 | invalidated |
| 3 | superseded |

### Outcome (1 byte)

| Value | Outcome |
|-------|---------|
| 0 | unset |
| 1 | success |
| 2 | failure |
| 3 | inconclusive |

### ArtifactMode (1 byte)

| Value | Mode |
|-------|------|
| 0 | path |
| 1 | embedded |

## Index file (`nodes.idx`)

```json
{
  "1": {"offset": 8, "length": 245, "checksum": 1234567890},
  "2": {"offset": 253, "length": 180, "checksum": 987654321}
}
```

- `offset`: byte position from start of `nodes.bin` (after 8-byte header)
- `length`: payload length in bytes
- `checksum`: CRC32-IEEE of the payload

## Reading

1. Open `nodes.bin`, read 8-byte header, validate magic `RTND` and a supported version (`1` or `2`)
2. Read `nodes.idx` to get offset/length/checksum per NodeID
3. For each node: seek to offset, read length bytes, verify CRC32, decode

## Writing

1. Write 8-byte header (`RTND` + v2 + zeros)
2. For each node: encode, append to buffer, record offset/length/CRC32
3. Write buffer to `nodes.bin.tmp`, rename to `nodes.bin`
4. Write index to `nodes.idx.tmp`, rename to `nodes.idx`

## Extensibility

For future format versions:
- Change `binVersionCurrent` to 3, 4, etc.
- Maintain compatibility readers as long as relevant historical roots exist.
- `ReadBinHeader` rejects unsupported versions with `ErrUnsupportedSchema`.
- Data migration uses the JSON path as a universal intermediate format.
