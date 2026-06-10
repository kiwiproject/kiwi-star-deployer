# kiwi-star-deployer

A CLI tool that automates releasing kiwiproject Java and Kotlin libraries in
the correct dependency order, handling version propagation, parallel stages,
Maven Central verification, changelog generation, and failure recovery.

## Overview

kiwiproject libraries have interdependencies: releasing them manually in the
right order, bumping POM versions between stages, and verifying downstream CI
is tedious and error-prone. `kiwi-star-deployer` automates the full sequence:

1. Builds a dependency graph from your config and computes release stages
2. Releases all libraries in each stage in parallel using `mvn release:perform`
3. Waits for each library to appear in Maven Central
4. Generates a changelog and closes the GitHub milestone for each library
5. Updates downstream POM files and verifies CI passes before the next stage
6. Writes incremental state so a failed run can be resumed from where it left off

## Prerequisites

The following tools must be installed and on your PATH before running:

- `git`
- `gh` (GitHub CLI), authenticated via `gh auth login`
- `mvn` (Maven)
- `.generate-kiwi-changelog` (the [kiwiproject changelog script](https://github.com/kiwiproject/kiwiproject-changelog))

Run `kiwi-star-deployer preflight` at any time to verify all prerequisites.

## Installation

Clone the repository and install:

```sh
git clone https://github.com/kiwiproject/kiwi-star-deployer
cd kiwi-star-deployer
make install
```

This builds the binary with the version embedded from the current git tag and
copies it to `$(go env GOPATH)/bin` (typically `~/go/bin`). To install
elsewhere:

```sh
make install INSTALL_DIR=/usr/local/bin
```

Other useful targets:

```sh
make test     # run tests
make vet      # run go vet
make lint     # run golangci-lint
make check    # vet + test + lint (matches CI)
make help     # list all targets
```

## Configuration

The tool reads a TOML config file. By default it looks for `kiwi-star-deployer.toml`
in the current directory; use `--config <path>` to specify a different location.

### Full annotated example

```toml
[settings]
# Directory where library repositories are cloned.
# Default: ~/.kiwi-star-deployer/workspace
workspace = "~/.kiwi-star-deployer/workspace"

# Path to the JSON file that tracks release run state (used by --resume).
# Default: ~/.kiwi-star-deployer/state.json
state_path = "~/.kiwi-star-deployer/state.json"

# Maven groupId shared by all libraries.
# Default: org.kiwiproject
group_id = "org.kiwiproject"

# Path or name of the changelog generation script.
# Default: .generate-kiwi-changelog
changelog_script = ".generate-kiwi-changelog"

# How long to wait for a released artifact to appear in Maven Central.
# Default: 1h
maven_central_max_wait = "1h"

# How often to poll Maven Central while waiting.
# Default: 30s
maven_central_poll_interval = "30s"

# Whether to verify GitHub Actions CI passes after each downstream POM update push.
# Set to false to skip CI verification entirely.
# Default: true
ci_verify = true

# How long to wait for CI runs to appear and complete after a POM update push.
# Default: 30m
ci_max_wait = "30m"

# How often to poll GitHub Actions while waiting for CI to complete.
# Default: 30s
ci_poll_interval = "30s"


# Each library to be released is declared as [library.<name>].
# The name is used as the Maven artifactId.

[library.kiwi-parent]
repo = "kiwiproject/kiwi-parent"   # GitHub repository (required)
type = "parent-pom"                 # optional; see Library types below

[library.kiwi-bom]
repo = "kiwiproject/kiwi-bom"
type = "bom"
depends_on = ["kiwi-parent"]       # releases after kiwi-parent; POM updated automatically

[library.kiwi]
repo = "kiwiproject/kiwi"
depends_on = ["kiwi-parent", "kiwi-bom"]

# kiwi-libraries-bom aggregates all library versions, so it depends on everything
# and is released last.
[library.kiwi-libraries-bom]
repo = "kiwiproject/kiwi-libraries-bom"
type = "library-bom"               # at most one library may have this type
depends_on = ["kiwi-parent", "kiwi-bom", "kiwi"]


# Version overrides force a specific release version regardless of what is in the POM.
# Useful when the next POM version would compute incorrectly (e.g. after a hotfix branch).
# Values must be in X.Y.Z format.

[release.overrides]
kiwi = "5.3.1"
```

### Settings reference

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `workspace` | string | `~/.kiwi-star-deployer/workspace` | Directory for cloned repos |
| `state_path` | string | `~/.kiwi-star-deployer/state.json` | Release run state file |
| `group_id` | string | `org.kiwiproject` | Maven groupId |
| `changelog_script` | string | `.generate-kiwi-changelog` | Changelog script name or path |
| `maven_central_max_wait` | duration | `1h` | Max wait for Maven Central publication |
| `maven_central_poll_interval` | duration | `30s` | Poll interval for Maven Central |
| `ci_verify` | bool | `true` | Verify CI after each POM update push |
| `ci_max_wait` | duration | `30m` | Max wait for CI runs to complete |
| `ci_poll_interval` | duration | `30s` | Poll interval for CI runs |

Duration values use Go duration syntax: `30s`, `5m`, `1h30m`.

### Library fields

| Field | Required | Description |
|-------|----------|-------------|
| `repo` | yes | GitHub repository in `owner/repo` format |
| `type` | no | Library type (see below) |
| `depends_on` | no | Names of libraries this one depends on |

### Library types

| Type | Description |
|------|-------------|
| (unset) | Regular library |
| `parent-pom` | Maven parent POM |
| `bom` | Bill of Materials POM |
| `library-bom` | Library-managed BOM; at most one per config |

> [!WARNING]
> A `library-bom` POM must declare each managed dependency's version as a
> property named exactly `<artifactId>.version` — for example
> `<kiwi.version>5.3.1</kiwi.version>` for artifactId `kiwi`. The tool uses
> `mvn versions:set-property` to update these properties before release. If a
> dependency uses a literal version element instead of a property, the update
> will silently do nothing and the release will proceed with the stale version.

## Changelog tool configuration

`kiwi-star-deployer` invokes the [kiwiproject changelog script](https://github.com/kiwiproject/kiwiproject-changelog)
after each library release. The script automatically picks up user preferences
from `~/.kiwi-changelog.yml`.

At minimum, set these two keys:

```yaml
useTagDateForRelease: true
addMilestoneLink: true
```

`useTagDateForRelease: true` stamps the changelog entry with the date of the
release tag rather than the date the changelog was generated, which gives
accurate dates when releasing multiple libraries in sequence.

`addMilestoneLink: true` links each changelog entry to its GitHub milestone,
making it easy to navigate from a release to the full set of issues it closed.

### Category configuration

The changelog script groups issues by label into categories. Configure the
mapping to match your label taxonomy. A full working example for kiwiproject
is available in the repository as
[sample-kiwi-changelog.yml](https://github.com/kiwiproject/kiwiproject-changelog/blob/main/sample-kiwi-changelog.yml).

A minimal `~/.kiwi-changelog.yml` looks like:

```yaml
useTagDateForRelease: true
addMilestoneLink: true

categories:
  - title: Breaking Changes
    labels:
      - API change
  - title: New Features
    labels:
      - new feature
  - title: Bug Fixes
    labels:
      - bug
  - title: Improvements
    labels:
      - enhancement
  - title: Dependency Updates
    labels:
      - dependencies
    emoji: 📦
  - title: Other Changes
    default: true
    labels:
      - documentation
      - chore
```

## Commands

All commands accept a `--config <path>` flag (default: `kiwi-star-deployer.toml`).

### release

Releases all libraries in dependency order.

```sh
kiwi-star-deployer release [flags]
```

| Flag | Description |
|------|-------------|
| `--dry-run` | Print the release plan without executing any steps |
| `--only <libs>` | Release only the named libraries (comma-separated or repeated) |
| `--resume` | Resume a previously failed run, skipping already-completed libraries |
| `--skip <lib>` | Treat a library as already released; requires `--resume` (repeatable) |
| `--summary <libname=text>` | Prepend inline summary text to the changelog for a library (repeatable) |
| `--summary-file <libname=/path>` | Prepend the contents of a file as a summary to the changelog for a library (repeatable) |
| `--interactive` | Pause for confirmation between stages; `--interactive=step` additionally pauses before and after CI verification within each stage transition (the stage-level prompt still fires too) |

**Examples**

Preview what would be released without doing anything:

```sh
kiwi-star-deployer release --dry-run
```

Release only specific libraries (patch release):

```sh
kiwi-star-deployer release --only kiwi-parent,kiwi-bom
```

Resume after a failure, treating a manually-released library as done:

```sh
kiwi-star-deployer release --resume --skip kiwi-test
```

Step through a release interactively, confirming each stage:

```sh
kiwi-star-deployer release --interactive
```

Step through with finer control, pausing before and after CI verification too:

```sh
kiwi-star-deployer release --interactive=step
```

Stopping at any prompt exits cleanly and preserves state so `--resume` can pick up where you left off. `--interactive` has no effect with `--dry-run`. When `ci_verify = false`, `--interactive=step` behaves the same as `--interactive=stage` since there are no CI steps to pause at.

Major release with per-library changelog summaries:

```sh
kiwi-star-deployer release \
  --summary kiwi="This release removes deprecated APIs from 4.x. See migration guide for details." \
  --summary kiwi-bom="Tracks kiwi 5.0.0."
```

Or using summary files:

```sh
kiwi-star-deployer release \
  --summary-file kiwi=/path/to/kiwi-summary.txt \
  --summary-file kiwi-bom=/path/to/kiwi-bom-summary.txt
```

---

### plan

Prints the computed release stages, dependency ordering, and resolved versions
without cloning repos or making any changes. Useful for verifying the
dependency graph before a release.

```sh
kiwi-star-deployer plan
```

Example output:

```
Stage 1:  kiwi-parent  2.9.0-SNAPSHOT -> 2.9.0  (next: 2.10.0-SNAPSHOT)
Stage 2:  kiwi-bom     1.3.0-SNAPSHOT -> 1.3.0  (next: 1.4.0-SNAPSHOT)
          kiwi         5.3.0-SNAPSHOT -> 5.3.0  (next: 5.4.0-SNAPSHOT)
Stage 3:  kiwi-libraries-bom  2.1.0-SNAPSHOT -> 2.1.0  (next: 2.2.0-SNAPSHOT)
```

---

### preflight

Verifies that all required tools are installed, on your PATH, and (for `gh`)
authenticated. Run this before your first release to catch configuration
problems early.

```sh
kiwi-star-deployer preflight
```

Example output:

```
[PASS]  git
[PASS]  gh
[PASS]  gh auth
[PASS]  mvn
[PASS]  .generate-kiwi-changelog
```

---

### status

Displays the state of the current or most recent release run: which libraries
completed, at what version and time, and which library failed (if any).

```sh
kiwi-star-deployer status
```

Example output:

```
Run: 2024-11-15T14:30:00Z

Completed:
  kiwi-parent          2.9.0  2024-11-15T14:31:22Z
  kiwi-bom             1.3.0  2024-11-15T14:45:10Z

Failed:
  library:  kiwi
  step:     maven-release
  error:    exit status 1
```

## How stages work

`kiwi-star-deployer` builds a directed acyclic graph from the `depends_on`
declarations in your config and performs a topological sort to determine
release order. Libraries with no dependencies on each other are grouped into
the same stage and released in parallel; libraries that depend on the output
of a previous stage wait until that stage completes.

For example, given this dependency graph:

```
kiwi-parent
    ├── kiwi-bom
    └── kiwi (depends on kiwi-parent and kiwi-bom)
            └── kiwi-libraries-bom (depends on all three)
```

The tool computes three stages:

```
Stage 1:  kiwi-parent
Stage 2:  kiwi-bom, kiwi          (parallel — neither depends on the other)
Stage 3:  kiwi-libraries-bom
```

After each stage completes, the tool updates the `pom.xml` of every library
in future stages that depends on a just-released library, commits the change,
pushes it, and (if `ci_verify = true`) waits for GitHub Actions CI to pass
before starting the next stage.

If any library in a stage fails, the entire run halts. Use `kiwi-star-deployer status`
to see what completed, then `kiwi-star-deployer release --resume` to pick up
where it left off.

## Log files

Each `release` run creates a timestamped directory under `<parent of workspace>/logs/`:

```
~/.kiwi-star-deployer/logs/
└── 2024-11-15T14-30-45/
    ├── kiwi-parent.log
    ├── kiwi-bom.log
    ├── kiwi.log
    ├── kiwi-libraries-bom.log
    └── kiwi-libraries-bom-pom-update.log
```

Each library gets one log file capturing all output from its `mvn release:perform`,
Maven Central verification, and changelog steps. POM update commits get a
separate `<library>-pom-update.log` file. Log files are written in real time,
so you can `tail -f` them during a long release run.
