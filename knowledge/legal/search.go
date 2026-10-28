package legal

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	ksqlite "github.com/xujian519/mady/knowledge/sqlite"
	"github.com/xujian519/mady/pkg/lawcite"
)

// Config 是检索器的构造参数。
type Config struct {
	// DBPath 是法律知识库路径；为空时只检索 Records（内存降级）。
	DBPath string
	// Records 是内存降级时的记录集，可来自调用方已加载的数据。
	Records []Record
}

// newMemorySearcher 创建一个只检索内存记录的检索器（无数据库时使用）。
func newMemorySearcher(records []Record) *Searcher {
	return &Searcher{records: records, baseCtx: context.Background()}
}

// Searcher 是法律知识库检索器。
//
// 有库时优先 FTS5（BM25），库无 FTS 表或查询过短时降级 LIKE；无库时在
// Records 上做子串匹配。三种通道都在 Hit.Via 中标明，调用方不必猜。
type Searcher struct {
	mu              sync.RWMutex
	db              *sql.DB
	dbPath          string
	hasLawFTS       bool
	hasGuidingTable bool
	guidingData     bool
	records         []Record
	baseCtx         context.Context
}

// Open 打开法律知识库。DBPath 非空但文件不是法律知识库（缺少 law 表）时报错，
// 不降级为空库——路径配错与「库里没数据」是两回事。
func Open(cfg Config) (*Searcher, error) {
	s := newMemorySearcher(cfg.Records)
	if cfg.DBPath == "" {
		return s, nil
	}
	if err := s.openDB(cfg.DBPath); err != nil {
		return nil, err
	}
	return s, nil
}

// ctxOrDefault 返回数据库操作使用的上下文；零值 Searcher 也能安全使用。
func (s *Searcher) ctxOrDefault() context.Context {
	if s.baseCtx == nil {
		return context.Background()
	}
	return s.baseCtx
}

func (s *Searcher) openDB(path string) error {
	db, err := ksqlite.OpenReadOnly(path)
	if err != nil {
		return fmt.Errorf("打开法律知识库失败: %w", err)
	}
	if !hasTable(s.ctxOrDefault(), db, "law") {
		_ = db.Close()
		return fmt.Errorf("法律知识库 %s 缺少 law 表，不是可用的法律知识库", path)
	}
	s.db = db
	s.dbPath = path
	s.hasLawFTS = hasTable(s.ctxOrDefault(), db, "law_fts")
	s.hasGuidingTable = hasTable(s.ctxOrDefault(), db, "guiding_cases")
	s.guidingData = s.detectGuidingData()
	return nil
}

// Close 关闭数据库连接。
func (s *Searcher) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return nil
	}
	err := s.db.Close()
	s.db = nil
	return err
}

// Reload 关闭当前连接并改开 path（安装新库后立即生效）。
func (s *Searcher) Reload(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		_ = s.db.Close()
		s.db = nil
		s.dbPath = ""
	}
	return s.openDB(path)
}

// DataSource 返回当前检索后端：db 或 memory。
func (s *Searcher) DataSource() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.db != nil {
		return "db"
	}
	return "memory"
}

// GuidingCaseDataAvailable 报告当前后端是否含指导性案例数据。
// 为 false 时检索结果应说明「本部署无数据」，而不是「没有匹配」。
func (s *Searcher) GuidingCaseDataAvailable() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.db != nil {
		return s.guidingData
	}
	for _, r := range s.records {
		if r.GuidingCaseNumber != "" {
			return true
		}
	}
	return false
}

// Search 按类别分派到四类检索之一。
func (s *Searcher) Search(scope Scope, query string, opts SearchOptions) ([]Hit, error) {
	switch scope {
	case ScopeRegulation:
		return s.SearchRegulations(query, opts)
	case ScopeJudicialInterpretation:
		return s.SearchJudicialInterpretations(query, opts)
	case ScopeCaseLaw:
		return s.SearchCases(query, opts)
	case ScopeGuidingCase:
		return s.SearchGuidingCases(query, opts)
	default:
		return nil, fmt.Errorf("未知的检索类别 %q", scope)
	}
}

