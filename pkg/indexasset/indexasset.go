// Package indexasset 提供随包专利事实索引（法条、费用）共用的资产格式。
//
// 法条索引与费用索引都是手工转录的内容，字段类型错误是部署错误而非可强转的输入：
// 每个读取器要么返回资产声明的值，要么带着字段名拒绝整份文件。因此读取器接受调用方
// 的错误工厂，让每个索引包保留自己可捕获的错误类型，同时共用一套字段规则。
//
// 本包是纯库：不注册插件、不持有状态、不做 I/O。
package indexasset

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Error 是随包索引资产加载失败的错误。
//
// Origin 是出错的文件路径（调用方未提供时为空），Message 描述具体字段问题。
type Error struct {
	// Origin 是资产文件的路径，供调用方定位。
	Origin string
	// Message 是对字段或结构的可读描述。
	Message string
}

// Error 返回带文件路径的错误文本。
func (e *Error) Error() string {
	if e.Origin == "" {
		return "indexasset: " + e.Message
	}
	return fmt.Sprintf("indexasset %s: %s", e.Origin, e.Message)
}

// Fail 是绑定到一份资产的错误工厂。
//
// 读取器接受它而不是固定的错误类型，使各索引包能保留自己的可捕获错误类型。
type Fail func(message string) error

// FailFor 返回绑定到指定文件路径的错误工厂。
func FailFor(origin string) Fail {
	return func(message string) error {
		return &Error{Origin: origin, Message: message}
	}
}

// ParseYAMLMapping 把资产文本解析为根映射。
//
// label 用于根节点类型错误的消息（例如「费用索引的根节点必须是映射」）。
func ParseYAMLMapping(source []byte, label string, fail Fail) (map[string]any, error) {
	var value any
	if err := yaml.Unmarshal(source, &value); err != nil {
		return nil, fail(fmt.Sprintf("YAML 解析失败：%v", err))
	}
	root, ok := value.(map[string]any)
	if !ok {
		return nil, fail(fmt.Sprintf("%s的根节点必须是映射", label))
	}
	return root, nil
}

// RequireKnownFields 拒绝 root 中不在 allowed 里的字段。
//
// 资产是手工转录的，多写一个字段多半意味着抄错了位置或用了旧版本格式；
// 静默忽略会让该字段既不生效也无告警，故按未知字段拒绝整份文件。
func RequireKnownFields(root map[string]any, allowed []string, where string, fail Fail) error {
	unknown := make([]string, 0)
	for field := range root {
		if !slices.Contains(allowed, field) {
			unknown = append(unknown, field)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	slices.Sort(unknown)
	return fail(fmt.Sprintf("%s 含未知字段：%s", where, strings.Join(unknown, ", ")))
}

// ReadMapping 要求 value 是映射。
func ReadMapping(value any, where string, fail Fail) (map[string]any, error) {
	mapping, ok := value.(map[string]any)
	if !ok {
		return nil, fail(fmt.Sprintf("%s 必须是映射", where))
	}
	return mapping, nil
}

// ReadString 读取必填的非空字符串字段。
func ReadString(root map[string]any, field string, fail Fail) (string, error) {
	value, ok := root[field]
	if !ok {
		return "", fail(fmt.Sprintf("字段 %s 必须是非空字符串", field))
	}
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return "", fail(fmt.Sprintf("字段 %s 必须是非空字符串", field))
	}
	return text, nil
}

// ReadOptionalString 读取可为缺失或 null 的字符串字段。
//
// 字段缺失与显式 null 都返回 nil；空字符串视为类型错误。
func ReadOptionalString(root map[string]any, field string, fail Fail) (*string, error) {
	value, ok := root[field]
	if !ok || value == nil {
		return nil, nil
	}
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return nil, fail(fmt.Sprintf("字段 %s 必须是 null 或非空字符串", field))
	}
	return &text, nil
}

