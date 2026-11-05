package main

import (
	"crypto/tls"
	"flag"
	"log"
	"net/http"
	"time"

	"github.com/edward-murrell/k8init/internal/server"
	"github.com/edward-murrell/k8init/internal/tlscert"
	"gopkg.in/jcmturner/gokrb5.v7/config"
)

func main() {
	listen := flag.String("listen", ":8080", "address to listen on, as host:port")
	tlsCert := flag.String("tls-cert", "", "TLS certificate file (PEM). Serves HTTPS when set with -tls-key")
	tlsKey := flag.String("tls-key", "", "TLS private key file (PEM)")
	flag.Parse()

	if (*tlsCert == "") != (*tlsKey == "") {
		log.Fatal("-tls-cert and -tls-key must be set together")
	}

	// Load Kerberos configuration
	cfg, err := config.Load("/etc/krb5.conf")
	if err != nil {
		log.Fatalf("Error loading Kerberos config: %v", err)
	}

	srv := &http.Server{
		Addr:              *listen,
		Handler:           server.Setup(cfg),
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