// SearchRegulations 检索法规条文；opts.Level 为空时查法律与行政法规。
func (s *Searcher) SearchRegulations(query string, opts SearchOptions) ([]Hit, error) {
	levels := defaultRegulationLevels
	if opts.Level != "" {
		if !opts.Level.Valid() {
			return nil, fmt.Errorf("未知的法规层级 %q", opts.Level)
		}
		levels = []Level{opts.Level}
	}
	return s.searchLaw(ScopeRegulation, query, opts, levels, "", nil)
}

// SearchJudicialInterpretations 检索司法解释。
func (s *Searcher) SearchJudicialInterpretations(query string, opts SearchOptions) ([]Hit, error) {
	return s.searchLaw(ScopeJudicialInterpretation, query, opts,
		[]Level{LevelJudicialInterpretation}, "", nil)
}

// SearchCases 检索裁判案例。
func (s *Searcher) SearchCases(query string, opts SearchOptions) ([]Hit, error) {
	return s.searchLaw(ScopeCaseLaw, query, opts, []Level{LevelCase}, "", nil)
}

// SearchGuidingCases 检索指导性案例，支持「指导案例第 N 号」直达。
//
// 有 guiding_cases 表时优先按编号精确查找，未命中或非编号查询时按关键词检索；
// 没有该表时回退到案例表中名称或正文含「指导案例」的记录。
func (s *Searcher) SearchGuidingCases(query string, opts SearchOptions) ([]Hit, error) {
	s.mu.RLock()
	hasTableData := s.db != nil && s.hasGuidingTable
	s.mu.RUnlock()

	if !hasTableData {
		return s.searchLaw(ScopeGuidingCase, query, opts, []Level{LevelCase},
			` AND (l.name LIKE ? ESCAPE '\' OR l.content LIKE ? ESCAPE '\')`,
			[]any{likePattern("指导案例"), likePattern("指导案例")})
	}

	if number, ok := parseGuidingCaseNumber(query); ok {
		hits, err := s.lookupGuidingCaseByNumber(number)
		if err != nil {
			return nil, err
		}
		if len(hits) > 0 {
			return hits, nil
		}
	}
	return s.searchGuidingCasesLike(query, opts)
}

