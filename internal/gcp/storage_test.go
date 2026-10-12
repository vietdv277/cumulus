package gcp

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

func TestParseGCSPath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		remote  bool
		bucket  string
		key     string
		wantErr string
	}{
		{"gs path", "gs://bkt/dir/key.txt", true, "bkt", "dir/key.txt", ""},
		{"gs bucket only", "gs://bkt", true, "bkt", "", ""},
		{"gs dir prefix", "gs://bkt/logs/", true, "bkt", "logs/", ""},
		{"local file", "file.txt", false, "", "", ""},
		{"local dir", "./dir", false, "", "", ""},
		{"gs empty", "gs://", true, "", "", "invalid gs path"},
		{"gs missing bucket", "gs:///key.txt", true, "", "", "invalid gs path"},
		{"s3 mismatch", "s3://bkt/key.txt", false, "", "", "use gs:// paths on GCP"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			remote, bucket, key, err := parseGCSPath(tt.path)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("parseGCSPath(%q) error = nil, want %q", tt.path, tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("parseGCSPath(%q) error = %q, want %q", tt.path, err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseGCSPath(%q) unexpected error: %v", tt.path, err)
			}
			if remote != tt.remote || bucket != tt.bucket || key != tt.key {
				t.Errorf("parseGCSPath(%q) = (%v, %q, %q), want (%v, %q, %q)",
					tt.path, remote, bucket, key, tt.remote, tt.bucket, tt.key)
			}
		})
	}
}

func TestResolveCopyDestKey(t *testing.T) {
	tests := []struct {
		name   string
		dstKey string
		srcKey string
		want   string
	}{
		{"gs dir key ending in slash", "reports/2024/", "data.csv", "reports/2024/data.csv"},
		{"empty dst uses source base", "", "data.csv", "data.csv"},
		{"bare slash", "/", "data.csv", "data.csv"},
		{"full dst key is kept", "reports/backup.csv", "data.csv", "reports/backup.csv"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveCopyDestKey(tt.dstKey, tt.srcKey); got != tt.want {
				t.Errorf("resolveCopyDestKey(%q, %q) = %q, want %q", tt.dstKey, tt.srcKey, got, tt.want)
			}
		})
	}
}

func TestNormalizePresignExpiry(t *testing.T) {
	tests := []struct {
		name    string
		in      int
		want    int
		wantErr string
	}{
		{"zero defaults to 1h", 0, 3600, ""},
		{"negative defaults to 1h", -5, 3600, ""},
		{"minimum 1s accepted", 1, 1, ""},
		{"valid value kept", 60, 60, ""},
		{"max 7 days accepted", 604800, 604800, ""},
		{"over 7 days rejected", 604801, 0, "exceeds the GCS limit"},
		{"far over 7 days rejected", 100000000, 0, "exceeds the GCS limit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizePresignExpiry(tt.in)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("normalizePresignExpiry(%d) unexpected error: %v", tt.in, err)
				}
				if got != tt.want {
					t.Errorf("normalizePresignExpiry(%d) = %d, want %d", tt.in, got, tt.want)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("normalizePresignExpiry(%d) error = %v, want %q", tt.in, err, tt.wantErr)
			}
		})
	}
}

func TestParseServiceAccountKey(t *testing.T) {
	saKey := `{"client_email":"sa@project.iam.gserviceaccount.com","private_key":"-----BEGIN PRIVATE KEY-----\n"}`
	userKey := `{"client_id":"123.apps.googleusercontent.com","refresh_token":"t"}`
	tests := []struct {
		name  string
		raw   string
		email string
		ok    bool
	}{
		{"service account key", saKey, "sa@project.iam.gserviceaccount.com", true},
		{"gcloud user credentials", userKey, "", false},
		{"invalid json", "{", "", false},
		{"empty", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			acct, ok := parseServiceAccountKey([]byte(tt.raw))
			if ok != tt.ok {
				t.Fatalf("parseServiceAccountKey(%s) ok = %v, want %v", tt.name, ok, tt.ok)
			}
			if ok && acct.ClientEmail != tt.email {
				t.Errorf("parseServiceAccountKey(%s) email = %q, want %q", tt.name, acct.ClientEmail, tt.email)
			}
		})
	}
}

