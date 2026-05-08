# kiwi-star-deployer — Design Document

## Purpose

`kiwi-star-deployer` is a Go CLI tool that automates the release of kiwiproject libraries
in the correct dependency order, handling version propagation, parallel stages, and failure
recovery. It orchestrates the existing release tooling rather than replacing it.

Repository: `kiwiproject/kiwi-star-deployer`

## Background

The kiwiproject organization maintains a suite of interdependent Java/Kotlin libraries
released to Maven Central. Releasing them manually requires:

1. Releasing libraries in the correct dependency order
2. Updating downstream library POMs to reference newly released versions
3. Running the changelog/release-notes tool for each library
4. Handling `kiwi-parent` and `kiwi-bom` as root dependencies that affect everything
5. Updating `kiwi-libraries-bom` last as an aggregator of all library versions

This is done every few months for a full release, or on-demand for CVE patches (which
typically only require releasing `kiwi-parent` and `kiwi-bom`).

## Prerequisites

The tool assumes the following are installed and configured on the host machine:

- **`gh` (GitHub CLI)** — installed and authenticated (`gh auth login`). All GitHub
  interactions and repo cloning flow through `gh`. Run `gh auth status` to verify before
  a release run. The tool itself runs this as a pre-flight check at startup.
- **`git`** — standard git, used for all working-copy operations after initial clone
- **`mvn`** — Maven, on the PATH, with appropriate `settings.xml` for Maven Central publishing
- **`kiwiproject-changelog`** wrapper script — installed locally per its `etc/install.sh`

No other credentials or configuration are required — `gh`'s auth context covers all
GitHub API and git remote operations for workspace clones (see Workspace section).

## Existing Tooling (v1 wraps these)

Each library repo contains a `maven-central-deploy.sh` script — a wrapper around the
Maven Release Plugin that blocks until the artifact is published to Maven Central
(configured in `kiwi-parent` to wait up to 30 minutes).

The Maven Release Plugin must be invoked in **batch mode** (`-B`) to suppress interactive
prompts, with the release and next development versions supplied explicitly:

```
mvn -B release:prepare release:perform \
  -DreleaseVersion=2.5.0 \
  -DdevelopmentVersion=2.5.1-SNAPSHOT
```

The existing `maven-central-deploy.sh` scripts will need to be updated to support this,
or the tool invokes Maven directly rather than through the script. The latter gives the
tool more control and avoids needing to modify every repo's script.

After each release, the `kiwiproject-changelog` tool is run to generate release notes,
create a GitHub release, and optionally close the current milestone and create the next one.

`kiwi-star-deployer` v1 orchestrates these two scripts. It does not reimplement them.

## Release Modes

### Full Release

All libraries released in topological order, with version propagation across the graph.
Used for the regular every-few-months release cycle.

### Patch Release

One or more specific libraries released without cascading through the full graph.
Used primarily for CVE responses (e.g. update `kiwi-parent` + `kiwi-bom` only).
Consumers (production services) pick up the new BOM version independently.

```
kiwi-star-deployer release --only kiwi-parent,kiwi-bom
```

## Configuration File

A TOML config file declares the library graph. The dependency order is explicit rather
than dynamically discovered from POMs (POM dependency resolution is complex, and explicit
config is easier to reason about and audit).

```toml
[library.kiwi-parent]
repo = "kiwiproject/kiwi-parent"
type = "parent-pom"
depends_on = []

[library.kiwi-bom]
repo = "kiwiproject/kiwi-bom"
type = "bom"
depends_on = ["kiwi-parent"]

[library.kiwi]
repo = "kiwiproject/kiwi"
depends_on = ["kiwi-parent", "kiwi-bom"]

[library.kiwi-test]
repo = "kiwiproject/kiwi-test"
depends_on = ["kiwi-parent", "kiwi-bom", "kiwi"]

[library.dropwizard-consul]
repo = "kiwiproject/dropwizard-consul"
depends_on = ["kiwi-parent", "kiwi-bom", "kiwi"]

# ... etc.

[library.kiwi-libraries-bom]
repo = "kiwiproject/kiwi-libraries-bom"
type = "bom-aggregator"
depends_on = ["kiwi"]   # logically depends on everything; released last
```

### Key config concepts

- `depends_on` lists only **internal** kiwiproject dependencies — external dependencies
  like Dropwizard itself are managed via `kiwi-bom` and don't appear here
