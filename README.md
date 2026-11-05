# k8init

A small web service that lets users change their own Kerberos password.

It works with MIT krb5, Heimdal and Active Directory/Samba KDCs. Password
changes use the kpasswd change-password protocol (RFC 3244, version 1), because
gokrb5's built-in set-password request is rejected by AD/Samba.

## Running

```sh
go run ./cmd/pwdserver
```

The server reads realms and KDCs from `/etc/krb5.conf` and listens on `:8080`.
Use `-listen host:port` to change the address.

By default users can pick any realm in `krb5.conf`. To limit them, pass a
comma-separated list. Realm names are case-sensitive.

```sh
go run ./cmd/pwdserver -realms EXAMPLE.COM,OTHER.ORG
```

Use `-title` to change the page title and heading.

The page footer shows the package name and version. Builds without a version
show `dev`. To set one, build with:

```sh
go build -ldflags "-X github.com/edward-murrell/k8init/internal/version.Version=1.2.3" ./cmd/pwdserver
```
It serves the password change form at `/` (embedded from
`internal/server/web/index.html`), so open `http://<host>:8080/` in a browser.

The server speaks plain HTTP by default. Don't expose plain HTTP beyond
localhost, because passwords cross the network. To serve HTTPS directly, pass
a PEM certificate and key:

```sh
go run ./cmd/pwdserver -listen :8443 -tls-cert cert.pem -tls-key key.pem
```

The server reloads the certificate when either file changes, so renewals need
no restart. You can also run it on localhost behind a TLS proxy.

## Debian package

The package installs `/usr/sbin/k8init` and a `k8init` systemd service. Set
options in `/etc/default/k8init`. By default the service listens on
`127.0.0.1:8080`.

To build a package for the release you're running, as root from the repo root:

```sh
packaging/build-deb.sh 0.1.0
```

Pushing a version tag runs the release workflow. It builds and tests packages
for each supported release and attaches them to the GitHub release. The tag
must match the version at the top of `debian/changelog`.

## Docker

Release tags publish an image to `ghcr.io/edward-murrell/k8init`. Mount the
host's `krb5.conf`, and pass flags after the image name:

```sh
docker run -d -p 8443:8443 \
  -v /etc/krb5.conf:/etc/krb5.conf:ro \
  -v /etc/k8init/tls:/tls:ro \
  ghcr.io/edward-murrell/k8init \
  -listen :8443 -tls-cert /tls/cert.pem -tls-key /tls/key.pem
```

The container runs as a non-root user, so the key must be readable by UID
65532. Without flags it serves plain HTTP on port 8080.

To build the image locally:

```sh
docker build --build-arg VERSION=0.1.0 -t k8init .
```

## API

- `GET /api/realms` – allowed realms and the default realm.
- `POST /api/change-password` – JSON body:
  `{"username", "realm", "oldPassword", "newPassword"}`.
  `realm` is optional and defaults to `default_realm`.
  Returns 200 on success, 401 for wrong current credentials, 400 if the new
  password fails the KDC's password policy or the realm isn't allowed, 415 if
  the request isn't `application/json`, and 500 for other errors.
