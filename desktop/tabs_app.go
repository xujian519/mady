//go:build darwin

package main

// tabs_app.go — 会话标签绑定方法（阶段 2.1：Go 侧 tabs 状态机）。
//
// 前端 TabBar 通过这些绑定驱动标签生命周期；标签数据在 ~/.mady/desktop-tabs.json
// 持久化（见 tabs.go）。「绑定方法按 tab 分派」（Chat/GetThread 等 ForTab 化）
// 在后续子步落地——当前标签的 ThreadID 关联由前端在新会话建立时写入。

// ListTabs 返回全部会话标签（含激活标签，TabBar 渲染用）。
func (a *App) ListTabs() ([]Tab, error) {
	if a.tabs == nil {
		return nil, errTabsNotReady
	}
	return a.tabs.List(), nil
}

// ActiveTabID 返回当前激活标签 ID（"" = 无标签，异常兜底）。
func (a *App) ActiveTabID() (string, error) {
	if a.tabs == nil {
		return "", errTabsNotReady
	}
	return a.tabs.ActiveID(), nil
}

// CreateTab 新建一个会话标签并设为激活；返回新标签。
func (a *App) CreateTab() (Tab, error) {
	if a.tabs == nil {
		return Tab{}, errTabsNotReady
	}
	return a.tabs.Create(), nil
}

// CloseTab 关闭指定标签；关闭激活标签时激活相邻标签（最后一个标签不可关闭）。
func (a *App) CloseTab(id string) error {
	if a.tabs == nil {
		return errTabsNotReady
	}
	return a.tabs.Close(id)
}

// ActivateTab 激活指定标签（前端点击 TabBar 时调用）。
func (a *App) ActivateTab(id string) error {
	if a.tabs == nil {
		return errTabsNotReady
	}
	return a.tabs.Activate(id)
}
