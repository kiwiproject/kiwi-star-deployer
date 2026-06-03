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
