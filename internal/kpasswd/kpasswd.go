// Package kpasswd implements a self-service Kerberos password change (RFC 3244).
//
// gokrb5's client.ChangePasswd uses the RFC 3244 "set password" message (version
// 0xff80) and always includes targname/targrealm. Samba AD (and Windows AD) treat a
// request carrying a target principal as an administrative password *reset*, which
// an ordinary user may not perform on their own account, so the DC replies with
// KRB5_KPASSWD_ACCESSDENIED ("Not permitted to change password"). MIT kadmind, on
// the other hand, rejects a set-password message *without* a target as malformed.
//
// Instead we use the original "change password" message (version 0x0001), whose
// KRB-PRIV payload is just the new password. It is supported by MIT, Heimdal,
// Samba and Windows AD, and is what MIT's own kpasswd client sends.
package kpasswd

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"gopkg.in/jcmturner/gokrb5.v7/client"
	"gopkg.in/jcmturner/gokrb5.v7/crypto"
	"gopkg.in/jcmturner/gokrb5.v7/kadmin"
	"gopkg.in/jcmturner/gokrb5.v7/messages"
	"gopkg.in/jcmturner/gokrb5.v7/types"
)

// Kpasswd result codes (RFC 3244).
const (
	Success           = 0
	Malformed         = 1
	HardError         = 2
	AuthError         = 3
	SoftError         = 4
	AccessDenied      = 5
	BadVersion        = 6
	InitialFlagNeeded = 7
)

const (
	timeout               = 10 * time.Second
	changePasswordVersion = 0x0001
)

// ResultError is returned when the kpasswd server rejects the change.
type ResultError struct {
	Code    uint16
	Message string
}

func (e *ResultError) Error() string {
	return fmt.Sprintf("kpasswd error %d: %s", e.Code, e.Message)
}

// IsPolicyRejection reports whether the server rejected the new password on policy
// grounds (length, complexity, history, ...), i.e. a user-correctable error.
func (e *ResultError) IsPolicyRejection() bool {
	return e.Code == SoftError
}

// ChangePassword changes the password of the client's own principal.
func ChangePassword(cl *client.Client, newPassword string) error {
	realm := cl.Credentials.Domain()
	cname := cl.Credentials.CName()

	// kpasswd requires an initial ticket for kadmin/changepw, obtained via AS exchange.
	asReq, err := messages.NewASReqForChgPasswd(realm, cl.Config, cname)
	if err != nil {
		return err
	}
	asRep, err := cl.ASExchange(realm, asReq, 0)
	if err != nil {
		return err
	}
	sessionKey := asRep.DecryptedEncPart.Key

	auth, err := types.NewAuthenticator(realm, cname)
	if err != nil {
		return fmt.Errorf("create authenticator: %w", err)
	}
	etype, err := crypto.GetEtype(sessionKey.KeyType)
	if err != nil {
		return fmt.Errorf("session key etype: %w", err)
	}
	if err := auth.GenerateSeqNumberAndSubKey(etype.GetETypeID(), etype.GetKeyByteSize()); err != nil {
		return fmt.Errorf("generate subkey: %w", err)
	}

	apReq, err := messages.NewAPReq(asRep.Ticket, sessionKey, auth)
	if err != nil {
		return fmt.Errorf("create AP-REQ: %w", err)
	}
	priv := messages.NewKRBPriv(messages.EncKrbPrivPart{
		UserData:       []byte(newPassword), // version 1: raw new password
		Timestamp:      auth.CTime,
		Usec:           auth.Cusec,
		SequenceNumber: auth.SeqNumber,
	})
	if err := priv.EncryptEncPart(auth.SubKey); err != nil {
		return fmt.Errorf("encrypt KRB-PRIV: %w", err)
	}

	req := kadmin.Request{APREQ: apReq, KRBPriv: priv}
	b, err := req.Marshal()
	if err != nil {
		return fmt.Errorf("marshal kpasswd request: %w", err)
	}
	// kadmin.Request.Marshal always writes protocol version 0xff80 (set password)
	// at bytes 2-3, after the 2-byte message length. Switch it to change password.
	binary.BigEndian.PutUint16(b[2:4], changePasswordVersion)

	rb, err := sendTCP(cl, realm, b)
	if err != nil {
		return err
	}

	var reply kadmin.Reply
	if err := reply.Unmarshal(rb); err != nil {
		return fmt.Errorf("unmarshal kpasswd reply: %w", err)
	}
	if reply.IsKRBError {
		return &ResultError{Code: reply.ResultCode, Message: reply.Result}
	}
	if err := reply.Decrypt(auth.SubKey); err != nil {
		return fmt.Errorf("decrypt kpasswd reply: %w", err)
	}
	if reply.ResultCode != Success {
		return &ResultError{Code: reply.ResultCode, Message: reply.Result}
	}
	return nil
}

// sendTCP sends the request to the first reachable kpasswd server using TCP framing.
func sendTCP(cl *client.Client, realm string, b []byte) ([]byte, error) {
	_, servers, err := cl.Config.GetKpasswdServers(realm, true)
	if err != nil {
		return nil, fmt.Errorf("locate kpasswd server: %w", err)
	}
	if len(servers) == 0 {
		return nil, errors.New("no kpasswd servers configured for realm " + realm)
	}

	var lastErr error
	for i := 1; i <= len(servers); i++ {
		rb, err := exchangeTCP(servers[i], b)
		if err == nil {
			return rb, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("kpasswd exchange failed: %w", lastErr)
}

func exchangeTCP(addr string, b []byte) ([]byte, error) {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return nil, err
	}

	hdr := make([]byte, 4)
	binary.BigEndian.PutUint32(hdr, uint32(len(b)))
	if _, err := conn.Write(append(hdr, b...)); err != nil {
		return nil, err
	}
	if _, err := io.ReadFull(conn, hdr); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(hdr)
	if n < 6 || n > 65535 {
		return nil, fmt.Errorf("invalid kpasswd reply length %d", n)
	}
	rb := make([]byte, n)
	if _, err := io.ReadFull(conn, rb); err != nil {
		return nil, err
	}
	return rb, nil
}
