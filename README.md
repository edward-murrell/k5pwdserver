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

## API

- `GET /api/realms` – allowed realms and the default realm.
- `POST /api/change-password` – JSON body:
  `{"username", "realm", "oldPassword", "newPassword"}`.
  `realm` is optional and defaults to `default_realm`.
  Returns 200 on success, 401 for wrong current credentials, 400 if the new
  password fails the KDC's password policy or the realm isn't allowed, 415 if
  the request isn't `application/json`, and 500 for other errors.
