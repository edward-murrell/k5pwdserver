# Development

## Tests

```sh
go vet ./...
go test ./...
```

GitHub Actions runs these, plus a `gofmt` check, on every push and pull
request.

## Version stamping

The page footer shows the package name and version. Builds without a version
show `dev`. To set one:

```sh
go build -ldflags "-X github.com/edward-murrell/k5pwdserver/internal/version.Version=1.2.3" ./cmd/pwdserver
```

The Debian package and the Docker image set this from their build version.

## Building a Debian package

`packaging/build-deb.sh` builds a `.deb` for the release it runs on. It
installs the Go version named in `go.mod`, because the distro `golang-go` is
too old on some releases. Run it as root, from the repo root, in a clean
container of the target release:

```sh
docker run --rm -v "$PWD":/src -w /src debian:13 packaging/build-deb.sh 0.1.0
```

The package lands in `out/`, with a `+<codename>` version suffix such as
`0.1.0+trixie`. The suffix decides which APT suite the package repo publishes
it in.

The script prepends a build entry to `debian/changelog`. Run
`git checkout debian/changelog` afterwards, or build from a copy of the repo.
Files it creates are owned by root.

## Building the Docker image

```sh
docker build --build-arg VERSION=0.1.0 -t k5pwdserver .
```

## Upgrading Vue

The page uses a bundled copy of Vue, embedded from
`internal/server/web/vue-<version>.global.prod.js`. To upgrade it:

1. Pick a release that has been out for at least two weeks:

   ```sh
   curl -s https://registry.npmjs.org/vue | jq -r '.time | to_entries[]
     | select(.key | test("^3\\.[0-9]+\\.[0-9]+$")) | "\(.value) \(.key)"' | sort | tail
   ```

2. Download the package and check it matches the registry's hash:

   ```sh
   V=3.5.44
   curl -s https://registry.npmjs.org/vue/$V | jq -r .dist.integrity
   curl -sO https://registry.npmjs.org/vue/-/vue-$V.tgz
   echo "sha512-$(openssl dgst -sha512 -binary vue-$V.tgz | base64 -w0)"
   ```

   The two hashes must be the same.

3. Replace the bundled file and licence:

   ```sh
   tar -xzf vue-$V.tgz package/dist/vue.global.prod.js package/LICENSE
   git rm internal/server/web/vue-*.global.prod.js
   cp package/dist/vue.global.prod.js internal/server/web/vue-$V.global.prod.js
   cp package/LICENSE internal/server/web/vue-LICENSE
   ```

4. Update the version in `internal/server/setup.go`, in both the `//go:embed`
   line and `vuePath`. Also update it in `TestVueAsset` in
   `internal/server/setup_test.go`.

5. Run `go test ./...` and load the page in a browser.

Use the `global.prod` build. It includes the template compiler the page needs.
