package updatekit

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
)

func TestReplacingConfigPreservesLiveModeAndOwner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	if e := os.WriteFile(path, []byte("original"), 0o640); e != nil {
		t.Fatal(e)
	}
	before, _ := os.Stat(path)
	owner := before.Sys().(*syscall.Stat_t)
	if e := replaceFile(path, []byte("changed config"), before.Mode().Perm()); e != nil {
		t.Fatal(e)
	}
	after, _ := os.Stat(path)
	changed := after.Sys().(*syscall.Stat_t)
	if after.Mode().Perm() != 0o640 || owner.Uid != changed.Uid || owner.Gid != changed.Gid {
		t.Fatal("permissions changed")
	}
}

func TestRootCacheRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "real")
	os.Mkdir(real, 0o755)
	link := filepath.Join(root, "link")
	if e := os.Symlink(real, link); e != nil {
		t.Fatal(e)
	}
	if secureCache(link) == nil {
		t.Fatal("cache symlink accepted")
	}
}

func TestGeneratedNginxConfigurationParsesInIsolatedPrefix(t *testing.T) {
	binary, e := exec.LookPath("nginx")
	if e != nil {
		t.Skip("nginx not installed")
	}
	root := t.TempDir()
	original := `server { listen 127.0.0.1:18977; add_header X-Content-Type-Options nosniff always; location ^~ /internal/ {return 404;} location / {proxy_pass http://127.0.0.1:15181; error_page 502 504 =503 @offline;} location @offline { return 503 '{"ok":false}'; } }`
	next, e := cloudConfig(original, root+"/web")
	if e != nil {
		t.Fatal(e)
	}
	config := "pid " + root + "/test.pid; error_log stderr; events {} http { access_log off; " + next + " }"
	path := filepath.Join(root, "nginx.conf")
	if e = os.WriteFile(path, []byte(config), 0o600); e != nil {
		t.Fatal(e)
	}
	raw, e := exec.Command(binary, "-t", "-p", root+"/", "-c", path).CombinedOutput()
	if e != nil {
		t.Fatal(string(raw), e)
	}
}

func TestNewPublicReleaseModesSurvivePrivateUmask(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source")
	os.MkdirAll(filepath.Join(source, "web"), 0o755)
	os.MkdirAll(filepath.Join(source, "bin"), 0o755)
	os.WriteFile(filepath.Join(source, "web/index.html"), []byte("public code"), 0o644)
	os.WriteFile(filepath.Join(source, "bin/layerhub"), []byte("public executable"), 0o755)
	old := syscall.Umask(0o077)
	defer syscall.Umask(old)
	target := filepath.Join(t.TempDir(), "new")
	if e := copyTree(source, target); e != nil {
		t.Fatal(e)
	}
	for name, mode := range map[string]os.FileMode{"": 0o755, "web": 0o755, "bin": 0o755, "web/index.html": 0o644, "bin/layerhub": 0o755} {
		s, e := os.Stat(filepath.Join(target, name))
		if e != nil || s.Mode().Perm() != mode {
			t.Fatal(name, s, e)
		}
	}
}
