// Package kdctest runs a fake KDC for tests. It asks for pre-authentication,
// then answers with a fixed KRB-ERROR. It records which services were asked for.
package kdctest

import (
	"encoding/binary"
	"io"
	"net"
	"sync"
	"testing"

	"github.com/jcmturner/gofork/encoding/asn1"
	"gopkg.in/jcmturner/gokrb5.v7/asn1tools"
	"gopkg.in/jcmturner/gokrb5.v7/config"
	"gopkg.in/jcmturner/gokrb5.v7/iana/asnAppTag"
	"gopkg.in/jcmturner/gokrb5.v7/iana/errorcode"
	"gopkg.in/jcmturner/gokrb5.v7/iana/etypeID"
	"gopkg.in/jcmturner/gokrb5.v7/iana/patype"
	"gopkg.in/jcmturner/gokrb5.v7/messages"
	"gopkg.in/jcmturner/gokrb5.v7/types"
)

// KDC is a fake TCP KDC for one realm.
type KDC struct {
	Addr  string
	realm string
	code  int32

	mu       sync.Mutex
	services []string
}

// Start runs a fake KDC. An AS-REQ without an encrypted timestamp gets
// KDC_ERR_PREAUTH_REQUIRED, as from a real KDC. Any other request gets code.
// The KDC stops when the test ends.
func Start(t *testing.T, realm string, code int32) *KDC {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	k := &KDC{Addr: ln.Addr().String(), realm: realm, code: code}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go k.serve(t, conn)
		}
	}()
	return k
}

// Config returns a krb5 config that sends this realm's requests to the fake
// KDC over TCP.
func (k *KDC) Config(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.NewConfigFromString(`[libdefaults]
 default_realm = ` + k.realm + `
 dns_lookup_kdc = false
 udp_preference_limit = 1
[realms]
 ` + k.realm + ` = {
  kdc = ` + k.Addr + `
  kpasswd_server = ` + k.Addr + `
 }
`)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// Services returns the service principals requested so far, in order.
func (k *KDC) Services() []string {
	k.mu.Lock()
	defer k.mu.Unlock()
	return append([]string(nil), k.services...)
}

func (k *KDC) serve(t *testing.T, conn net.Conn) {
	defer conn.Close()
	hdr := make([]byte, 4)
	if _, err := io.ReadFull(conn, hdr); err != nil {
		return
	}
	req := make([]byte, binary.BigEndian.Uint32(hdr))
	if _, err := io.ReadFull(conn, req); err != nil {
		return
	}
	var asReq messages.ASReq
	if err := asReq.Unmarshal(req); err != nil {
		t.Errorf("fake KDC: unmarshal AS-REQ: %v", err)
		return
	}
	k.mu.Lock()
	k.services = append(k.services, asReq.ReqBody.SName.PrincipalNameString())
	k.mu.Unlock()

	krbErr := messages.NewKRBError(asReq.ReqBody.SName, k.realm, k.code, "fake KDC")
	if !hasEncTimestamp(asReq) {
		krbErr = messages.NewKRBError(asReq.ReqBody.SName, k.realm, errorcode.KDC_ERR_PREAUTH_REQUIRED, "")
		edata, err := preauthHint(k.realm + asReq.ReqBody.CName.PrincipalNameString())
		if err != nil {
			t.Errorf("fake KDC: marshal pre-auth hint: %v", err)
			return
		}
		krbErr.EData = edata
	}
	b, err := asn1.Marshal(krbErr)
	if err != nil {
		t.Errorf("fake KDC: marshal KRB-ERROR: %v", err)
		return
	}
	b = asn1tools.AddASNAppTag(b, asnAppTag.KRBError)
	binary.BigEndian.PutUint32(hdr, uint32(len(b)))
	conn.Write(append(hdr, b...))
}

func hasEncTimestamp(req messages.ASReq) bool {
	for _, pa := range req.PAData {
		if pa.PADataType == patype.PA_ENC_TIMESTAMP {
			return true
		}
	}
	return false
}

// preauthHint returns METHOD-DATA telling the client to pre-authenticate
// with an AES256 key derived using salt.
func preauthHint(salt string) ([]byte, error) {
	info, err := asn1.Marshal(types.ETypeInfo2{{EType: etypeID.AES256_CTS_HMAC_SHA1_96, Salt: salt}})
	if err != nil {
		return nil, err
	}
	return asn1.Marshal(types.MethodData{{PADataType: patype.PA_ETYPE_INFO2, PADataValue: info}})
}
