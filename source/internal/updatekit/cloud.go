package updatekit

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	pathpkg "path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type location struct {
	start, end int
	body       string
}

func staticHeaders(body string) (string, error) {
	var statements []string
	var current strings.Builder
	quote := byte(0)
	escaped := false
	comment := false
	for i := 0; i < len(body); i++ {
		ch := body[i]
		if comment {
			if ch == '\n' {
				comment = false
				current.WriteByte(' ')
			}
			continue
		}
		if escaped {
			current.WriteByte(ch)
			escaped = false
			continue
		}
		if ch == '\\' {
			current.WriteByte(ch)
			escaped = true
			continue
		}
		if quote != 0 {
			current.WriteByte(ch)
			if ch == quote {
				quote = 0
			}
			continue
		}
		if ch == '\'' || ch == '"' {
			quote = ch
			current.WriteByte(ch)
			continue
		}
		if ch == '#' {
			comment = true
			continue
		}
		if ch == '{' || ch == '}' {
			return "", errors.New("自定义网关规则需要人工复核，未改写")
		}
		current.WriteByte(ch)
		if ch == ';' {
			statements = append(statements, strings.TrimSpace(current.String()))
			current.Reset()
		}
	}
	if quote != 0 || strings.TrimSpace(current.String()) != "" {
		return "", errors.New("网关指令解析失败")
	}
	allowed := map[string]bool{"root": true, "index": true, "try_files": true, "expires": true, "add_header": true, "proxy_pass": true, "error_page": true, "proxy_read_timeout": true, "proxy_send_timeout": true, "proxy_connect_timeout": true, "proxy_buffering": true}
	var headers []string
	for _, statement := range statements {
		parts := strings.Fields(statement)
		if len(parts) == 0 {
			continue
		}
		if !allowed[parts[0]] {
			return "", errors.New("location 内有自定义安全/访问规则，需要人工复核，未覆盖")
		}
		if parts[0] == "add_header" {
			headers = append(headers, statement)
		}
	}
	return strings.Join(headers, "\n        "), nil
}

