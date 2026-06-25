# Retention & pruning

`siphon dumps prune` applies a retention policy to the dump catalog, deleting old
backups while guaranteeing it never orphans an incremental from its base. It
works against any storage backend (local or S3), since it prunes through the
same `Store.Delete` the catalog already uses.

## Table of contents

- [The chain is the unit](#the-chain-is-the-unit)
- [Policy rules](#policy-rules)
- [Configuration](#configuration)
- [The CLI](#the-cli)
- [Safety properties](#safety-properties)

## The chain is the unit

An incremental dump depends on its base (and any intermediate incrementals) to
restore. Retention therefore operates on **chains**, not individual dumps: a base
plus its incrementals is kept or deleted as a whole. A full backup is a
single-member chain. Counting and bucketing are done over chains, so "keep the
last 7" means seven restorable backups, not seven files.

A chain's age is the timestamp of its **newest** member, so a chain that is
still being appended to (an old base with a fresh incremental) is treated as
young and is never pruned mid-life.

## Policy rules

Three rules, each independently enableable. A chain is kept if it satisfies
**any** active rule (union) — adding a rule can only protect more chains, never
fewer, so a policy change can't silently delete data a rule meant to keep.

| Rule | Meaning |
| --- | --- |
| `keep_last: N` | keep the N newest chains |
| `max_age: <duration>` | keep chains younger than the duration (Go duration string, e.g. `720h`) |
| `gfs: {daily, weekly, monthly}` | grandfather-father-son: keep the newest chain in each of the most-recent N days / ISO weeks / months |

An **all-zero / omitted policy keeps everything** — prune becomes a no-op. The
destructive direction always requires explicit configuration.

## Configuration

Retention lives in a `retention:` block. Set a default for all profiles, and
optionally override per profile (the profile block **replaces** the default
block wholesale — it is not field-merged).

```yaml
defaults:
  retention:
    keep_last: 7
    max_age: 720h            # 30 days
    gfs: { daily: 7, weekly: 4, monthly: 6 }

profiles:
  prod:
    driver: postgres
    # ...
    retention:               # prod keeps more, independent of the default
      keep_last: 30
      gfs: { daily: 14, weekly: 8, monthly: 12 }
```

Precedence, highest first: **CLI flags → profile block → defaults block →
built-in (keep everything)**. An invalid block (negative count, unparseable
duration) fails fast at config load.

## The CLI

```bash
# Dry-run (default): show which chains the policy would prune, for one profile.
siphon dumps prune --profile prod

# Override the configured policy for this run, then actually delete.
siphon dumps prune --profile prod --keep-last 14 --apply

# Pure flag-driven policy (no config), GFS only.
siphon dumps prune --gfs-daily 7 --gfs-weekly 4 --gfs-monthly 6 --apply
```

`--apply` performs deletions; without it, prune only prints the plan. Flags
(`--keep-last`, `--max-age`, `--gfs-daily/-weekly/-monthly`) override the
config-derived policy, but only when explicitly set — an unset flag never zeroes
out a configured rule.

## Safety properties

- **Dry-run by default** — deletion requires an explicit `--apply`.
- **Keep-everything on empty** — a missing or all-zero policy prunes nothing.
- **Union semantics** — every rule can only protect; none can cause a surprise deletion.
- **Chain-atomic** — a base is only deleted when its whole chain is pruned, so an incremental is never left unrestorable.
- **Leaf-inward deletion** — within a pruned chain, incrementals are deleted before the base, so an interrupted prune (Ctrl-C, network blip) leaves at worst a complete shorter chain, never a base missing under a surviving incremental.
- **Collected failures** — a single dump that fails to delete is reported and the run exits non-zero, but the remaining chains are still pruned.