- `type = "bom-aggregator"` marks `kiwi-libraries-bom` for special handling (see below)
- `type = "parent-pom"` / `type = "bom"` may warrant special handling for `kiwi-parent`
  and `kiwi-bom` (e.g. they don't have downstream POM version updates in the same way)

## Workspace

The tool maintains a dedicated workspace directory for all clones and release operations,
completely separate from the developer's own working copies of the repos. This is essential
because:

- Dev repos may have uncommitted changes, stashed work, or be on a feature branch
- The Maven Release Plugin creates commits and tags that would appear mixed in with normal work
- Parallel releases within a stage require independent working copies
- A clean, known-good state is required for `release:rollback` to work correctly if needed

### Workspace Location

Configurable in the config file, defaulting to `~/.kiwi-star-deployer/workspace/`.

```toml
[settings]
workspace = "~/.kiwi-star-deployer/workspace"
```

### Cloning

Initial clones use `gh repo clone`, which configures the remote URL with `gh`'s credential
helper. All subsequent `git` operations in those working copies (fetch, commit, push, tags)
use that same credential context automatically — no separate git credentials are needed.
The Maven Release Plugin's git operations also go through the same credential helper since
they use the working copy's configured remote.

```
gh repo clone kiwiproject/kiwi ~/.kiwi-star-deployer/workspace/kiwi
```

### Persistent vs. Fresh Clones

The workspace is persistent across runs rather than re-cloned every time (cloning all repos
adds unnecessary time). At the start of each library's release step, the tool verifies the
working copy is on the default branch with no uncommitted changes, then does a `git fetch`
and reset before proceeding. If the working copy is in an unexpected state the tool halts
rather than trying to repair it.

## Version Determination

The Maven Release Plugin requires explicit release and next development versions when run
in batch mode. The tool determines these as follows:

### Default: Derive from Current POM SNAPSHOT Version

The current POM version is the authoritative source. If the POM reads `2.5.1-SNAPSHOT`,
the release version is `2.5.1` and the next development version is `2.5.2-SNAPSHOT`.
This is deterministic and requires no external lookups.

GitHub Milestones are intentionally **not** used as a source of truth — multiple milestones
may exist (next patch, next minor, next major), they may not be in sync with the POM, and
inferring intent from milestone names programmatically is fragile.

### Override: Explicit Version in Config

For cases where a minor or major bump is intended, the config file (or a separate
release-specific override file) accepts explicit version overrides:

```toml
[release.overrides]
kiwi = "3.0.0"      # major bump this cycle
kiwi-test = "2.6.0" # minor bump
# all other libraries use the default patch derivation
```

The next development version after any release is always patch+1-SNAPSHOT (e.g.
`3.0.0` → `3.0.1-SNAPSHOT`), regardless of whether the release was a major, minor,
or patch bump.

### Pre-flight Version Plan

The `plan` command and `--dry-run` flag both display the resolved versions for every
library before any release is triggered, allowing review of the full version bump plan
as a unit before committing to it:

```
Stage 1:  kiwi-parent        3.0.33-SNAPSHOT  →  3.0.33  (next: 3.0.34-SNAPSHOT)
Stage 2:  kiwi-bom           2.2.9-SNAPSHOT   →  2.2.9   (next: 2.2.10-SNAPSHOT)
Stage 3:  kiwi               4.1.0-SNAPSHOT   →  4.1.0 [OVERRIDE] (next: 4.1.1-SNAPSHOT)
          kiwi-test          3.2.4-SNAPSHOT   →  3.2.4   (next: 3.2.5-SNAPSHOT)
...
```

## Release Graph Execution

### Topological Sort + Parallel Stages

Libraries are grouped into stages by topological sort of the dependency graph. Libraries
within the same stage have no dependencies on each other and can be released in parallel.
Go's concurrency model (goroutines + channels) maps naturally to this.

Example stages for a full release:
```
Stage 1: kiwi-parent
Stage 2: kiwi-bom                          (depends on kiwi-parent)
Stage 3: kiwi, kiwi-test, ...              (depend on kiwi-bom, no inter-dependencies)
Stage 4: dropwizard-consul, kiwi-spring, . (depend on kiwi)
Stage 5: kiwi-libraries-bom               (released last, always)
```

### Per-Library Release Flow

For each library:

1. **Prepare workspace** — verify working copy is on default branch and clean, `git fetch`
   and reset to latest (clone first if not yet present in workspace)
2. **Resolve versions** — determine release version and next development version
   (from POM snapshot version, with any config overrides applied)
3. **Run Maven release** — invoke `mvn -B release:prepare release:perform` with explicit
   `-DreleaseVersion` and `-DdevelopmentVersion`; blocks until published or times out
   (up to 30 min, configured in `kiwi-parent`)
4. **Verify in Maven Central** — single check after Maven returns as a sanity check;
   enters a polling loop only if Maven timed out (see Failure Handling)
5. **Run `kiwiproject-changelog`** — generates release notes, creates GitHub release,
   closes milestone, creates next milestone
6. **Update downstream POMs** — for each library in subsequent stages that lists this
   library in its `depends_on`, update the dependency version in its POM and commit to
   the default branch

Step 6 happens *after* Maven Central verification, before the next stage begins.

### POM Version Updates

The tool makes commits directly to the default branch of downstream repos with a consistent
commit message, e.g.:

```
chore: update kiwi dependency to 2.5.0
```

This bypasses the normal PR review process during a release run, which is acceptable given
releases are intentional, controlled operations. A future option could auto-create PRs instead,
but that adds latency and human approval gates between stages.

### `kiwi-libraries-bom` Special Handling

`kiwi-libraries-bom` is not a regular library — nothing depends on it at build time.
Its release step is different: rather than updating a `<dependency>` version, it updates
a curated list of all kiwiproject library versions it manages, then releases.

It is always released last, after all other libraries in the graph.

## Failure Handling

### State File

A JSON state file is written after each successfully completed library release, recording:
- Which libraries have been released in this run
- The version released
- Timestamp
- Which step completed last

```json
{
  "run_id": "2025-11-15T14:30:00Z",
  "completed": [
    { "library": "kiwi-parent", "version": "3.1.0", "completed_at": "2025-11-15T14:31:22Z" },
    { "library": "kiwi-bom",    "version": "2.4.0", "completed_at": "2025-11-15T14:45:10Z" }
  ],
  "failed": {
    "library": "kiwi",
    "step": "maven-central-deploy",
    "error": "timeout after 30 minutes"
  }
}
```

### Failure Modes

| Failure | Behaviour |
|---|---|
| `maven-central-deploy.sh` times out | Enter polling loop against Maven Central; halt after configurable max wait |
| Maven Central verify fails after successful deploy | Halt and report — may indicate a transient issue |
| `kiwiproject-changelog` fails | Halt — GitHub API issues or missing milestone; can be re-run manually |
| POM update commit fails | Halt — permissions issue or unexpected conflict |
| Downstream build fails due to upstream incompatibility | Halt — this is a substantive problem requiring investigation |

The tool always halts on failure. It does not attempt to silently skip or continue.

### Resume and Skip

```
# Resume from the last failed step, skipping already-completed libraries
kiwi-star-deployer release --resume

# Skip a specific library (e.g. you fixed it manually) and continue
kiwi-star-deployer release --resume --skip kiwi-test
```

The `--resume` flag reads the state file to determine where to continue.

## CLI Interface (sketch)

```
kiwi-star-deployer release                              # full release
kiwi-star-deployer release --only kiwi-parent,kiwi-bom # patch release
kiwi-star-deployer release --resume                     # resume after failure
kiwi-star-deployer release --resume --skip kiwi-test
kiwi-star-deployer release --dry-run                    # show plan without executing

kiwi-star-deployer plan                                 # print release stages, order, and resolved versions
kiwi-star-deployer status                               # show state file for current/last run
kiwi-star-deployer preflight                            # verify gh auth, mvn, git, changelog script
```

Every `release` invocation runs `preflight` checks automatically before touching anything.

## Implementation Notes (Go)

- The dependency graph + topological sort is straightforward to implement cleanly in Go
- Parallel stage execution uses goroutines with a `sync.WaitGroup` or `errgroup`
- Each library's release steps run sequentially within their goroutine
- **POM version updates** are handled entirely by the Maven Versions Plugin — no XML
  parsing or string manipulation in Go. The tool constructs and invokes the appropriate
  `mvn versions:*` commands:
  - `mvn versions:set -DnewVersion=2.5.0 -DgenerateBackupPoms=false` — sets the
    project's own version before release
  - `mvn versions:use-dep-version -Dincludes=org.kiwiproject:kiwi -DdepVersion=4.1.0 -DgenerateBackupPoms=false`
    — updates a specific kiwiproject dependency version in a downstream POM
  - `-DgenerateBackupPoms=false` is required on all calls to prevent `pom.xml.versionsBackup`
    files accumulating in the workspace
  - Note: verify that `versions:use-dep-version` is available in the Versions Plugin
    version declared in `kiwi-parent`; if not, `versions:set-property` with `<properties>`
    blocks for dependency versions is the fallback
- **External command execution** — `gh`, `git`, and `mvn` are all invoked via `os/exec`;
  wrap these behind interfaces from the start to keep the logic testable
- **`gh`** — used for initial workspace clones and any direct GitHub API calls the tool
  needs to make itself (e.g. verifying a release was created)
- **`git`** — used for all working-copy operations (fetch, reset, commit, push) after
  the initial clone; credentials flow transparently through `gh`'s credential helper
- **Maven Central verification** — simple HTTP GET against the repository URL or
  Maven Central search API; no additional library needed
- Single distributable binary, no JVM dependency — convenient for running on any dev machine

## v1 Scope vs. Future

**v1 includes:**
- Config file parsing and graph validation
- Full release mode with topological ordering and parallel stages
- Patch release mode (`--only`)
- Per-library flow: `maven-central-deploy.sh` + Maven Central verify + changelog script + POM updates
- State file, `--resume`, `--skip`
- `--dry-run` showing the release plan
- `kiwi-libraries-bom` special handling

**Future / out of scope for v1:**
- Updating production service repos to consume the new `kiwi-libraries-bom` version
- Generic hook system for non-kiwiproject use cases
- Web UI or dashboard
- Slack/email notifications
