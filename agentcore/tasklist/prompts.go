package tasklist

// Tool description prompts. Follow docs/tone-style-guide.md:
// - No absolute claims
// - Clear when-to-use / when-not-to-use guidance
//
// Chinese variants are intentionally omitted: the LLM tool-description path
// has no i18n selector, and these strings are sent to the model as-is.
// The prompts themselves are self-documenting enough for bilingual models.

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

const taskGetDesc = `Use this tool to retrieve a task by its ID from the task list.

## When to Use

- Before starting work on a task, to get full description and context
- To check task dependencies (what it blocks, what blocks it)
- After being assigned a task, to get complete requirements`

const taskListDesc = `Use this tool to list all tasks in the task list.

## When to Use

- To see available tasks (status: pending, no owner, not blocked)
- To check overall progress
- After completing a task, to find the next available work

Tasks are sorted by priority (urgent > high > normal > low), then by ID.
Archived tasks are excluded by default; set include_archived=true to see them.

## Output

Each task shows: ID, status, subject, priority, and dependency info.`
