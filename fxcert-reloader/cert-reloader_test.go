package fxcert_reloader

import (
	"context"
	"crypto/x509"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

const certFile1 string = `-----BEGIN CERTIFICATE-----
MIICKTCCAdCgAwIBAgIUBpCIedd+/9Uvo70xumUQqLlE7kQwCgYIKoZIzj0EAwIw
SDELMAkGA1UEBhMCVVMxCzAJBgNVBAgTAkNBMRYwFAYDVQQHEw1TYW4gRnJhbmNp
c2NvMRQwEgYDVQQDEwtleGFtcGxlLm5ldDAeFw0yMDExMDUxMDE2MDBaFw0yNTEx
MDQxMDE2MDBaMEcxCzAJBgNVBAYTAlVTMQswCQYDVQQIEwJDQTEWMBQGA1UEBxMN
U2FuIEZyYW5jaXNjbzETMBEGA1UEAxMKd2FycC1hZ2VudDBZMBMGByqGSM49AgEG
CCqGSM49AwEHA0IABAKSdVMNuuMMu5FmFnOjJdKzkl2d0NhoWLs4TE8X9CwIUJQH
1/CnSwFOoK0B5bHMVVTEps5fS1aP9KIRUbv+CZSjgZgwgZUwDgYDVR0PAQH/BAQD
AgWgMBMGA1UdJQQMMAoGCCsGAQUFBwMBMAwGA1UdEwEB/wQCMAAwHQYDVR0OBBYE
FPxyoPLf1q/7RFXFAocQ5fw4Gg4PMB8GA1UdIwQYMBaAFP8Z5PHehXpn/1HQ68LA
evBhvYiPMCAGA1UdEQQZMBeCCWxvY2FsaG9zdIIKd2FycC1hZ2VudDAKBggqhkjO
PQQDAgNHADBEAiBhf776NdaHmh/dI4ilQ7Pr6Pv4s+69soUppwhbQS6ftAIgdy64
tLXcMuUrMrsJaDq9cH4Imq8DIujr1gIMlYss0u0=
-----END CERTIFICATE-----`
const certFile2 string = `-----BEGIN CERTIFICATE-----
MIICKDCCAc2gAwIBAgIUJTBHNTgJtXCFNujFThGTcdYzdgIwCgYIKoZIzj0EAwIw
SDELMAkGA1UEBhMCVVMxCzAJBgNVBAgTAkNBMRYwFAYDVQQHEw1TYW4gRnJhbmNp
c2NvMRQwEgYDVQQDEwtleGFtcGxlLm5ldDAeFw0yMTA3MjAxNTUxMDBaFw0yNjA3
MTkxNTUxMDBaMFAxCzAJBgNVBAYTAlVTMQswCQYDVQQIEwJDQTEWMBQGA1UEBxMN
U2FuIEZyYW5jaXNjbzEcMBoGA1UEAxMTc2VydmVyMi5leGFtcGxlLm5ldDBZMBMG
ByqGSM49AgEGCCqGSM49AwEHA0IABGSaXcyoTD2lnh9+NYaDXgWwJEOX3L9WtV4K
rKfPVzaBiOzlRtKa7fP1bl0hmkmzmfmT9ZmsQS9U/IABm6xJtxSjgYwwgYkwDgYD
VR0PAQH/BAQDAgWgMBMGA1UdJQQMMAoGCCsGAQUFBwMBMAwGA1UdEwEB/wQCMAAw
HQYDVR0OBBYEFMIg3suAs+yp9NYf+nB4gU1Q1jfGMB8GA1UdIwQYMBaAFP8Z5PHe
hXpn/1HQ68LAevBhvYiPMBQGA1UdEQQNMAuCCWxvY2FsaG9zdDAKBggqhkjOPQQD
AgNJADBGAiEAwbeCEqile1ogOVWzuQdp+BOqjv47ZAn1lfQ3Q/T2T0UCIQCzkY9P
1fx1WnUJPV2x1MIIYs8Z9TShgTMcL9MQmbYleQ==
-----END CERTIFICATE-----`

const keyFile1 string = `-----BEGIN EC PRIVATE KEY-----
MHcCAQEEILC2U1Jgl8/tivLNX2jRRSSj6SrKdkoLy1Oc8vZVtOmWoAoGCCqGSM49
AwEHoUQDQgAEApJ1Uw264wy7kWYWc6Ml0rOSXZ3Q2GhYuzhMTxf0LAhQlAfX8KdL
AU6grQHlscxVVMSmzl9LVo/0ohFRu/4JlA==
-----END EC PRIVATE KEY-----`
const keyFile2 string = `-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIKv1aEtj/08E7Ci0BAKfBssLSofhVrDiOnhd4VEEZ3MroAoGCCqGSM49
AwEHoUQDQgAEZJpdzKhMPaWeH341hoNeBbAkQ5fcv1a1Xgqsp89XNoGI7OVG0prt
8/VuXSGaSbOZ+ZP1maxBL1T8gAGbrEm3FA==
-----END EC PRIVATE KEY-----`

func TestNewCertReloader(t *testing.T) {
	t.Run("Should return an error if the certificate can't be eagerly loaded", func(t *testing.T) {
		conf := &CertReloaderConfig{}
		reloader, err := NewCertReloader(conf, zap.NewNop())

		assert.Error(t, err)
		assert.Nil(t, reloader)
	})

	t.Run("Should return a reloader with the cert eagerly loaded", func(t *testing.T) {
		certFile, err := os.CreateTemp("", "cert")
		assert.NoError(t, err, "Failed to create temporary certFile")
		defer os.Remove(certFile.Name())

		keyFile, err := os.CreateTemp("", "key")
		assert.NoError(t, err, "Failed to create temporary keyFile")
		defer os.Remove(keyFile.Name())

		_, err = certFile.WriteString(certFile1)
		assert.NoError(t, err, "Failed to write certFile")
		assert.NoError(t, certFile.Close(), "Failed to close certFile")

		_, err = keyFile.WriteString(keyFile1)
		assert.NoError(t, err, "Failed to write keyFile")
		assert.NoError(t, keyFile.Close(), "Failed to close keyFile")

		conf := &CertReloaderConfig{
			CertFile: certFile.Name(),
			KeyFile:  keyFile.Name(),
		}
		reloader, err := NewCertReloader(conf, zap.NewNop())
		assert.NoError(t, err)
		if assert.NotNil(t, reloader) {
			cert, err := reloader.GetCertificate(nil)
			assert.NoError(t, err)
			if assert.NotNil(t, cert) {
				pCert, err := x509.ParseCertificate(cert.Certificate[0])
				assert.NoError(t, err)
				assert.Equal(t, "warp-agent", pCert.Subject.CommonName)
			}

			cert, err = reloader.GetClientCertificate(nil)
			assert.NoError(t, err)
			if assert.NotNil(t, cert) {
				pCert, err := x509.ParseCertificate(cert.Certificate[0])
				assert.NoError(t, err)
				assert.Equal(t, "warp-agent", pCert.Subject.CommonName)
			}
		}
	})
}

func TestCertReloader(t *testing.T) {
	// We don't really have a lot of ways to check what happens just from the
	// output alone
	// We will be relying on the logs that CertReloader emits to assert
	// that it took the right action.
	// It is a bit brittle, but the best I could come up with for now
	t.Run("Should reload the cert when it changes", func(t *testing.T) {
		logobserver, logs := observer.New(zapcore.DebugLevel)
		logger := zap.New(logobserver)

		certFile, err := os.CreateTemp("", "cert")
		assert.NoError(t, err, "Failed to create temporary certFile")
		defer os.Remove(certFile.Name())

		keyFile, err := os.CreateTemp("", "key")
		assert.NoError(t, err, "Failed to create temporary keyFile")
		defer os.Remove(keyFile.Name())

		_, err = certFile.WriteString(certFile1)
		assert.NoError(t, err, "Failed to write certFile")
		assert.NoError(t, certFile.Close(), "Failed to close certFile")

		_, err = keyFile.WriteString(keyFile1)
		assert.NoError(t, err, "Failed to write keyFile")
		assert.NoError(t, keyFile.Close(), "Failed to close keyFile")

		conf := &CertReloaderConfig{
			CertFile:       certFile.Name(),
			KeyFile:        keyFile.Name(),
			ReloadInterval: 100 * time.Millisecond,
		}
		reloader, err := NewCertReloader(conf, logger)
		assert.NoError(t, err)

		err = reloader.Start(context.Background())
		defer reloader.Stop(context.Background()) //nolint:errcheck
		assert.NoError(t, err)

		// Assert that we emit the initial cert first
		cert, err := reloader.GetCertificate(nil)
		assert.NoError(t, err)
		pCert, err := x509.ParseCertificate(cert.Certificate[0])
		assert.NoError(t, err)
		assert.Equal(t, "warp-agent", pCert.Subject.CommonName)

		cert, err = reloader.GetClientCertificate(nil)
		assert.NoError(t, err)
		pCert, err = x509.ParseCertificate(cert.Certificate[0])
		assert.NoError(t, err)
		assert.Equal(t, "warp-agent", pCert.Subject.CommonName)

		// Update certs on disk
		fd1, err := os.Create(certFile.Name())
		assert.NoError(t, err)
		_, err = fd1.WriteString(certFile2)
		assert.NoError(t, err)
		assert.NoError(t, fd1.Close())
		fd2, err := os.Create(keyFile.Name())
		assert.NoError(t, err)
		_, err = fd2.WriteString(keyFile2)
		assert.NoError(t, err)
		assert.NoError(t, fd2.Close())

		// Wait for rate limit period
		time.Sleep(200 * time.Millisecond)

		// Assert that we emit the second cert
		cert2, err := reloader.GetCertificate(nil)
		assert.NoError(t, err)
		pCert2, err := x509.ParseCertificate(cert2.Certificate[0])
		assert.NoError(t, err)
		assert.Equal(t, "server2.example.net", pCert2.Subject.CommonName)

		cert2, err = reloader.GetClientCertificate(nil)
		assert.NoError(t, err)
		pCert2, err = x509.ParseCertificate(cert2.Certificate[0])
		assert.NoError(t, err)
		assert.Equal(t, "server2.example.net", pCert2.Subject.CommonName)

		// Assert that reload logic triggered by examining logs
		assert.NotEmpty(t, logs.FilterMessage("Certificate was updated. Scheduling update."))
		assert.NotEmpty(t, logs.FilterMessage("Reloading certificate"))
	})

	t.Run("Should not reload if another file changes", func(t *testing.T) {
		logobserver, logs := observer.New(zapcore.DebugLevel)
		logger := zap.New(logobserver)

		certFile, err := os.CreateTemp("", "cert")
		assert.NoError(t, err, "Failed to create temporary certFile")
		defer os.Remove(certFile.Name())

		keyFile, err := os.CreateTemp("", "key")
		assert.NoError(t, err, "Failed to create temporary keyFile")
		defer os.Remove(keyFile.Name())

		_, err = certFile.WriteString(certFile1)
		assert.NoError(t, err, "Failed to write certFile")
		assert.NoError(t, certFile.Close(), "Failed to close certFile")

		_, err = keyFile.WriteString(keyFile1)
		assert.NoError(t, err, "Failed to write keyFile")
		assert.NoError(t, keyFile.Close(), "Failed to close keyFile")

		conf := &CertReloaderConfig{
			CertFile:       certFile.Name(),
			KeyFile:        keyFile.Name(),
			ReloadInterval: 100 * time.Millisecond,
		}
		reloader, err := NewCertReloader(conf, logger)
		assert.NoError(t, err)

		err = reloader.Start(context.Background())
		defer reloader.Stop(context.Background()) //nolint:errcheck
		assert.NoError(t, err)

		// Assert that we emit the initial cert first
		cert, err := reloader.GetCertificate(nil)
		assert.NoError(t, err)
		pCert, err := x509.ParseCertificate(cert.Certificate[0])
		assert.NoError(t, err)
		assert.Equal(t, "warp-agent", pCert.Subject.CommonName)

		cert, err = reloader.GetClientCertificate(nil)
		assert.NoError(t, err)
		pCert, err = x509.ParseCertificate(cert.Certificate[0])
		assert.NoError(t, err)
		assert.Equal(t, "warp-agent", pCert.Subject.CommonName)

		// Add an extra file in the folder
		// We're assuming CreateTemp always uses the same folder
		fd1, err := os.CreateTemp("", "extra")
		assert.NoError(t, err)
		_, err = fd1.WriteString(certFile2)
		assert.NoError(t, err)
		assert.NoError(t, fd1.Close())

		// Wait for rate limit period
		time.Sleep(200 * time.Millisecond)

		// Assert that we still emit the initial cert
		cert2, err := reloader.GetCertificate(nil)
		assert.NoError(t, err)
		pCert2, err := x509.ParseCertificate(cert2.Certificate[0])
		assert.NoError(t, err)
		assert.Equal(t, "warp-agent", pCert2.Subject.CommonName)

		cert2, err = reloader.GetClientCertificate(nil)
		assert.NoError(t, err)
		pCert2, err = x509.ParseCertificate(cert2.Certificate[0])
		assert.NoError(t, err)
		assert.Equal(t, "warp-agent", pCert2.Subject.CommonName)

		// Assert that reload logic triggered by examining logs
		assert.Empty(t, logs.FilterMessage("Certificate was updated. Scheduling update."))
		assert.Empty(t, logs.FilterMessage("Reloading certificate"))
		assert.NotEmpty(t, logs.FilterMessage("Event for untracked file. Ignoring event."))
	})

	t.Run("Should return the initial cert if reloading fails", func(t *testing.T) {
		logobserver, logs := observer.New(zapcore.DebugLevel)
		logger := zap.New(logobserver)

		certFile, err := os.CreateTemp("", "cert")
		assert.NoError(t, err, "Failed to create temporary certFile")
		defer os.Remove(certFile.Name())

		keyFile, err := os.CreateTemp("", "key")
		assert.NoError(t, err, "Failed to create temporary keyFile")
		defer os.Remove(keyFile.Name())

		_, err = certFile.WriteString(certFile1)
		assert.NoError(t, err, "Failed to write certFile")
		assert.NoError(t, certFile.Close(), "Failed to close certFile")

		_, err = keyFile.WriteString(keyFile1)
		assert.NoError(t, err, "Failed to write keyFile")
		assert.NoError(t, keyFile.Close(), "Failed to close keyFile")

		conf := &CertReloaderConfig{
			CertFile:       certFile.Name(),
			KeyFile:        keyFile.Name(),
			ReloadInterval: 100 * time.Millisecond,
		}
		reloader, err := NewCertReloader(conf, logger)
		assert.NoError(t, err)

		err = reloader.Start(context.Background())
		defer reloader.Stop(context.Background()) //nolint:errcheck
		assert.NoError(t, err)

		// Assert that we emit the initial cert first
		cert, err := reloader.GetCertificate(nil)
		assert.NoError(t, err)
		pCert, err := x509.ParseCertificate(cert.Certificate[0])
		assert.NoError(t, err)
		assert.Equal(t, "warp-agent", pCert.Subject.CommonName)

		cert, err = reloader.GetClientCertificate(nil)
		assert.NoError(t, err)
		pCert, err = x509.ParseCertificate(cert.Certificate[0])
		assert.NoError(t, err)
		assert.Equal(t, "warp-agent", pCert.Subject.CommonName)

		// Update certs on disk
		fd1, err := os.Create(certFile.Name())
		assert.NoError(t, err)
		_, err = fd1.WriteString("foobar")
		assert.NoError(t, err)
		assert.NoError(t, fd1.Close())

		// Wait for rate limit period
		time.Sleep(200 * time.Millisecond)

		// Assert that we still emit the initial cert
		cert2, err := reloader.GetCertificate(nil)
		assert.NoError(t, err)
		pCert2, err := x509.ParseCertificate(cert2.Certificate[0])
		assert.NoError(t, err)
		assert.Equal(t, "warp-agent", pCert2.Subject.CommonName)

		cert2, err = reloader.GetClientCertificate(nil)
		assert.NoError(t, err)
		pCert2, err = x509.ParseCertificate(cert2.Certificate[0])
		assert.NoError(t, err)
		assert.Equal(t, "warp-agent", pCert2.Subject.CommonName)

		// Assert that reload logic triggered by examining logs
		assert.NotEmpty(t, logs.FilterMessage("Certificate was updated. Scheduling update."))
		assert.NotEmpty(t, logs.FilterMessage("Reloading certificate"))
		assert.NotEmpty(t, logs.FilterMessage("Failed to reload certificate"))
	})
}
