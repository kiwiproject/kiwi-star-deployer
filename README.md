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

If a release has already gone wrong, see [RUNBOOK.md](RUNBOOK.md) for
recovery procedures instead of this document.

## Prerequisites

The following tools must be installed and on your PATH before running:

- `git`
- `gh` (GitHub CLI) version 2.40.0 or later, authenticated via `gh auth login`
- `mvn` (Maven)
- `.generate-kiwi-changelog` (the [kiwiproject changelog script](https://github.com/kiwiproject/kiwiproject-changelog))

Run `kiwi-star-deployer preflight` at any time to verify all prerequisites.

Every configured library's local clone must be on the `main` branch with no
uncommitted changes before `release` touches it; `main` is reset to match
`origin/main` first. There is no support for releasing from any other branch.

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

The tool reads a TOML config file. The config path is resolved in this order:

1. `--config <path>` flag
2. `KIWI_STAR_DEPLOYER_CONFIG` environment variable
3. `kiwi-star-deployer.toml` in the current directory (default)

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

# Per-library timeout for mvn release:prepare release:perform.
# Default: 1h
maven_release_timeout = "1h"

# How long to wait for a released artifact to appear in Maven Central.
# Default: 1h
maven_central_max_wait = "1h"

# How often to poll Maven Central while waiting.
# Default: 30s
maven_central_poll_interval = "30s"

# How long to wait for CI runs to appear and complete after a POM update push.
# Default: 30m
ci_max_wait = "30m"

# How often to poll GitHub Actions while waiting for CI to complete.
# Default: 30s
ci_poll_interval = "30s"

# Age in days after which run log directories are automatically deleted at
# the end of a successful release. 0 disables auto-purge.
# Default: 0
log_retention_days = 30


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
```

### Settings reference

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `workspace` | string | `~/.kiwi-star-deployer/workspace` | Directory for cloned repos |
| `state_path` | string | `~/.kiwi-star-deployer/state.json` | Release run state file |
| `group_id` | string | `org.kiwiproject` | Maven groupId |
| `changelog_script` | string | `.generate-kiwi-changelog` | Changelog script name or path |
| `maven_release_timeout` | duration | `1h` | Per-library timeout for mvn release:prepare release:perform |
| `maven_central_max_wait` | duration | `1h` | Max wait for Maven Central publication |
| `maven_central_poll_interval` | duration | `30s` | Poll interval for Maven Central |
| `ci_max_wait` | duration | `30m` | Max wait for CI runs to complete |
| `ci_poll_interval` | duration | `30s` | Poll interval for CI runs |
| `log_retention_days` | int | `0` | Auto-purge run logs older than this many days after each successful release; `0` disables auto-purge |

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
| `library-bom` | Library-managed BOM; at most one per config, always released last, and no library may list it in `depends_on` |

> [!WARNING]
> Every library whose POM is updated by this tool (all non-`parent-pom` types)
> must declare each kiwiproject dependency version as a property named exactly
> `<artifactId>.version` — for example `<kiwi-bom.version>3.2.0</kiwi-bom.version>`
> for artifactId `kiwi-bom`. The tool uses `mvn versions:set-property` to update
> these properties before release. If a dependency uses a literal version element
> or a differently-named property, the update will silently do nothing and the
> release will proceed with the stale version. The only exception is `parent-pom`
> dependencies, which are declared as a literal version in `<parent>` and updated
> via `mvn versions:use-dep-version`.

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

All commands accept a `--config <path>` flag. The config path can also be set via the `KIWI_STAR_DEPLOYER_CONFIG` environment variable. If neither is provided, `kiwi-star-deployer.toml` in the current directory is used.

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
| `--skip <lib>` | Treat a library as already released; requires `--resume` (repeatable). The version is resolved from the latest release tag and verified to exist on Maven Central |
| `--summary <libname=text>` | Prepend inline summary text to the changelog for a library (repeatable) |
| `--summary-file <libname=/path>` | Prepend the contents of a file as a summary to the changelog for a library (repeatable) |
| `--no-auto-skip` | Release every library, even ones with no changes since their last release (cannot be combined with `--resume`) |

Before touching anything, `release` runs the same checks as `preflight`
and `check-versions` and refuses to start if any fail (both are skipped
for `--dry-run`). In particular, every library's POM SNAPSHOT version
must have a matching open GitHub milestone. With `--only`, the milestone
check covers just the selected libraries, so an unrelated library's
mismatch cannot block a patch release that does not involve it. The
milestone check is skipped on `--resume`: a run that failed between
publishing and the changelog step legitimately has no next milestone
yet, and the resume itself creates it while recovering.

While computing the plan, both `plan` and `release` also validate the
config against each library's actual POM: any dependency in the POM
(parent, dependencies, or dependencyManagement) whose groupId matches
`group_id` and whose artifactId names another configured library must be
listed in that library's `depends_on`. A missing edge would silently
corrupt release ordering, so it fails the plan before anything is
released. This applies to the `library-bom` too, where the stakes are
highest: an artifact managed in the BOM's POM but missing from its
`depends_on` would never get its version property bumped, so the BOM
would be released pointing at a stale version.

Libraries with no changes since their last release are skipped
automatically: if a library's two most recent commits are the
maven-release-plugin commits from its previous release, there is nothing
new to release. A skipped library still has its Maven Central
availability verified, and if the GitHub release for its tag is missing
(a previous run failed between publishing and the changelog step), the
changelog step is run for the already-released version. Pass
`--no-auto-skip` to force every library to release regardless. Because
resume recovery depends on this auto-skip detection, `--no-auto-skip`
cannot be combined with `--resume`: a resumed run with it disabled would
release a new version on top of an unrecorded previous one. If a
`--no-auto-skip` run fails partway, resume it normally — forced releases
of still-unchanged libraries are skipped — and afterwards force just
those libraries with a fresh `release --no-auto-skip --only <libs>`.

The Maven release build itself runs with tests skipped
(`-Darguments=-DskipTests`); each repository's CI is the quality gate.

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

`--skip` asserts that the library's release already succeeded: the
version is taken from its latest release tag and must exist on Maven
Central, otherwise the run refuses to start. To recover a library whose
release *failed*, resume without `--skip` — auto-skip detection verifies
Maven Central and re-runs the changelog step as needed, which `--skip`
would bypass. For recovery procedures beyond a plain `--resume` — a
release that died before Maven Central confirmed it, or before
`release:prepare` even finished — see [RUNBOOK.md](RUNBOOK.md).

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

Prints the computed release stages, dependency ordering, and resolved versions.
Versions are read from each library's `pom.xml` as committed on `origin/main`:
repos missing from the workspace are cloned first and existing clones are
fetched, but local working copies are never modified and nothing is pushed or
released. Useful for verifying the dependency graph before a release.

Note that `plan` derives everything from POMs and deliberately does not check
GitHub milestones, so a clean plan can still be blocked by the milestone gate
when `release` runs. Run [check-versions](#check-versions) alongside `plan` to
verify milestone state ahead of time; `release` runs it automatically.

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
[PASS]  gh version
[PASS]  gh auth
[PASS]  mvn
[PASS]  .generate-kiwi-changelog

Note: GPG signing and Maven Central publishing credentials cannot be verified
in advance; mvn release:perform is the first step that exercises them. Ensure
gpg-agent and ~/.m2/settings.xml are configured before the first release.
```

---

### check-versions

Verifies that every configured library's POM SNAPSHOT version has a matching
open GitHub milestone. Both are read via the GitHub API, so nothing is cloned
and the workspace is not touched. The same check runs automatically at the
start of every `release`. A failure usually means a milestone was renamed,
closed early, or never created after a previous release.

```sh
kiwi-star-deployer check-versions
```

Example output:

```
[PASS]  kiwi-parent  3.0.16
[FAIL]  kiwi         5.3.0  pom says 5.3.0 but open milestones are: 5.4.0
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

---

### logs

Inspects and manages release run log directories.

```sh
kiwi-star-deployer logs list
kiwi-star-deployer logs purge --older-than 30d
```

`logs list` prints one line per run: the run ID, how many libraries completed,
and whether the run failed (and at which step). `logs purge` deletes run
directories older than the given age — Go durations like `24h` or day counts
like `30d` — after listing what will be deleted and prompting for
confirmation; pass `--yes` to skip the prompt.

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
pushes it, and waits for GitHub Actions CI to pass before starting the next
stage.

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

Set `log_retention_days` to automatically delete run directories older than
that many days at the end of each successful release, without prompting. It's
disabled by default (`0`); nothing is deleted unless you set it. Past runs can
be listed and manually purged with the [logs](#logs) command.

## Resetting to a clean slate

If the workspace or state file get into a confusing state, no special
command is needed — the tool's mutable state lives in exactly two places,
both plain files or directories:

- `workspace` (default `~/.kiwi-star-deployer/workspace`) — cloned repos
- `state_path` (default `~/.kiwi-star-deployer/state.json`) — run state

To start over:

```
rm -rf ~/.kiwi-star-deployer/workspace ~/.kiwi-star-deployer/state.json
```

(use your configured paths if you changed `workspace`/`state_path`)

Before deleting anything, run `kiwi-star-deployer status`. If it shows a
Failed entry, or the run otherwise looks incomplete, some libraries may be
only partially released — check the affected repos on GitHub first.
Deleting `state.json` loses the ability to `--resume`, and deleting
`workspace` can discard commits that only exist in the local clone and were
never pushed.

Log files are unaffected by this and are managed separately; see
[Log files](#log-files) above.
