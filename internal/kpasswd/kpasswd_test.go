package kpasswd

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strings"
	"testing"

	"github.com/edward-murrell/k5pwdserver/internal/kdctest"
	"gopkg.in/jcmturner/gokrb5.v7/client"
	"gopkg.in/jcmturner/gokrb5.v7/config"
	"gopkg.in/jcmturner/gokrb5.v7/iana/errorcode"
	"gopkg.in/jcmturner/gokrb5.v7/krberror"
)

// fakeServer accepts one framed request and replies with the given payload,
// framed with the given length.
func fakeServer(t *testing.T, replyLen uint32, reply []byte) (addr string, got chan []byte) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	got = make(chan []byte, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		hdr := make([]byte, 4)
		if _, err := io.ReadFull(conn, hdr); err != nil {
			return
		}
		req := make([]byte, binary.BigEndian.Uint32(hdr))
		if _, err := io.ReadFull(conn, req); err != nil {
			return
		}
		got <- req
		binary.BigEndian.PutUint32(hdr, replyLen)
		conn.Write(append(hdr, reply...))
	}()
	return ln.Addr().String(), got
}

func closedAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	return addr
}

func TestExchangeTCPFraming(t *testing.T) {
	reply := []byte("reply-payload")
	addr, got := fakeServer(t, uint32(len(reply)), reply)

	rb, err := exchangeTCP(addr, []byte("request"))
	if err != nil {
		t.Fatal(err)
	}
	if string(<-got) != "request" {
		t.Error("server did not receive the unframed request")
	}
	if string(rb) != string(reply) {
		t.Errorf("reply = %q, want %q", rb, reply)
	}
}

func TestExchangeTCPRejectsBadLength(t *testing.T) {
	for _, n := range []uint32{0, 5, 65536} {
		addr, _ := fakeServer(t, n, nil)
		if _, err := exchangeTCP(addr, []byte("x")); err == nil ||
			!strings.Contains(err.Error(), "invalid kpasswd reply length") {
			t.Errorf("length %d: err = %v, want invalid length error", n, err)
		}
	}
}

func TestSendTCPFailsOver(t *testing.T) {
	reply := []byte("good-reply")
	good, _ := fakeServer(t, uint32(len(reply)), reply)
	bad := closedAddr(t)

	cfg, err := config.NewConfigFromString(`[libdefaults]
 default_realm = EXAMPLE.COM
 dns_lookup_kdc = false
[realms]
 EXAMPLE.COM = {
  kdc = 127.0.0.1:88
  kpasswd_server = ` + bad + `
  kpasswd_server = ` + good + `
 }
`)
	if err != nil {
		t.Fatal(err)
	}
	cl := client.NewClientWithPassword("user", "EXAMPLE.COM", "pw", cfg)

	rb, err := sendTCP(cl, "EXAMPLE.COM", []byte("req"))
	if err != nil {
		t.Fatal(err)
	}
	if string(rb) != string(reply) {
		t.Errorf("reply = %q, want %q", rb, reply)
	}
}

func TestResultError(t *testing.T) {
	var err error = &ResultError{Code: SoftError, Message: "too short"}
	var kerr *ResultError
	if !errors.As(err, &kerr) || !kerr.IsPolicyRejection() {
		t.Error("soft error should be a policy rejection")
	}
	if err.Error() != "kpasswd error 4: too short" {
		t.Errorf("Error() = %q", err.Error())
	}
	if (&ResultError{Code: AccessDenied}).IsPolicyRejection() {
		t.Error("access denied should not be a policy rejection")
	}
}

func TestChangePasswordKDCErrors(t *testing.T) {
	tests := []struct {
		name      string
		code      int32
		wantLogin bool
	}{
		{"wrong password", errorcode.KDC_ERR_PREAUTH_FAILED, true},
		{"unknown user", errorcode.KDC_ERR_C_PRINCIPAL_UNKNOWN, true},
		{"account disabled", errorcode.KDC_ERR_CLIENT_REVOKED, true},
		{"KDC fault", errorcode.KDC_ERR_ETYPE_NOSUPP, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kdc := kdctest.Start(t, "EXAMPLE.COM", tt.code)
			cl := client.NewClientWithPassword("alice", "EXAMPLE.COM", "pw", kdc.Config(t), client.DisablePAFXFAST(true))

			err := ChangePassword(cl, "new")
			var lerr *LoginError
			if got := errors.As(err, &lerr); got != tt.wantLogin {
				t.Errorf("LoginError = %v, want %v (err %v)", got, tt.wantLogin, err)
			}
			for _, svc := range kdc.Services() {
				if svc != "kadmin/changepw" {
					t.Errorf("requested a ticket for %s, want only kadmin/changepw", svc)
				}
			}
		})
	}
}

func TestCredentialsRejectedWithoutPreauth(t *testing.T) {
	// Without pre-authentication, a wrong password shows up as a reply the
	// client can't decrypt.
	err := krberror.NewErrorf(krberror.KRBMsgError, "AS Exchange Error: AS_REP is not valid or client password/keytab incorrect")
	if !credentialsRejected(err) {
		t.Error("undecryptable AS-REP should count as rejected credentials")
	}
	if credentialsRejected(errors.New("connection refused")) {
		t.Error("a plain network error should not count as rejected credentials")
	}
}
