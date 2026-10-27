package updatekit

import (
	"bufio"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

//go:embed state.py
var stateHelper string

type Printer struct {
	ID        int64  `json:"id"`
	Enabled   bool   `json:"enabled"`
	Connected bool   `json:"connected"`
	Updated   int64  `json:"updated"`
	RawState  string `json:"rawState"`
	Percent   int    `json:"percent"`
	File      string `json:"file"`
}
type Workspace struct {
	User struct {
		Owner bool `json:"owner"`
	} `json:"user"`
	Printers []Printer `json:"printers"`
	Jobs     []struct {
		ID      int64  `json:"id"`
		Printer int64  `json:"printer"`
		State   string `json:"state"`
	} `json:"jobs"`
}
type API struct {
	base    string
	client  *http.Client
	cookies []*http.Cookie
	csrf    string
}

func newAPI(base string) (*API, error) {
	u, e := url.Parse(base)
	if e != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" && u.Path != "/" || u.Scheme != "http" || u.Hostname() != "127.0.0.1" {
		return nil, errors.New("更新检查只允许本机回环 HTTP 地址")
	}
	return &API{base: strings.TrimSuffix(base, "/"), client: &http.Client{Timeout: 8 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (a *API) call(path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = strings.NewReader(string(raw))
	}
	method := "GET"
	if body != nil {
		method = "POST"
	}
	r, e := http.NewRequest(method, a.base+path, reader)
	if e != nil {
		return e
	}
	r.Header.Set("Origin", a.base)
	r.Header.Set("Content-Type", "application/json")
	if a.csrf != "" {
		r.Header.Set("X-CSRF-Token", a.csrf)
	}
	// Explicit cookies are sent only to the pinned loopback API. Browser Secure
	// cookie rules remain unchanged; a remote HTTP origin is never accepted.
	for _, cookie := range a.cookies {
		r.AddCookie(cookie)
	}
	resp, e := a.client.Do(r)
	if e != nil {
		return errors.New("无法连接本机 LayerHub 接口")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("业务接口 HTTP %d", resp.StatusCode)
	}
	if path == "/api/login" {
		a.cookies = resp.Cookies()
	}
	if out != nil {
		return json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(out)
	}
	return nil
}
func (a *API) login(username, password string) error {
	var value struct {
		CSRF string `json:"csrf"`
	}
	if e := a.call("/api/login", map[string]string{"Username": username, "Password": password}, &value); e != nil {
		return e
	}
	if value.CSRF == "" {
		return errors.New("登录未返回会话校验令牌")
	}
	a.csrf = value.CSRF
	return nil
}
func (a *API) guard(now int64) (Workspace, error) {
	var w Workspace
	if e := a.call("/api/workspace", nil, &w); e != nil {
		return w, e
	}
	if !w.User.Owner {
		return w, errors.New("升级检查需要空间所有者账号")
	}
	var restore struct {
		Pending bool `json:"pending"`
		Preview bool `json:"previewReady"`
	}
	if e := a.call("/api/backup/status", nil, &restore); e != nil {
		return w, e
	}
	if restore.Pending || restore.Preview {
		return w, errors.New("有备份恢复或预览未结束，暂不更新")
	}
	return w, guardWorkspace(w, now)
}
func guardWorkspace(w Workspace, now int64) error {
	printers := map[int64]Printer{}
	for _, p := range w.Printers {
		printers[p.ID] = p
		if p.Enabled {
			age := now - p.Updated
			if !p.Connected || age < 0 || age >= 60 {
				return errors.New("启用的打印机必须在线并有新鲜遥测")
			}
			if p.RawState != "FINISH" && p.RawState != "IDLE" && p.RawState != "READY" {
				return errors.New("打印机尚未空闲，更新已取消")
			}
		}
	}
	for _, job := range w.Jobs {
		switch job.State {
		case "uploading", "starting", "paused", "pause", "stopped", "checking":
			return errors.New("有传输、启动、暂停或待核对任务，更新已取消")
		case "printing":
			p := printers[job.Printer]
			if !p.Connected || now-p.Updated < 0 || now-p.Updated >= 60 || p.RawState != "FINISH" || p.Percent != 100 || p.File != fmt.Sprintf("job-%d", job.ID) {
				return errors.New("有进行中的打印任务，更新已取消")
			}
		}
	}
	return nil
}
func authenticate(a *API, data string, input *bufio.Reader, w io.Writer) error {
	if data != "" {
		if raw, e := os.ReadFile(filepath.Join(data, "bootstrap-admin.txt")); e == nil {
			values := map[string]string{}
			for _, line := range strings.Split(string(raw), "\n") {
				k, v, ok := strings.Cut(line, "=")
				if ok {
					values[k] = strings.TrimSuffix(v, "\r")
				}
			}
			if values["username"] != "" && values["password"] != "" {
				if a.login(values["username"], values["password"]) == nil {
					return nil
				}
			}
		}
	}
	fmt.Fprint(w, "空间所有者账号 / Owner username [admin]: ")
	username, _ := input.ReadString('\n')
	username = strings.TrimSpace(username)
	if username == "" {
		username = "admin"
	}
	fmt.Fprint(w, "密码（不回显、不保存）/ Password: ")
	password, e := readPassword()
	fmt.Fprintln(w)
	if e != nil {
		return e
	}
	e = a.login(username, password)
	password = ""
	return e
}
func command(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, name, args...)
	raw, e := c.Output()
	if e != nil {
		return nil, fmt.Errorf("系统操作 %s 未成功，请查看本机服务日志", filepath.Base(name))
	}
	return raw, nil
}
func state(mode, data, backup, environment string) ([]byte, error) {
	args := []string{"-c", stateHelper, mode, data}
	if mode == "backup" {
		args = append(args, backup, environment)
	}
	out, e := command("python3", args...)
	if e != nil {
		return nil, errors.New("数据库/备份检查失败：请核对任务、恢复标记、磁盘空间和 Python 3；未发送设备指令")
	}
	return out, nil
}
func cleanEnvFile(path string) (map[string]string, error) {
	if e := ensureExistsRegular(path); e != nil {
		return nil, e
	}
	raw, e := os.ReadFile(path)
	if e != nil {
		return nil, e
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if key != "LAYERHUB_DATA_DIR" && key != "LAYERHUB_ADDRESS" && key != "LAYERHUB_WEB_DIR" {
			continue
		}
		value = strings.TrimSpace(value)
		if len(value) > 1 && (value[0] == '\'' && value[len(value)-1] == '\'' || value[0] == '"' && value[len(value)-1] == '"') {
			value = value[1 : len(value)-1]
		}
		if strings.ContainsAny(value, "\r\n\x00") {
			return nil, errors.New("环境路径包含控制字符")
		}
		values[key] = value
	}
	return values, nil
}
func inside(root, path string) bool {
	rel, e := filepath.Rel(root, path)
	return e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel)
}
func copyFile(src, dst string, mode os.FileMode) error {
	if e := ensureExistsRegular(src); e != nil {
		return e
	}
	in, e := os.Open(src)
	if e != nil {
		return e
	}
	defer in.Close()
	out, e := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if e != nil {
		return e
	}
	_, e = io.Copy(out, in)
	ce := out.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	return os.Chmod(dst, mode)
}
func copyTree(src, dst string) error {
	if e := os.Mkdir(dst, 0o755); e != nil {
		return e
	}
	if e := os.Chmod(dst, 0o755); e != nil {
		return e
	}
	return filepath.WalkDir(src, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == src {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("发布目录含符号链接")
		}
		rel, e := filepath.Rel(src, path)
		if e != nil || !inside(src, path) {
			return errors.New("复制路径越界")
		}
		target := filepath.Join(dst, rel)
		if entry.IsDir() {
			if e := os.Mkdir(target, 0o755); e != nil {
				return e
			}
			return os.Chmod(target, 0o755)
		}
		mode := os.FileMode(0o644)
		if strings.HasPrefix(filepath.ToSlash(rel), "bin/") || strings.HasSuffix(rel, ".sh") {
			mode = 0o755
		}
		return copyFile(path, target, mode)
	})
}
func switchLink(link, target string) error {
	if s, e := os.Lstat(link); e != nil || s.Mode()&os.ModeSymlink == 0 {
		return errors.New("current 不是已存在的发布软链接，未修改")
	}
	temporary := link + fmt.Sprintf(".update-%d", time.Now().UnixNano())
	if e := os.Symlink(target, temporary); e != nil {
		return e
	}
	return os.Rename(temporary, link)
}

