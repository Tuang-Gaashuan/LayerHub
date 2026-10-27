package updatekit

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestRateLimitExplainsRecoveryWithoutLeakingResponse(t *testing.T) {
	r := &http.Response{StatusCode: http.StatusForbidden, Header: http.Header{}}
	r.Header.Set("X-RateLimit-Remaining", "0")
	r.Header.Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10))
	err := responseError(r)
	if !strings.Contains(err.Error(), "Retry after:") || !strings.Contains(err.Error(), "离线更新") {
		t.Fatal(err)
	}
	r.Header.Set("X-RateLimit-Reset", "invalid-untrusted-header")
	if strings.Contains(responseError(r).Error(), "invalid-untrusted-header") {
		t.Fatal("untrusted response header exposed")
	}
	r.StatusCode = http.StatusNotFound
	if strings.Contains(responseError(r).Error(), "rate limit reached") {
		t.Fatal("non-rate-limit error misclassified")
	}
}

func TestChooseChannelSemver(t *testing.T) {
	rows := []Release{{Tag: "v0.9.0-alpha"}, {Tag: "v0.2.9-beta"}, {Tag: "v0.2.10-beta"}, {Tag: "v0.3.0-beta", Draft: true}, {Tag: "v0.2.10-beta.2"}, {Tag: "v1.0.0"}}
	r, e := choose(rows, "beta")
	if e != nil || r.Tag != "v0.2.10-beta.2" {
		t.Fatal(r, e)
	}
	r, e = choose(rows, "alpha")
	if e != nil || r.Tag != "v0.9.0-alpha" {
		t.Fatal(r, e)
	}
	if _, e = compare("v0.1.0-beta", "v0.4.0-alpha"); e == nil {
		t.Fatal("cross-channel comparison accepted")
	}
	if _, e = choose([]Release{{Tag: "v0.1.0-alpha"}}, "beta"); e == nil {
		t.Fatal("selected another channel")
	}
}
func TestOfficialURLs(t *testing.T) {
	for _, raw := range []string{"https://api.github.com/repos/a/b", "https://github.com/a/b", "https://release-assets.githubusercontent.com/file"} {
		if !officialURL(raw) {
			t.Fatal(raw)
		}
	}
	for _, raw := range []string{"http://github.com/a", "https://github.com.evil.test/a", "https://githubusercontent.com.evil.test/a", "https://user@github.com/a", "https://github.com:8443/a", "file:///tmp/a"} {
		if officialURL(raw) {
			t.Fatal(raw)
		}
	}
}
func TestChecksumRequiresUniqueExactFilename(t *testing.T) {
	sum := strings.Repeat("a", 64)
	if got, e := checksum([]byte(sum+"  x.zip\n"), "x.zip"); e != nil || got != sum {
		t.Fatal(got, e)
	}
	for _, raw := range []string{sum + "  x.zip\n" + sum + "  x.zip\n", "invalid  x.zip\n", sum + "  y.zip\n"} {
		if _, e := checksum([]byte(raw), "x.zip"); e == nil {
			t.Fatal("bad checksum accepted")
		}
	}
}
func TestAssetOriginAndSize(t *testing.T) {
	valid := Asset{Name: "asset", URL: "https://api.github.com/repos/" + Repository + "/releases/assets/1", Size: 5}
	if _, e := asset(Release{Assets: []Asset{valid}}, "asset"); e != nil {
		t.Fatal(e)
	}
	for _, a := range []Asset{{Name: "asset", URL: "https://api.github.com/repos/evil/project/releases/assets/1", Size: 5}, {Name: "asset", URL: valid.URL, Size: maxAsset + 1}} {
		if _, e := asset(Release{Assets: []Asset{a}}, "asset"); e == nil {
			t.Fatal("bad asset")
		}
	}
	if _, e := asset(Release{Assets: []Asset{valid, valid}}, "asset"); e == nil {
		t.Fatal("duplicate asset")
	}
}
func TestResumeAndIgnoredRange(t *testing.T) {
	for _, resumes := range []bool{false, true} {
		t.Run(map[bool]string{false: "server_ignores_range", true: "range_supported"}[resumes], func(t *testing.T) {
			body := []byte("correct immutable payload")
			sum := sha256.Sum256(body)
			file := filepath.Join(t.TempDir(), "payload")
			if e := os.WriteFile(file+".part", body[:7], 0o600); e != nil {
				t.Fatal(e)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Range") != "bytes=7-" {
					t.Errorf("range %q", r.Header.Get("Range"))
				}
				if resumes {
					w.Header().Set("Content-Range", "bytes 7-24/25")
					w.WriteHeader(206)
					w.Write(body[7:])
				} else {
					w.Write(body)
				}
			}))
			defer server.Close()
			if e := downloadAsset(server.Client(), Asset{URL: server.URL, Size: int64(len(body))}, file, hex.EncodeToString(sum[:])); e != nil {
				t.Fatal(e)
			}
			raw, _ := os.ReadFile(file)
			if !bytes.Equal(raw, body) {
				t.Fatal("bad payload")
			}
		})
	}
}
func TestWrongHashNeverBecomesFinalFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("bad")) }))
	defer server.Close()
	file := filepath.Join(t.TempDir(), "archive")
	if e := downloadAsset(server.Client(), Asset{URL: server.URL, Size: 3}, file, strings.Repeat("a", 64)); e == nil {
		t.Fatal("bad digest accepted")
	}
	if _, e := os.Stat(file); !os.IsNotExist(e) {
		t.Fatal("unverified final file exists")
	}
}
func TestArchivePathValidation(t *testing.T) {
	for _, name := range []string{"/escape", "LayerHub-0.1.0-beta/../escape", "LayerHub-0.1.0-beta/C:/escape", "LayerHub-0.1.0-beta/a\\b", "LayerHub-0.1.0-beta/NUL.txt", "LayerHub-0.1.0-beta/a. ", "other/root"} {
		if _, e := cleanMember(name, "LayerHub-0.1.0-beta", map[string]bool{}); e == nil {
			t.Fatal(name)
		}
	}
	seen := map[string]bool{}
	cleanMember("LayerHub-0.1.0-beta/A", "LayerHub-0.1.0-beta", seen)
	if _, e := cleanMember("LayerHub-0.1.0-beta/a", "LayerHub-0.1.0-beta", seen); e == nil {
		t.Fatal("case conflict accepted")
	}
}
func fixtureZip(t *testing.T, path string, extra string) {
	t.Helper()
	f, e := os.Create(path)
	if e != nil {
		t.Fatal(e)
	}
	z := zip.NewWriter(f)
	files := map[string]string{"VERSION": "0.1.0-beta\n", "web/index.html": "index", "web/source-offer.html": "offer", "web/source/layerhub-source.tar.gz": "source"}
	for name, value := range files {
		w, _ := z.Create("LayerHub-0.1.0-beta/" + name)
		io.WriteString(w, value)
	}
	if extra != "" {
		w, _ := z.Create(extra)
		io.WriteString(w, "invalid")
	}
	z.Close()
	f.Close()
}
func TestExtractNewFolderAndProtectExistingData(t *testing.T) {
	root := t.TempDir()
	archive := filepath.Join(root, "bundle.zip")
	fixtureZip(t, archive, "")
	dest := filepath.Join(root, "app")
	if _, e := Extract(archive, dest, "v0.1.0-beta"); e != nil {
		t.Fatal(e)
	}
	os.Mkdir(filepath.Join(dest, "data"), 0o700)
	os.WriteFile(filepath.Join(dest, "data/private"), []byte("keep"), 0o600)
	if _, e := Extract(archive, dest, "v0.1.0-beta"); e == nil {
		t.Fatal("existing directory accepted")
	}
	raw, _ := os.ReadFile(filepath.Join(dest, "data/private"))
	if string(raw) != "keep" {
		t.Fatal("private data changed")
	}
}
func TestExtractRejectsTraversalAndTarLinks(t *testing.T) {
	root := t.TempDir()
	zipPath := filepath.Join(root, "bad.zip")
	fixtureZip(t, zipPath, "LayerHub-0.1.0-beta/../../escaped")
	if _, e := Extract(zipPath, filepath.Join(root, "bad"), "v0.1.0-beta"); e == nil {
		t.Fatal("traversal")
	}
	if _, e := os.Stat(filepath.Join(root, "escaped")); !os.IsNotExist(e) {
		t.Fatal("escaped")
	}
	tarPath := filepath.Join(root, "link.tar.gz")
	f, _ := os.Create(tarPath)
	g := gzip.NewWriter(f)
	writer := tar.NewWriter(g)
	writer.WriteHeader(&tar.Header{Name: "LayerHub-0.1.0-beta/link", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"})
	writer.Close()
	g.Close()
	f.Close()
	if _, e := Extract(tarPath, filepath.Join(root, "links"), "v0.1.0-beta"); e == nil {
		t.Fatal("symlink accepted")
	}
}
