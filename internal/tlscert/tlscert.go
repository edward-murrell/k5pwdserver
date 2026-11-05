// Package tlscert serves a TLS certificate from files and reloads it when they change.
package tlscert

import (
	"crypto/tls"
	"fmt"
	"log"
	"os"
	"sync"
	"time"
)

// Reloader holds a certificate and key pair loaded from disk. It checks the
// files' modification times on each handshake and reloads them when they
// change, so renewed certificates are picked up without a restart.
type Reloader struct {
	certFile, keyFile string

	mu      sync.Mutex
	cert    *tls.Certificate
	certMod time.Time
	keyMod  time.Time
}

// New loads the certificate and key, and fails if they can't be read.
func New(certFile, keyFile string) (*Reloader, error) {
	r := &Reloader{certFile: certFile, keyFile: keyFile}
	if err := r.reload(); err != nil {
		return nil, err
	}
	return r, nil
}

// GetCertificate implements tls.Config.GetCertificate.
func (r *Reloader) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.changed() {
		// A failed reload keeps the old certificate. This covers a renewal
		// caught half written, which the next handshake retries.
		if err := r.reload(); err != nil {
			log.Printf("TLS certificate reload failed, keeping the current one: %v", err)
		}
	}
	return r.cert, nil
}

func (r *Reloader) changed() bool {
	certMod, keyMod, err := r.modTimes()
	if err != nil {
		return false
	}
	return !certMod.Equal(r.certMod) || !keyMod.Equal(r.keyMod)
}

func (r *Reloader) modTimes() (time.Time, time.Time, error) {
	ci, err := os.Stat(r.certFile)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	ki, err := os.Stat(r.keyFile)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	return ci.ModTime(), ki.ModTime(), nil
}

func (r *Reloader) reload() error {
	certMod, keyMod, err := r.modTimes()
	if err != nil {
		return err
	}
	cert, err := tls.LoadX509KeyPair(r.certFile, r.keyFile)
	if err != nil {
		return fmt.Errorf("load TLS key pair: %w", err)
	}
	r.cert, r.certMod, r.keyMod = &cert, certMod, keyMod
	return nil
}
