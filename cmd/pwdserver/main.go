package main

import (
	"crypto/tls"
	"flag"
	"log"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/edward-murrell/k5pwdserver/internal/handler"
	"github.com/edward-murrell/k5pwdserver/internal/ratelimit"
	"github.com/edward-murrell/k5pwdserver/internal/server"
	"github.com/edward-murrell/k5pwdserver/internal/tlscert"
	"gopkg.in/jcmturner/gokrb5.v7/config"
)

func main() {
	listen := flag.String("listen", ":8080", "address to listen on, as host:port")
	tlsCert := flag.String("tls-cert", "", "TLS certificate file (PEM). Serves HTTPS when set with -tls-key")
	tlsKey := flag.String("tls-key", "", "TLS private key file (PEM)")
	title := flag.String("title", server.DefaultTitle, "page title and heading")
	realms := flag.String("realms", "", "comma-separated realms users may change passwords in (default: every realm in krb5.conf)")
	userFailures := flag.Int("user-failures", 5, "failed logins allowed per user in each -failure-window; 0 disables")
	ipFailures := flag.Int("ip-failures", 20, "failed logins allowed per client IP in each -failure-window; 0 disables")
	failureWindow := flag.Duration("failure-window", 15*time.Minute, "period over which failed logins are counted")
	trustedProxies := flag.String("trusted-proxies", "", "comma-separated proxy IPs or CIDRs whose X-Forwarded-For header is trusted")
	flag.Parse()

	if (*tlsCert == "") != (*tlsKey == "") {
		log.Fatal("-tls-cert and -tls-key must be set together")
	}

	// Load Kerberos configuration
	cfg, err := config.Load("/etc/krb5.conf")
	if err != nil {
		log.Fatalf("Error loading Kerberos config: %v", err)
	}

	opts := server.Options{
		Realms: splitList(*realms),
		Title:  *title,
		Limits: handler.Limits{
			User: ratelimit.New(*userFailures, *failureWindow),
			IP:   ratelimit.New(*ipFailures, *failureWindow),
		},
		TrustedProxies: splitList(*trustedProxies),
	}
	for _, realm := range opts.Realms {
		if !slices.ContainsFunc(cfg.Realms, func(r config.Realm) bool { return r.Realm == realm }) {
			log.Printf("Warning: realm %s is not in /etc/krb5.conf", realm)
		}
	}

	router, err := server.Setup(cfg, opts)
	if err != nil {
		log.Fatalf("Error setting up server: %v", err)
	}

	srv := &http.Server{
		Addr:              *listen,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	if *tlsCert == "" {
		log.Printf("Server starting on http://%s", *listen)
		log.Fatal(srv.ListenAndServe())
	}

	certs, err := tlscert.New(*tlsCert, *tlsKey)
	if err != nil {
		log.Fatalf("Error loading TLS certificate: %v", err)
	}
	srv.TLSConfig = &tls.Config{
		MinVersion:     tls.VersionTLS12,
		GetCertificate: certs.GetCertificate,
	}
	log.Printf("Server starting on https://%s", *listen)
	log.Fatal(srv.ListenAndServeTLS("", ""))
}

// splitList splits a comma-separated flag value and drops empty entries.
func splitList(s string) []string {
	var out []string
	for _, item := range strings.Split(s, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}
