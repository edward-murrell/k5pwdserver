# How password changes work

k5pwdserver changes passwords with the kpasswd protocol from RFC 3244. This
page explains which form of that protocol it uses, and why.

## Two request formats

RFC 3244 defines two kpasswd request formats:

- **Change password (version `0x0001`).** The payload is just the new password.
  It changes the password of the principal that holds the ticket.
- **Set password (version `0xff80`).** The payload holds the new password and
  an optional target principal and realm.

## KDC differences

The two KDC families disagree on set password requests:

- Samba and Windows AD treat a set password request with a target as an
  administrator reset. They refuse it for ordinary users with "Not permitted
  to change password" (result code 5, access denied).
- MIT kadmind rejects a set password request without a target as malformed.

gokrb5's `client.ChangePasswd` always sends set password with a target, so it
fails on Samba and AD. No form of set password works on both.

`internal/kpasswd` sends change password instead. MIT, Heimdal, Samba and
Windows AD all accept it, and MIT's own `kpasswd` client sends it too.

## Request details

The request is built with gokrb5, then the version field is rewritten to
`0x0001`.

- The ticket comes from an AS exchange for `kadmin/changepw`. KDCs require an
  initial ticket for password changes, so a TGS ticket won't work.
- Requests go over TCP to the realm's `kpasswd_server`. If that isn't set,
  they go to the `admin_server` host on port 464. gokrb5 skips `admin_server`
  entries written without a port, so set `kpasswd_server` to be safe.
- With `dns_lookup_kdc = true`, gokrb5 looks up `_kpasswd._tcp` SRV records
  instead, then `_kerberos-adm._tcp`.
- A soft error (result code 4) means the new password failed the KDC's policy.
  The API returns it as a 400 with the KDC's message.
