// Package updatekit downloads fixed release assets and updates existing installations.
package updatekit

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const Repository = "Tuang-Gaashuan/LayerHub"
const maxAsset int64 = 1 << 30
const maxExpanded int64 = 3 << 30

var tagPattern = regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)-(alpha|beta)(?:\.(\d+))?$`)
var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type Asset struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Size   int64  `json:"size"`
	Digest string `json:"digest"`
}
type Release struct {
	Tag    string  `json:"tag_name"`
	Draft  bool    `json:"draft"`
	Assets []Asset `json:"assets"`
}
type Receipt struct {
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
	Platform   string `json:"platform"`
	Filename   string `json:"filename"`
	SHA256     string `json:"sha256"`
	Size       int64  `json:"size"`
}
type Bundle struct {
	Receipt            Receipt
	Archive, Directory string
}

func version(tag string) ([4]int, string, error) {
	var result [4]int
	m := tagPattern.FindStringSubmatch(tag)
	if m == nil {
		return result, "", errors.New("版本标签格式无效 / Invalid version tag")
	}
	for i, text := range []string{m[1], m[2], m[3], m[5]} {
		if text == "" {
			continue
		}
		n, err := strconv.Atoi(text)
		if err != nil {
			return result, "", errors.New("版本号超出范围")
		}
		result[i] = n
	}
	return result, m[4], nil
}
func compare(a, b string) (int, error) {
	x, c, e := version(a)
	if e != nil {
		return 0, e
	}
	y, d, e := version(b)
	if e != nil {
		return 0, e
	}
	if c != d {
		return 0, errors.New("禁止自动切换 Alpha/Beta 通道 / Cross-channel update refused")
	}
	for i := range x {
		if x[i] < y[i] {
			return -1, nil
		}
		if x[i] > y[i] {
			return 1, nil
		}
	}
	return 0, nil
}
func choose(releases []Release, channel string) (Release, error) {
	var valid []Release
	for _, r := range releases {
		_, c, e := version(r.Tag)
		if e == nil && !r.Draft && c == channel {
			valid = append(valid, r)
		}
	}
	if len(valid) == 0 {
		return Release{}, errors.New("该通道没有可用发布 / No release in this channel")
	}
	sort.SliceStable(valid, func(i, j int) bool { n, _ := compare(valid[i].Tag, valid[j].Tag); return n > 0 })
	return valid[0], nil
}
func officialURL(raw string) bool {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return false
	}
	h := strings.ToLower(u.Hostname())
	return h == "api.github.com" || h == "github.com" || h == "githubusercontent.com" || strings.HasSuffix(h, ".githubusercontent.com")
}
func client() *http.Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = systemProxy
	t.DisableCompression = true
	return &http.Client{Transport: t, Timeout: 15 * time.Minute, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) > 8 || !officialURL(r.URL.String()) {
			return errors.New("下载重定向不属于 GitHub / Untrusted redirect")
		}
		return nil
	}}
}
func request(c *http.Client, raw, accept string) (*http.Response, error) {
	if !officialURL(raw) {
		return nil, errors.New("非官方发布地址 / Untrusted release URL")
	}
	r, e := http.NewRequest("GET", raw, nil)
	if e != nil {
		return nil, e
	}
	r.Header.Set("User-Agent", "LayerHub-UpdateKit/0.1.1-beta")
	r.Header.Set("Accept", accept)
	r.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	response, e := c.Do(r)
	if e != nil {
		return nil, errors.New("网络请求失败，请检查 GitHub 连接和系统代理 / Network request failed")
	}
	if response.StatusCode != 200 {
		response.Body.Close()
		return nil, responseError(response)
	}
	return response, nil
}
func responseError(response *http.Response) error {
	if response.StatusCode == http.StatusTooManyRequests || response.StatusCode == http.StatusForbidden && response.Header.Get("X-RateLimit-Remaining") == "0" {
		wait := "请稍后重试 / Please retry later"
		if reset, e := strconv.ParseInt(response.Header.Get("X-RateLimit-Reset"), 10, 64); e == nil && reset > time.Now().Unix() && reset < time.Now().Add(24*time.Hour).Unix() {
			wait = "预计可重试时间 / Retry after: " + time.Unix(reset, 0).Local().Format("2006-01-02 15:04:05 MST")
		}
		return fmt.Errorf("GitHub 公共下载查询额度已用完 / Public API rate limit reached. %s。已下载文件和运行数据未被覆盖。也可在可信电脑下载 NAS 包后使用离线更新入口。发布页 / Releases: https://github.com/%s/releases", wait, Repository)
	}
	return fmt.Errorf("GitHub HTTP %d；请检查网络或访问限制 / Check network access. Releases: https://github.com/%s/releases", response.StatusCode, Repository)
}
func latest(c *http.Client, channel, tag string) (Release, error) {
	if channel != "alpha" && channel != "beta" {
		return Release{}, errors.New("通道只能为 alpha 或 beta")
	}
	if tag != "" {
		_, ch, e := version(tag)
		if e != nil {
			return Release{}, e
		}
		if ch != channel {
			return Release{}, errors.New("标签与通道不一致")
		}
		r, e := request(c, "https://api.github.com/repos/"+Repository+"/releases/tags/"+tag, "application/vnd.github+json")
		if e != nil {
			return Release{}, e
		}
		defer r.Body.Close()
		var release Release
		e = json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(&release)
		if e != nil || release.Tag != tag || release.Draft {
			return Release{}, errors.New("发布元数据无效")
		}
		return release, nil
	}
	var all []Release
	for page := 1; page <= 10; page++ {
		r, e := request(c, fmt.Sprintf("https://api.github.com/repos/%s/releases?per_page=100&page=%d", Repository, page), "application/vnd.github+json")
		if e != nil {
			return Release{}, e
		}
		var rows []Release
		e = json.NewDecoder(io.LimitReader(r.Body, 8<<20)).Decode(&rows)
		r.Body.Close()
		if e != nil {
			return Release{}, errors.New("无法读取发布列表")
		}
		all = append(all, rows...)
		if len(rows) < 100 {
			return choose(all, channel)
		}
	}
	return Release{}, errors.New("发布列表过长，请使用 --version 指定版本")
}
func asset(r Release, name string) (Asset, error) {
	var found []Asset
	for _, a := range r.Assets {
		if a.Name == name {
			found = append(found, a)
		}
	}
	if len(found) != 1 {
		return Asset{}, fmt.Errorf("发布缺少唯一附件 %s", name)
	}
	a := found[0]
	u, e := url.Parse(a.URL)
	if e != nil || u.Scheme != "https" || u.Host != "api.github.com" || !strings.HasPrefix(u.Path, "/repos/"+Repository+"/releases/assets/") || a.Size <= 0 || a.Size > maxAsset {
		return Asset{}, errors.New("附件元数据不可信")
	}
	return a, nil
}
func hashFile(path string) (string, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return "", e
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func checksum(raw []byte, name string) (string, error) {
	found := ""
	for _, line := range strings.Split(string(raw), "\n") {
		parts := strings.Fields(strings.TrimPrefix(line, "\ufeff"))
		if len(parts) == 2 && parts[1] == name {
			if found != "" || !digestPattern.MatchString(parts[0]) {
				return "", errors.New("校验和重复或格式无效")
			}
			found = parts[0]
		}
	}
	if found == "" {
		return "", errors.New("校验文件未包含所选附件")
	}
	return found, nil
}
func ensureRegular(path string) error {
	s, e := os.Lstat(path)
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	if !s.Mode().IsRegular() {
		return errors.New("目标为链接或非普通文件，已停止")
	}
	return nil
}
func downloadAsset(c *http.Client, a Asset, path, expected string) error {
	if !digestPattern.MatchString(expected) {
		return errors.New("缺少有效 SHA256")
	}
	if e := ensureRegular(path); e != nil {
		return e
	}
	if h, e := hashFile(path); e == nil {
		if h == expected {
			return nil
		}
		return errors.New("目标文件已存在但校验不同，请更换下载目录")
	}
	partial := path + ".part"
	if e := ensureRegular(partial); e != nil {
		return e
	}
	for attempt := 0; attempt < 3; attempt++ {
		var offset int64
		if s, e := os.Stat(partial); e == nil {
			offset = s.Size()
		}
		if offset > a.Size {
			return errors.New("未完成下载大小异常，请更换目录")
		}
		if offset < a.Size {
			r, _ := http.NewRequest("GET", a.URL, nil)
			r.Header.Set("User-Agent", "LayerHub-UpdateKit/0.1.1-beta")
			r.Header.Set("Accept", "application/octet-stream")
			r.Header.Set("X-GitHub-Api-Version", "2022-11-28")
			if offset > 0 {
				r.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
			}
			response, e := c.Do(r)
			if e != nil {
				if attempt < 2 {
					time.Sleep(time.Duration(attempt+1) * time.Second)
					continue
				}
				return errors.New("下载中断；保留 .part，下次可继续")
			}
			if response.StatusCode != 200 && response.StatusCode != 206 {
				response.Body.Close()
				return responseError(response)
			}
			flags := os.O_CREATE | os.O_WRONLY
			if response.StatusCode == 206 {
				if !strings.HasPrefix(response.Header.Get("Content-Range"), fmt.Sprintf("bytes %d-", offset)) {
					response.Body.Close()
					return errors.New("续传响应范围错误")
				}
				flags |= os.O_APPEND
			} else {
				offset = 0
				flags |= os.O_TRUNC
			}
			f, e := os.OpenFile(partial, flags, 0o644)
			if e != nil {
				response.Body.Close()
				return e
			}
			n, copyErr := io.Copy(f, io.LimitReader(response.Body, a.Size-offset+1))
			syncErr := f.Sync()
			f.Close()
			response.Body.Close()
			if n > a.Size-offset {
				return errors.New("附件超过声明大小")
			}
			if copyErr != nil || syncErr != nil || n != a.Size-offset {
				if attempt < 2 {
					time.Sleep(time.Duration(attempt+1) * time.Second)
					continue
				}
				return errors.New("下载未完成；请重新运行继续下载")
			}
		}
		h, e := hashFile(partial)
		if e != nil {
			return e
		}
		if h != expected {
			return errors.New("SHA256 不匹配，未接受该下载；请更换目录后重试")
		}
		return os.Rename(partial, path)
	}
	return errors.New("下载未完成")
}
func Download(channel, platform, tag, out string, extract bool, w io.Writer) (Bundle, error) {
	if platform != "windows" && platform != "linux" {
		return Bundle{}, errors.New("平台只能为 windows 或 linux")
	}
	c := client()
	r, e := latest(c, channel, tag)
	if e != nil {
		return Bundle{}, e
	}
	if e = os.MkdirAll(out, 0o755); e != nil {
		return Bundle{}, e
	}
	fmt.Fprintln(w, "版本 / Version:", r.Tag, "·", platform, "amd64")
	ext := ".tar.gz"
	if platform == "windows" {
		ext = ".zip"
	}
	name := "LayerHub-" + strings.TrimPrefix(r.Tag, "v") + "-" + platform + "-amd64" + ext
	a, e := asset(r, name)
	if e != nil {
		return Bundle{}, e
	}
	sums, e := asset(r, "SHA256SUMS")
	if e != nil {
		return Bundle{}, e
	}
	response, e := request(c, sums.URL, "application/octet-stream")
	if e != nil {
		return Bundle{}, e
	}
	raw, e := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	response.Body.Close()
	if e != nil {
		return Bundle{}, e
	}
	sumHash := sha256.Sum256(raw)
	if sums.Digest != "" && sums.Digest != "sha256:"+hex.EncodeToString(sumHash[:]) {
		return Bundle{}, errors.New("SHA256SUMS 与 GitHub 附件摘要不符")
	}
	expected, e := checksum(raw, name)
	if e != nil {
		return Bundle{}, e
	}
	if a.Digest != "" && a.Digest != "sha256:"+expected {
		return Bundle{}, errors.New("附件摘要与校验文件不符")
	}
	path := filepath.Join(out, name)
	fmt.Fprintln(w, "正在下载并校验 / Downloading and verifying:", name)
	if e = downloadAsset(c, a, path, expected); e != nil {
		return Bundle{}, e
	}
	receipt := Receipt{Repository: Repository, Tag: r.Tag, Platform: platform, Filename: name, SHA256: expected, Size: a.Size}
	b := Bundle{Receipt: receipt, Archive: path}
	if e = writeJSON(path+".verified.json", receipt, 0o644); e != nil {
		return b, e
	}
	if extract {
		destination := filepath.Join(out, "LayerHub-"+strings.TrimPrefix(r.Tag, "v")+"-"+platform)
		if s, er := os.Lstat(destination); er == nil && s.IsDir() {
			raw, er := os.ReadFile(filepath.Join(destination, "VERSION"))
			if er != nil || strings.TrimSpace(string(raw)) != strings.TrimPrefix(r.Tag, "v") {
				return b, errors.New("已有解压目录版本不匹配，未覆盖")
			}
			b.Directory = destination
			fmt.Fprintln(w, "保留已有解压目录，不覆盖其中的运行数据 / Existing folder preserved")
		} else {
			b.Directory, e = Extract(path, destination, r.Tag)
			if e != nil {
				return b, e
			}
		}
	}
	fmt.Fprintln(w, "校验完成 / Verified:", path)
	return b, nil
}
func writeJSON(path string, v any, mode os.FileMode) error {
	raw, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	if e = ensureRegular(path); e != nil {
		return e
	}
	return os.WriteFile(path, append(raw, '\n'), mode)
}

func cleanMember(name, prefix string, seen map[string]bool) (string, error) {
	if name == "" || strings.ContainsAny(name, "\\:\x00\r\n") || strings.HasPrefix(name, "/") {
		return "", errors.New("归档路径无效")
	}
	parts := strings.Split(strings.TrimSuffix(name, "/"), "/")
	if len(parts) < 1 || parts[0] != prefix {
		return "", errors.New("归档根目录与版本不符")
	}
	for _, p := range parts {
		if p == "" || p == "." || p == ".." || strings.HasSuffix(p, ".") || strings.HasSuffix(p, " ") {
			return "", errors.New("归档包含不安全路径")
		}
		base := strings.ToUpper(strings.SplitN(p, ".", 2)[0])
		if regexp.MustCompile(`^(CON|PRN|AUX|NUL|COM[0-9]|LPT[0-9])$`).MatchString(base) {
			return "", errors.New("归档包含保留路径")
		}
	}
	key := strings.ToLower(strings.TrimSuffix(name, "/"))
	if seen[key] {
		return "", errors.New("归档包含大小写冲突或重复路径")
	}
	seen[key] = true
	if len(parts) == 1 {
		return "", nil
	}
	return filepath.Join(parts[1:]...), nil
}
func Extract(archive, destination, tag string) (string, error) {
	if _, _, e := version(tag); e != nil {
		return "", e
	}
	if _, e := os.Lstat(destination); !os.IsNotExist(e) {
		return "", errors.New("解压目标已存在，未覆盖现有文件或数据；请更换目录")
	}
	parent := filepath.Dir(destination)
	if e := os.MkdirAll(parent, 0o755); e != nil {
		return "", e
	}
	stage, e := os.MkdirTemp(parent, ".layerhub-stage-")
	if e != nil {
		return "", e
	}
	// Failed staging directories are left for inspection; never recursively delete a user path.
	seen := map[string]bool{}
	var total int64
	count := 0
	prefix := "LayerHub-" + strings.TrimPrefix(tag, "v")
	put := func(name string, size int64, mode os.FileMode, reader io.Reader, isDir bool) error {
		count++
		if count > 50000 || size < 0 || size > maxExpanded || total > maxExpanded-size {
			return errors.New("归档展开超过限制")
		}
		total += size
		rel, e := cleanMember(name, prefix, seen)
		if e != nil {
			return e
		}
		if rel == "" {
			if !isDir {
				return errors.New("归档根目录类型错误")
			}
			return nil
		}
		target := filepath.Join(stage, rel)
		if isDir {
			return os.MkdirAll(target, 0o755)
		}
		if e = os.MkdirAll(filepath.Dir(target), 0o755); e != nil {
			return e
		}
		perm := os.FileMode(0o644)
		if strings.HasPrefix(filepath.ToSlash(rel), "bin/") || strings.HasSuffix(rel, ".sh") {
			perm = 0o755
		}
		f, e := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
		if e != nil {
			return e
		}
		n, e := io.Copy(f, io.LimitReader(reader, size+1))
		ce := f.Close()
		if e != nil {
			return e
		}
		if ce != nil {
			return ce
		}
		if n != size {
			return errors.New("归档成员大小不匹配")
		}
		return nil
	}
	if strings.HasSuffix(archive, ".zip") {
		z, e := zip.OpenReader(archive)
		if e != nil {
			return "", e
		}
		defer z.Close()
		for _, entry := range z.File {
			m := entry.Mode()
			if !m.IsRegular() && !m.IsDir() {
				return "", errors.New("归档包含链接或特殊文件")
			}
			if entry.UncompressedSize64 > uint64(maxExpanded) {
				return "", errors.New("归档成员过大")
			}
			r, e := entry.Open()
			if e != nil {
				return "", e
			}
			e = put(entry.Name, int64(entry.UncompressedSize64), m, r, m.IsDir())
			r.Close()
			if e != nil {
				return "", e
			}
		}
	} else {
		f, e := os.Open(archive)
		if e != nil {
			return "", e
		}
		defer f.Close()
		g, e := gzip.NewReader(f)
		if e != nil {
			return "", e
		}
		defer g.Close()
		t := tar.NewReader(g)
		for {
			h, e := t.Next()
			if e == io.EOF {
				break
			}
			if e != nil {
				return "", e
			}
			if h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeDir {
				return "", errors.New("归档包含链接或特殊文件")
			}
			if e = put(h.Name, h.Size, 0, t, h.Typeflag == tar.TypeDir); e != nil {
				return "", e
			}
		}
	}
	raw, e := os.ReadFile(filepath.Join(stage, "VERSION"))
	if e != nil || strings.TrimSpace(string(raw)) != strings.TrimPrefix(tag, "v") {
		return "", errors.New("归档 VERSION 不匹配")
	}
	for _, p := range []string{"web/index.html", "web/source-offer.html", "web/source/layerhub-source.tar.gz"} {
		if e = ensureExistsRegular(filepath.Join(stage, filepath.FromSlash(p))); e != nil {
			return "", e
		}
	}
	if e = os.Rename(stage, destination); e != nil {
		return "", e
	}
	return destination, nil
}
func ensureExistsRegular(path string) error {
	s, e := os.Lstat(path)
	if e != nil || !s.Mode().IsRegular() {
		return errors.New("发布包缺少必要的普通文件")
	}
	return nil
}
