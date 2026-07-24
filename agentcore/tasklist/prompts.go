package tasklist

// 双语工具描述提示词。遵循 docs/tone-style-guide.md：
// - 不使用绝对化表述
// - 提供清晰的何时用/何时不用指南
// - 中英文对等翻译

const taskCreateDesc = `Use this tool to create a structured task list for tracking progress on complex work.

## When to Use

- Complex multi-step tasks requiring 3 or more distinct steps
- Tasks requiring careful planning or multiple operations
- When the user provides multiple tasks (numbered or comma-separated)
- After receiving new instructions — capture requirements as tasks immediately
- When starting work on a task — mark it in_progress BEFORE beginning

## When NOT to Use

Skip this tool when:
- There is only a single, straightforward task
- The task is trivial and tracking provides no benefit
- The task is purely conversational or informational

## Task Fields

- **subject**: A brief, actionable title in imperative form (e.g., "Search prior art for claim 1")
- **description**: Detailed description including context and acceptance criteria
- **priority**: "urgent", "high", "normal" (default), or "low"
- **active_form**: Present continuous form shown in progress indicator when in_progress (e.g., "Searching prior art")

All tasks are created with status "pending". After creating tasks, use TaskUpdate to set up dependencies (blocks/blocked_by) if needed.`

//nolint:unused // 预留供工具描述使用
const taskCreateDescChinese = `使用此工具创建结构化的任务列表，用于追踪复杂工作的进度。

## 何时使用

- 复杂的多步骤任务，需要 3 个或以上不同步骤
- 需要仔细规划或多个操作的任务
- 用户提供多个任务时（编号或逗号分隔）
- 收到新指令后——立即将需求记录为任务
- 开始处理任务时——在开始工作前将其标记为 in_progress

## 何时不使用

在以下情况跳过：
- 只有一个简单的任务
- 任务很简单，追踪没有意义
- 纯对话或信息查询

## 任务字段

- **subject**：简短的可操作标题，使用祈使句（如"检索权利要求1的现有技术"）
- **description**：详细描述，包括上下文和验收标准
- **priority**："urgent"、"high"、"normal"（默认）或 "low"
- **active_form**：in_progress 状态时在进度指示器中显示的进行时文案（如"正在检索现有技术"）

所有任务创建时状态为 "pending"。创建后如需设置依赖关系（blocks/blocked_by），请使用 TaskUpdate。`

const taskUpdateDesc = `Use this tool to update a task in the task list.

## When to Use

**Mark progress:**
- When you start work: set status to "in_progress"
- When you complete work: set status to "completed"
- Keep tasks as in_progress if you encounter errors or blockers
- Do NOT mark completed if: tests are failing, implementation is partial, or errors remain

**Archive tasks:**
- Setting status to "archived" removes the task from the active list but retains it for audit
- Use for tasks that are no longer relevant or were created in error

**Set up dependencies:**
- addBlocks: tasks that cannot start until this one completes
- addBlockedBy: tasks that must complete before this one can start

## Fields

- **status**: "pending", "in_progress", "completed", or "archived"
- **priority**: Change task priority ("urgent", "high", "normal", "low")
- **addBlocks**: Task IDs this task blocks
- **addBlockedBy**: Task IDs that block this task
- **owner**: Agent name claiming this task

## Status Workflow

pending → in_progress → completed

Use "archived" to remove from active list while preserving audit history.`

//nolint:unused // 预留供工具描述使用
const taskUpdateDescChinese = `使用此工具更新任务列表中的任务。

## 何时使用

**标记进度：**
- 开始工作时：将状态设为 "in_progress"
- 完成工作时：将状态设为 "completed"
- 遇到错误或阻塞时，保持任务为 in_progress
- 在以下情况下不要标记为已完成：测试失败、实现不完整、仍有未解决错误

**归档任务：**
- 将状态设为 "archived" 会从活动列表中移除，但保留审计记录
- 用于不再相关或创建错误的任务

**设置依赖：**
- addBlocks：在此任务完成前不能开始的任务
- addBlockedBy：必须在本任务开始前完成的任务

## 字段

- **status**："pending"、"in_progress"、"completed" 或 "archived"
- **priority**：更改任务优先级（"urgent"、"high"、"normal"、"low"）
- **addBlocks**：本任务阻塞的任务 ID
- **addBlockedBy**：阻塞本任务的任务 ID
- **owner**：认领该任务的 Agent 名称

## 状态流程

pending → in_progress → completed

使用 "archived" 从活动列表移除，同时保留审计历史。`

const taskGetDesc = `Use this tool to retrieve a task by its ID from the task list.

## When to Use

- Before starting work on a task, to get full description and context
- To check task dependencies (what it blocks, what blocks it)
- After being assigned a task, to get complete requirements`

//nolint:unused // 预留供工具描述使用
const taskGetDescChinese = `使用此工具通过 ID 从任务列表中获取单个任务。

## 何时使用

- 开始处理任务前，获取完整描述和上下文
- 查看任务依赖关系（它阻塞什么，什么阻塞它）
- 被分配任务后，获取完整需求`

const taskListDesc = `Use this tool to list all tasks in the task list.

## When to Use

- To see available tasks (status: pending, no owner, not blocked)
- To check overall progress
- After completing a task, to find the next available work

Tasks are sorted by priority (urgent > high > normal > low), then by ID.
Archived tasks are excluded by default; set include_archived=true to see them.

## Output

Each task shows: ID, status, subject, priority, and dependency info.`

//nolint:unused // 预留供工具描述使用
const taskListDescChinese = `使用此工具列出任务列表中的所有任务。

## 何时使用

- 查看可处理的任务（状态为 pending、无所有者、未被阻塞）
- 检查整体进度
- 完成任务后查找下一个可用工作

任务按优先级（urgent > high > normal > low）然后按 ID 排序。
默认排除归档任务；设置 include_archived=true 可查看归档任务。

## 输出

每个任务显示：ID、状态、标题、优先级和依赖信息。`
