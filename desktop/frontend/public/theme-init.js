/**
 * 三态主题防 FOUC 初始化脚本（W4-T7）。
 *
 * 在 React 挂载前根据 localStorage（mady-theme-mode）与系统
 * prefers-color-scheme 设置 <html>.dark class，避免深色模式用户
 * 首帧闪烁为浅色。逻辑与 src/theme/provider.tsx 的
 * loadMode / resolveMode 保持一致；改动需同步两处。
 *
 * 作为独立文件置于 public/ 下（CSP script-src 'self' 允许），
 * 由 index.html 在模块脚本前加载。
 */
(function () {
  try {
    var mode = localStorage.getItem('mady-theme-mode')
    var dark =
      mode === 'dark' ||
      (mode !== 'light' &&
        window.matchMedia('(prefers-color-scheme: dark)').matches)
    var root = document.documentElement
    if (dark) root.classList.add('dark')
    root.setAttribute('data-theme', dark ? 'dark' : 'light')
  } catch (e) {
    // localStorage 不可用（隐私模式等）时静默跳过，由 provider 默认 system 处理
  }
})()
