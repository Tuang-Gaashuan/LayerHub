package updatekit

import (
	"errors"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"

	"golang.org/x/sys/windows"
)

var proxyCache sync.Map

func systemProxy(r *http.Request) (*url.URL, error) {
	if p, e := http.ProxyFromEnvironment(r); p != nil || e != nil {
		return p, e
	}
	if cached, ok := proxyCache.Load(r.URL.Host); ok {
		if cached.(string) == "" {
			return nil, nil
		}
		return url.Parse(cached.(string))
	}
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", `$u=[Console]::ReadLine();[System.Net.WebRequest]::GetSystemWebProxy().GetProxy([Uri]$u).AbsoluteUri`)
	cmd.Stdin = strings.NewReader(r.URL.String() + "\n")
	raw, e := cmd.Output()
	if e != nil {
		return nil, errors.New("无法读取 Windows 系统代理，请设置 HTTPS_PROXY 或检查 PowerShell")
	}
	text := strings.TrimSpace(string(raw))
	if text == r.URL.String() {
		proxyCache.Store(r.URL.Host, "")
		return nil, nil
	}
	p, e := url.Parse(text)
	if e != nil || p.Host == "" || (p.Scheme != "http" && p.Scheme != "https") {
		return nil, errors.New("系统代理格式无效")
	}
	proxyCache.Store(r.URL.Host, text)
	return p, nil
}
func readPassword() (string, error) {
	h := windows.Handle(os.Stdin.Fd())
	var mode uint32
	if e := windows.GetConsoleMode(h, &mode); e != nil {
		return "", errors.New("请输入密码的终端不可用")
	}
	if e := windows.SetConsoleMode(h, mode&^windows.ENABLE_ECHO_INPUT); e != nil {
		return "", e
	}
	defer windows.SetConsoleMode(h, mode)
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
	return errors.New("NAS/云端更新仅支持 Linux；Windows 入口用于下载")
}
func acquireLock(path string) (func(), error)                { return nil, errors.New("更新锁仅支持 Linux") }
func secureCache(path string) error                          { return rootRequired() }
func preserveOwner(reference os.FileInfo, path string) error { return nil }
