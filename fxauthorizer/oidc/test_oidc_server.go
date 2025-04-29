package oidc

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
)

// setOIDCTest creates a key, OIDCServer and initilises an OIDC provider
func setupOIDCTest(t *testing.T, bodyValues map[string]map[string]string) (*httptest.Server, *rsaKey) {
	t.Helper()
	// Generate key
	key, err := newRSAKey()
	if err != nil {
		t.Fatal(err)
	}

	body := make(map[string]string)
	// URL encode bodyValues into body
	for method, values := range bodyValues {
		q := url.Values{}
		for k, v := range values {
			q.Set(k, v)
		}
		body[method] = q.Encode()
	}

	// Set up oidc server
	server := NewOIDCServer(t, key, body)
	t.Cleanup(server.Close)

	return server, key
}

// OIDCServer is used in the OIDC Tests to mock an OIDC server
type OIDCServer struct {
	t    *testing.T
	url  string
	body map[string]string // method -> body
	key  *rsaKey
}

func NewOIDCServer(t *testing.T, key *rsaKey, body map[string]string) *httptest.Server {
	t.Helper()
	handler := &OIDCServer{t: t, key: key, body: body}
	server := httptest.NewServer(handler)
	handler.url = server.URL
	return server
}

func (s *OIDCServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)

	switch r.URL.Path {
	case "/.well-known/openid-configuration":
		// Open id config
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"issuer":"`+s.url+`",
			"authorization_endpoint":"`+s.url+`/auth",
			"token_endpoint":"`+s.url+`/token",
			"jwks_uri":"`+s.url+`/jwks"
		}`)
	case "/token":
		// Token request
		// Check body
		if b, ok := s.body["token"]; ok {
			if b != string(body) {
				s.t.Fatal("Unexpected request body, expected", b, "got", string(body))
			}
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"access_token":"123456789",
			"id_token":"id_123456789"
		}`)
	case "/jwks":
		// Key request
		w.Header().Set("Content-Type", "application/json")
		pubkey, err := s.key.publicJWK()
		if err != nil {
			s.t.Fatalf("Failed to get public key: %v", err)
		}
		fmt.Fprint(w, `{"keys":[`+pubkey+`]}`)
	default:
		s.t.Fatal("Unrecognised request: ", r.URL, string(body))
	}
}

// rsaKey is used in the OIDCServer tests to sign and verify requests
type rsaKey struct {
	key     *rsa.PrivateKey
	alg     jose.SignatureAlgorithm
	jwkPub  *jose.JSONWebKey
	jwkPriv *jose.JSONWebKey
}

func newRSAKey() (*rsaKey, error) {
	key, err := rsa.GenerateKey(rand.Reader, 1028)
	if err != nil {
		return nil, err
	}

	return &rsaKey{
		key: key,
		alg: jose.RS256,
		jwkPub: &jose.JSONWebKey{
			Key:       key.Public(),
			Algorithm: string(jose.RS256),
		},
		jwkPriv: &jose.JSONWebKey{
			Key:       key,
			Algorithm: string(jose.RS256),
		},
	}, nil
}

func (k *rsaKey) publicJWK() (string, error) {
	b, err := k.jwkPub.MarshalJSON()
	if err != nil {
		return "", err
	}

	return string(b), nil
}

// sign creates a JWS using the private key from the provided payload.
func (k *rsaKey) sign(payload []byte) (string, error) {
	signer, err := jose.NewSigner(jose.SigningKey{
		Algorithm: k.alg,
		Key:       k.key,
	}, nil)
	if err != nil {
		return "", err
	}
	jws, err := signer.Sign(payload)
	if err != nil {
		return "", err
	}

	data, err := jws.CompactSerialize()
	if err != nil {
		return "", err
	}
	return data, nil
}

func (k *rsaKey) createIDToken(serverURL, user, email string, groups []string) (string, error) {
	input := []byte(`{
		"iss": "` + serverURL + `",
		"exp":` + strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10) + `,
		"aud": "idtest",
		"sub": "` + user + `",
		"email": "` + email + `",
		"groups": ` + printStringSlice(groups) + `,
		"email_verified": true
	}`)
	return k.sign(input)
}

func printStringSlice(input []string) string {
	if len(input) == 0 {
		return "[]"
	}
	return `["` + strings.Join(input, "\", \"") + `"]`
}