// ReadOptionalDate 读取可为缺失或 null 的 ISO 日期字段（YYYY-MM-DD）。
//
// yaml.v3 会把未加引号的 2026-09-26 解析成 time.Time，引号形式则是 string；
// 随包资产两种写法都存在，故两者都接受并统一成 YYYY-MM-DD 文本。
// 除格式外还验证日期真实存在（拒绝 2026-13-45 这类正则能通过的写法）。
func ReadOptionalDate(root map[string]any, field string, fail Fail) (*string, error) {
	value, ok := root[field]
	if !ok || value == nil {
		return nil, nil
	}
	var text string
	switch v := value.(type) {
	case string:
		text = v
	case time.Time:
		text = v.Format("2006-01-02")
	default:
		return nil, fail(fmt.Sprintf("字段 %s 必须是 null 或 YYYY-MM-DD", field))
	}
	if !isoDatePattern.MatchString(text) {
		return nil, fail(fmt.Sprintf("字段 %s 必须是 null 或 YYYY-MM-DD", field))
	}
	if _, err := time.Parse("2006-01-02", text); err != nil {
		return nil, fail(fmt.Sprintf("字段 %s 不是有效日期：%s", field, text))
	}
	return &text, nil
}

// ReadCount 要求 value 是正整数。
func ReadCount(value any, where string, fail Fail) (int, error) {
	count, ok := value.(int)
	if !ok || count < 1 {
		return 0, fail(fmt.Sprintf("%s 必须是正整数", where))
	}
	return count, nil
}

// ReadOptionalCount 读取可为缺失或 null 的正整数字段。
func ReadOptionalCount(root map[string]any, field string, fail Fail) (*int, error) {
	value, ok := root[field]
	if !ok || value == nil {
		return nil, nil
	}
	count, err := ReadCount(value, "字段 "+field, fail)
	if err != nil {
		return nil, err
	}
	return &count, nil
}

// ReadOptionalBoolean 读取可为缺失或 null 的布尔字段。
func ReadOptionalBoolean(root map[string]any, field string, fail Fail) (*bool, error) {
	value, ok := root[field]
	if !ok || value == nil {
		return nil, nil
	}
	flag, ok := value.(bool)
	if !ok {
		return nil, fail(fmt.Sprintf("字段 %s 必须是 null 或布尔值", field))
	}
	return &flag, nil
}

// ReadEnum 要求 value 是 allowed 中的一个取值。
func ReadEnum(value any, allowed []string, where string, fail Fail) (string, error) {
	accepted := strings.Join(allowed, " / ")
	text, ok := value.(string)
	if !ok {
		return "", fail(fmt.Sprintf("%s 必须是 %s 之一，得到：%v", where, accepted, value))
	}
	if !slices.Contains(allowed, text) {
		return "", fail(fmt.Sprintf("%s 必须是 %s 之一，得到：%s", where, accepted, text))
	}
	return text, nil
}

// RecordedSource 是单条转录内容记录的来源与核验日期。
//
// 两个字段在有条文/金额转录之前都为 nil，让消费方区分「已转录」与「仅有骨架」。
type RecordedSource struct {
	// SourceDoc 是内容抄录所依据的官方文书。
	SourceDoc *string
	// VerifiedOn 是人工对照官方文书核验的日期。
	VerifiedOn *string
}

// ReadRecordedSource 读取单条记录的来源字段。
func ReadRecordedSource(entry map[string]any, fail Fail) (RecordedSource, error) {
	sourceDoc, err := ReadOptionalString(entry, "sourceDoc", fail)
	if err != nil {
		return RecordedSource{}, err
	}
	verifiedOn, err := ReadOptionalDate(entry, "verifiedOn", fail)
	if err != nil {
		return RecordedSource{}, err
	}
	return RecordedSource{SourceDoc: sourceDoc, VerifiedOn: verifiedOn}, nil
}