// testPrivPEM generates an RSA key for the test and returns it as the PKCS#8
// PEM a service-account key file carries.
func testPrivPEM(t *testing.T) (*rsa.PrivateKey, string) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	pemKey := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	return priv, pemKey
}

func TestSignedV4GetURL(t *testing.T) {
	priv, pemKey := testPrivPEM(t)
	acct := &serviceAccountKey{
		ClientEmail: "sa@project.iam.gserviceaccount.com",
		PrivateKey:  pemKey,
	}

	now := time.Date(2026, 6, 15, 10, 30, 0, 0, time.UTC)
	urlStr, err := signedV4GetURL(acct, "my-bucket", "dir/key with space.txt", 3600, now)
	if err != nil {
		t.Fatalf("signedV4GetURL unexpected error: %v", err)
	}

	u, err := url.Parse(urlStr)
	if err != nil {
		t.Fatalf("parse signed URL: %v", err)
	}
	if u.Scheme != "https" || u.Host != "storage.googleapis.com" {
		t.Errorf("host = %s://%s, want https://storage.googleapis.com", u.Scheme, u.Host)
	}
	if u.EscapedPath() != "/my-bucket/dir/key%20with%20space.txt" {
		t.Errorf("path = %q, want /my-bucket/dir/key%%20with%%20space.txt", u.EscapedPath())
	}

	// Everything below is written from the GCS V4 spec layout for this fixed
	// input, independent of the package's own helpers.

	const wantQuery = "X-Goog-Algorithm=GOOG4-RSA-SHA256" +
		"&X-Goog-Credential=sa%40project.iam.gserviceaccount.com%2F20260615%2Fauto%2Fstorage%2Fgoog4_request" +
		"&X-Goog-Date=20260615T103000Z" +
		"&X-Goog-Expires=3600" +
		"&X-Goog-SignedHeaders=host"

	const canonicalRequest = "GET\n" +
		"/my-bucket/dir/key%20with%20space.txt\n" +
		"X-Goog-Algorithm=GOOG4-RSA-SHA256" +
		"&X-Goog-Credential=sa%40project.iam.gserviceaccount.com%2F20260615%2Fauto%2Fstorage%2Fgoog4_request" +
		"&X-Goog-Date=20260615T103000Z" +
		"&X-Goog-Expires=3600" +
		"&X-Goog-SignedHeaders=host\n" +
		"host:storage.googleapis.com\n" +
		"\n" +
		"host\n" +
		"UNSIGNED-PAYLOAD"

	const wantStringToSign = "GOOG4-RSA-SHA256\n" +
		"20260615T103000Z\n" +
		"20260615/auto/storage/goog4_request\n" +
		"081fb15c62c04c1318ea430a3b0fe6c753289fe22c77009d0cb59c1cc64df5c7"

	// Guard the hard-coded digest in wantStringToSign against the canonical
	// request literal it was derived from.
	reqDigest := sha256.Sum256([]byte(canonicalRequest))
	if got := hex.EncodeToString(reqDigest[:]); !strings.Contains(wantStringToSign, got) {
		t.Fatalf("hard-coded string-to-sign digest mismatch: canonical request hashes to %s", got)
	}

	// The query before the signature must be the canonical query string.
	const sigParam = "X-Goog-Signature="
	rawQuery := u.RawQuery
	idx := strings.Index(rawQuery, sigParam)
	if idx < 0 {
		t.Fatalf("signed URL has no %s param: %s", sigParam, rawQuery)
	}
	query := rawQuery[:idx-1]
	sig, err := hex.DecodeString(rawQuery[idx+len(sigParam):])
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}
	if query != wantQuery {
		t.Errorf("canonical query =\n%s\nwant\n%s", query, wantQuery)
	}

	// The signature covers the spec string-to-sign — verify with the public key.
	msgDigest := sha256.Sum256([]byte(wantStringToSign))
	if err := rsa.VerifyPKCS1v15(&priv.PublicKey, crypto.SHA256, msgDigest[:], sig); err != nil {
		t.Errorf("signature verification failed: %v", err)
	}
}

