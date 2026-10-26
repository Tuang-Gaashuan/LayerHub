package app

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"time"
)

const BuildVersion = "0.3.0-alpha"

// Diagnostics never include environment values, addresses, paths, user content,
// raw errors or credentials. A successful inspection is not a print permission.
type DiagnosticCheck struct {
	ID         string `json:"id"`
	Group      string `json:"group"`
	Status     string `json:"status"`
	Label      string `json:"label"`
	Detail     string `json:"detail"`
	Fix        string `json:"fix,omitempty"`
	PrinterID  int64  `json:"printerId,omitempty"`
	ObservedAt int64  `json:"observedAt,omitempty"`
}
type DiagnosticReport struct {
	OK        bool              `json:"ok"`
	Schema    int               `json:"schema"`
	Version   string            `json:"version"`
	Platform  string            `json:"platform"`
	CheckedAt int64             `json:"checkedAt"`
	Mode      string            `json:"mode"`
	Checks    []DiagnosticCheck `json:"checks"`
}

func newDiagnosticReport(mode string) DiagnosticReport {
	return DiagnosticReport{OK: true, Schema: 1, Version: BuildVersion, Platform: runtime.GOOS + "/" + runtime.GOARCH, CheckedAt: time.Now().UnixMilli(), Mode: mode, Checks: []DiagnosticCheck{}}
}
func (r *DiagnosticReport) add(id, group, status, label, detail, fix string) {
	r.Checks = append(r.Checks, DiagnosticCheck{ID: id, Group: group, Status: status, Label: label, Detail: detail, Fix: fix})
}
func readableDiagnosticFile(path string) bool {
	f, e := os.Open(path)
	if e != nil {
		return false
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil || !st.Mode().IsRegular() || st.Size() == 0 {
		return false
	}
	var one [1]byte
	_, e = f.Read(one[:])
	return e == nil
}

// InspectDeployment can run before first startup. It does not Open a workspace,
// load a device driver, open SQLite, run an engine or contact any remote host.
// Only the explicitly requested probe creates and removes its own temp file.
func InspectDeployment(c Config, probeWrite bool) DiagnosticReport {
	r := newDiagnosticReport("local")
	host, port, e := net.SplitHostPort(c.Address)
	n, portErr := strconv.Atoi(port)
	if e != nil || portErr != nil || n < 1 || n > 65535 {
		r.add("listener", "server", "error", "监听配置", "监听地址格式无效。", "检查 LAYERHUB_ADDRESS，使用主机:端口格式。")
	} else if host == "localhost" || net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback() {
		r.add("listener", "server", "ok", "监听配置", "应用仅监听本机回环地址。", "")
	} else {
		r.add("listener", "server", "warning", "监听配置", "应用配置为非回环监听；没有探测端口或防火墙。", "云网关方案应使用回环监听；有意开放局域网时请核对防火墙与 HTTPS。")
	}
	switch c.DeviceMode {
	case "offline":
		r.add("device_mode", "safety", "disabled", "设备连接", "设备连接关闭，不会接入真实打印机。", "需要实机时按部署文档配置 live 模式。")
	case "simulated":
		r.add("device_mode", "safety", "warning", "设备连接", "当前是模拟驱动，不能证明真实打印机连接。", "实机验收前核对 live 模式与设备权限。")
	case "live":
		r.add("device_mode", "safety", "ok", "设备连接", "已配置真实驱动；连通情况以设备遥测为准。", "")
	default:
		r.add("device_mode", "safety", "error", "设备连接", "设备模式无效。", "LAYERHUB_DEVICE_MODE 仅支持 offline、simulated、live。")
	}
	if c.EnablePrintStart {
		r.add("print_start", "safety", "warning", "打印启动开关", "远程启动已启用，仍受设备检查与人工确认约束。", "首次部署验收时建议关闭 LAYERHUB_ENABLE_PRINT_START。")
	} else {
		r.add("print_start", "safety", "ok", "打印启动开关", "远程启动关闭；诊断不会更改此开关。", "")
	}
	st, e := os.Stat(c.DataDir)
	if os.IsNotExist(e) {
		r.add("data_directory", "storage", "unknown", "数据目录", "数据目录尚不存在，首次启动前不能确认写入权限。", "核对 LAYERHUB_DATA_DIR；诊断不会创建工作区。")
	} else if e != nil || !st.IsDir() {
		r.add("data_directory", "storage", "error", "数据目录", "数据目录无法访问或不是目录。", "检查服务账户、挂载状态与目录权限。")
	} else if !probeWrite {
		r.add("data_directory", "storage", "unknown", "数据目录", "目录存在；没有执行写入测试。", "可由服务账户运行 --doctor --probe-write 检查临时文件读写。")
	} else {
		status := "ok"
		detail := "临时文件写入、读回与清理通过；业务文件未修改。"
		if !probeDiagnosticWrite(c.DataDir) {
			status = "error"
			detail = "临时文件读写或清理失败。"
		}
		r.add("data_directory", "storage", status, "数据目录", detail, "如失败，请检查服务账户权限、磁盘空间和只读挂载。")
	}
	if readableDiagnosticFile(filepath.Join(c.WebDir, "index.html")) && readableDiagnosticFile(filepath.Join(c.WebDir, "app.js")) && readableDiagnosticFile(filepath.Join(c.WebDir, "locales", "en.generated.js")) {
		r.add("web_assets", "server", "ok", "网页资源", "入口、应用脚本与语言包可读。", "")
	} else {
		r.add("web_assets", "server", "error", "网页资源", "必要网页资源缺失或不可读。", "检查 LAYERHUB_WEB_DIR，使用完整解压后的 web 目录。")
	}
	if readableDiagnosticFile(filepath.Join(c.WebDir, "source", "layerhub-source.tar.gz")) && readableDiagnosticFile(filepath.Join(c.WebDir, "source", "LICENSE.txt")) {
		r.add("source_offer", "server", "ok", "对应源码下载", "源码包与许可证文件可读；未核对其与程序的一致性。", "修改部署版本后重新生成对应源码包。")
	} else {
		r.add("source_offer", "server", "warning", "对应源码下载", "源码包或许可证缺失、不可读。", "使用完整发布包，或运行 tools/make-source.py 生成对应源码。")
	}
	if c.SlicerPath == "" && c.SlicerProfiles == "" {
		r.add("slicer", "slicing", "optional", "切片引擎", "未配置可选切片引擎；仍可管理工作区和预览模型。", "需要切片时设置 LAYERHUB_SLICER_PATH 和 LAYERHUB_SLICER_PROFILES。")
	} else {
		_, engineErr := exec.LookPath(c.SlicerPath)
		profilesOK := true
		for _, name := range []string{"machine.json", "process.json", "filament.json"} {
			f, e := os.Open(filepath.Join(c.SlicerProfiles, name))
			if e != nil {
				profilesOK = false
				continue
			}
			raw, e := io.ReadAll(io.LimitReader(f, (4<<20)+1))
			f.Close()
			var v map[string]any
			if e != nil || len(raw) > 4<<20 || json.Unmarshal(raw, &v) != nil || len(v) == 0 {
				profilesOK = false
			}
		}
		if c.SlicerPath != "" && c.SlicerProfiles != "" && engineErr == nil && profilesOK {
			r.add("slicer", "slicing", "ok", "切片引擎", "启动文件与三份配置可用；未执行引擎或切片。", "首次安装仍需独立验证引擎运行库与真实切片结果。")
		} else {
			r.add("slicer", "slicing", "error", "切片引擎", "启动文件不可执行，或三份配置缺失、格式无效。", "检查引擎可执行权限和 machine/process/filament.json；参阅切片部署说明。")
		}
	}
	if _, e := exec.LookPath("ffmpeg"); e == nil {
		r.add("video", "camera", "ok", "视频组件", "PATH 中找到 FFmpeg；没有连接摄像头。", "实际画面还取决于设备视频权限和协议。")
	} else {
		r.add("video", "camera", "optional", "视频组件", "未找到可选 FFmpeg；部分设备的视频预览可能不可用。", "需要时安装 FFmpeg 并使服务账户能够找到它。")
	}
	u, e := url.Parse(c.PublicURL)
	publicOK := e == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.RawQuery == "" && u.Fragment == ""
	if c.PublicURL == "" {
		r.add("public_url", "gateway", "optional", "公网入口配置", "未配置公网 HTTPS 地址；可用于本机试用。", "需要云端访问或邮件时配置 LAYERHUB_PUBLIC_URL。")
	} else if !publicOK {
		r.add("public_url", "gateway", "error", "公网入口配置", "公网地址配置无效；没有访问该地址。", "使用不含账号、查询参数或片段的 HTTPS 地址。")
	} else {
		r.add("public_url", "gateway", "ok", "公网入口配置", "已填写 HTTPS 地址；没有验证证书、DNS 或隧道。", "从浏览器打开公网入口，另行检查网关与 NAS 链路。")
	}
	if publicOK && !c.CookieSecure {
		r.add("secure_cookie", "gateway", "unknown", "会话 Cookie", "未强制 Secure Cookie；HTTPS 或可信回环代理仍可能启用安全策略。", "请从 HTTPS 公网入口打开系统诊断，核对当前请求的 Cookie 策略。")
	} else if c.CookieSecure {
		r.add("secure_cookie", "gateway", "ok", "会话 Cookie", "Secure Cookie 已开启；应通过 HTTPS 登录。", "")
	} else {
		r.add("secure_cookie", "gateway", "optional", "会话 Cookie", "Secure Cookie 关闭，适用于本机 HTTP 试用。", "公网部署完成 HTTPS 后再开启 Secure Cookie。")
	}
	if c.MailToken == "" {
		r.add("mail_config", "mail", "optional", "邮件通道配置", "未配置可选邮件通道，不影响网页任务管理。", "需要邮件时按邮件部署说明配置私有令牌和云端 SMTP 工作器。")
	} else if len(c.MailToken) < 32 || !publicOK {
		r.add("mail_config", "mail", "error", "邮件通道配置", "邮件令牌长度或公网 HTTPS 配置不满足要求。", "核对至少 32 字符的私有邮件令牌和 HTTPS 入口；不要公开令牌。")
	} else {
		r.add("mail_config", "mail", "ok", "邮件通道配置", "邮件通道配置满足基本要求；没有登录 SMTP 或发送邮件。", "运行时还需核对邮件工作器心跳。")
	}
	return r
}
func probeDiagnosticWrite(dir string) bool {
	f, e := os.CreateTemp(dir, ".layerhub-doctor-*")
	if e != nil {
		return false
	}
	name := f.Name()
	good := true
	if _, e = f.WriteString("LayerHub deployment probe\n"); e != nil {
		good = false
	}
	if e = f.Close(); e != nil {
		good = false
	}
	raw, e := os.ReadFile(name)
	if e != nil || string(raw) != "LayerHub deployment probe\n" {
		good = false
	}
	if e = os.Remove(name); e != nil {
		good = false
	}
	return good
}
func LocalDiagnostics(c Config, probe bool) DiagnosticReport {
	r := InspectDeployment(c, probe)
	path := filepath.Join(c.DataDir, "layerhub.sqlite")
	f, e := os.Open(path)
	if os.IsNotExist(e) {
		r.add("database", "storage", "unknown", "数据库", "数据库尚不存在；没有创建或初始化。", "全新安装首次启动后再运行检查。")
	} else if e != nil {
		r.add("database", "storage", "error", "数据库", "数据库文件不可读。", "检查服务账户与数据目录权限。")
	} else {
		var header [16]byte
		_, err := io.ReadFull(f, header[:])
		f.Close()
		if err != nil || string(header[:]) != "SQLite format 3\x00" {
			r.add("database", "storage", "error", "数据库", "数据库文件头无效；未尝试修复。", "停止误用该数据目录，保留文件并按备份排障说明处理。")
		} else {
			r.add("database", "storage", "ok", "数据库", "SQLite 文件头可读；未打开数据库或检查完整性。", "运行中可在系统诊断查看数据库查询状态。")
		}
	}
	r.add("runtime", "server", "unknown", "运行时与设备", "本地检查不验证运行中的服务、隧道、SMTP 或打印机。", "登录工作区后打开系统诊断，读取已有运行状态。")
	return r
}
func (s *Service) Diagnostics(ctx context.Context, u User) (DiagnosticReport, error) {
	fresh, e := currentUser(s.DB, u.ID)
	if e != nil {
		return DiagnosticReport{}, e
	}
	if !fresh.Admin {
		return DiagnosticReport{}, forbidden("系统诊断仅对管理员开放")
	}
	r := InspectDeployment(s.Config, false)
	r.Mode = "runtime"
	bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var version int
	if e := s.DB.QueryRowContext(bounded, "PRAGMA user_version").Scan(&version); e != nil {
		r.add("database", "storage", "error", "数据库", "运行中数据库查询失败；未执行修复。", "查看服务日志、存储状态和备份，不要重复提交未确认任务。")
	} else {
		r.add("database", "storage", "ok", "数据库", "运行中的数据库只读查询成功；未检查完整性。", "")
	}
	var seen int64
	var ready bool
	e = s.DB.QueryRowContext(bounded, "SELECT seen,ready FROM mail_worker WHERE id=1").Scan(&seen, &ready)
	if s.Config.MailToken != "" {
		status, detail, fix := "unknown", "尚无有效邮件工作器心跳。", "检查云端工作器服务、私有令牌与隧道；此处不会发送测试邮件。"
		age := time.Now().Unix() - seen
		if s.mailConfigured() && e == nil && age >= 0 && age < 60 {
			if ready {
				status = "ok"
				detail = "邮件工作器心跳新鲜并报告就绪；不代表邮件已投递。"
				fix = ""
			} else {
				status = "warning"
				detail = "邮件工作器在线，但报告 SMTP 尚未就绪。"
			}
		} else if e == nil {
			status = "warning"
			detail = "邮件工作器心跳过期或时间异常。"
		}
		r.add("mail_worker", "mail", status, "邮件工作器", detail, fix)
		if seen > 0 {
			r.Checks[len(r.Checks)-1].ObservedAt = seen * 1000
		}
	}
	var uncertain int
	if e = s.DB.QueryRowContext(bounded, "SELECT COUNT(*) FROM printer_commands WHERE state IN ('pending','uncertain')").Scan(&uncertain); e != nil {
		return DiagnosticReport{}, e
	}
	if uncertain > 0 {
		r.add("command_review", "safety", "warning", "控制结果核对", "有待确认或待人工核对的设备指令。", "打开设备控制记录并核对实际机器状态；不要重复发送或自动重试。")
	} else {
		r.add("command_review", "safety", "ok", "控制结果核对", "没有待确认或待人工核对的设备指令。", "")
	}
	rows, e := s.DB.QueryContext(bounded, "SELECT id,enabled FROM printers ORDER BY id")
	if e != nil {
		return DiagnosticReport{}, e
	}
	type entry struct {
		id      int64
		enabled bool
	}
	printers := []entry{}
	for rows.Next() {
		var p entry
		if e = rows.Scan(&p.id, &p.enabled); e != nil {
			rows.Close()
			return DiagnosticReport{}, e
		}
		printers = append(printers, p)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return DiagnosticReport{}, e
	}
	if len(printers) == 0 {
		r.add("printers", "printers", "optional", "打印机", "尚未登记打印机。", "可先完成离线试用，再按设备接入说明登记。")
	}
	for _, p := range printers {
		status, detail, fix := "disabled", "设备已停用，不执行连接测试。", ""
		observed := int64(0)
		if p.enabled {
			if s.Config.DeviceMode != "live" {
				status = "unknown"
				detail = "设备驱动不是 live；不将模拟或离线状态算作实机在线。"
				fix = "需要真实接入时核对驱动模式与局域网权限。"
			} else {
				telemetry := s.Device.Status(p.id)
				observed = telemetry.Updated.UnixMilli()
				if telemetry.Updated.IsZero() {
					observed = 0
				}
				age := time.Since(telemetry.Updated)
				status = "warning"
				detail = "设备连接不可用或没有新鲜遥测。"
				fix = "检查 NAS 与打印机的局域网、供电、访问码和权限；诊断不会尝试控制。"
				if telemetry.Connected && age >= 0 && age < 60*time.Second {
					status = "ok"
					detail = "已有连接与遥测新鲜；不等于所有控制或打印均获授权。"
					fix = ""
					if telemetry.Equipment.ControlIssue == "authorization_required" {
						r.add("printer_authorization", "safety", "warning", "设备控制授权", "设备报告了控制授权限制；遥测在线不代表控制可用。", "在打印机上核对局域网与开发者模式，再到控制台处理已有授权告警；不要重复发送指令。")
						r.Checks[len(r.Checks)-1].PrinterID = p.id
					}
					if telemetry.PrintError != 0 || len(telemetry.Equipment.HMS) > 0 {
						r.add("printer_alert", "safety", "warning", "设备告警", "设备存在已上报的告警；这里不包含原始告警内容。", "打开设备控制台并核对打印机屏幕，按实际告警处理，不自动恢复或重启打印。")
						r.Checks[len(r.Checks)-1].PrinterID = p.id
					}
				}
			}
		}
		r.add("printer", "printers", status, "打印机", detail, fix)
		last := &r.Checks[len(r.Checks)-1]
		last.PrinterID = p.id
		last.ObservedAt = observed
	}
	return r, nil
}
func LocalizeDiagnosticReport(r DiagnosticReport, lang string) DiagnosticReport {
	r.Checks = append([]DiagnosticCheck(nil), r.Checks...)
	for i := range r.Checks {
		c := &r.Checks[i]
		c.Label = languageText(lang, c.Label)
		c.Detail = languageText(lang, c.Detail)
		c.Fix = languageText(lang, c.Fix)
	}
	return r
}