// CheckRecordedSource 要求条目的转录记录是全有或全无的。
//
// text 已转录却没有来源或核验日期时，该条会被报成「已核验」而副本没有记录来源；
// 只有来源却没有条文时，又声明了一份不存在的转录。两者都是加载错误，避免手工编辑
// 的索引把核验器设计上要避免的 fail-open 状态造出来。
func CheckRecordedSource(text *string, source RecordedSource, where string, fail Fail) error {
	transcribed := text != nil
	recorded := source.SourceDoc != nil || source.VerifiedOn != nil
	complete := source.SourceDoc != nil && source.VerifiedOn != nil

	if transcribed && !complete {
		return fail(fmt.Sprintf("%s 的 text 已转录，但 sourceDoc 与 verifiedOn 必须同时记录", where))
	}
	if !transcribed && recorded {
		return fail(fmt.Sprintf("%s 尚未转录 text，不能单独记录 sourceDoc 或 verifiedOn", where))
	}
	return nil
}

// IndexedArticle 是法条索引中的一条。
type IndexedArticle struct {
	// Article 是条号。
	Article int
	// Paragraphs 是已知存在的款号；未核验时为空。
	Paragraphs []int
	// Topics 是命题匹配用的主题词，不是条文原文。
	Topics []string
	// Text 是条文原文；尚未转录时为空。
	Text *string
	// RecordedSource 是该条的来源与核验日期。
	RecordedSource
}

// ReadEntryTopics 读取一条记录的主题词列表。
func ReadEntryTopics(value any, where string, fail Fail) ([]string, error) {
	items, ok := value.([]any)
	if !ok || len(items) == 0 {
		return nil, fail(fmt.Sprintf("%s 的 topics 必须是非空数组", where))
	}
	topics := make([]string, 0, len(items))
	for _, item := range items {
		topic, ok := item.(string)
		if !ok || strings.TrimSpace(topic) == "" {
			return nil, fail(fmt.Sprintf("%s 的 topics 只能包含非空字符串", where))
		}
		topics = append(topics, topic)
	}
	return topics, nil
}

// ReadArticleEntries 读取 articles 列表，并拒绝重复条号。
func ReadArticleEntries(value any, fail Fail) ([]IndexedArticle, error) {
	if value == nil {
		return nil, nil
	}
	items, ok := value.([]any)
	if !ok {
		return nil, fail("字段 articles 必须是数组")
	}
	seen := make(map[int]struct{}, len(items))
	articles := make([]IndexedArticle, 0, len(items))
	for _, item := range items {
		entry, err := ReadMapping(item, "articles 的每一项", fail)
		if err != nil {
			return nil, err
		}
		article, err := ReadOptionalCount(entry, "article", fail)
		if err != nil {
			return nil, err
		}
		if article == nil {
			return nil, fail("article 必须是正整数")
		}
		if _, exists := seen[*article]; exists {
			return nil, fail(fmt.Sprintf("article 重复：%d", *article))
		}
		seen[*article] = struct{}{}

		where := fmt.Sprintf("第%d条", *article)
		text, err := ReadOptionalString(entry, "text", fail)
		if err != nil {
			return nil, err
		}
		source, err := ReadRecordedSource(entry, fail)
		if err != nil {
			return nil, err
		}
		if err := CheckRecordedSource(text, source, where, fail); err != nil {
			return nil, err
		}
		paragraphs, err := readParagraphs(entry["paragraphs"], where, fail)
		if err != nil {
			return nil, err
		}
		topics, err := ReadEntryTopics(entry["topics"], where, fail)
		if err != nil {
			return nil, err
		}
		articles = append(articles, IndexedArticle{
			Article:        *article,
			Paragraphs:     paragraphs,
			Topics:         topics,
			Text:           text,
			RecordedSource: source,
		})
	}
	return articles, nil
}

// readParagraphs 读取一条记录的款项列表。
func readParagraphs(value any, where string, fail Fail) ([]int, error) {
	if value == nil {
		return nil, nil
	}
	items, ok := value.([]any)
	if !ok || len(items) == 0 {
		return nil, fail(fmt.Sprintf("%s的 paragraphs 必须是 null 或非空数组", where))
	}
	paragraphs := make([]int, 0, len(items))
	for _, item := range items {
		number, ok := item.(int)
		if !ok || number < 1 {
			return nil, fail(fmt.Sprintf("%s的 paragraphs 只能是正整数", where))
		}
		paragraphs = append(paragraphs, number)
	}
	return paragraphs, nil
}

var isoDatePattern = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)
