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

Clone the repository and build the binary:

```sh
git clone https://github.com/kiwiproject/kiwi-star-deployer
cd kiwi-star-deployer
go build -o kiwi-star-deployer .
```

Copy the binary to a directory on your PATH, for example:

```sh
cp kiwi-star-deployer /usr/local/bin/
```

To embed a version number in the binary:

```sh
go build -ldflags "-X github.com/kiwiproject/kiwi-star-deployer/cmd.version=1.0.0" \
  -o kiwi-star-deployer .
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
