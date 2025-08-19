package fxcert_reloader

import (
	"context"
	"crypto/tls"
	"log"
	"net/http"
	"time"

	"go.uber.org/zap"
)

func ExampleCertReloader_GetCertificate() {
	conf := &CertReloaderConfig{
		CertFile:       "/path/to/cert.pem",
		KeyFile:        "/path/to/key.pem",
		ReloadInterval: 1 * time.Hour,
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

func ExampleCertReloader_GetClientCertificate() {
	conf := &CertReloaderConfig{
		CertFile:       "/path/to/cert.pem",
		KeyFile:        "/path/to/key.pem",
		ReloadInterval: 1 * time.Hour,
	}
	reloader, err := NewCertReloader(conf, zap.NewNop())
	if err != nil {
		log.Fatal(err)
	}

	if err := reloader.Start(context.Background()); err != nil {
		log.Fatal(err)
	}
	defer reloader.Stop(context.Background()) //nolint:errcheck

	httpclient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				GetClientCertificate: reloader.GetClientCertificate,
			},
		},
	}

	_ = httpclient
	// httpclient..Get("https://example.com")

}
