package updatekit

import (
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

func secureCache(path string) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) == "/" {
		return errors.New("缓存目录必须为独立绝对路径")
	}
	for p := filepath.Clean(path); p != "/"; p = filepath.Dir(p) {
		if s, e := os.Lstat(p); e == nil && s.Mode()&os.ModeSymlink != 0 {
			return errors.New("更新缓存目录不能经过符号链接")
		}
	}
	if e := os.MkdirAll(path, 0o755); e != nil {
		return e
	}
	s, e := os.Stat(path)
	if e != nil {
		return e
	}
	owner, ok := s.Sys().(*syscall.Stat_t)
	if !ok || owner.Uid != 0 || s.Mode().Perm()&0o022 != 0 {
		return errors.New("更新缓存目录必须由 root 所有，且不可被普通用户写入")
	}
	return nil
}
func preserveOwner(reference os.FileInfo, path string) error {
	owner, ok := reference.Sys().(*syscall.Stat_t)
	if !ok {
		return errors.New("无法读取原配置所有者")
	}
	return os.Chown(path, int(owner.Uid), int(owner.Gid))
}
func systemProxy(r *http.Request) (*url.URL, error) { return http.ProxyFromEnvironment(r) }
func readPassword() (string, error) {
	fd := int(os.Stdin.Fd())
	term, e := unix.IoctlGetTermios(fd, unix.TCGETS)
	if e != nil {
		return "", errors.New("请在交互终端输入管理员密码")
	}
	next := *term
	next.Lflag &^= unix.ECHO
	if e = unix.IoctlSetTermios(fd, unix.TCSETS, &next); e != nil {
		return "", e
	}
	defer unix.IoctlSetTermios(fd, unix.TCSETS, term)
	var raw []byte
	one := make([]byte, 1)
	for {
		n, e := os.Stdin.Read(one)
		if e != nil {
			return "", e
		}
		if n > 0 {
			if one[0] == '\n' {
				break
			}
			raw = append(raw, one[0])
		}
	}
	return strings.TrimSuffix(string(raw), "\r"), nil
}
func rootRequired() error {
	if os.Geteuid() != 0 {
		return errors.New("请使用 sudo 运行 NAS/云端更新")
	}
	return nil
}
func acquireLock(path string) (func(), error) {
	fd, e := unix.Open(path, unix.O_CREAT|unix.O_WRONLY|unix.O_NOFOLLOW, 0o600)
	if e != nil {
		return nil, e
	}
	if e = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); e != nil {
		unix.Close(fd)
		return nil, errors.New("已有更新正在执行")
	}
	return func() { unix.Flock(fd, unix.LOCK_UN); unix.Close(fd) }, nil
}
