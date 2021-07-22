package fxcert_reloader

import (
	"context"
	"crypto/tls"
	"log"
	"time"

	"go.uber.org/zap"
)

func ExampleNewCertReloader() {
	conf := &CertReloaderConfig{
		CertFile:       "/path/to/cert.pem",
		KeyFile:        "/path/to/key.pem",
		ReloadInterval: 10 * time.Second,
	}
	reloader, err := NewCertReloader(conf, zap.NewNop())
	if err != nil {
		log.Fatal(err)
	}

	if err := reloader.Start(context.Background()); err != nil {
		log.Fatal(err)
	}
	defer reloader.Stop(context.Background()) //nolint:errcheck

	cfg := &tls.Config{GetCertificate: reloader.GetCertificate}

	listener, err := tls.Listen("tcp", ":2000", cfg)
	if err != nil {
		log.Fatal(err)
	}
	_ = listener
}
