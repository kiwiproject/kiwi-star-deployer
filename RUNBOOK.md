# kiwi-star-deployer — Recovery Runbook

This document is for when a release has already gone wrong: the process
died, a step failed, or the state on disk and the state on GitHub/Maven
Central no longer agree. For how to run a release normally, see
[README.md](README.md).

Some of the scenarios below are now explained inline in the error message
you'll actually see — this document exists for the cases where the tool
can only tell you *what* is wrong, not the full recipe for fixing it, and
for the couple of cases it can't detect at all.

## Start here: diagnose before acting

Run:

```sh
kiwi-star-deployer status
```

If the last run didn't finish cleanly, this prints a `Failed:` block with
the library, the step it failed at, and the error. Use the step to find
the right scenario below:

| Failed step          | What it means                                                                  |
| --------------------- | ------------------------------------------------------------------------------- |
| `maven-release`       | Died during `release:prepare`/`release:perform` itself — [Scenario 2](#scenario-2-originmain-left-on-a-non-snapshot-version) |
| `maven-central-verify` | The release was pushed, but whether it reached Central is unconfirmed — [Scenario 1](#scenario-1-release-pushed-but-maven-central-status-unknown) |
| `changelog`            | Release confirmed on Central, changelog/GitHub-release/milestone step didn't finish — [Scenario 4](#scenario-4-milestone-left-stale-after-a-partial-changelog-run) |
| `pom-update` / `ci-verify` | A downstream library's dependency bump or its CI failed — not a release failure for the library itself; fix the downstream repo (or its CI) and run `--resume`, which re-verifies the new commit |

## Scenario 1: release pushed, but Maven Central status unknown

**Symptom:** `release --resume` fails at `maven-central-verify`. Since this
is the auto-skip recovery path — the tool found the release commits and
tag already pushed from a previous run — the error itself now says:

> auto-skip found release tag vX already pushed; check `<artifact URL>` to
> tell which recovery applies: if the artifact is still being published by
> Maven Central, wait and run release --resume again — consider raising
> maven_central_max_wait — but if the earlier run died before
> release:perform uploaded anything, deploy vX manually from its tag and
> then run release --resume to finish the changelog and GitHub release

**Steps:**

1. Open the artifact URL from the error in a browser (or construct it
   yourself: `https://repo1.maven.org/maven2/<group-path>/<artifactId>/<version>/`,
   with dots in the groupId replaced by slashes).
2. **Artifact is listed** — Central is just slow to finish publishing.
   Wait, then run `release --resume` again. If this keeps happening,
   raise `maven_central_max_wait` in the config.
3. **Artifact is not listed** — it was never uploaded. Deploy it manually:
   - `cd <workspace>/<library>` (the persistent clone directory)
   - If `release.properties` is still present there (it's untracked, so
     it survives unless something removed it), the Maven Release Plugin
     can finish the job it started:
     ```sh
     mvn release:perform -Darguments=-DskipTests
     ```
   - If `release.properties` is missing, deploy the already-tagged
     commit directly instead:
     ```sh
     git checkout v<version>
     mvn -B clean deploy -DskipTests
     git checkout main
     ```
4. Run `kiwi-star-deployer release --resume`. Auto-skip will re-verify
   Central (should now pass) and finish the changelog, GitHub release,
   and milestone steps automatically.

## Scenario 2: origin/main left on a non-SNAPSHOT version

**Symptom:** `plan` or `release` fails immediately, before doing anything,
with an error that a library's POM version is not a SNAPSHOT. Because
`plan` always builds the whole release graph up front, this blocks every
library, not just the affected one.

**Why:** `release:prepare` commits and tags the release version, then
commits the next development SNAPSHOT version, then pushes both. If the
process dies after only the first commit is pushed, `origin/main`'s HEAD
is the release commit itself — a non-SNAPSHOT version with no follow-up
commit.

**Steps:**

1. Check whether the tag was also pushed: `git ls-remote --tags origin
   | grep v<version>` in the library's repo. If it exists, the release
   itself may or may not have reached Central — see Scenario 1 once this
   step is resolved.
2. Complete what `release:prepare` would have done: push a commit to
   `origin/main` that bumps the POM's `<version>` to the next development
   SNAPSHOT (a patch bump — `X.Y.(Z+1)-SNAPSHOT` — unless this cycle was
   an intentional minor/major bump, in which case use the version you
   intended).
3. Follow Scenario 1's steps to confirm the release itself was deployed.
4. Run `release --resume`.

## Scenario 3: `--skip` vs. plain `--resume`

For a library that failed at `maven-central-verify` or `changelog`,
prefer plain `--resume` over `--resume --skip <lib>`. Auto-skip runs the
full recovery (Central re-check, GitHub release, changelog, milestones);
`--skip` only records the library as done — it verifies the version
against Maven Central first, so it can no longer silently accept a
library that was never released, but it still skips the changelog and
GitHub-release recovery that plain `--resume` would have done. See the
[`--skip` documentation](README.md#release) for the mechanics.

## Scenario 4: milestone left stale after a partial changelog run

**Symptom:** on a later run, `check-versions` (or the release gate)
reports a mismatch for a library that was actually already released: the
POM is at the released version, but no open milestone matches it.

**Why:** the changelog step closes the current milestone, creates the
next one, and generates the GitHub release in one invocation. If it dies
partway, the GitHub release may already exist — so the auto-skip recovery
path sees it and reports success — while the milestones are left stale.

**Steps:**

1. On GitHub, find the milestone matching the version that was just
   released (for example `2.5.1`) and close it if it's still open.
2. Create the next milestone if it doesn't already exist, titled with the
   next patch version only (for example `2.5.2` — no `v` prefix, no other
   text; this must match exactly what `check-versions` compares against).
3. Run `check-versions` to confirm it now passes for that library.

## Scenario 5: redoing a release

Prefer rolling forward. Maven Central is immutable, so once a version is
actually deployed, releasing "again" at the same version isn't possible —
the next normal release cycle picks up any additional fix commits.

A forced same-version redo only makes sense when the version was never
actually deployed (tagged, maybe even changelog'd, but the Central upload
itself failed or never ran):

1. Delete the GitHub release, if one exists:
   `gh release delete v<version> --repo <org/repo>`
2. Reopen the milestone that was closed for this version.
3. Delete the remote tag: `git push origin :refs/tags/v<version>`
4. Delete the local workspace clone rather than trying to reset it — a
   plain `git fetch` does not remove local tags after the matching remote
   tag is gone, so a stale local tag would keep confusing the tool:
   ```sh
   rm -rf <workspace>/<library>
   ```
   It is re-cloned automatically on the next run.
5. Run `release` (or `release --resume`, if other libraries in the same
   run already completed).

## Scenario 6: state.json is wrong

See [Resetting to a clean slate](README.md#resetting-to-a-clean-slate)
for the general reset procedure. Beyond that: `state.json` is a small,
plain JSON file, and it's safe to hand-edit if only one entry is wrong —
no need to reset everything:

```json
{
  "run_id": "2026-08-11T09:00:00Z",
  "completed": [
    { "library": "kiwi-parent", "version": "3.0.0", "completed_at": "2026-08-11T09:05:00Z" }
  ],
  "failed": null
}
```

Remove a library's entry from `completed` if the tool believes it's done
but it actually needs to be released again. Add an entry (version and an
RFC 3339 `completed_at` timestamp) if a library was released entirely
outside the tool and should be treated as already done on the next
`--resume`.
