# Releasing

1. Add an entry to the top of `debian/changelog` with the new version.
2. Commit and push `main`.
3. Push a matching tag, such as `v0.1.1`.

The tag must match the `debian/changelog` version, or the workflow stops at
its first job. Push `main` before the tag, so the run sees the new changelog.

## What the release workflow does

`.github/workflows/release.yml` runs on version tags. It:

- builds `.deb` packages for jammy, noble, bookworm and trixie, on amd64 and
  arm64
- installs each package in a clean container and checks the server starts and
  shows the right version
- builds a multi-arch image and pushes it to
  `ghcr.io/edward-murrell/k5pwdserver`, tagged with the version and `latest`
- attaches the packages to the GitHub release
- asks the [packages repo](https://github.com/edward-murrell/packages) to
  republish

You can also run it by hand from the Actions tab. A manual run builds and
tests everything but doesn't publish.

## Package repo

The packages repo pulls `.deb` files from the releases of each project in its
`projects.txt`. The republish request needs a `PACKAGES_DISPATCH_TOKEN` secret
in this repo. Without it, the packages repo picks up the release on its
nightly run.

## Fixing a failed release

If a run fails before anything is published, fix the problem on `main`, then
move the tag:

```sh
git push github :refs/tags/v0.1.1
git tag -f v0.1.1
git push github v0.1.1
```

If the release was published, release a new version instead.