func locations(text, header string) ([]location, error) {
	var result []location
	offset := 0
	for {
		index := strings.Index(text[offset:], header)
		if index < 0 {
			break
		}
		index += offset
		open := index + len(header) - 1
		depth := 0
		quote := byte(0)
		comment := false
		escaped := false
		end := -1
		for i := open; i < len(text); i++ {
			ch := text[i]
			if comment {
				if ch == '\n' {
					comment = false
				}
				continue
			}
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if quote != 0 {
				if ch == quote {
					quote = 0
				}
				continue
			}
			if ch == '\'' || ch == '"' {
				quote = ch
				continue
			}
			if ch == '#' {
				comment = true
				continue
			}
			if ch == '{' {
				depth++
			}
			if ch == '}' {
				depth--
				if depth == 0 {
					end = i + 1
					break
				}
			}
		}
		if end < 0 {
			return nil, errors.New("Nginx location 括号不完整")
		}
		result = append(result, location{index, end, text[open+1 : end-1]})
		offset = end
	}
	return result, nil
}
func cloudConfig(original, web string) (string, error) {
	if strings.ContainsAny(web, "\r\n\x00\"'$;{} ") || !pathpkg.IsAbs(web) {
		return "", errors.New("静态目录路径不受支持")
	}
	blocks, e := locations(original, "location / {")
	if e != nil {
		return "", e
	}
	var candidates []location
	for _, block := range blocks {
		body := block.body
		if strings.Contains(body, "proxy_pass http://127.0.0.1:15181;") || strings.Contains(body, "root /usr/share/layerhub-gateway/releases/") || strings.Contains(body, "# layerhub-updatekit") {
			candidates = append(candidates, block)
		}
	}
	if len(candidates) != 1 {
		return "", errors.New("未找到唯一的 LayerHub 网关位置；请人工确认 --nginx-config，未自动重写其他站点")
	}
	block := candidates[0]
	headers, e := staticHeaders(block.body)
	if e != nil {
		return "", e
	}
	apiHeader := "location /api/ {"
	healthHeader := "location = /healthz {"
	extra := ""
	if strings.Contains(block.body, "proxy_pass ") {
		api, _ := locations(original, apiHeader)
		if len(api) == 0 {
			extra += "\n    " + apiHeader + block.body + "}\n"
		}
		health, _ := locations(original, healthHeader)
		if len(health) == 0 {
			extra += "\n    " + healthHeader + block.body + "}\n"
		}
	} else {
		api, _ := locations(original, apiHeader)
		health, _ := locations(original, healthHeader)
		if len(api) == 0 || len(health) == 0 {
			return "", errors.New("现有静态站点缺少独立业务代理，未自动推断")
		}
	}
	updated := original[:block.start] + "location / {\n        # layerhub-updatekit: immutable public UI; business remains on NAS\n        root " + web + ";\n        index index.html;\n        try_files $uri $uri/ =404;\n        expires -1;\n        " + headers + "\n    }" + extra + original[block.end:]
	// Replace only the previous known source redirect. Matching corresponding
	// source is now served from the new immutable web directory.
	source, e := locations(updated, "location = /source/layerhub-source.tar.gz {")
	if e != nil {
		return "", e
	}
	if len(source) > 1 {
		return "", errors.New("源码 location 重复")
	}
	if len(source) == 1 {
		b := source[0]
		if strings.Contains(b.body, "https://github.com/"+Repository+"/releases/download/") {
			updated = updated[:b.start] + updated[b.end:]
		}
	}
	return updated, nil
}
func rootFromConfig(text string) (string, error) {
	blocks, e := locations(text, "location / {")
	if e != nil {
		return "", e
	}
	re := regexp.MustCompile(`(?m)^\s*root\s+(/[^;\s]+);`)
	var roots []string
	for _, b := range blocks {
		if m := re.FindStringSubmatch(b.body); m != nil && (strings.HasPrefix(m[1], "/usr/share/layerhub-gateway/releases/") || strings.Contains(b.body, "# layerhub-updatekit:")) {
			roots = append(roots, m[1])
		}
	}
	if len(roots) > 1 {
		return "", errors.New("多个静态站点，无法确定当前版本")
	}
	if len(roots) == 0 {
		return "", nil
	}
	return roots[0], nil
}
func pageVersion(web string) (string, error) {
	raw, e := os.ReadFile(filepath.Join(web, "source-offer.html"))
	if e != nil {
		return "", e
	}
	match := regexp.MustCompile(`<h1>LayerHub ([^<]+)</h1>`).FindSubmatch(raw)
	if match == nil {
		return "", errors.New("当前页面缺少版本声明")
	}
	tag := "v" + string(match[1])
	_, _, e = version(tag)
	return tag, e
}
func replaceFile(path string, raw []byte, mode os.FileMode) error {
	if e := ensureExistsRegular(path); e != nil {
		return e
	}
	info, e := os.Stat(path)
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".layerhub-update-")
	if e != nil {
		return e
	}
	name := f.Name()
	if _, e = f.Write(raw); e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	if e = os.Chmod(name, mode); e != nil {
		return e
	}
	if e = preserveOwner(info, name); e != nil {
		return e
	}
	return os.Rename(name, path)
}
func UpdateCloud(o Options, input *bufio.Reader, w io.Writer) error {
	if e := rootRequired(); e != nil {
		return e
	}
	if e := secureCache(o.Output); e != nil {
		return e
	}
	unlock, e := acquireLock(filepath.Join(o.Output, "cloud-update.lock"))
	if e != nil {
		return e
	}
	defer unlock()
	config := o.Nginx
	if config == "" {
		for _, path := range []string{"/etc/nginx/sites-available/layerhub", "/etc/nginx/conf.d/layerhub.conf"} {
			if _, e := os.Stat(path); e == nil {
				if config != "" {
					return errors.New("发现多份网关配置，请指定 --nginx-config")
				}
				config = path
			}
		}
	}
	if config == "" {
		return errors.New("没有检测到既有 Nginx 网关；本工具不代替首次 HTTPS 配置")
	}
	if e = ensureExistsRegular(config); e != nil {
		return e
	}
	old, e := os.ReadFile(config)
	if e != nil {
		return e
	}
	info, e := os.Stat(config)
	if e != nil {
		return e
	}
	web, e := rootFromConfig(string(old))
	if e != nil {
		return e
	}
	if web != "" {
		tag, e := pageVersion(web)
		if e != nil {
			return e
		}
		_, ch, e := version(tag)
		if e != nil {
			return e
		}
		if ch != o.Channel {
			return errors.New("云端当前页面通道不同，未自动切换")
		}
		if o.Package == "" && o.PackageDir == "" {
			release, er := latest(client(), o.Channel, o.Tag)
			if er != nil {
				return er
			}
			n, er := compare(release.Tag, tag)
			if er != nil {
				return er
			}
			if n <= 0 {
				fmt.Fprintln(w, "云端界面已是该通道最新发布，未下载或重载 / Already up to date")
				return nil
			}
			o.Tag = release.Tag
		}
	}
	b, e := bundleFor(o, w)
	if e != nil {
		return e
	}
	if web != "" {
		tag, _ := pageVersion(web)
		n, e := compare(b.Receipt.Tag, tag)
		if e != nil {
			return e
		}
		if n <= 0 {
			fmt.Fprintln(w, "云端界面已是该版本或更高版本，未重载 Nginx / Already up to date")
			return nil
		}
	}
	base := o.Origin
	if base == "" {
		base = "http://127.0.0.1:15181"
	}
	a, e := newAPI(base)
	if e != nil {
		return e
	}
	a.allowRunning = o.AllowRunning
	if e = authenticate(a, "", input, w); e != nil {
		return e
	}
	defer a.call("/api/logout", map[string]any{}, nil)
	var d struct {
		Version string `json:"version"`
	}
	if e = a.call("/api/diagnostics", nil, &d); e != nil {
		return e
	}
	if "v"+d.Version != b.Receipt.Tag {
		return errors.New("NAS 版本与目标不同，请先更新 NAS，避免前后端不匹配")
	}
	b, e = prepareBundle(b, o.Output)
	if e != nil {
		return e
	}
	if !filepath.IsAbs(o.StaticRoot) || o.StaticRoot == "/" {
		return errors.New("静态发布目录不合法")
	}
	target := filepath.Join(o.StaticRoot, fmt.Sprintf("%s-%s-%d", strings.TrimPrefix(b.Receipt.Tag, "v"), b.Receipt.SHA256[:12], time.Now().UnixNano()))
	next, e := cloudConfig(string(old), filepath.Join(target, "web"))
	if e != nil {
		return e
	}
	if !o.Apply {
		fmt.Fprintln(w, "云端更新预检完成，未修改 Nginx；使用 --apply 执行 / Plan only")
		return nil
	}
	fmt.Fprint(w, "公网 HTTPS 地址（用于验证，不保存密码）/ Public HTTPS URL: ")
	text, _ := input.ReadString('\n')
	public, e := url.Parse(strings.TrimSpace(text))
	if e != nil || public.Scheme != "https" || public.Host == "" || public.User != nil || public.RawQuery != "" || public.Fragment != "" || public.Path != "" && public.Path != "/" {
		return errors.New("请输入有效的公网 HTTPS 根地址")
	}
	if e = confirm(input, w, "将保存现有网关配置、切换同版本静态文件并重载 Nginx。证书、SSH 和 NAS 数据不改动。"); e != nil {
		return e
	}
	if e = os.MkdirAll(o.StaticRoot, 0o755); e != nil {
		return e
	}
	if e = os.Mkdir(target, 0o755); e != nil {
		return e
	}
	if e = os.Chmod(target, 0o755); e != nil {
		return e
	}
	if e = copyTree(filepath.Join(b.Directory, "web"), filepath.Join(target, "web")); e != nil {
		return e
	}
	if e = copyFile(filepath.Join(b.Directory, "VERSION"), filepath.Join(target, "VERSION"), 0o644); e != nil {
		return e
	}
	if o.BackupRoot == "" {
		o.BackupRoot = "/var/backups/layerhub-update"
	}
	if e = secureCache(o.BackupRoot); e != nil {
		return e
	}
	if !filepath.IsAbs(o.BackupRoot) || o.BackupRoot == "/" {
		return errors.New("备份目录必须是独立绝对路径")
	}
	backup := filepath.Join(o.BackupRoot, fmt.Sprintf("%s-cloud-%d", time.Now().Format("20060102-150405"), time.Now().UnixNano()))
	if e = os.MkdirAll(backup, 0o700); e != nil {
		return e
	}
	if e = os.WriteFile(filepath.Join(backup, "nginx.conf"), old, 0o600); e != nil {
		return e
	}
	if e = writeJSON(filepath.Join(backup, "original-permissions.json"), map[string]any{"mode": uint32(info.Mode().Perm())}, 0o600); e != nil {
		return e
	}
	if e = replaceFile(config, []byte(next), info.Mode().Perm()); e != nil {
		return e
	}
	rollback := func() error {
		if er := replaceFile(config, old, info.Mode().Perm()); er != nil {
			return er
		}
		if _, er := command("nginx", "-t"); er != nil {
			return er
		}
		_, er := command("systemctl", "reload", "nginx")
		return er
	}
	if _, e = command("nginx", "-t"); e != nil {
		_ = rollback()
		return errors.New("Nginx 校验失败，已尝试恢复原配置；未重启 NAS")
	}
	if _, e = command("systemctl", "reload", "nginx"); e != nil {
		_ = rollback()
		return errors.New("网关重载失败，已尝试恢复原配置")
	}
	checkClient := &http.Client{Timeout: 6 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	verify := func() bool {
		for _, name := range []string{"index.html", "app.js", "source-offer.html"} {
			response, e := checkClient.Get(strings.TrimSuffix(public.String(), "/") + "/" + name)
			if e != nil {
				return false
			}
			raw, e := io.ReadAll(io.LimitReader(response.Body, 8<<20))
			response.Body.Close()
			expected, er := os.ReadFile(filepath.Join(target, "web", name))
			if e != nil || er != nil || response.StatusCode != 200 || !bytes.Equal(sha256sum(raw), sha256sum(expected)) {
				return false
			}
		}
		return true
	}
	for attempt := 0; attempt < 5; attempt++ {
		if verify() {
			fmt.Fprintln(w, "云端更新完成。备份：", backup)
			return writeJSON(filepath.Join(backup, "result.json"), map[string]any{"status": "updated", "tag": b.Receipt.Tag, "database_touched": false}, 0o600)
		}
		time.Sleep(time.Second)
	}
	if e = rollback(); e != nil {
		return fmt.Errorf("网关验证失败，自动恢复原配置也未确认成功。保留现场，备份：%s", backup)
	}
	return fmt.Errorf("网关验证未通过，已恢复原网关；NAS 与数据未回退。备份：%s", backup)
}
func sha256sum(raw []byte) []byte { sum := sha256.Sum256(raw); return sum[:] }
