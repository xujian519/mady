// a2ui-demo 演示如何使用 A2UI Builder API 构造声明式 Agent UI 信封，
// 并通过 AG-UI CUSTOM 事件将界面推送给前端渲染器。
//
// 本示例展示了：
//   - 用 Builder API 创建表面（createSurface）
//   - 添加各种组件（Text, Button, Column, List, Tabs 等）
//   - 使用数据绑定（Dynamic.Bind）将组件属性绑定到数据模型
//   - 更新数据模型（updateDataModel）
//   - 将 A2UI 信封转换为 AG-UI CUSTOM 事件
//   - 将 A2UI 信封转换为 A2A Data Part
package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/xujian519/mady/a2a"
	"github.com/xujian519/mady/a2ui"
	"github.com/xujian519/mady/agui"
)

func main() {
	// 1. 使用 Builder API 构建一个简单的用户资料界面
	fmt.Println("=== 示例 1: Builder API - 用户资料界面 ===")
	demoProfile()
	fmt.Println()

	// 2. 使用数据绑定的动态列表
	fmt.Println("=== 示例 2: 数据绑定 - 任务列表 ===")
	demoTaskList()
	fmt.Println()

	// 3. 转换为 AG-UI CUSTOM 事件
	fmt.Println("=== 示例 3: 转换为 AG-UI CUSTOM 事件 ===")
	demoAGUIBinding()
	fmt.Println()

	// 4. 转换为 A2A Data Part
	fmt.Println("=== 示例 4: 转换为 A2A Data Part ===")
	demoA2ABinding()
	fmt.Println()
}

// demoProfile 演示如何创建一个用户资料编辑界面。
func demoProfile() {
	envs := a2ui.NewSurface("profile", a2ui.BasicCatalogID).
		Theme(map[string]any{"primaryColor": "#0066CC"}).
		Add(
			a2ui.Column("root",
				"header", "name", "email", "saveBtn",
			),
			a2ui.Text("header", "用户资料编辑"),
			a2ui.TextField("name", "/user/name"),
			a2ui.TextField("email", "/user/email"),
			a2ui.Button("saveBtn", "保存", a2ui.EventAction("save_profile", nil)),
		).
		Data("/user/name", "张三").
		Data("/user/email", "zhangsan@example.com").
		Build()

	printEnvelopes(envs)
}

// demoTaskList 演示数据绑定和模板化子节点。
func demoTaskList() {
	// 创建任务 item 模板组件
	taskTemplate := a2ui.NewComponent("taskTmpl", "Row", map[string]any{
		"children": a2ui.StaticChildren("checkbox", "label"),
	})
	_ = taskTemplate // 模板由服务端保留，实际会被渲染引擎实例化

	envs := a2ui.NewSurface("tasks", a2ui.BasicCatalogID).
		Add(
			a2ui.Text("title", "我的任务"),
			a2ui.TemplateList("taskList", "/tasks", "taskTmpl"),
			// 绑定到数据模型路径的按钮：提交所有任务
			a2ui.Button("submit", "提交", a2ui.EventAction("submit_tasks", map[string]any{
				"source": a2ui.Bind("/tasks"),
			})),
		).
		Data("/tasks", []map[string]any{
			{"done": false, "text": "完成报告"},
			{"done": true, "text": "回邮件"},
			{"done": false, "text": "准备会议材料"},
		}).
		Build()

	printEnvelopes(envs)
}

// demoAGUIBinding 演示如何将 A2UI 信封包装为 AG-UI CUSTOM 事件。
// 这是 A2UI 在 Mady 中主要的传输方式。
func demoAGUIBinding() {
	env := a2ui.NewUpdateComponents("profile",
		a2ui.Text("status", "数据已保存"),
	)

	// 转换为 AG-UI CustomEvent
	customEv := a2ui.ToCustomEvent(env)
	fmt.Printf("AG-UI 事件名称: %s\n", customEv.Name)
	fmt.Printf("AG-UI 事件值类型: %T\n", customEv.Value)

	data, _ := json.MarshalIndent(customEv, "", "  ")
	fmt.Println("AG-UI CustomEvent JSON:")
	fmt.Println(string(data))

	// 从 AG-UI CustomEvent 恢复
	recovered, ok, err := a2ui.FromCustomEvent(customEv)
	if err != nil {
		log.Fatalf("恢复失败: %v", err)
	}
	fmt.Printf("恢复成功: %v, 类型: %s\n", ok, recovered.Kind())
}

// demoA2ABinding 演示如何将 A2UI 信封包装为 A2A Data Part。
func demoA2ABinding() {
	env := a2ui.NewDeleteSurface("profile")

	part, err := a2ui.EnvelopeToDataPart(env)
	if err != nil {
		log.Fatalf("创建 A2A Data Part 失败: %v", err)
	}

	fmt.Printf("A2A Part 类型: %s\n", part.Type)
	fmt.Printf("A2A Data MIME: %s\n", part.Data.MIMEType)
	fmt.Printf("A2A Data: %+v\n", part.Data.Data)

	// 包装为 A2A Message
	msg, err := a2ui.EnvelopesToMessage(string(a2a.RoleAgent), []a2ui.Envelope{env})
	if err != nil {
		log.Fatalf("创建 A2A Message 失败: %v", err)
	}
	fmt.Printf("A2A Message 角色: %s, Parts 数: %d\n", msg.Role, len(msg.Parts))
}

// printEnvelopes 以 JSON 格式打印 A2UI 信封序列。
func printEnvelopes(envs []a2ui.Envelope) {
	for i, env := range envs {
		// 标记忽略的字段（version 由 marshal 自动填充）
		_ = i

		data, err := json.MarshalIndent(env, "", "  ")
		if err != nil {
			log.Fatalf("信封序列化失败: %v", err)
		}
		fmt.Printf("信封 #%d (%s):\n%s\n", i, env.Kind(), string(data))

		// 验证信封可以往返解析
		parsed, err := a2ui.ParseEnvelope(data)
		if err != nil {
			log.Fatalf("信封解析失败: %v", err)
		}
		if parsed.Kind() != env.Kind() {
			log.Fatalf("类型不匹配: %v != %v", parsed.Kind(), env.Kind())
		}
	}
}

// 确保 agui.CustomEvent 被引用（绑定_agui.go 的 ToCustomEvent 已使用）
var _ agui.CustomEvent