type Options struct {
	Channel, Tag, Output, Root, Env, Origin, Nginx, StaticRoot string
	Apply                                                      bool
	Package                                                    string
	PackageDir                                                 string
	BackupRoot                                                 string
}

func bundleFor(o Options, w io.Writer) (Bundle, error) {
	if o.Package == "" && o.PackageDir != "" {
		entries, e := os.ReadDir(o.PackageDir)
		if e != nil {
			return Bundle{}, errors.New("离线 downloads 目录不存在，请先下载 NAS/Linux 包和收据")
		}
		var best Receipt
		var path string
		for _, entry := range entries {
			if !strings.HasSuffix(entry.Name(), "-linux-amd64.tar.gz.verified.json") || entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			raw, e := os.ReadFile(filepath.Join(o.PackageDir, entry.Name()))
			if e != nil || len(raw) > 65536 {
				continue
			}
			var receipt Receipt
			if json.Unmarshal(raw, &receipt) != nil || receipt.Repository != Repository || receipt.Platform != "linux" {
				continue
			}
			_, channel, e := version(receipt.Tag)
			if e != nil || channel != o.Channel || filepath.Base(receipt.Filename) != receipt.Filename {
				continue
			}
			if path == "" {
				best = receipt
				path = filepath.Join(o.PackageDir, receipt.Filename)
			} else if n, e := compare(receipt.Tag, best.Tag); e == nil && n > 0 {
				best = receipt
				path = filepath.Join(o.PackageDir, receipt.Filename)
			}
		}
		if path == "" {
			return Bundle{}, errors.New("没有找到此通道的 Linux 包与收据；请在 Windows 点击 Download-Beta-NAS.cmd，并复制完整工具文件夹")
		}
		o.Package = path
	}
	if o.Package == "" {
		return Download(o.Channel, "linux", o.Tag, o.Output, false, w)
	}
	if e := ensureExistsRegular(o.Package); e != nil {
		return Bundle{}, e
	}
	raw, e := os.ReadFile(o.Package + ".verified.json")
	if e != nil {
		return Bundle{}, errors.New("离线包需要同目录的 .verified.json 收据，请由下载工具生成并一同复制")
	}
	var receipt Receipt
	if json.Unmarshal(raw, &receipt) != nil || receipt.Repository != Repository || receipt.Platform != "linux" || receipt.Filename != filepath.Base(o.Package) {
		return Bundle{}, errors.New("离线收据与包不匹配")
	}
	_, channel, e := version(receipt.Tag)
	if e != nil || channel != o.Channel {
		return Bundle{}, errors.New("离线包与通道不一致")
	}
	if o.Tag != "" && o.Tag != receipt.Tag {
		return Bundle{}, errors.New("离线包与指定版本不一致")
	}
	hash, e := hashFile(o.Package)
	s, se := os.Stat(o.Package)
	if e != nil || se != nil || hash != receipt.SHA256 || s.Size() != receipt.Size {
		return Bundle{}, errors.New("离线包完整性校验失败")
	}
	fmt.Fprintln(w, "离线导入：请确认包与收据由可信设备下载和传递；收据不是数字签名。")
	return Bundle{Receipt: receipt, Archive: o.Package}, nil
}
func prepareBundle(b Bundle, cache string) (Bundle, error) {
	parent, e := os.MkdirTemp(cache, "stage-")
	if e != nil {
		return b, e
	}
	b.Directory, e = Extract(b.Archive, filepath.Join(parent, "bundle"), b.Receipt.Tag)
	return b, e
}
func confirm(input *bufio.Reader, w io.Writer, message string) error {
	fmt.Fprintln(w, message)
	fmt.Fprint(w, "继续？输入 yes / Type yes to continue: ")
	line, _ := input.ReadString('\n')
	if strings.TrimSpace(line) != "yes" {
		return errors.New("已取消，未停止服务 / Cancelled before stopping services")
	}
	return nil
}

