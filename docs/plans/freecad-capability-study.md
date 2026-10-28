# FreeCAD 能力边界研究（T-R-01）

> 任务来源：`docs/plans/dsh-domain-import-tasks.md` 的 W-1 / T-R-01
> 目的：判断 dsh 的 FreeCAD 结构图能力是否值得移植进 Mady，以及移植前必须满足什么条件
> 研究日期：2026-10-28
> 研究方式：阅读 dsh `patent-tools/src/figure/freecad-*.ts` 与 `scripts/prepare-ci-cad.sh`、
> 本机环境探测、既有技术债记录；**未做安装验证**，结论中的安装路径为候选方案

---

## 一、结论

**建议：暂不立项 Go 版结构图移植。** 若确有结构图需求，先做「外部 FreeCAD + 复用 dsh 的 Python 脚本」
的最小验证（1–2 人日），验证通过后再决定是否值得重写为 Go 模块。

理由：
1. 能力依赖 FreeCAD **1.1 的实测 API**（`TechDraw.viewPartAsSvg`、`Shape.distToShape` 等），
   该 API 在 0.21 及更早版本不存在；版本错配会直接失败而非降级。
2. CI 只有 **Linux x86_64** 的非交互安装路径（pinned AppImage 1.1.4 解包，无 FUSE），
   macOS 没有经过验证的等价路径。
3. 本机 `freecadcmd` 缺失，`inkscape` 在沙箱内执行受限，无法在当前环境完成端到端验证。
4. dsh 侧该能力**默认关闭**（`structureFigureEnabled=true` 才启用），未安装时返回 `setup_required`，
   说明上游自己也不把它当作开箱能力。

---

## 二、FreeCAD 在 dsh 中的三种用途

| 用途 | 入口 | 依赖 API | 产物 |
|---|---|---|---|
| TechDraw 结构线稿 | `buildStructureScript` → `freecadcmd script.py` | `Part.read`、`Shape.distToShape`、`TechDraw::DrawViewPart`、`TechDraw.viewPartAsSvg`、`DrawViewPart.projectPoint` | 多视图黑白线稿 SVG + `manifest.json` |
| 剖切几何 | `buildSectionScript` | `Part.read`、`Shape.slice`、`Part.Face`、`Part.makeFace` | `section-geometry.json`（环点列） |
| 剖面线 | `buildHatchScript` | `TechDraw.makeGeomHatch`、`Part.Face` | `section-hatch.json` + `.pat` |

能力范围（来自脚本注释与代码）：
- 装配体：多个 STEP 文件作为独立 `Part::Feature` 挂到同一视图，一次 HLR 处理全部零件；
- 件号锚点：按零件真实几何核对，容差 `STRUCTURE_ANCHOR_TOLERANCE_MM`；
- 局部放大：`TechDraw::DrawViewDetail` + 自绘「放大部位」标记圆（`ShowHighlight` 不进 SVG）；
- 剖切：不建 Document/Page/模板，纯几何运算；
- 剖面线：与剖切轮廓共用同一离散化点列，避免「按精确曲线裁剪、画的是内接多边形」地伸出。

**明确不做**：模板边框、标题栏、图号（片段天然满足审查指南 4.3 的干净底图要求）。

---

## 三、版本与平台约束

| 约束 | 证据 | 影响 |
|---|---|---|
| 需要 FreeCAD **1.1.x** | `prepare-ci-cad.sh`：Ubuntu 包 0.21 低于 1.1 API，改用 1.1.4 AppImage | 包管理器默认版本不可用 |
| CI 安装仅 Linux x86_64 | 脚本显式检查 `uname -s`/`uname -m` 并退出 | macOS 需另寻路径 |
| 解包而非挂载 AppImage | 无需 FUSE；AppRun 携带 `PYTHONHOME` | 直接执行 `usr/bin/freecadcmd` 缺环境 |
| 冷启动慢 | `freecadRenderTimeoutMs` 默认 120000ms | 单图 2 分钟级，交互体验差 |
| 默认关闭 | `structureFigureEnabled` 默认 false，工具返回 `setup_required` | 部署需显式开启 |

---

## 四、已知失败模式（移植前必须复现）

1. **空结果静默**：平面位于实体之外时 `Shape.slice` 返回空列表，既不抛异常也非 Null；
   脚本必须显式判环数为 0 并报错。
