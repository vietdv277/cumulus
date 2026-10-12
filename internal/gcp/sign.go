package gcp

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// V4 signing constants for presigned GCS GET URLs.
const (
	goog4Algorithm = "GOOG4-RSA-SHA256"
	goog4Request   = "go4_request"
	gcsHost        = "storage.googleapis.com"
	// gcsScopeRegion: GCS accepts "auto" for the region in the credential scope.
	gcsScopeRegion = "auto"
)

// serviceAccountKey is the subset of a service-account key file that V4
// signing needs.
type serviceAccountKey struct {
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
}

// parseServiceAccountKey reports whether raw (ADC credential JSON) is a
// service-account key and returns its fields; user-quota and metadata
// credentials have no client_email/private_key and cannot sign.
func parseServiceAccountKey(raw []byte) (*serviceAccountKey, bool) {
	var k serviceAccountKey
	if err := json.Unmarshal(raw, &k); err != nil || k.ClientEmail == "" || k.PrivateKey == "" {
		return nil, false
	}
	return &k, true
}

// signedV4GetURL builds a V4 presigned GET URL for bucket/object, signed by
// acct at clock time now and valid for expiry seconds.
func signedV4GetURL(acct *serviceAccountKey, bucket, object string, expiry int, now time.Time) (string, error) {
	priv, err := parsePrivateKeyPEM(acct.PrivateKey)
	if err != nil {
		return "", fmt.Errorf("presign: %w", err)
	}

	timestamp := now.UTC().Format("20060102T150405Z")
	datestamp := timestamp[:8]
	scope := datestamp + "/" + gcsScopeRegion + "/storage/" + goog4Request
	credential := acct.ClientEmail + "/" + scope

	canonicalResource := "/" + bucket + "/" + gcsObjectEscape(object)
	canonicalRequest := v4CanonicalRequest(canonicalResource, v4Query(credential, timestamp, expiry))
	digest := sha256.Sum256([]byte(canonicalRequest))
	toSign := strings.Join([]string{
		goog4Algorithm,
		timestamp,
		scope,
		hex.EncodeToString(digest[:]),
	}, "\n")

	msgDigest := sha256.Sum256([]byte(toSign))
	sig, err := rsa.SignPKCS1v15(rand.Reader, priv, crypto.SHA256, msgDigest[:])
	if err != nil {
		return "", fmt.Errorf("presign: %w", err)
	}

	return fmt.Sprintf("https://%s/%s/%s?%s&X-Goog-Signature=%s",
		gcsHost, bucket, gcsObjectEscape(object),
		v4Query(credential, timestamp, expiry),
		hex.EncodeToString(sig),
	), nil
}

// v4Query builds the canonical, already-escaped query string: the five
// X-Goog-* parameters in sort order, values percent-encoded.
func v4Query(credential, timestamp string, expiry int) string {
	return strings.Join([]string{
		"X-Goog-Algorithm=" + goog4Algorithm,
		"X-Goog-Credential=" + v4Escape(credential),
		"X-Goog-Date=" + timestamp,
		"X-Goog-Expires=" + strconv.Itoa(expiry),
		"X-Goog-SignedHeaders=host",
	}, "&")
}

// v4CanonicalRequest assembles the V4 canonical request for a presigned GET
// against the GCS host (only the host header is signed; payload is unsigned).
func v4CanonicalRequest(canonicalResource, canonicalQuery string) string {
	return strings.Join([]string{
		"GET",
		canonicalResource,
		canonicalQuery,
		"host:" + gcsHost,
		"",
		"host",
		"UNSIGNED-PAYLOAD",
	}, "\n")
}

// v4Escape percent-encodes s: every byte outside the RFC 3986 unreserved set.
func v4Escape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '-' || c == '_' || c == '.' || c == '~',
			c >= '0' && c <= '9',
			c >= 'a' && c <= 'z',
			c >= 'A' && c <= 'Z':
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// gcsObjectEscape encodes an object name for the canonical resource, keeping
// the '/' separators.
func gcsObjectEscape(object string) string {
	parts := strings.Split(object, "/")
	for i, p := range parts {
		parts[i] = v4Escape(p)
	}
	return strings.Join(parts, "/")
}

// parsePrivateKeyPEM parses the RSA private key from the PEM in a
// service-account key file (PKCS#8 or PKCS#1).
func parsePrivateKeyPEM(pemKey string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemKey))
	if block == nil {
		return nil, fmt.Errorf("no PEM block in service-account private key")
	}
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("service-account key is not RSA")
		}
		return rsaKey, nil
	}
	rsaKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse service-account private key: %w", err)
	}
	return rsaKey, nil
}
