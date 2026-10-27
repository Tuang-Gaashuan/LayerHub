package updatekit

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func Run(args []string, in io.Reader, out io.Writer) error {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(out, "LayerHub UpdateKit 0.1.1-beta\nCommands: download / nas / cloud / menu\nUse <command> --help for options. Default channel: beta. Updates are preflight-only unless --apply is supplied. No first-time cloud setup or automatic cross-channel migration.")
		return nil
	}
	input := bufio.NewReader(in)
	if len(args) == 0 || args[0] == "menu" {
		fmt.Fprintln(out, "LayerHub 下载/更新工具 · 0.1.1-beta\n1. 下载 Beta Windows 包\n2. 下载 Beta NAS/Linux 包\n3. 更新此 NAS（Linux，需 sudo）\n4. 更新此云端界面（Linux，需 sudo）\n0. 退出\n默认 Beta；不会自动把 Alpha 切成 Beta。")
		fmt.Fprint(out, "选择 / Choice: ")
		choice, _ := input.ReadString('\n')
		switch strings.TrimSpace(choice) {
		case "1":
			args = []string{"download", "--platform", "windows", "--extract"}
		case "2":
			args = []string{"download", "--platform", "linux"}
		case "3":
			args = []string{"nas", "--apply"}
		case "4":
			args = []string{"cloud", "--apply"}
		case "0":
			return nil
		default:
			return errors.New("无效选择")
		}
	}
	fs := flag.NewFlagSet("layerhub-update "+args[0], flag.ContinueOnError)
	fs.SetOutput(out)
	channel := fs.String("channel", "beta", "beta or alpha; never automatically switch channels")
	tag := fs.String("version", "", "fixed release tag, e.g. v0.1.1-beta; default: newest channel version")
	output := fs.String("output", "", "download cache directory")
	platform := fs.String("platform", runtime.GOOS, "windows or linux (amd64)")
	extract := fs.Bool("extract", false, "extract to a new version folder; preserve existing folders")
	apply := fs.Bool("apply", false, "apply update after interactive confirmation; default is preflight only")
	root := fs.String("root", "/opt/layerhub", "existing NAS install root")
	env := fs.String("env-file", "/etc/layerhub/layerhub.env", "existing NAS environment file (never rewritten)")
	origin := fs.String("api-origin", "", "loopback API origin; NAS defaults to environment address, cloud to 15181")
	nginx := fs.String("nginx-config", "", "existing LayerHub Nginx site; no certificate or SSH installation")
	static := fs.String("static-root", "/usr/share/layerhub-gateway/releases", "new cloud UI release parent")
	pack := fs.String("package", "", "offline Linux archive with matching .verified.json receipt")
	packageDir := fs.String("package-dir", "", "offline-only directory containing downloaded Linux packages and receipts")
	backupRoot := fs.String("backup-root", "/var/backups/layerhub-update", "private backup parent, independent of application data")
	if e := fs.Parse(args[1:]); e != nil {
		if errors.Is(e, flag.ErrHelp) {
			return nil
		}
		return e
	}
	if fs.NArg() != 0 {
		return errors.New("存在未知参数")
	}
	if *channel != "alpha" && *channel != "beta" {
		return errors.New("通道只能为 alpha 或 beta")
	}
	if *output == "" {
		if args[0] == "download" {
			exe, e := os.Executable()
			if e != nil {
				return e
			}
			*output = filepath.Join(filepath.Dir(exe), "downloads")
		} else {
			*output = "/var/cache/layerhub-update"
		}
	}
	o := Options{Channel: *channel, Tag: *tag, Output: *output, Root: *root, Env: *env, Origin: *origin, Nginx: *nginx, StaticRoot: *static, Apply: *apply, Package: *pack, PackageDir: *packageDir, BackupRoot: *backupRoot}
	switch args[0] {
	case "download":
		b, e := Download(*channel, *platform, *tag, *output, *extract, out)
		if e != nil {
			return e
		}
		if b.Directory != "" {
			fmt.Fprintln(out, "解压位置 / Extracted folder:", b.Directory)
		}
		return nil
	case "nas":
		return UpdateNAS(o, input, out)
	case "cloud":
		return UpdateCloud(o, input, out)
	default:
		return errors.New("支持命令：download / nas / cloud / menu")
	}
}