func TestSignedV4GetURLBadKey(t *testing.T) {
	acct := &serviceAccountKey{
		ClientEmail: "sa@project.iam.gserviceaccount.com",
		PrivateKey:  "not a pem",
	}
	if _, err := signedV4GetURL(acct, "bkt", "obj", 3600, time.Now()); err == nil {
		t.Errorf("signedV4GetURL with a broken key error = nil, want error")
	}
}

// testStorageProvider builds a GCPStorageProvider wired to a test endpoint.
func testStorageProvider(t *testing.T, endpoint, project string) *GCPStorageProvider {
	t.Helper()
	client := &Client{
		credentials: &google.Credentials{
			TokenSource: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "test-token"}),
		},
		project: project,
	}
	p := NewStorageProvider(client)
	p.endpoint = endpoint
	return p
}

func TestListBucketsPagination(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if got := r.URL.Query().Get("project"); got != "test-project" {
			t.Errorf("project param = %q, want test-project", got)
		}
		if r.URL.Query().Get("pageToken") == "" {
			io.WriteString(w, `{"nextPageToken":"t1","items":[`+
				`{"name":"b1","location":"EUROPE-WEST1","timeCreated":"2026-01-02T03:04:05Z"},`+
				`{"name":"b2"}]}`)
			return
		}
		io.WriteString(w, `{"items":[{"name":"b3","timeCreated":"2026-02-03T04:05:06Z"}]}`)
	}))
	defer srv.Close()

	p := testStorageProvider(t, srv.URL, "test-project")
	buckets, err := p.ListBuckets(context.Background())
	if err != nil {
		t.Fatalf("ListBuckets unexpected error: %v", err)
	}
	if calls != 2 {
		t.Errorf("made %d page calls, want 2", calls)
	}
	if len(buckets) != 3 {
		t.Fatalf("got %d buckets, want 3", len(buckets))
	}
	if buckets[0].Name != "b1" || buckets[0].Provider != "gcp" || buckets[0].Region != "EUROPE-WEST1" {
		t.Errorf("b1 = %+v", buckets[0])
	}
	if buckets[0].CreatedAt != time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) {
		t.Errorf("b1 created = %v", buckets[0].CreatedAt)
	}
	if !buckets[1].CreatedAt.IsZero() {
		t.Errorf("b2 created without timeCreated = %v, want zero", buckets[1].CreatedAt)
	}
}

func TestListObjectsPagination(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if got := r.URL.Query().Get("prefix"); got != "logs/" {
			t.Errorf("prefix param = %q, want logs/", got)
		}
		if r.URL.Query().Get("pageToken") == "" {
			io.WriteString(w, `{"nextPageToken":"t1","items":[`+
				`{"name":"logs/a.txt","size":"100","updated":"2026-01-02T03:04:05Z","etag":"\"e1\""},`+
				`{"name":"logs/b.txt","size":"200"}]}`)
			return
		}
		io.WriteString(w, `{"items":[{"name":"logs/c.txt","size":"300"}]}`)
	}))
	defer srv.Close()

	p := testStorageProvider(t, srv.URL, "test-project")
	objects, err := p.ListObjects(context.Background(), "objects-bkt", "logs/")
	if err != nil {
		t.Fatalf("ListObjects unexpected error: %v", err)
	}
	if calls != 2 {
		t.Errorf("made %d page calls, want 2", calls)
	}
	if len(objects) != 3 {
		t.Fatalf("got %d objects, want 3", len(objects))
	}
	if objects[0].Key != "logs/a.txt" || objects[0].Size != 100 || objects[0].ETag != "e1" {
		t.Errorf("a.txt = %+v", objects[0])
	}
	if objects[0].LastModified != time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) {
		t.Errorf("a.txt last modified = %v", objects[0].LastModified)
	}
}