2. **`isNull()` 不可靠**：不相交布尔的返回是 `Compound`、`isNull()` 为 False、`Solids` 为 0。
3. **异常类型穿透**：`Part.makeFace([])` 抛 `Base.FreeCADError`，不是 Python `Exception`，
   最外层必须捕获 `BaseException`。
4. **坐标系两帧不同**：`projectPoint` 返回未居中、y 向上；`viewPartAsSvg` 返回已居中、y 向下。
   直接 `scale(1,-1)` 会把图上下镜像。
5. **中文 locale**：FreeCAD 自带 Python 在空 `LANG`/`LC_ALL`/`LC_CTYPE` 下按 ASCII 编码
   `json.dump(..., ensure_ascii=False)` 并抛 `UnicodeEncodeError`；需强制 UTF-8 locale。
6. **并发缓存竞争**：早期问题（模板被拷到 `/`）已通过 `doc.TransientDir` 修复；
   仍有「单独跑通过、带覆盖率插桩失败」的待复现记录。
7. **剖面线静默失败**：图案名不在 `.pat`、`patFile` 不可读时 `makeGeomHatch` 返回 `None`；
   面不在 z=0 平面时返回空 Compound。

---

## 五、安装路径候选（macOS，未验证）

| 路径 | 可行性 | 风险 |
|---|---|---|
| Homebrew cask `freecad` | 待确认版本是否 ≥1.1 | cask 版本随上游更新，可能低于 1.1 |
| 官方 macOS `.dmg`（arm64） | 待确认 1.1 是否有 Apple Silicon 构建 | 需手工安装、路径与 `freecadcmd` 名称需探测 |
| 官方 AppImage（Linux） | ❌ | AppImage 不能在 macOS 运行 |
| Conda-forge `freecad` | 待确认版本 | 环境体积大，与系统 Python 隔离 |

**判定**：三条候选都未验证，且都需要人工安装。这与「可重复、可 CI 化」的工程要求冲突。

---

## 六、与现有渲染器的分工

| 图型 | 推荐渲染器 | 系统依赖 | 状态 |
|---|---|---|---|
| 流程图/框图/时序图 | Graphviz `dot` | 无（wasm）或 `dot` | 已具备 |
| 剖面图（2D 几何） | 自绘 SVG | 无 | 可移植（F2） |
| 机械结构线稿 | FreeCAD TechDraw | FreeCAD 1.1 | **本报告建议不立项** |
| SVG 文字转路径 | Inkscape | Inkscape | 可用（本机已装，沙箱内执行受限） |
| 简单结构式/化学式 | 专用结构式渲染器 | 无 | 另议（Mady 有 chemistry 抽取） |

---

## 七、若仍要立项：最小验证清单

按顺序执行，任一步失败即停止：

- [ ] 在 macOS 上安装 FreeCAD ≥1.1，确认 `freecadcmd --version` 输出 1.1.x
- [ ] 用 dsh 的 `buildStructureScript` 生成脚本，对 1 个真实 STEP 文件跑通，产出 SVG
- [ ] 在 `LANG=en_US.UTF-8` 下验证中文标注不抛 `UnicodeEncodeError`
- [ ] 验证冷启动耗时与内存占用（目标：单图 < 120s）
- [ ] 连续渲染 10 张图，验证无缓存竞争导致的失败
- [ ] 确认安装可脚本化（一条命令 + 版本锁定），否则不进入 CI

---

## 八、对 W4 的影响

- F1（矢量图 + Graphviz）、F2（图型 + 渲染核验）、F4（一致性与交付）不受影响，按原计划 26 人日。
- 原 F3（FreeCAD 结构图 12–20 人日）**不进入实施波次**；本报告的验证清单若通过，
  再单独提出立项与估算。
- `generate_structure_figure`、`structure-model/-render/-types` 在任务清单附录 A 中标记为「待研究」。

---

## 九、复现命令

```bash
scripts/check-domain-deps
```

当前本机结果：`dot` 16.1.0、Chrome、pandoc 3.11、typst 0.15.1、pdftoppm、Inkscape 可用；
`freecadcmd` 缺失；`soffice` 缺失；`python-docx` 仅在 `/opt/homebrew/bin/python3.11`（3.11.16）下可用。