func UpdateNAS(o Options, input *bufio.Reader, w io.Writer) error {
	if e := rootRequired(); e != nil {
		return e
	}
	if e := secureCache(o.Output); e != nil {
		return e
	}
	unlock, e := acquireLock(filepath.Join(o.Output, "nas-update.lock"))
	if e != nil {
		return e
	}
	defer unlock()
	root, e := filepath.Abs(o.Root)
	if e != nil || root == string(os.PathSeparator) {
		return errors.New("安装根目录无效")
	}
	current := filepath.Join(root, "current")
	old, e := filepath.EvalSymlinks(current)
	if e != nil || !inside(filepath.Join(root, "releases"), old) {
		return errors.New("没有检测到受支持的既有安装；此工具不执行全新安装")
	}
	raw, e := os.ReadFile(filepath.Join(old, "VERSION"))
	if e != nil {
		return e
	}
	currentTag := "v" + strings.TrimSpace(string(raw))
	_, channel, e := version(currentTag)
	if e != nil {
		return e
	}
	if channel != o.Channel {
		return errors.New("现有安装与所选通道不同，未跨通道升级")
	}
	if o.Package == "" && o.PackageDir == "" {
		release, er := latest(client(), o.Channel, o.Tag)
		if er != nil {
			return er
		}
		n, er := compare(release.Tag, currentTag)
		if er != nil {
			return er
		}
		if n <= 0 {
			fmt.Fprintln(w, "当前已是该通道最新发布，无需下载或重启 / Already up to date:", currentTag)
			return nil
		}
		o.Tag = release.Tag
	}
	b, e := bundleFor(o, w)
	if e != nil {
		return e
	}
	delta, e := compare(b.Receipt.Tag, currentTag)
	if e != nil {
		return e
	}
	if delta <= 0 {
		fmt.Fprintln(w, "当前已是该版本或更高版本，未备份或重启 / Already up to date:", currentTag)
		return nil
	}
	settings, e := cleanEnvFile(o.Env)
	if e != nil {
		return e
	}
	data := settings["LAYERHUB_DATA_DIR"]
	if !filepath.IsAbs(data) {
		return errors.New("配置必须使用绝对数据目录，未猜测数据位置")
	}
	if inside(old, data) || inside(data, root) || inside(root, data) {
		return errors.New("数据目录必须独立于安装目录")
	}
	if inside(data, o.Output) || inside(o.Output, data) {
		return errors.New("下载缓存不能包含业务数据，或位于业务数据目录内")
	}
	if web := settings["LAYERHUB_WEB_DIR"]; web != "" && filepath.Clean(web) != filepath.Join(root, "current", "web") {
		return errors.New("检测到自定义网页目录，请人工确认更新路径，未自动修改")
	}
	if o.Origin == "" {
		address := settings["LAYERHUB_ADDRESS"]
		if address == "" {
			address = "127.0.0.1:5181"
		}
		o.Origin = "http://" + address
	}
	a, e := newAPI(o.Origin)
	if e != nil {
		return e
	}
	if e = authenticate(a, data, input, w); e != nil {
		return e
	}
	defer a.call("/api/logout", map[string]any{}, nil)
	var diagnostic struct {
		Version string `json:"version"`
	}
	if e = a.call("/api/diagnostics", nil, &diagnostic); e != nil {
		return e
	}
	if "v"+diagnostic.Version != currentTag {
		return errors.New("运行版本与安装目录不一致，未更新")
	}
	if _, e = a.guard(time.Now().Unix()); e != nil {
		return e
	}
	if _, e = state("inspect", data, "", ""); e != nil {
		return e
	}
	b, e = prepareBundle(b, o.Output)
	if e != nil {
		return e
	}
	bin := filepath.Join(b.Directory, "bin/layerhub")
	if e = ensureExistsRegular(bin); e != nil {
		return e
	}
	probe := exec.Command(bin, "--doctor", "--json")
	probe.Env = append(os.Environ(), "LAYERHUB_DATA_DIR="+data, "LAYERHUB_WEB_DIR="+filepath.Join(b.Directory, "web"), "LAYERHUB_DEVICE_MODE=offline", "LAYERHUB_ENABLE_PRINT_START=0")
	doctor, e := probe.Output()
	if e != nil {
		return errors.New("新程序启动前只读自检失败，旧服务未停止")
	}
	var report struct {
		Version string `json:"version"`
	}
	if json.Unmarshal(doctor, &report) != nil || report.Version != strings.TrimPrefix(b.Receipt.Tag, "v") {
		return errors.New("新程序实际版本与附件标签不匹配")
	}
	pidRaw, e := command("systemctl", "show", "layerhub.service", "-p", "MainPID", "--value")
	if e != nil {
		return e
	}
	exe, e := filepath.EvalSymlinks(filepath.Join("/proc", strings.TrimSpace(string(pidRaw)), "exe"))
	if e != nil || !inside(filepath.Join(old, "bin"), exe) {
		return errors.New("运行进程不属于当前发布目录")
	}
	binaryName := filepath.Base(exe)
	if binaryName != "layerhub" && binaryName != "layerhub-linux-amd64" {
		return errors.New("服务程序路径不受支持")
	}
	target := filepath.Join(root, "releases", fmt.Sprintf("%s-%s-%d", strings.TrimPrefix(b.Receipt.Tag, "v"), b.Receipt.SHA256[:12], time.Now().UnixNano()))
	fmt.Fprintln(w, "NAS:", currentTag, "→", b.Receipt.Tag, "；将备份完整数据和配置，保留旧程序。")
	if !o.Apply {
		fmt.Fprintln(w, "仅准备和检查完成，未停止服务；使用 --apply 执行 / Plan only")
		return nil
	}
	if e = confirm(input, w, "确认设备空闲后，短暂停止 LayerHub 完成切换；不会重启隧道或打印机。"); e != nil {
		return e
	}
	if e = copyTree(b.Directory, target); e != nil {
		return e
	}
	if binaryName != "layerhub" {
		if e = copyFile(filepath.Join(target, "bin/layerhub"), filepath.Join(target, "bin", binaryName), 0o755); e != nil {
			return e
		}
	}
	if _, e = a.guard(time.Now().Unix()); e != nil {
		return e
	}
	before, e := state("inspect", data, "", "")
	if e != nil {
		return e
	}
	if o.BackupRoot == "" {
		o.BackupRoot = "/var/backups/layerhub-update"
	}
	if e = secureCache(o.BackupRoot); e != nil {
		return e
	}
	if !filepath.IsAbs(o.BackupRoot) || o.BackupRoot == "/" || inside(data, o.BackupRoot) || inside(o.BackupRoot, data) {
		return errors.New("备份目录必须独立于业务数据目录")
	}
	backup := filepath.Join(o.BackupRoot, fmt.Sprintf("%s-%s-%d", time.Now().Format("20060102-150405"), strings.TrimPrefix(b.Receipt.Tag, "v"), time.Now().UnixNano()))
	if e = os.MkdirAll(filepath.Dir(backup), 0o700); e != nil {
		return e
	}
	e = activate(activation{before: before, stop: func() error { _, er := command("systemctl", "stop", "layerhub.service"); return er }, start: func() error { _, er := command("systemctl", "start", "layerhub.service"); return er }, inspect: func() ([]byte, error) { return state("inspect", data, "", "") }, backup: func() error {
		if _, er := state("backup", data, backup, o.Env); er != nil {
			return er
		}
		return writeJSON(filepath.Join(backup, "upgrade.json"), map[string]any{"previous": old, "next": target, "tag": b.Receipt.Tag, "package_sha256": b.Receipt.SHA256, "data_directory": data, "state": "backed_up"}, 0o600)
	}, selectNew: func() error { return switchLink(current, target) }, selectOld: func() error { return switchLink(current, old) }})
	if e != nil {
		return fmt.Errorf("版本切换未完成：%v。没有自动覆盖业务数据，请核对 current 与服务状态；备份位置：%s", e, backup)
	}
	reauthenticated := false
	for i := 0; i < 20; i++ {
		var health struct {
			OK      bool   `json:"ok"`
			Backend string `json:"backend"`
		}
		if a.call("/healthz", nil, &health) == nil && health.OK && health.Backend == "go" {
			var d struct {
				Version string `json:"version"`
			}
			er := a.call("/api/diagnostics", nil, &d)
			if er != nil && !reauthenticated {
				reauthenticated = true
				if er = authenticate(a, data, input, w); er != nil {
					return er
				}
				er = a.call("/api/diagnostics", nil, &d)
			}
			_, ready := a.guard(time.Now().Unix())
			if er == nil && ready == nil && d.Version == strings.TrimPrefix(b.Receipt.Tag, "v") {
				fmt.Fprintln(w, "NAS 更新完成，工作区与新鲜遥测验证通过。备份：", backup, "；下一步更新云端界面。")
				return writeJSON(filepath.Join(backup, "result.json"), map[string]any{"status": "updated", "tag": b.Receipt.Tag, "database_restored": false, "configuration_rewritten": false}, 0o600)
			}
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("新版本健康验证未通过，保留现场，未自动恢复旧数据库。备份：%s", backup)
}
