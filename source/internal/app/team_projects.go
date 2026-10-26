package app

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Independent geometry snapshots: no reference grants access to a private model,
// project, result, machine, spool or draft. Existing tables/formats stay intact.
const teamProjectSchema = `
CREATE TABLE IF NOT EXISTS team_projects(id INTEGER PRIMARY KEY,user_id INTEGER NOT NULL REFERENCES users(id),title TEXT NOT NULL,description TEXT NOT NULL,latest INTEGER NOT NULL,withdrawn INTEGER NOT NULL DEFAULT 0,withdrawn_by INTEGER REFERENCES users(id),updated INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS team_project_versions(id INTEGER PRIMARY KEY,project_id INTEGER NOT NULL REFERENCES team_projects(id),number INTEGER NOT NULL,title TEXT NOT NULL,description TEXT NOT NULL,note TEXT NOT NULL,state TEXT NOT NULL,created INTEGER NOT NULL,UNIQUE(project_id,number));
CREATE TABLE IF NOT EXISTS team_project_sources(version_id INTEGER NOT NULL REFERENCES team_project_versions(id),local_id INTEGER NOT NULL,filename TEXT NOT NULL,stl BLOB NOT NULL,triangles INTEGER NOT NULL,bounds TEXT NOT NULL,PRIMARY KEY(version_id,local_id));
CREATE TABLE IF NOT EXISTS team_project_receipts(user_id INTEGER NOT NULL REFERENCES users(id),request_key TEXT NOT NULL,kind TEXT NOT NULL,digest TEXT NOT NULL,result_id INTEGER NOT NULL,PRIMARY KEY(user_id,request_key));
CREATE TABLE IF NOT EXISTS team_project_copies(project_id INTEGER PRIMARY KEY REFERENCES slicer_projects(id) ON DELETE CASCADE,version_id INTEGER NOT NULL REFERENCES team_project_versions(id),created INTEGER NOT NULL);
CREATE INDEX IF NOT EXISTS team_project_owner ON team_projects(user_id,id DESC);
`

