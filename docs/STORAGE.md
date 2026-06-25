# Storage backends

By default siphon keeps the dump catalog — every dump body plus its sidecar
metadata — in a local directory. Phase G adds a pluggable storage layer so the
catalog can instead live in an S3 (or S3-compatible) bucket, with `backup`,
`restore`, `verify`, and `dumps` all reading and writing through it transparently.

## Table of contents

- [How it works](#how-it-works)
- [Configuration](#configuration)
- [Integrity across backends](#integrity-across-backends)
- [Scope and limitations](#scope-and-limitations)

## How it works

The dump catalog (`internal/dumps`) holds a `storage.Store` and addresses
objects by opaque keys — `<id>.dump` for the envelope-prefixed dump body and
`<id>.meta.json` for the sidecar — never by filesystem path. The `Store`
interface is small and backend-neutral:

```go
type Store interface {
    Put(ctx, key, io.Reader) error                 // durable, atomic-on-complete
    Get(ctx, key) (io.ReadCloser, error)           // one-shot forward stream
    Delete(ctx, key) error                          // idempotent
    List(ctx) ([]string, error)
    Stat(ctx, key) (size int64, exists bool, err error)
}
```

Two backends ship today (`internal/storage`):

- **local** — a single directory. `Put` writes to a temp file and renames, so a
  failed or cancelled write never leaves a partial dump under its final key.
  Keys map verbatim to file names, so a pre-Phase-G local catalog keeps working
  with no migration.
- **s3** — an S3 or S3-compatible bucket (AWS, MinIO, Cloudflare R2). `Put`
  streams through the SDK's transfer manager, so the object only becomes
  addressable once the upload completes — the same atomic-on-complete guarantee
  the local backend gets from rename.

A backup stages the dump body to a local temp file (the dump tool needs a real
fd), then streams `envelope ++ body` into the store in a single `Put`, teeing
through SHA-256 as the bytes flow. Restore and verify open the dump with `Get`
and stream it straight into the envelope reader — no full local download.

## Configuration

Storage is selected by a top-level `storage:` block in the config file. Omitting
it (or `type: local`) uses the local filesystem at `defaults.dump_dir`.

```yaml
version: 1
defaults:
  dump_dir: ~/.local/share/siphon/dumps   # used by the local backend

storage:
  type: s3                # "local" (default) | "s3"
  bucket: my-siphon-dumps # required for s3
  prefix: prod            # optional key prefix within the bucket
  region: us-east-1
  endpoint: ""            # optional: custom endpoint for S3-compatible services
```

For an S3-compatible service such as MinIO or R2, set `endpoint` to its URL
(path-style addressing is used automatically).

**Credentials are never stored in the config file.** The S3 backend resolves
them from the standard AWS chain — `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY`
environment variables, the shared `~/.aws/config`, or an instance/role profile —
so the config stays safe to commit.

## Integrity across backends

The SHA-256 checksum is computed over the `envelope ++ body` stream at write
time and recomputed over the `Get` stream at verify time — both in siphon, not
in the backend. A dump written to S3 and a dump written locally therefore verify
identically, and `siphon verify` catches corruption regardless of where the dump
lives. siphon does not trust the backend's own ETag/MD5 (multipart uploads do
not expose a plain object MD5).

The live S3 path is integration-tested in CI against MinIO via testcontainers
(`internal/storage/s3_integration_test.go`) using the same `RunStoreSuite`
contract that the local backend runs, so the streaming upload, ranged read,
listing, and not-found mapping all execute against a real object store — not
just compile.

## Scope and limitations

- Backends covered: **local** and **s3** (incl. S3-compatible). **GCS and Azure
  Blob are not implemented yet** — they are a fast-follow, and will get the full
  correctness bar for free by running the same `RunStoreSuite` contract.
- `dumps list` over S3 issues one `ListObjectsV2` plus one read per metadata
  object (N+1). This is fine at expected catalog sizes; no pagination/parallel
  optimization is done yet.
- Each `Get` is a fresh one-shot forward stream — callers must not assume the
  returned reader is seekable.
- Retention/lifecycle (chain-aware pruning over remote storage) remains a
  separate Phase G concern; the `Store.Delete` it needs is delivered here.
