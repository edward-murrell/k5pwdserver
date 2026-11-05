package main

import (
	"flag"
	"log"

	"github.com/edward-murrell/k8init/internal/server"
	"gopkg.in/jcmturner/gokrb5.v7/config"
)

func main() {
	listen := flag.String("listen", ":8080", "address to listen on, as host:port")
	flag.Parse()

	// Load Kerberos configuration
	cfg, err := config.Load("/etc/krb5.conf")
	if err != nil {
		log.Fatalf("Error loading Kerberos config: %v", err)
	}

	// Setup and run the server
	router := server.Setup(cfg)

	log.Printf("Server starting on %s", *listen)
	log.Fatal(router.Run(*listen))
}