type TeamProject struct {
	ID          int64       `json:"id"`
	Author      AuditPerson `json:"author"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Latest      int         `json:"latest"`
	Withdrawn   bool        `json:"withdrawn"`
	AuthorOff   bool        `json:"authorDisabled"`
	Updated     int64       `json:"updated"`
	CanPublish  bool        `json:"canPublish"`
	CanManage   bool        `json:"canManage"`
	CanRestore  bool        `json:"canRestore"`
}
type TeamVersion struct {
	ID          int64         `json:"id"`
	Number      int           `json:"number"`
	Title       string        `json:"title"`
	Description string        `json:"description"`
	Note        string        `json:"note"`
	Created     int64         `json:"created"`
	State       SlicerState   `json:"state"`
	Sources     []ModelSource `json:"sources"`
}
type TeamVersionInfo struct {
	ID      int64  `json:"id"`
	Number  int    `json:"number"`
	Note    string `json:"note"`
	Created int64  `json:"created"`
}
type TeamDetail struct {
	OK       bool              `json:"ok"`
	Project  TeamProject       `json:"project"`
	Version  TeamVersion       `json:"version"`
	Versions []TeamVersionInfo `json:"versions"`
}

const teamSelect = `SELECT p.id,u.id,u.username,u.name,p.title,p.description,p.latest,p.withdrawn,u.disabled,p.updated,COALESCE(p.withdrawn_by,0) FROM team_projects p JOIN users u ON u.id=p.user_id`

func scanTeamProject(row interface{ Scan(...any) error }, u User) (TeamProject, error) {
	var p TeamProject
	var by int64
	err := row.Scan(&p.ID, &p.Author.ID, &p.Author.Username, &p.Author.Name, &p.Title, &p.Description, &p.Latest, &p.Withdrawn, &p.AuthorOff, &p.Updated, &by)
	p.CanPublish = p.Author.ID == u.ID && !p.Withdrawn
	p.CanManage = p.Author.ID == u.ID || u.Admin
	p.CanRestore = p.CanManage && (u.Admin || by == u.ID)
	return p, err
}
func teamAccess(q queryer, u User, id int64) (TeamProject, error) {
	p, e := scanTeamProject(q.QueryRow(teamSelect+" WHERE p.id=?", id), u)
	if e == sql.ErrNoRows || e == nil && (p.Withdrawn || p.AuthorOff) && !p.CanManage {
		return p, missing("共享工程不存在或已撤回")
	}
	return p, e
}
func (s *Service) teamProjects(u User, scope, search string, before int64) (any, error) {
	fresh, e := currentUser(s.DB, u.ID)
	if e != nil {
		return nil, e
	}
	u = fresh
	if scope == "" {
		scope = "team"
	}
	if before < 0 || utf8.RuneCountInString(search) > 80 || (scope != "team" && scope != "mine" && scope != "manage") {
		return nil, invalid("共享工程筛选无效")
	}
	if scope == "manage" && !u.Admin {
		return nil, forbidden("需要管理员权限")
	}
	query := teamSelect + " WHERE (?=0 OR p.id<?) AND instr(lower(p.title||' '||p.description),lower(?))>0"
	args := []any{before, before, search}
	if scope == "team" {
		query += " AND p.withdrawn=0 AND u.disabled=0"
	}
	if scope == "mine" {
		query += " AND p.user_id=?"
		args = append(args, u.ID)
	}
	rows, e := s.DB.Query(query+" ORDER BY p.id DESC LIMIT 21", args...)
	if e != nil {
		return nil, e
	}
	projects := []TeamProject{}
	for rows.Next() {
		p, err := scanTeamProject(rows, u)
		if err != nil {
			rows.Close()
			return nil, err
		}
		projects = append(projects, p)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	var next int64
	if len(projects) > 20 {
		projects = projects[:20]
		next = projects[19].ID
	}
	return map[string]any{"ok": true, "projects": projects, "nextCursor": next}, nil
}
func (s *Service) teamProjectDetail(u User, id, versionID int64) (TeamDetail, error) {
	out := TeamDetail{OK: true, Versions: []TeamVersionInfo{}}
	tx, e := s.DB.Begin()
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	u, e = currentUser(tx, u.ID)
	if e != nil {
		return out, e
	}
	out.Project, e = teamAccess(tx, u, id)
	if e != nil {
		return out, e
	}
	query := "SELECT id,number,title,description,note,state,created FROM team_project_versions WHERE project_id=?"
	args := []any{id}
	if versionID > 0 {
		query += " AND id=?"
		args = append(args, versionID)
	} else {
		query += " AND number=?"
		args = append(args, out.Project.Latest)
	}
	var raw string
	v := &out.Version
	e = tx.QueryRow(query, args...).Scan(&v.ID, &v.Number, &v.Title, &v.Description, &v.Note, &raw, &v.Created)
	if e == sql.ErrNoRows {
		return out, missing("共享版本不存在")
	}
	if e != nil {
		return out, e
	}
	if e = json.Unmarshal([]byte(raw), &v.State); e != nil {
		return out, e
	}
	v.Sources = []ModelSource{}
	rows, e := tx.Query("SELECT local_id,filename,triangles,bounds FROM team_project_sources WHERE version_id=? ORDER BY local_id", v.ID)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var m ModelSource
		var b string
		if e = rows.Scan(&m.ID, &m.Filename, &m.Triangles, &b); e != nil {
			rows.Close()
			return out, e
		}
		if e = json.Unmarshal([]byte(b), &m.Bounds); e != nil {
			rows.Close()
			return out, e
		}
		v.Sources = append(v.Sources, m)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	rows, e = tx.Query("SELECT id,number,note,created FROM team_project_versions WHERE project_id=? ORDER BY number DESC", id)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var item TeamVersionInfo
		if e = rows.Scan(&item.ID, &item.Number, &item.Note, &item.Created); e != nil {
			rows.Close()
			return out, e
		}
		out.Versions = append(out.Versions, item)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	return out, tx.Commit()
}

func teamText(value string, max int, required, multiline bool) (string, error) {
	v := strings.TrimSpace(value)
	if !utf8.ValidString(v) || utf8.RuneCountInString(v) > max || required && v == "" || strings.ContainsFunc(v, func(r rune) bool {
		return unicode.IsControl(r) && !(multiline && (r == '\n' || r == '\r' || r == '\t'))
	}) {
		return "", invalid("名称或说明长度无效，不能包含控制字符")
	}
	return v, nil
}

var teamRequestPattern = regexp.MustCompile(`^[a-zA-Z0-9-]{16,64}$`)

func teamDigest(v any) string {
	raw, _ := json.Marshal(v)
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}
func teamReceipt(tx *sql.Tx, u User, key, kind, digest string) (int64, error) {
	var id int64
	var gotKind, gotDigest string
	e := tx.QueryRow("SELECT kind,digest,result_id FROM team_project_receipts WHERE user_id=? AND request_key=?", u.ID, key).Scan(&gotKind, &gotDigest, &id)
	if e == sql.ErrNoRows {
		return 0, nil
	}
	if e != nil {
		return 0, e
	}
	if gotKind != kind || gotDigest != digest {
		return 0, conflict("此次提交标识已用于其他内容，请重新打开操作")
	}
	return id, nil
}

// Always operate on a private decoded state. IDs are local to each shared
// snapshot, and new copies receive fresh board/instance identities and no tasks.
func remapTeamState(state SlicerState, ids map[int64]int64, fresh bool) (SlicerState, error) {
	if len(state.Plates) == 0 {
		state.Plates = []SlicerPlate{{ID: "main", Name: "打印板 01", ModelID: state.ModelID, Options: state.Options, ActiveCopy: state.ActiveCopy}}
		state.ActivePlate = 0
	}
	state.FormatVersion = 4
	for i := range state.Plates {
		p := &state.Plates[i]
		p.TaskID = 0
		if fresh {
			p.ID = "team-" + randomID(10)
		}
		if p.ModelID == 0 {
			continue
		}
		primary := p.ModelID
		mapped, ok := ids[primary]
		if !ok {
			return state, invalid("共享工程缺少源模型")
		}
		p.Options.Copies = platePlacements(primary, p.Options)
		for j := range p.Options.Copies {
			o := &p.Options.Copies[j]
			mapped, ok := ids[o.ModelID]
			if !ok {
				return state, invalid("共享工程缺少对象来源")
			}
			o.ModelID = mapped
			if fresh || o.InstanceID == "" {
				o.InstanceID = "object-" + randomID(10)
			}
		}
		p.ModelID = mapped
	}
	return state, normalizeSlicerState(&state)
}

type TeamPublish struct {
	ProjectID       int64  `json:"projectId"`
	Revision        int64  `json:"revision"`
	SharedID        int64  `json:"sharedId"`
	ExpectedVersion int    `json:"expectedVersion"`
	Title           string `json:"title"`
	Description     string `json:"description"`
	Note            string `json:"note"`
	RequestKey      string `json:"requestKey"`
	Acknowledge     string `json:"acknowledge"`
}

func (s *Service) publishTeamProject(u User, in TeamPublish) (int64, bool, error) {
	if !teamRequestPattern.MatchString(in.RequestKey) || in.Acknowledge != "team-models-and-settings" || in.ProjectID <= 0 || in.Revision <= 0 || in.SharedID < 0 || in.ExpectedVersion < 0 {
		return 0, false, invalid("请确认分享范围及已保存的工程版本")
	}
	var e error
	if in.Title, e = teamText(in.Title, 80, true, false); e != nil {
		return 0, false, e
	}
	if in.Description, e = teamText(in.Description, 1000, false, true); e != nil {
		return 0, false, e
	}
	if in.Note, e = teamText(in.Note, 500, true, true); e != nil {
		return 0, false, e
	}
	digest := teamDigest(in)
	var versionID int64
	replayed := false
	e = s.transaction(func(tx *sql.Tx) error {
		var err error
		u, err = currentUser(tx, u.ID)
		if err != nil {
			return err
		}
		versionID, err = teamReceipt(tx, u, in.RequestKey, "publish", digest)
		if err != nil {
			return err
		}
		if versionID > 0 {
			replayed = true
			return nil
		}
		p, err := scanSlicerProject(tx.QueryRow(projectSelect+" WHERE p.id=? AND p.user_id=?", in.ProjectID, u.ID))
		if err == sql.ErrNoRows {
			return missing("只能分享自己的已保存工程")
		}
		if err != nil {
			return err
		}
		if p.Revision != in.Revision {
			return conflict("原工程已变化，请重新检查后分享")
		}
		if err = validateSlicerState(tx, u, p.SlicerState); err != nil {
			return err
		}
		sharedID, number := in.SharedID, 1
		if sharedID == 0 {
			if in.ExpectedVersion != 0 {
				return invalid("新共享工程版本无效")
			}
			var count int
			if err = tx.QueryRow("SELECT COUNT(*) FROM team_projects WHERE user_id=?", u.ID).Scan(&count); err != nil {
				return err
			}
			if count >= 10 {
				return conflict("每人最多 10 个共享工程，请复用已有条目发布新版")
			}
		} else {
			prior, err := teamAccess(tx, u, sharedID)
			if err != nil {
				return err
			}
			if prior.Author.ID != u.ID {
				return forbidden("只有作者可以发布此工程的新版本")
			}
			if prior.Withdrawn {
				return conflict("请先恢复此共享工程，再发布新版本")
			}
			if prior.Latest != in.ExpectedVersion {
				return conflict("共享工程已有新版本，请刷新后检查")
			}
			if prior.Latest >= 20 {
				return conflict("此共享工程最多保留 20 个版本，请新建共享条目")
			}
			number = prior.Latest + 1
		}
		ids := []int64{}
		seen := map[int64]bool{}
		boards := p.Plates
		if len(boards) == 0 {
			boards = []SlicerPlate{{ModelID: p.ModelID, Options: p.Options}}
		}
		for _, board := range boards {
			if board.ModelID == 0 {
				continue
			}
			sources, err := plateSourceIDs(board.ModelID, board.Options)
			if err != nil {
				return err
			}
			for _, id := range sources {
				if !seen[id] {
					seen[id] = true
					ids = append(ids, id)
				}
			}
		}
		var bytes int64
		for _, id := range ids {
			var n int64
			if err = tx.QueryRow("SELECT length(stl) FROM model_sources WHERE id=? AND user_id=?", id, u.ID).Scan(&n); err != nil {
				return err
			}
			bytes += n
		}
		var authorBytes, totalBytes int64
		if err = tx.QueryRow("SELECT COALESCE(SUM(length(s.stl)),0) FROM team_project_sources s JOIN team_project_versions v ON v.id=s.version_id JOIN team_projects p ON p.id=v.project_id WHERE p.user_id=?", u.ID).Scan(&authorBytes); err != nil {
			return err
		}
		if err = tx.QueryRow("SELECT COALESCE(SUM(length(stl)),0) FROM team_project_sources").Scan(&totalBytes); err != nil {
			return err
		}
		if bytes+authorBytes > 128<<20 || bytes+totalBytes > 512<<20 {
			return conflict("共享模型空间不足（每人 128 MB、团队 512 MB，包含保留版本）；没有发布任何内容")
		}
		now := time.Now().UnixMilli()
		if sharedID == 0 {
			r, err := tx.Exec("INSERT INTO team_projects(user_id,title,description,latest,updated) VALUES(?,?,?,?,?)", u.ID, in.Title, in.Description, number, now)
			if err != nil {
				return err
			}
			sharedID, err = r.LastInsertId()
			if err != nil {
				return err
			}
		} else {
			if _, err = tx.Exec("UPDATE team_projects SET title=?,description=?,latest=?,updated=? WHERE id=?", in.Title, in.Description, number, now, sharedID); err != nil {
				return err
			}
		}
		mapping := map[int64]int64{}
		for i, id := range ids {
			mapping[id] = int64(i + 1)
		}
		state, err := remapTeamState(p.SlicerState, mapping, false)
		if err != nil {
			return err
		}
		raw, err := json.Marshal(state)
		if err != nil {
			return err
		}
		r, err := tx.Exec("INSERT INTO team_project_versions(project_id,number,title,description,note,state,created) VALUES(?,?,?,?,?,?,?)", sharedID, number, in.Title, in.Description, in.Note, string(raw), now)
		if err != nil {
			return err
		}
		versionID, err = r.LastInsertId()
		if err != nil {
			return err
		}
		for _, id := range ids {
			if _, err = tx.Exec("INSERT INTO team_project_sources(version_id,local_id,filename,stl,triangles,bounds) SELECT ?,?,filename,stl,triangles,bounds FROM model_sources WHERE id=? AND user_id=?", versionID, mapping[id], id, u.ID); err != nil {
				return err
			}
		}
		_, err = tx.Exec("INSERT INTO team_project_receipts(user_id,request_key,kind,digest,result_id) VALUES(?,?,'publish',?,?)", u.ID, in.RequestKey, digest, versionID)
		return err
	})
	return versionID, replayed, e
}

type TeamCopy struct {
	VersionID  int64  `json:"versionId"`
	RequestKey string `json:"requestKey"`
	Name       string `json:"name"`
}

func (s *Service) copyTeamProject(u User, in TeamCopy) (SlicerProject, bool, error) {
	var out SlicerProject
	replayed := false
	if in.VersionID <= 0 || !teamRequestPattern.MatchString(in.RequestKey) {
		return out, false, invalid("请选择明确的共享版本")
	}
	var e error
	in.Name, e = teamText(in.Name, 80, true, false)
	if e != nil {
		return out, false, e
	}
	digest := teamDigest(in)
	e = s.transaction(func(tx *sql.Tx) error {
		var err error
		u, err = currentUser(tx, u.ID)
		if err != nil {
			return err
		}
		id, err := teamReceipt(tx, u, in.RequestKey, "copy", digest)
		if err != nil {
			return err
		}
		if id > 0 {
			out, err = scanSlicerProject(tx.QueryRow(projectSelect+" WHERE p.id=? AND p.user_id=?", id, u.ID))
			if err == sql.ErrNoRows {
				return conflict("此前复制成功的工程已删除，请重新打开共享版本后操作")
			}
			replayed = true
			return err
		}
		var sharedID int64
		var raw string
		if err = tx.QueryRow("SELECT project_id,state FROM team_project_versions WHERE id=?", in.VersionID).Scan(&sharedID, &raw); err == sql.ErrNoRows {
			return missing("共享版本不存在")
		}
		if err != nil {
			return err
		}
		shared, err := teamAccess(tx, u, sharedID)
		if err != nil {
			return err
		}
		if shared.Withdrawn || shared.AuthorOff {
			return conflict("此共享工程已下架或作者已停用，不能继续复制")
		}
		var projects, models int
		var used, bytes int64
		if err = tx.QueryRow("SELECT COUNT(*) FROM slicer_projects WHERE user_id=?", u.ID).Scan(&projects); err != nil {
			return err
		}
		if projects >= 20 {
			return conflict("我的工程已满（20 个），当前工程和草稿未改变")
		}
		var duplicate int
		if err = tx.QueryRow("SELECT COUNT(*) FROM slicer_projects WHERE user_id=? AND name=? COLLATE NOCASE", u.ID, in.Name).Scan(&duplicate); err != nil {
			return err
		}
		if duplicate > 0 {
			return conflict("已有同名私人工程，请换个名称")
		}
		if err = tx.QueryRow("SELECT COUNT(*),COALESCE(SUM(length(stl)),0) FROM model_sources WHERE user_id=?", u.ID).Scan(&models, &used); err != nil {
			return err
		}
		var count int
		if err = tx.QueryRow("SELECT COUNT(*),COALESCE(SUM(length(stl)),0) FROM team_project_sources WHERE version_id=?", in.VersionID).Scan(&count, &bytes); err != nil {
			return err
		}
		if count < 1 || count > 30 || models+count > 30 || used+bytes > 128<<20 {
			return conflict("我的模型空间不足（30 种 / 128 MB），没有复制任何内容")
		}
		state := SlicerState{}
		if err = json.Unmarshal([]byte(raw), &state); err != nil {
			return err
		}
		rows, err := tx.Query("SELECT local_id FROM team_project_sources WHERE version_id=? ORDER BY local_id", in.VersionID)
		if err != nil {
			return err
		}
		locals := []int64{}
		for rows.Next() {
			var n int64
			if err = rows.Scan(&n); err != nil {
				rows.Close()
				return err
			}
			locals = append(locals, n)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		now := time.Now()
		mapping := map[int64]int64{}
		for _, local := range locals {
			r, err := tx.Exec("INSERT INTO model_sources(user_id,filename,stl,triangles,bounds,created) SELECT ?,filename,stl,triangles,bounds,? FROM team_project_sources WHERE version_id=? AND local_id=?", u.ID, now.Unix(), in.VersionID, local)
			if err != nil {
				return err
			}
			mapping[local], err = r.LastInsertId()
			if err != nil {
				return err
			}
		}
		state, err = remapTeamState(state, mapping, true)
		if err != nil {
			return err
		}
		if err = validateSlicerState(tx, u, state); err != nil {
			return err
		}
		r, err := tx.Exec("INSERT INTO slicer_projects(user_id,model_id,name,options,active_copy,revision,updated) VALUES(?,?,?,?,?,1,?)", u.ID, state.ModelID, in.Name, encodeSlicerOptions(state), state.ActiveCopy, now.UnixMilli())
		if err != nil {
			return err
		}
		id, err = r.LastInsertId()
		if err != nil {
			return err
		}
		if _, err = tx.Exec("INSERT INTO team_project_copies(project_id,version_id,created) VALUES(?,?,?)", id, in.VersionID, now.UnixMilli()); err != nil {
			return err
		}
		if _, err = tx.Exec("INSERT INTO team_project_receipts(user_id,request_key,kind,digest,result_id) VALUES(?,?,'copy',?,?)", u.ID, in.RequestKey, digest, id); err != nil {
			return err
		}
		out = SlicerProject{ID: id, Name: in.Name, Revision: 1, Updated: now.UnixMilli(), SlicerState: state}
		return tx.QueryRow("SELECT filename FROM model_sources WHERE id=? AND user_id=?", state.ModelID, u.ID).Scan(&out.Filename)
	})
	return out, replayed, e
}

type TeamVisibility struct {
	ID              int64 `json:"id"`
	ExpectedVersion int   `json:"expectedVersion"`
	Withdrawn       bool  `json:"withdrawn"`
}

func (s *Service) setTeamVisibility(u User, in TeamVisibility) error {
	return s.transaction(func(tx *sql.Tx) error {
		var e error
		u, e = currentUser(tx, u.ID)
		if e != nil {
			return e
		}
		p, e := teamAccess(tx, u, in.ID)
		if e != nil {
			return e
		}
		if !p.CanManage {
			return forbidden("只能撤回自己的发布；管理员可以下架共享条目")
		}
		if in.ExpectedVersion != p.Latest {
			return conflict("共享版本已变化，请重新读取后操作")
		}
		if !in.Withdrawn && !p.CanRestore {
			return forbidden("此条目由管理员下架，请联系管理员恢复")
		}
		if p.Withdrawn == in.Withdrawn {
			return nil
		}
		_, e = tx.Exec("UPDATE team_projects SET withdrawn=?,withdrawn_by=?,updated=? WHERE id=?", in.Withdrawn, u.ID, time.Now().UnixMilli(), in.ID)
		return e
	})
}

func (s *Service) teamCopyOrigin(u User, id int64) (any, error) {
	var shared, version int64
	var number int
	var title string
	e := s.DB.QueryRow(`SELECT p.id,v.id,v.number,v.title FROM team_project_copies c JOIN slicer_projects own ON own.id=c.project_id JOIN team_project_versions v ON v.id=c.version_id JOIN team_projects p ON p.id=v.project_id WHERE own.id=? AND own.user_id=?`, id, u.ID).Scan(&shared, &version, &number, &title)
	if e == sql.ErrNoRows {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	return map[string]any{"sharedId": shared, "versionId": version, "number": number, "title": title, "label": fmt.Sprintf("来自团队工程 v%d；本人的独立副本", number)}, nil
}