func TestCopyRewriteLoop(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/b/src-b/o/src-o/rewriteTo/b/dst-b/o/dst-o") {
			t.Errorf("path = %s", r.URL.Path)
		}
		switch r.URL.Query().Get("rewriteToken") {
		case "":
			io.WriteString(w, `{"done": false, "rewriteToken": "t1"}`)
		case "t1":
			io.WriteString(w, `{"done": false, "rewriteToken": "t2"}`)
		default:
			io.WriteString(w, `{"done": true, "resource": {"name": "dst-o"}}`)
		}
	}))
	defer srv.Close()

	p := testStorageProvider(t, srv.URL, "test-project")
	if err := p.Copy(context.Background(), "gs://src-b/src-o", "gs://dst-b/dst-o"); err != nil {
		t.Fatalf("Copy gs->gs unexpected error: %v", err)
	}
	if calls != 3 {
		t.Errorf("rewrite took %d calls, want 3 (loop until done)", calls)
	}
}

func TestCopyErrors(t *testing.T) {
	p := testStorageProvider(t, "http://unused.invalid", "test-project")
	ctx := context.Background()

	if err := p.Copy(ctx, "s3://bkt/key.txt", "gs://bkt2/key.txt"); err == nil {
		t.Errorf("Copy s3:// error = nil, want scheme mismatch error")
	}
	if err := p.Copy(ctx, "local.txt", "other.txt"); err == nil {
		t.Errorf("Copy local->local error = nil, want error")
	}
	if _, err := p.Presign(ctx, "local.txt", 3600); err == nil {
		t.Errorf("Presign local path error = nil, want error")
	}
}

func TestCopyUploadAndDownload(t *testing.T) {
	var uploaded string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			if !strings.HasPrefix(r.URL.Path, "/upload/storage/v1/b/up-b/o") {
				t.Errorf("upload path = %s", r.URL.Path)
			}
			body, _ := io.ReadAll(r.Body)
			uploaded = string(body)
			io.WriteString(w, `{"name": "file.txt"}`)
		case http.MethodGet:
			if !strings.HasSuffix(r.URL.Path, "/b/dl-b/o/dir/file.bin") {
				t.Errorf("download path = %s", r.URL.Path)
			}
			io.WriteString(w, "downloaded content")
		default:
			t.Errorf("unexpected method %s", r.Method)
		}
	}))
	defer srv.Close()

	p := testStorageProvider(t, srv.URL, "test-project")
	ctx := context.Background()

	// Local -> gs, with a directory-style destination that must receive the
	// source base name.
	srcDir := t.TempDir()
	src := filepath.Join(srcDir, "file.txt")
	if err := os.WriteFile(src, []byte("hello upload"), 0o600); err != nil {
		t.Fatalf("write src: %v", err)
	}
	if err := p.Copy(ctx, src, "gs://up-b/folder/"); err != nil {
		t.Fatalf("Copy upload unexpected error: %v", err)
	}
	if !strings.Contains(uploaded, "hello upload") || !strings.Contains(uploaded, "folder/file.txt") {
		t.Errorf("upload body missing object name or content: %s", uploaded)
	}

	// gs -> local directory: temp file staged in the destination dir, renamed
	// into place, nothing left behind on success.
	dstDir := t.TempDir()
	if err := p.Copy(ctx, "gs://dl-b/dir/file.bin", dstDir); err != nil {
		t.Fatalf("Copy download unexpected error: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dstDir, "file.bin"))
	if err != nil {
		t.Fatalf("read download: %v", err)
	}
	if string(got) != "downloaded content" {
		t.Errorf("download content = %q, want %q", string(got), "downloaded content")
	}
	entries, err := os.ReadDir(dstDir)
	if err != nil {
		t.Fatalf("read dst dir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "file.bin" {
		for _, e := range entries {
			t.Errorf("leftover download artifact: %s", e.Name())
		}
	}
}

func TestPresignNeedsServiceAccount(t *testing.T) {
	// gcloud user credentials (no private key) cannot sign.
	client := &Client{
		credentials: &google.Credentials{JSON: []byte(`{"client_id":"x","refresh_token":"y"}`)},
	}
	p := NewStorageProvider(client)
	_, err := p.Presign(context.Background(), "gs://bkt/key.txt", 3600)
	if err == nil {
		t.Fatalf("Presign with user credentials error = nil, want error")
	}
	if err.Error() != "presign needs service-account credentials (key file or impersonation)" {
		t.Errorf("Presign error = %q", err.Error())
	}
}