// searchLaw 在 law 表上检索指定层级，并按效力过滤。
func (s *Searcher) searchLaw(scope Scope, query string, opts SearchOptions, levels []Level,
	extraSQL string, extraArgs []any) ([]Hit, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	limit := normalizeLimit(opts.Limit)
	asOf, err := resolveAsOf(opts.AsOf)
	if err != nil {
		return nil, err
	}

	// 持读锁贯穿查询：Reload 会关闭旧连接，仅复制指针无法阻止关闭发生。
	s.mu.RLock()
	defer s.mu.RUnlock()
	db := s.db
	hasFTS := s.hasLawFTS
	records := s.records

	if db == nil {
		return memoryHits(records, scope, query, opts, limit, asOf), nil
	}

	// 效力过滤会丢弃部分行，按 3 倍召回再截断。
	fetch := limit * 3
	useFTS := hasFTS && len([]rune(query)) >= 3
	via := viaLike
	var rows *sql.Rows
	if useFTS {
		via = viaFTS
		rows, err = s.queryLawFTS(db, query, levels, extraSQL, extraArgs, fetch)
	} else {
		rows, err = s.queryLawLike(db, query, levels, extraSQL, extraArgs, fetch)
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var hits []Hit
	for rows.Next() {
		hit, expired, rowErr := scanLawHit(rows, scope, via)
		if rowErr != nil {
			return nil, rowErr
		}
		if isDatedScope(scope) {
			hit.Validity = resolveValidity(expired, hit.EffectiveDate, asOf)
			if !visible(hit.Validity, opts.includeUnverified()) {
				continue
			}
		}
		hits = append(hits, hit)
		if len(hits) >= limit {
			break
		}
	}
	return hits, rows.Err()
}

// queryLawFTS 走 FTS5（BM25）通道。
func (s *Searcher) queryLawFTS(db *sql.DB, query string, levels []Level,
	extraSQL string, extraArgs []any, limit int) (*sql.Rows, error) {
	levelSQL, levelArgs := levelFilter(levels)
	args := []any{ftsPhrase(query)}
	args = append(args, levelArgs...)
	args = append(args, extraArgs...)
	args = append(args, limit)
	return db.QueryContext(s.ctxOrDefault(), `
		SELECT `+lawColumns+`, bm25(law_fts)
		FROM law_fts
		JOIN law l ON l.rowid = law_fts.rowid
		LEFT JOIN category c ON c.id = l.category_id
		WHERE law_fts MATCH ?`+levelSQL+extraSQL+`
		ORDER BY bm25(law_fts)
		LIMIT ?`, args...)
}

// queryLawLike 走 LIKE 通道（无 FTS 表或查询过短时）。
func (s *Searcher) queryLawLike(db *sql.DB, query string, levels []Level,
	extraSQL string, extraArgs []any, limit int) (*sql.Rows, error) {
	levelSQL, levelArgs := levelFilter(levels)
	pattern := likePattern(query)
	args := []any{pattern, pattern}
	args = append(args, levelArgs...)
	args = append(args, extraArgs...)
	args = append(args, limit)
	return db.QueryContext(s.ctxOrDefault(), `
		SELECT `+lawColumns+`, 0.0
		FROM law l
		LEFT JOIN category c ON c.id = l.category_id
		WHERE (l.name LIKE ? ESCAPE '\' OR l.content LIKE ? ESCAPE '\')`+levelSQL+extraSQL+`
		ORDER BY COALESCE(l."order", 0), l.name
		LIMIT ?`, args...)
}

// lawColumns 是 law 行与 category 名的取值列，末尾由调用方补上分数列。
const lawColumns = `l.id, l.level, l.name, COALESCE(l.subtitle, ''), COALESCE(l.content, ''),
	COALESCE(l.publish, ''), COALESCE(l.valid_from, ''), l.expired, COALESCE(c.name, '')`

// scanLawHit 扫描一行 law 结果；第二个返回值是来源库的「已废止」标记。
func scanLawHit(rows *sql.Rows, scope Scope, via string) (Hit, bool, error) {
	var h Hit
	var expired int
	err := rows.Scan(&h.ID, &h.Level, &h.Name, &h.Subtitle, &h.Content,
		&h.PublishDate, &h.EffectiveDate, &expired, &h.Category, &h.RelevanceScore)
	if err != nil {
		return Hit{}, false, err
	}
	h.Scope = scope
	h.Content = truncateContent(h.Content)
	h.Via = via
	return h, expired != 0, nil
}

// isDatedScope 报告该类别是否带效力信息。
func isDatedScope(scope Scope) bool {
	return scope == ScopeRegulation || scope == ScopeJudicialInterpretation
}

// memoryHits 在内存记录上做子串匹配，作为无库降级通道。
func memoryHits(records []Record, scope Scope, query string, opts SearchOptions,
	limit int, asOf string) []Hit {
	lowerQuery := strings.ToLower(query)
	var hits []Hit
	for _, r := range records {
		if !recordInScope(r, scope, opts) {
			continue
		}
		validity := Validity("")
		if isDatedScope(scope) {
			validity = resolveValidity(r.Expired, r.EffectiveDate, asOf)
			if !visible(validity, opts.includeUnverified()) {
				continue
			}
		}
		haystack := strings.ToLower(strings.Join([]string{
			r.Name, r.Subtitle, r.Content, r.KeyPoints, r.FullText, r.GuidingCaseNumber,
		}, " "))
		score := float64(strings.Count(haystack, lowerQuery))
		if score == 0 {
			continue
		}
		hit := recordToHit(r, scope, score, viaMemory)
		hit.Validity = validity
		hits = append(hits, hit)
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].RelevanceScore != hits[j].RelevanceScore {
			return hits[i].RelevanceScore > hits[j].RelevanceScore
		}
		return hits[i].Name < hits[j].Name
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}

// recordToHit 把内存记录转成命中。
func recordToHit(r Record, scope Scope, score float64, via string) Hit {
	return Hit{
		Scope:          scope,
		ID:             r.ID,
		Level:          r.Level,
		Name:           r.Name,
		Subtitle:       r.Subtitle,
		Content:        truncateContent(r.Content),
		Category:       r.Category,
		PublishDate:    r.PublishDate,
		EffectiveDate:  r.EffectiveDate,
		CaseNumber:     r.GuidingCaseNumber,
		Court:          r.Court,
		KeyPoints:      truncateContent(r.KeyPoints),
		RelatedLaws:    r.RelatedLaws,
		RelevanceScore: score,
		Via:            via,
	}
}

// recordInScope 判断内存记录是否属于检索类别。
func recordInScope(r Record, scope Scope, opts SearchOptions) bool {
	switch scope {
	case ScopeRegulation:
		if opts.Level != "" {
			return r.Level == opts.Level
		}
		return r.Level == LevelLaw || r.Level == LevelAdministrativeRegulation
	case ScopeJudicialInterpretation:
		return r.Level == LevelJudicialInterpretation
	case ScopeCaseLaw:
		return r.Level == LevelCase
	case ScopeGuidingCase:
		return r.GuidingCaseNumber != ""
	default:
		return false
	}
}

// lookupGuidingCaseByNumber 按编号精确查找指导性案例。
func (s *Searcher) lookupGuidingCaseByNumber(number int) ([]Hit, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	db := s.db
	if db == nil {
		return nil, nil
	}
	candidates := []any{
		fmt.Sprintf("指导案例%d号", number),
		fmt.Sprintf("指导案例第%d号", number),
		fmt.Sprintf("第%d号", number),
	}
	rows, err := db.QueryContext(s.ctxOrDefault(), `
		SELECT case_id, guiding_case_number, case_name, COALESCE(court, ''),
		       COALESCE(decision_date, ''), COALESCE(case_type, ''),
		       COALESCE(key_points, ''), COALESCE(related_laws, ''), COALESCE(full_text, '')
		FROM guiding_cases
		WHERE guiding_case_number IN (?, ?, ?)
		LIMIT 1`, candidates...)
	if err != nil {
		return nil, fmt.Errorf("指导性案例编号检索失败: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanGuidingRows(rows)
}

// searchGuidingCasesLike 在 guiding_cases 表上按关键词检索。
func (s *Searcher) searchGuidingCasesLike(query string, opts SearchOptions) ([]Hit, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	db := s.db
	if db == nil {
		return nil, nil
	}
	pattern := likePattern(query)
	rows, err := db.QueryContext(s.ctxOrDefault(), `
		SELECT case_id, guiding_case_number, case_name, COALESCE(court, ''),
		       COALESCE(decision_date, ''), COALESCE(case_type, ''),
		       COALESCE(key_points, ''), COALESCE(related_laws, ''), COALESCE(full_text, '')
		FROM guiding_cases
		WHERE case_name LIKE ? ESCAPE '\'
		   OR key_points LIKE ? ESCAPE '\'
		   OR full_text LIKE ? ESCAPE '\'
		ORDER BY guiding_case_number
		LIMIT ?`, pattern, pattern, pattern, normalizeLimit(opts.Limit))
	if err != nil {
		return nil, fmt.Errorf("指导性案例检索失败: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanGuidingRows(rows)
}

func scanGuidingRows(rows *sql.Rows) ([]Hit, error) {
	var hits []Hit
	for rows.Next() {
		var h Hit
		var relatedLaws string
		if err := rows.Scan(&h.ID, &h.CaseNumber, &h.Name, &h.Court, &h.DecisionDate,
			&h.CaseType, &h.KeyPoints, &relatedLaws, &h.Content); err != nil {
			return nil, err
		}
		h.Scope = ScopeGuidingCase
		h.KeyPoints = truncateContent(h.KeyPoints)
		h.Content = truncateContent(h.Content)
		h.RelatedLaws = splitRelatedLaws(relatedLaws)
		h.Via = viaLike
		hits = append(hits, h)
	}
	return hits, rows.Err()
}

// splitRelatedLaws 解析 related_laws 列：JSON 数组或逗号分隔文本。
func splitRelatedLaws(value string) []string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	if strings.HasPrefix(trimmed, "[") {
		var out []string
		if err := json.Unmarshal([]byte(trimmed), &out); err == nil {
			return out
		}
	}
	var out []string
	for _, part := range strings.Split(trimmed, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// detectGuidingData 判断当前库是否含指导性案例数据（调用方需持有连接）。
func (s *Searcher) detectGuidingData() bool {
	if s.db == nil {
		return false
	}
	if s.hasGuidingTable {
		var n int
		if err := s.db.QueryRowContext(s.ctxOrDefault(), `SELECT COUNT(*) FROM guiding_cases`).Scan(&n); err == nil && n > 0 {
			return true
		}
	}
	var n int
	err := s.db.QueryRowContext(s.ctxOrDefault(), `
		SELECT COUNT(*) FROM law
		WHERE level = ? AND (name LIKE ? ESCAPE '\' OR content LIKE ? ESCAPE '\')`,
		string(LevelCase), likePattern("指导案例"), likePattern("指导案例")).Scan(&n)
	return err == nil && n > 0
}

// ---------- SQL 辅助 ----------

func hasTable(ctx context.Context, db *sql.DB, name string) bool {
	var n int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&n)
	return err == nil && n > 0
}

func levelFilter(levels []Level) (string, []any) {
	if len(levels) == 0 {
		return "", nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(levels)), ",")
	args := make([]any, len(levels))
	for i, level := range levels {
		args[i] = string(level)
	}
	return " AND l.level IN (" + placeholders + ")", args
}

// ftsPhrase 把查询包成 FTS5 短语，内部双引号按 FTS5 约定转义。
func ftsPhrase(query string) string {
	return `"` + strings.ReplaceAll(query, `"`, `""`) + `"`
}

// likePattern 转义 LIKE 元字符并加通配符。
func likePattern(text string) string {
	escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(text)
	return "%" + escaped + "%"
}

// guidingCaseNumberRe 匹配「指导案例第 12 号」「指导性案例12号」。
var guidingCaseNumberRe = regexp.MustCompile(`指导(?:性)?案例\s*第?\s*([0-9]+|[〇零一二两三四五六七八九十百千]+)\s*号`)

// bareCaseNumberRe 匹配只给编号的查询「第 12 号」。
var bareCaseNumberRe = regexp.MustCompile(`^第?\s*([0-9]+|[〇零一二两三四五六七八九十百千]+)\s*号$`)

// parseGuidingCaseNumber 从查询中解析指导案例编号（支持阿拉伯数字与中文数字）。
func parseGuidingCaseNumber(query string) (int, bool) {
	trimmed := strings.TrimSpace(query)
	var raw string
	if m := guidingCaseNumberRe.FindStringSubmatch(trimmed); m != nil {
		raw = m[1]
	} else if m := bareCaseNumberRe.FindStringSubmatch(trimmed); m != nil {
		raw = m[1]
	} else {
		return 0, false
	}
	if n, err := strconv.Atoi(raw); err == nil {
		return n, n > 0
	}
	n, ok := lawcite.ParseChineseNumber(raw)
	return n, ok && n > 0
}
