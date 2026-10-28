# 领域索引资产格式与目录约定

> 适用范围：随包分发的专利/法律事实索引（法条、费用、工作日历、规则包）。
> 实现基座：`pkg/indexasset`（T-W0-01）。
> 配套：`docs/design/fail-closed-matrix.md`（拒绝语义）、`scripts/asset-fingerprint`（漂移校验）。

## 1. 两类资产，两种分发方式

| 类别 | 判据 | 存放位置 | 加载方式 | 例子 |
|---|---|---|---|---|
| **随包资产** | 小（< 1 MB）、必需、版本随代码 | 消费模块内 `data/` | `//go:embed` | 专利法精校索引（82 条范围内已转录的 37 条）、实施条例 45 条、审查指南 68 节点、费用索引、2025–2026 节假日 |
| **外置资产** | 大、可更新、用户可覆盖 | `$MADY_HOME/knowledge/` 或仓库根 `data/` | 运行时路径解析 | 7,910 份法规条号骨架、knowledge.db、laws-full.db |

**判据的解释**：
- 「必需」指缺失会导致对应工具不可用（法条核验、费用测算、期限顺延）。
- 「可更新」指数据由外部权威按年/按版本发布（法规、判例），代码发布节奏不应绑定其更新。
- 外置资产必须有明确的缺失行为（见 fail-closed 矩阵），不得静默当作空集。

## 2. 目录布局

```
pkg/lawcite/data/law/
  cn-patent-law.yaml            # 专利法（精校子集）
  cn-implementing-regulations.yaml
  cn-examination-guidelines.yaml
  cn-patent-law-lineage.yaml    # 修订沿革

domains/fees/data/
  cn-fees.yaml

domains/deadline/data/
  cn-holidays.yaml

data/law/skeleton/              # 外置：7,910 份条号骨架（运行时加载，可覆盖）
data/ASSETS.sha256              # 随包资产指纹清单
```

规则：
1. 随包资产只在**消费它的 Go 包**下，便于 `go:embed` 与责任归属。
2. 跨模块共享且体积大的骨架走外置，路径由消费方配置，不写死。
3. 新增资产目录时必须同步更新本文档与指纹清单。

## 3. 文件结构

每个资产文件是一个 YAML 映射，根节点必须为映射。公共信封字段：

```yaml
document: 中华人民共和国专利法        # 必填：资产对应文书
revision: 2020 年第四次修正           # 必填：版本标识
maxArticle: 82                       # 可选：最大条号（超出判 OutOfRange）
maxVerifiedOn: 2026-09-26            # 可选：该法族最近核验日期
maxSource: 官方全文                   # 可选：最近核验依据
verifiedOn: 2026-09-26               # 可选：本文件整体核验日期
```

条目按资产类型追加（法条用 `articles`，费用用 `items`，日历用 `years`）。

## 4. 字段读取规则

由 `pkg/indexasset` 统一执行，任何类型错误都**拒绝整份文件**，不做强制转换：

| 读取器 | 接受 | 拒绝 |
|---|---|---|
| `ReadString` | 非空字符串 | 缺失、null、空串/纯空白、数字 |
| `ReadOptionalString` | 缺失/null → nil；非空字符串 | 空串、数字 |
| `ReadOptionalDate` | 缺失/null → nil；`YYYY-MM-DD` 字符串或 yaml 时间戳 | 格式不符、不存在的日期（2026-13-45）、数字 |
| `ReadCount` | 正整数 | 0、负数、小数、字符串 |
| `ReadOptionalBoolean` | 缺失/null → nil；bool | 字符串 `"true"` |
| `ReadEnum` | 闭集内的字符串 | 集外值、非字符串 |
| `ReadEntryTopics` | 非空数组，元素非空字符串 | 空数组、数字元素 |
| `ReadArticleEntries` | 条号唯一、`paragraphs` 为 null 或非空正整数数组 | 条号重复/为零、空数组、非整数 |

**为什么拒绝而不是转换**：资产是手工转录的权威事实，类型错误意味着抄错了字段，
静默转换会让错误以"看起来正常"的数值进入法律结论。

## 5. 来源与核验语义（关键）

每条转录内容携带三态：

| text | sourceDoc / verifiedOn | 含义 | 允许 |
|---|---|---|---|
| 有 | 都有 | **已核验**：可报 `Verified` | ✅ |
| 有 | 缺任一 | 声明了转录却没有来源 | ❌ 加载失败 |
| 无 | 都无 | **仅骨架**：只能报 `ExistOnly` | ✅ |
| 无 | 有任一 | 有来源却没有转录 | ❌ 加载失败 |

`CheckRecordedSource` 强制「全有或全无」。这条规则是引用核验 fail-closed 的基础：
加载期就杜绝「有条文、无来源、仍被判为已核验」的状态。

**核验日期是人工日期**：`verifiedOn` 记录的是某人对照 `sourceDoc` 核对过的日期，
不是文件修改时间，也不是抓取时间。自动化流程不得代填。

## 6. 加载与错误

```go
fail := indexasset.FailFor(path)
root, err := indexasset.ParseYAMLMapping(source, "专利法索引", fail)
```

- 错误类型为 `*indexasset.Error`，携带 `Origin`（文件路径），可用 `errors.As` 判定。
- 错误消息为中文，包含字段名或条号，避免"某处格式错误"这类无法定位的提示。
- 一个文件解析失败即整体失败，不返回部分结果。

## 7. 资产指纹

```bash
scripts/asset-fingerprint           # 生成 data/ASSETS.sha256
scripts/asset-fingerprint --check   # 校验（CI 用）
```

用途：随包资产是从官方文书逐字转录的，手改会导致与来源漂移。指纹清单在 CI 中校验，
任何资产变更都必须同步更新清单，并在 PR 说明中给出变更依据（文号/版本/核验日期）。

## 8. 新增资产检查清单

- [ ] 根节点映射，公共信封字段齐全（`document`、`revision`）
- [ ] 每条转录内容都填 `sourceDoc` + `verifiedOn`，或两者都留空
- [ ] 未转录条目有 `topics`（否则无法用于命题匹配）
- [ ] 资产加载测试使用 `pkg/indexasset` 读取，不自定义解析
- [ ] 更新 `data/ASSETS.sha256`
- [ ] 更新本文档第 2 节目录布局
