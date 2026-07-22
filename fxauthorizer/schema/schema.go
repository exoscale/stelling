package schema

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"net"
	"net/http"
	"net/url"

	"github.com/coreos/go-oidc/v3/oidc"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"
)

func NewHeaders(headers map[string][]string) map[string]*HeaderValues {
	output := make(map[string]*HeaderValues, len(headers))
	for k, vals := range headers {
		output[http.CanonicalHeaderKey(k)] = &HeaderValues{Value: vals}
	}
	return output
}

// jwtCustomClaims holds the non-standard claims we lift out of an ID token's claim set.
// They're optional in the OIDC spec, so a provider that doesn't set them just leaves these zero.
type jwtCustomClaims struct {
	Email         string   `json:"email"`
	EmailVerified bool     `json:"email_verified"`
	Groups        []string `json:"groups"`
}

func NewJWT(token *oidc.IDToken) *JWT {
	if token == nil {
		return nil
	}

	var custom jwtCustomClaims
	// Claims() only fails if the token has no claim set at all, which can't happen for a verified
	// token; providers that omit email/groups just leave those fields at their zero value.
	_ = token.Claims(&custom)

	return &JWT{
		Subject:       token.Subject,
		Email:         custom.Email,
		EmailVerified: custom.EmailVerified,
		Groups:        custom.Groups,
		Issuer:        token.Issuer,
		Audience:      token.Audience,
		IssuedAt:      timestamppb.New(token.IssuedAt),
		Expiry:        timestamppb.New(token.Expiry),
	}
}

func NewTLS(cert *x509.Certificate) *TLS {
	return &TLS{
		Version:        uint64(cert.Version),
		SerialNumber:   cert.SerialNumber.Uint64(),
		Issuer:         tlsName(&cert.Issuer),
		Subject:        tlsName(&cert.Subject),
		NotBefore:      timestamppb.New(cert.NotBefore),
		NotAfter:       timestamppb.New(cert.NotAfter),
		IsCa:           cert.IsCA,
		DnsNames:       cert.DNSNames,
		EmailAddresses: cert.EmailAddresses,
		IpAddresses:    ipaddresses(cert.IPAddresses),
		Uris:           uris(cert.URIs),
	}
}

func tlsName(name *pkix.Name) *TLSName {
	return &TLSName{
		Country:            name.Country,
		Organization:       name.Organization,
		OrganizationalUnit: name.OrganizationalUnit,
		Locality:           name.Locality,
		Province:           name.Province,
		StreetAddress:      name.StreetAddress,
		PostalCode:         name.PostalCode,
		SerialNumber:       name.SerialNumber,
		CommonName:         name.CommonName,
	}
}

func ipaddresses(input []net.IP) []string {
	output := make([]string, len(input))

	for i := range input {
		output[i] = input[i].String()
	}

	return output
}

func uris(input []*url.URL) []string {
	output := make([]string, len(input))

	for i := range input {
		output[i] = input[i].String()
	}

	return output
}
