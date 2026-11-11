# k5pwdserver

[![CI](https://github.com/edward-murrell/k5pwdserver/actions/workflows/ci.yml/badge.svg)](https://github.com/edward-murrell/k5pwdserver/actions/workflows/ci.yml)

A small web service that lets users change their own Kerberos password.

It works with MIT krb5, Heimdal and Active Directory/Samba KDCs. See
[How password changes work](docs/kpasswd.md) for the protocol details.

<p>
  <img src="docs/screenshot-light.png" alt="Password change form, light theme" width="49%">
  <img src="docs/screenshot-dark.png" alt="Password change form, dark theme" width="49%">
</p>

The page follows the browser's light or dark setting. The button at the top
right overrides it.

## Running

```sh
go run ./cmd/pwdserver
```

The server reads realms and KDCs from `/etc/krb5.conf` and listens on `:8080`.
Open `http://<host>:8080/` in a browser.

## Options

| Flag               | Default                    | Description                                           |
|--------------------|----------------------------|-------------------------------------------------------|
| `-listen`          | `:8080`                    | Address to listen on, as `host:port`.                 |
| `-tls-cert`        |                            | PEM certificate file. Set with `-tls-key` for HTTPS.  |
| `-tls-key`         |                            | PEM private key file.                                 |
| `-realms`          | every realm in `krb5.conf` | Comma-separated realms users may pick.                |
| `-title`           | `Change Kerberos Password` | Page title and heading.                               |
| `-user-failures`   | `5`                        | Failed logins allowed per user in each window. `0` turns it off. |
| `-ip-failures`     | `20`                       | Failed logins allowed per client IP in each window. `0` turns it off. |
| `-failure-window`  | `15m`                      | Period over which failed logins are counted.          |
| `-trusted-proxies` |                            | Comma-separated proxy IPs or CIDRs to trust for `X-Forwarded-For`. |

Realm names are case-sensitive. If `krb5.conf`'s default realm isn't in
`-realms`, the first listed realm becomes the default.

```sh
go run ./cmd/pwdserver -realms EXAMPLE.COM,OTHER.ORG -title "Example Corp passwords"
```

## Failed login limits

Each change request checks the current password with the KDC, so the server
limits failed logins. Only wrong passwords, unknown users, and disabled or
locked accounts count. Once a user or client IP reaches its limit, requests
get a 429 until the window ends, and don't reach the KDC.

- Keep `-user-failures` below the KDC's lockout threshold. Then guessing
  through this page can't lock the account in AD. Anyone can still use up a
  user's attempts here, which blocks that user on this page until the window
  ends.
- Usernames are matched without case, because AD ignores case.
- Counts are kept in memory, so they reset when the server restarts.

Behind a reverse proxy, every request comes from the proxy's IP. Set
`-trusted-proxies` to the proxy's address so each client gets its own limit.
The server ignores `X-Forwarded-For` from anyone else.

## TLS

The server speaks plain HTTP by default. Don't expose plain HTTP beyond
localhost, because passwords cross the network. To serve HTTPS directly, pass
a PEM certificate and key:

```sh
go run ./cmd/pwdserver -listen :8443 -tls-cert cert.pem -tls-key key.pem
```

The server reloads the certificate when either file changes, so renewals need
no restart. You can also run it on localhost behind a TLS proxy.

## Debian package

The package installs `/usr/sbin/k5pwdserver` and a `k5pwdserver` systemd
service. Set options in `/etc/default/k5pwdserver`, then restart the service.
By default it listens on `127.0.0.1:8080`.

```sh
LISTEN=:443
TLS_CERT=/etc/letsencrypt/live/example.com/fullchain.pem
TLS_KEY=/etc/letsencrypt/live/example.com/privkey.pem
REALMS=EXAMPLE.COM
TITLE=Example Corp passwords
USER_FAILURES=3
TRUSTED_PROXIES=127.0.0.1
```

The service runs as a dynamic user, so it must be able to read the TLS key.
The comments in `/etc/default/k5pwdserver` show how to grant access.

Packages for Debian 12 and 13 and Ubuntu 22.04 and 24.04 are attached to each
[GitHub release](https://github.com/edward-murrell/k5pwdserver/releases).

## Docker

Release tags publish an image to `ghcr.io/edward-murrell/k5pwdserver`. Mount
the host's `krb5.conf`, and pass flags after the image name:

```sh
docker run -d -p 8443:8443 \
  -v /etc/krb5.conf:/etc/krb5.conf:ro \
  -v /etc/k5pwdserver/tls:/tls:ro \
  ghcr.io/edward-murrell/k5pwdserver \
  -listen :8443 -tls-cert /tls/cert.pem -tls-key /tls/key.pem
```

The container runs as a non-root user, so the key must be readable by UID
65532. Without flags it serves plain HTTP on port 8080.

## API

- `GET /api/realms` – allowed realms and the default realm.
- `POST /api/change-password` – JSON body:
  `{"username", "realm", "oldPassword", "newPassword"}`.
  `realm` is optional and defaults to the default realm.
  Returns 200 on success, 401 for wrong current credentials, 400 if the new
  password fails the KDC's password policy or the realm isn't allowed, 415 if
  the request isn't `application/json`, 429 with `Retry-After` after too many
  failed logins, and 500 for other errors.

## More documentation

- [How password changes work](docs/kpasswd.md): the kpasswd protocol and KDC
  differences.
- [Development](docs/development.md): tests, local builds and upgrading Vue.
- [Releasing](docs/releasing.md): release steps and the release workflow.
