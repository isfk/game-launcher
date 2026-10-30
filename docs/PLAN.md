# Game Launcher 规划

一个仿 PlayStation 5 主页风格的游戏启动器：扫描本机 Steam 库，全屏、手柄操作，
Mac 与 Windows 双端发布。

- 状态：**M0 / M1 完成，M4 进行中**（图标 ✅、全屏保险 ✅、安装包待定）
- 最后更新：2026-10-29

---

## 1. 技术选型（结论）

| 部分 | 选择 | 理由 |
|---|---|---|
| 外壳 | **mygo 0.7.2**（Go 1.27，纯 Go 无 cgo） | 任何机器可交叉编译所有平台；官方手柄包；系统 WebView 不打包浏览器 |
| 界面 | **Vue 3 + TypeScript + Vite + UnoCSS + Pinia** | PS5 那种模糊/渐变/弹簧动画靠 CSS 表现力最强；生态与既有 skills 一致 |
| 数据 | Steam 本地文件 + JSON（用户设置/收藏在 localStorage） | 零账号、零 API key、断网可用 |
| 音效 | WebAudio 现场合成 | 不打包音频资源 |

### 为什么不是 nsh（Native Shell）

| 需求 | mygo | nsh |
|---|---|---|
| Windows 打包 | 任意机器 `-platform windows/amd64` | 文档明确要求**在 Windows 上打包**（runner 按本机构建） |
| 手柄 | `plugins/game/gamepad`：macOS GameController / Windows XInput / Linux evdev / 浏览器 Gamepad API | 全站文档 0 命中，无原始输入 API |
| Steam 扫描 + 起进程 | Go 标准库直读文件、`exec` | TSX 跑在 QuickJS 沙盒 + capabilities 里；Rust 路线可行但框架太年轻 |

已知代价：Mac 开发是 **WKWebView（Safari 内核）**，Windows 发布是 **WebView2（Chromium）**，
关键视觉效果需双端各验一次。

---

## 2. 架构

```
┌──────────────── WebView（Vue 3）────────────────┐
│ Home：横向卡片轨道 / 详情面板 / 设置 / 启动浮层      │
│ focus engine：手柄与键盘共用一个 reducer            │
│ usePad ← Go 推来的手柄帧（Channel 流）              │
└──────────────────┬──────────────────────────────┘
                   │ `mygo generate` 生成的类型化客户端（src/mygo.ts）
┌──────────────────┴──────────────────────────────┐
│ Go 后端                                          │
│  Launcher（app.go）    Library / Launch / Pads … │
│  internal/steam         VDF/ACF 解析器 + 扫描       │
│  internal/library       Game 模型 + Provider 接口  │
│  internal/pad           手柄帧（量化 + 签名去重）   │
│  internal/launch        启动外部程序（不经 shell）  │
└─────────────────────────────────────────────────┘
```

### Go 侧服务（`app.go`）

| 方法 | 作用 |
|---|---|
| `Library(ctx, refresh)` | 扫 Steam 库；扫不到就返回演示数据（`source: "demo"`） |
| `Launch(ctx, key)` | `steam:<appid>` → `steam://rungameid/<appid>` |
| `Pads(ctx, channel)` | 60Hz 轮询手柄，只在变化时推一帧 |
| `PadNames()` | 设置页显示已接手柄 |
| `ToggleFullScreen / IsFullScreen` | 全屏切换 |
| `Quit()` | 退出 |

**Provider 抽象**（加 Epic / WeGame 只写一个实现，界面不动）：

```go
type Provider interface {
    ID() string
    Scan(ctx context.Context) ([]Game, error)
    Launch(ctx context.Context, g Game) error
}
```

---

## 3. Steam 本地库接入（当前做法）

不登录、不调 Web API，只读客户端留下的文件：

| 平台 | 根目录 |
|---|---|
| macOS | `~/Library/Application Support/Steam` |
| Windows | 注册表 `HKCU\Software\Valve\Steam\SteamPath`，兜底 `HKLM\...\WOW6432Node` 与 `Program Files (x86)` |
| Linux | `~/.steam/steam`、`~/.local/share/Steam` |

1. `steamapps/libraryfolders.vdf` → 所有库目录（兼容新旧两种格式）
2. 每个库里 `steamapps/appmanifest_<appid>.acf` → 名称、安装目录、大小、上次游玩
3. **只收 `StateFlags & 4`**（真正装好的，排除下载中/仅清单）
4. 封面从本地 `appcache/librarycache/<appid>/` 取（`library_600x900.jpg` / `library_hero.jpg` / `logo.png`）→ **离线可用、不分发版权素材**
5. 启动一律走 `steam://rungameid/<appid>`，让 Steam 客户端接管 DRM / overlay / 云存档 / 反作弊
6. 开发/排查：`LAUNCHER_STEAM_DIR=/path/to/Steam` 覆盖根目录

> 封面经 `art://` 协议喂给页面（见 3.5 节）。待办（M1）：`localconfig.vdf` 里的收藏/分类标签与真实游玩时长。

---

## 3.5 封面与图片（`art://` 协议）

**问题**：WebView 读不了 `C:\...\cover.jpg` 这种文件路径，直接把路径塞进 `<img src>` 会一张图都不显示。

**做法**（`internal/asset`）：

```
Go: scan 得到本地路径 → Store.URL(path) 登记 → 返回 URL
     macOS / Linux:  art://localhost/<id>
     Windows:        http://art.localhost/<id>   ← WebView2 的要求
页面: <img src="art://localhost/<id>"> → Go 的 http.Handler 流式回文件
```

**美术资源的取用顺序（本地 → CDN）**

Steam 只缓存它自己库页面加载过的素材，所以本机 `appcache` 是残缺的：
有的游戏只有封面、有的只有 hero、有的什么都没有。因此每个素材都按
「本地多个候选名 → Steam 公开 CDN」的顺序取：

| 素材 | 本地候选 | CDN 兜底 |
|---|---|---|
| 封面 | `library_600x900.jpg`、`library_600x900_2x.jpg`、`library_capsule.jpg`、`capsule_616x353.jpg`、`header.jpg` | `cdn.cloudflare.steamstatic.com/steam/apps/<appid>/library_600x900.jpg` |
| hero | `library_hero.jpg`、`library_hero_blur.jpg` | `…/library_hero.jpg` |
| logo | `logo.png`、`logo_2x.png` | `…/logo.png` |

- 本地命中 → `art://`；未命中 → CDN 的 https 地址（页面 `<img>` 直接能加载，不需要 key）
- 前端还有一层兜底：没有 hero 就拿封面模糊放大当背景；图挂了退回渐变占位
- 非游戏条目（`Steamworks Common Redistributables`、`Steam Linux Runtime`、`Proton…`）在扫描阶段就被过滤

- `id` 是 **sha256(路径) 前 20 位 + 原后缀**，页面拿不到真实路径，也不能顺着 id 读别的文件
- 只服务登记过的 id，未登记 → 404；文件被删 → 404；`Cache-Control: max-age=3600`
- 必须在 `NewWindow` 之前 `mygo.Protocol.Handle("art", …)` 注册
- 任一张图加载失败时，前端 `@error` 退回渐变占位（不会出现破图）

**离线缓存**（`internal/artcache`）：扫到走 CDN 的图就在后台下载到
`<cache>/artwork/<appid>/`（并发 3，先写 `.part` 再改名，单图上限 8MB）。
下载完成后补上主色并通过 `artworkReady` 事件通知页面就地换图；
下次启动直接命中缓存，断网也有封面。

**主色**：`Launcher.Accent(key)` 按需从封面提取（`internal/steam/accent.go`）——
网格采样、丢掉近黑白像素、以饱和度为权重求平均，再用 HSL 推到「高饱和 + 中等亮度」。
结果按 key 缓存，选中哪部才取哪部，不拖慢扫描。

## 3.6 游玩时长与分类

读 `userdata/<steamid>/config/localconfig.vdf`（挑 mtime 最新的那个账号）：

```
UserLocalConfigStore → Software → Valve → Steam → Apps → <appid>
  Playtime  累计分钟数      → Game.PlaytimeMinutes
  LastPlayed  unix 秒       → Game.LastPlayed
  tags      收藏夹分类名     → Game.Tags
```

只读文件，读不到就跳过，不影响扫描。

## 3.7 游戏退出后回到前台

`steam://` 不给 PID，所以按「安装目录里还有没有活进程」推断（`internal/proc`）：

- macOS / Linux：`ps -Ao args=ww`，比较命令行首段与安装目录
- Windows：Toolhelp 快照 + `QueryFullProcessImageNameW`（不用已弃用的 wmic）

`Launch` 成功后起一个守护 goroutine：等进程出现（最多 120 秒，Steam 冷启动慢）→
广播 `gameRunning: true`（卡片显示「运行中」）→ 每 2 秒轮询 → 进程消失后广播
`gameRunning: false` 并 `Show() + Focus()` 把启动器拉回前台。

## 4. 交互模型

| 输入 | 动作 |
|---|---|
| ← → / 左摇杆 / 十字键 | 移动焦点（按住连发：380ms 后每 130ms） |
| ✕ / A / Enter | 启动游戏 |
| △ / Y / ↑ | 详情面板 |
| □ / X | 收藏（localStorage，卡片右上角星标） |
| ○ / B / Esc | 返回上一层；无浮层时退出全屏（永远留退路） |
| Options / Start | 设置 |
| F | 全屏切换 |
| R | 重新扫描 |

手柄数据流：Go 每 1/60 秒读 `gamepad.Pads()` → 量化成 `{name, buttons[], dir}` →
**只有签名变化才推一帧** → 前端做边沿检测与连发。多手柄以最后一只的方向为准。

> 北通等 XInput 手柄：Windows 上走 XInput 直接可用；macOS 上 XInput 模式通常不被
> GameController.framework 接管，此时键盘仍是完整替代。

---

## 4.3 键位表

**提示条默认显示手柄键位（Xbox）**：没接手柄也按 Xbox 画，因为 XInput 最常见。
接上手柄后按真实设备画 —— Xbox 用 A/B/X/Y，PS 用 ✕○□△，任天堂用 B/A/Y/X
（识别靠设备名，见 `internal/pad.KindOf`，认不出来一律按 Xbox）。
玩家按了键盘才切到按键提示，再动手柄又切回来。

手柄（北通等 XInput 设备，走 mygo 的 gamepad 插件统一 Xbox/PS/任天堂布局）：

| 操作 | 按键（south = 下方那颗） | Xbox 显示 | PS 显示 | Switch 显示 |
|---|---|---|---|---|
| 切换游戏 | 十字键 / 左摇杆 左右 | — | — | — |
| 面板内选焦点 | 十字键 / 左摇杆 上下 | — | — | — |
| 确认（焦点项） | south | A | ✕ | B |
| 详情页 | north | Y | △ | X |
| 收藏 | west | X | □ | Y |
| 返回 / 关闭 | east | B | ○ | A |
| 设置 | start | Menu | Options | + |
| 全屏切换 | guide | Guide | PS | Home |

键盘：

| 操作 | 按键 |
|---|---|
| 切换游戏 | ← →（详情展开时也可用） |
| 面板内选焦点 | ↑ ↓ |
| 确认（焦点项） | Enter / Space |
| 详情页 | ↑ |
| 收藏 | X |
| 卸载（详情内） | U |
| 设置 | S |
| 返回 / 关闭 | Esc |
| 全屏切换 | F |
| 重新扫描库 | R |

提示条会跟着最近一次输入切换（`App.vue` 的 `usingPad` → `HintBar` 的 `pad` prop），
并且按层次换内容（`HintBar` 的 `context`：`home` / `detail` / `settings`）——
首页能左右翻卡片、详情里能上下选按钮，挂同一排提示等于没提示。

键位符号表在 `src/lib/glyphs.ts`（纯函数，可直接 `bun -e` 验证）。

## 4.3.1 面板内的竖向导航

右侧浮层里的按钮**不用鼠标也能到达**，这是手柄玩家的地基。

焦点不是浏览器 DOM 焦点，而是**画出来的**（accent 描边 + 抬起）：

- 状态只有「一个下标」，键盘和手柄共用同一份 —— 见 `src/composables/usePanelNav.ts`
- 面板用 `defineExpose({ move, activate, reset, isFocused })` 暴露给 `App.vue`；
  `App` 只负责把按键翻译成「上/下/确认」，不关心面板里有什么
- 动作列表是 `computed`：换游戏后「卸载」可能消失，下标自动收敛，不会指到不存在的按钮
- 鼠标 `mouseenter` 也会把焦点挪过去，两种输入不会打架
- 换游戏、重开面板都会 `reset()` 回第一项（详情默认停在「启动」）

分工：

| 层次 | ↑↓ | ←→ | Enter | Esc |
|---|---|---|---|---|
| 首页 | 打开详情 | 切换游戏 | 启动 | 退出全屏 |
| 详情 | 选按钮 | 切换游戏 | 确认焦点项 | 收起抽屉 |
| 设置 | 选项 | 选项 | 确认焦点项 | 关闭面板 |

> 详情抽屉特意**不拦左右键**：抽屉只占右边三成，卡片带还在，边看详情边翻卡片才顺手。

## 4.34 右侧浮层（PanelShell + DetailDrawer + SettingsSheet）

详情和设置都用**右侧滑出的浮层**，共用外壳 `PanelShell.vue`。

```
┌───────────────────────────┬──────────────────┐
│                           │  封面 56vh        │ ← 竖版封面，顶部对齐裁切
│   卡片带（仍可见、可切换）  │                  │   底部渐隐进面板底色
│                           │  LOGO            │
│                           ├──────────────────┤
│                           │ STEAM · 已收藏   │ ← eyebrow：来源 / 状态
│                           │ 总时长│上次│空间  │ ← 发丝线分隔，标签小数值大
│                           │ [魂类][想白金]    │
│                           │ 详情             │
│                           │ AppID      1245620│ ← 发丝线行 + 等宽字体
│                           │ 上次启动   2026-… │
│                           │ 安装位置   …/ELDEN│
│                           ├──────────────────┤
│                           │ [启动游戏   Enter]│
│                           │ [X 收藏][Esc 收起]│
└───────────────────────────┴──────────────────┘
```

排版原则（照主机系统来，不照网页后台来）：

1. **图打头**：封面铺满上半截，`veil` 渐变收到面板底色，LOGO 压在图上，不用额外标题占位
2. **数字靠字排，不靠卡片**：三个关键数字用发丝线分开；没有值的那个自己消失，不留一排「—」
3. **次要信息收进「详情」段**：发丝线行 + 等宽字体；长路径 `shortenPath()` 中间省略（尾部目录名比盘符有用）
4. **操作区固定底部**：启动是唯一主按钮（整行），收藏 / 收起是次级按钮平分一行

外壳 `PanelShell` 负责所有浮层通用的东西：

- 定位 `--drawer-w: min(28rem, 32vw)`、层级 `--z-detail`、`translate3d(100%)` ↔ `0` 的 0.42s 滑入滑出
- 左侧 **18rem 横向蒙版**（越往左越透明）+ 面板内缘压暗，所以不需要边框，也不会「一刀切」
- 主色微光：`radial-gradient` 叠在底色上，`--accent` 由调用方传入

首页配合（`App.vue`）：

- `.home.expanded`（详情 **或** 设置展开时）→ 轨道 `--rail-nudge: calc(var(--drawer-w) / -2)` 往左让半个侧栏
- 展开时 TopBar 淡出、HintBar 的 `right` 收到侧栏左边，保持相对居中
- 设置面板底部附键盘捷径速查，顺便把面板下半截填满

> 试过两个更简的方案：A「左栏就地展开」信息量太少、丢掉封面（`max-height` 动画实测高度恒为 0）；
> 居中弹窗则遮住卡片、多一层 z-index、还要焦点陷阱。侧栏是折中后最顺的。

## 4.34.1 卸载（二次确认）

详情面板底部有「卸载」，`U` 键也能进；只对 **Steam 游戏**显示（手动 / 演示数据没有平台可托付）。

流程：

```
点「卸载」/ 按 U
   ↓  底部换成确认条（红色「确认卸载」+「取消」）
   ↓  Enter / A 确认，Esc / ○ / B 取消，8 秒无操作自动撤销
Launcher.Uninstall(key)  →  steam://uninstall/<appid>
   ↓  Steam 客户端再弹自己的确认框、开始删
卸载完成 → 库指纹轮询发现 appmanifest 少了 → LibraryChanged → 卡片自动消失
```

几个刻意的取舍：

- **不由启动器删目录**：Steam 卸载还要清 appmanifest、更新已装索引、处理云存档标记，`rm -rf` 只会把客户端搞成「文件缺失」的半损状态
- **二次确认是自己的**：Steam 那边还会问一次，但那一步要求玩家切窗口；手柄误触时先在自己这一层拦住
- **确认态是独立上下文**：只认 Enter / Esc，方向键不会再把卡片换掉（换游戏或收起详情都会撤销确认态）
- **确认态会超时**：8 秒自动撤销，避免一直停在危险状态
- `openURL` 做成 `app.go` 里的变量，测试替换掉就能断言「打开的是 `steam://uninstall/1245620`」而不真去开 Steam

## 4.34.2 设置持久化（`internal/config`）

用户设置落在**后端**的 JSON 文件里，不用 localStorage：

```
macOS: ~/Library/Application Support/Game Launcher/settings.json
Windows: %AppData%\Game Launcher\settings.json
```

（`mygo.App.Path(mygo.PathUserData)`，首次使用自动建目录；启动日志会打一行路径）

```json
{
  "soundEnabled": false,
  "favorites": ["steam:1245620"]
}
```

为什么放后端：

- 设置要**跨重启 / 跨窗口**一致；以后「手动添加的游戏」也要一起落盘
- 一份数据一个来源，不会出现「界面记得、后端不知道」的撕扯
- 浏览器里跑 dev server 时没有后端，收藏退回 localStorage（只为调样式）

实现要点（`internal/config/config.go`）：

| 关注点 | 做法 |
|---|---|
| 文件不存在 | 正常情况，返回 `Default()`（音效开） |
| 文件损坏 | 改名成 `settings.json.broken` 留档 + 回默认值 —— 宁可丢一次设置，不能开不了机 |
| 写入 | 先写 `.tmp` 再 `rename`，中途崩溃不会留下半个 JSON |
| 老版本读新文件 | 未知字段忽略，不报错 |
| 并发 | `Store` 内部一把锁；`Launcher` 侧用已有的 `mu` |

前端不做真相来源，只做镜像：`useLibrary` 里的 `soundEnabled` / `favorites`

- 启动时 `loadSettings()` 拉一次，`watch(lib.soundEnabled)` 同步到 `sfx`
- 收藏是**乐观更新**：点了立刻变色，再异步落盘（点收藏不该等 IO）
- 落盘失败时界面保持乐观结果，下次启动回到磁盘状态（不弹错误框）

## 4.34.3 手动添加的游戏（`internal/manual`）

Steam 之外还有一堆「双击 exe 就开」的游戏，所以给了两条入口：

- **设置 → 添加游戏** → `Dialog.Open` 选一个 `.app` / `.exe` / `.bat` / `.cmd` / `.lnk`
- 记录写进 `settings.json` 的 `manualGames`，key 由**路径哈希**算出（`manual:<sha256 前 10 位>`），
  同一个文件加两次不会变成两条

几条边界：

- **不猜平台信息**：手动游戏没有 appid，所以不能卸载给 Steam、取不到商店图，
  封面退回渐变占位（`artwork` 为空 → `Backdrop` 用主色兜底）
- **移除 ≠ 删除**：`RemoveManualGame` 只从库里摘掉记录，磁盘上的文件一个字都不动；
  卸载按钮的文案也跟着变成「移除」，确认条写明「磁盘上的文件不动」
- **路径失效就跳过**：`manual.Games()` 滤掉空路径，文件被挪走后这条记录静默消失，
  不会在界面上留一张点不开的卡
- **手动游戏排最前**：`rescan` 把它们 prepend 到 Steam 结果前面，玩家自己加的排在显眼处
- **启动 = 交给系统**：复用 `launch.OpenURL`（macOS `open` / Windows `ShellExecuteW` / Linux `xdg-open`），
  不自己 fork 子进程——`.app` 要走 LaunchServices 才有正确的环境
- 文件选择框抽成包级变量 `pickExecutable`，测试里替换掉就能跑完整流程（真对话框要人点）

## 4.34.4 搜索与收藏置顶

- **搜索是「跳转」不是「过滤」**：`/` 唤起顶部搜索条，输入即把焦点移到第一个匹配项，
  卡片带本身不动。过滤会把玩家正在看的那张卡抽走、还会让 ←→ 的邻居关系突然变掉
- 搜索打开时键盘事件全让给输入框（只兜 Esc/Enter 退出），手柄只认 east 关闭
- **收藏置顶**：`favorites` 里的游戏排到最前面，其余保持后端顺序（手动的在前、Steam 按上次游玩）；
  排序发生在数据层（`withFavoritesFirst`）而不是渲染层——卡片下标就是焦点下标，两边顺序必须一致，
  否则 ←→ 会跳着走。排序后按 key 找回焦点，收藏/取消收藏不会把光标甩走

## 4.34.5 清理（2026-10-29）

- 删掉两个已无引用的壳：`DetailPanel.vue`（最早那版弹窗）和 `ModalShell.vue`（设置迁到 `PanelShell` 之后）
- `steam.tagsOf()` 加了 `sort.Strings`：`tags` 是 map，Go 遍历顺序随机，不排序同一部游戏两次打开标签顺序会变

## 4.34.6 手柄只在窗口前台时生效（Windows 双启动 Bug）

**现象**：进游戏后在游戏里按 A，启动器又启动了一次游戏。

**原因**：手柄是**全局轮询**的（`gamepad.Pads()` 直接问 XInput），跟窗口有没有焦点无关。
启动器虽然退到后台，60Hz 循环还在读，按 A 照样变成一次「启动游戏」意图。

**修法（三道闸）**：

1. **失焦就不读**（`Launcher.padStates` / `gatePadStates`）：窗口不在前台时返回空切片。
   空帧还有个副作用是好的——它会把界面上的「按住」状态清掉，回到前台不会残留连发。
2. **焦点靠轮询兜底**（`focusedNow`）：`win.OnFocus/OnBlur` 只用来记状态，
   真正判定是每 200ms 问一次 `win.IsFocused()`
   （Windows 上就是 `GetForegroundWindow() == hwnd`，最实在，也不怕事件漏送）。
   没有窗口时（测试）退回事先记下的值，不会被误判成失焦。
3. **防双开**（`Launch` 的 `starting` / `runningKey`）：
   发出启动请求到进程被确认之间有个窗口期，这段时间再点 A 直接拒绝；
   已经有游戏在跑时也拒绝，并报出是哪部在跑。

## 4.36 Epic Provider（`internal/epic`）

和 Steam 一样只读本地、不登录、不调 Web API。每个装好的游戏在
`Manifests` 目录里有一份 `<AppName>.item`（JSON），里面就有显示名、安装目录、体积。

**目录**（`DefaultRoots`，可用 `LAUNCHER_EPIC_DIR` 覆盖，测试和开发都靠它）：

| 平台 | 路径 |
|---|---|
| Windows | `%ProgramData%\Epic\EpicGamesLauncher\Data\Manifests` |
| macOS | `~/Library/Application Support/Epic/EpicGamesLauncher/Data/Manifests` |
| Linux | `~/.local/share/Epic/EpicGamesLauncher/Data/Manifests` |

**过滤规则**（这块坑最多——Manifests 目录里躺的不只是游戏）：

- `AppCategories` 里必须有 `games`：否则会把 Unreal Engine、插件一起扫进来
- `MainGameAppName` 非空 → 是 DLC，不给独立一格
- `IsInDevelopment` / `bIsIncompleteInstall` → 跳过（开发版、下了一半的）
- 缺 `AppName` / `DisplayName` / `InstallLocation` 的一律不要，宁可少一格也不显示空白卡
- 单份清单 JSON 坏了就当没看见，不影响其他游戏

**启动与卸载**都交给 Epic 客户端自己的协议：

```
com.epicgames.launcher://apps/<AppName>?action=launch&silent=true
com.epicgames.launcher://apps/<AppName>?action=uninstall&silent=true
```

自己跑 exe 会绕开它的云存档和 DRM；自己删目录会把客户端搞成半损状态。

**已知取舍**：Epic 本地没有库封面（图在它自己的缓存里、跟账号绑定），
所以卡片和背景走**渐变占位**（主色 + 字母水印），不是 bug。
想补图得调它的 GraphQL（要 client token），暂时不做。

**多平台合并**：`rescan` 遍历 `providers()`（steam + epic），任一方扫失败只记日志、
不影响另一方；`Source` 用 `+` 拼起来（`"steam+epic"`、`"epic"`、`"manual"`、`"demo"`），
界面按它显示「Steam + Epic · N 部」。库指纹也是两边拼起来（`librarySignature`）。

## 4.37 logo 白条（`internal/imgcheck`）

**现象**：标题区出现一条白条。

**原因**：有些游戏的 `logo.png` 本身就是一片纯白（或者极浅灰渐变），
画到深色界面上就是一条白带。它不是「没有 logo」，所以 `v-if` 拦不住。

**修法**：在 Go 侧判定「视觉空白」，判为空白就当没有 logo——
界面自然退回 `<h1>` 游戏名（有 logo 显示 logo，没有就显示名字）。

为什么不在前端：页面加载图片走 `art://`（Windows 上是 `http://art.localhost`），
属于独立源，画进 canvas 会被标记污染，`getImageData` 直接抛异常。

判定规则（`imgcheck.Blank`，均匀抽样最多 4096 像素）：

1. 全透明 → 空白
2. 覆盖面积 < 95%（透明底上的白字 logo）→ **不是**空白，留着
3. 铺满整幅，且「纯色」或「均值亮度 ≥ 0.88」→ 空白（白条/白底带）
4. 白底黑字的牌子 → 不算（亮度跨度大、均值也够低）

只对**本地文件**判定：CDN 地址一律放行，不为了判断多花一次网络请求。

## 4.35 层级（z-index）

全应用只从 `style.css` 的层级表取，别写裸数字：

| 变量 | 值 | 用途 |
|---|---|---|
| `--z-rail` | 2 | 卡片带 |
| `--z-meta` 语义同 3 | 3 | 标题 / 标签块 |
| `--z-topbar` | 8 | 右上角状态 |
| `--z-hints` | 9 | 底部提示条 |
| `--z-detail` | 20 | 详情页（整页推开） |
| `--z-sheet` | 40 | 设置等浮层，必须压过详情页 |
| `--z-launch` | 60 | 启动提示 |
| `--z-toast` | 80 | toast |

> 踩过的坑：设置页和启动浮层原来**没有 z-index**，被 `z-index: 5` 的卡片带压住，
> 打开设置会看到面板从卡片底下穿出来。

## 4.4 封面 / hero 的取材（含新发售游戏）

**四层兜底**：

1. 本机 Steam 缓存 `<Steam>/appcache/librarycache/<appid>/`
2. 老式 CDN `https://cdn.cloudflare.steamstatic.com/steam/apps/<appid>/library_600x900.jpg`
3. **Store 资源接口**（下面详述）
4. 渐变占位（按标题散列颜色）

### 为什么刚发售的游戏没有封面

2024 年之后 Steam 把库封面挪到带内容哈希的目录下：

```
https://shared.akamai.steamstatic.com/store_item_assets/steam/apps/<appid>/<哈希>/library_capsule_2x.jpg
```

旧地址 `cdn.cloudflare.steamstatic.com/steam/apps/<appid>/library_600x900.jpg`
**只有老游戏才有**，新产品一律 404 —— 而哈希猜不出来。

解决办法是问一次 Steam 客户端自己在用的公开接口（不需要登录、不需要 API key）：

```
GET https://api.steampowered.com/IStoreBrowseService/GetItems/v1/?input_json=...
    {"ids":[{"appid":4115450}],"context":{"language":"schinese","country_code":"CN"},
     "data_request":{"include_assets":true}}
```

响应里的 `assets` 直接给 `library_capsule_2x`（600×900）、`library_hero_2x`（3840×1240），
配合 `asset_url_format` 拼出完整地址（见 `internal/storeart`）。

**实测**（影之刃零 appid 4115450）：卡面 600×900、hero 3840×1240，均正常。

踩过的三个坑：

- **只查失败之后补不行**：后台补图靠 `ArtworkReady` 事件，首次启动时页面还没订阅，事件会丢。
  所以补图放在扫描阶段（`fillStoreArtwork`），有磁盘缓存时不联网。
- **文件名带上了时间戳**：Store 地址是 `...jpg?t=1793242975`，直接 `path.Base` 会把
  `?t=...` 写进文件名，之后 `art://` 地址就坏了。已在 `artcache.fileName` 统一剥掉 query/fragment。
- **失败的 hero 被永久拉黑**：`Backdrop.vue` 原来按 key 记失败，拿到新地址也不重试；
  现在记的是**失败的地址**，地址一变就会重新加载。

查询结果缓存在 `<cache>/storeassets/<appid>.json`（TTL 7 天），图片本身在 `<cache>/artwork/<appid>/`。
设 `LAUNCHER_NO_STORE_ASSETS=1` 可完全关掉这个接口（退化为前三层兜底）。

## 4.5 新装游戏的自动发现

**问题**：库只在启动时扫一次并缓存，在 Steam 里刚装好的游戏必须重启应用（或手动按 `R`）才出现。

**做法**：后台每 5 秒给 Steam 库拍一次「指纹」（`internal/steam/signature.go`）——
每个库的 `libraryfolders.vdf` 修改时间 + 每个 `appmanifest_*.acf` 的名字/大小/修改时间。
指纹变了就等 1.5 秒（避开 Steam 连续写文件的中间状态）再重扫，广播 `libraryChanged`，
界面就地替换列表并 toast「发现 N 部新游戏」。

用指纹轮询而不是文件监听的理由：跨平台零依赖，Windows 上也不用处理
`ReadDirectoryChangesW` 的缓冲区溢出；一轮只是几十上百次 `stat`，开销可忽略。

**踩过的坑**：后台事件送来的对象在 JS 侧是**冻结**的，store 里就地写
`game.accent = ...` 会抛 `TypeError`（严格模式）导致整个 handler 中断、列表换不掉。
所以合并状态必须构造新对象（`{ ...game, accent: ... }`）。

**验证**：`scripts/e2e-library-watch.py` —— 起一个 e2e 构建，往假库里塞一份 appmanifest，
轮询首页卡片标题，6 秒内应该出现新游戏。

## 5. 全屏与任务栏

**Windows 的实际做法**（读 mygo 源码确认）：`SetFullScreen` 会去掉 `WS_CAPTION|WS_THICKFRAME`，
并 `SetWindowPos` 到**整个显示器**（不是工作区），所以任务栏区域会被铺满。
但任务栏自身是 topmost 的，因此应用内再加一层：窗口获得焦点时 `SetAlwaysOnTop(true)`，
失焦时撤销 —— 这样既不露出任务栏，也不会压在启动后的游戏上面。

**应用图标**（`scripts/genicon/main.go`）—— 两种模式：

```bash
# A. 用外部素材（当前采用）：居中缩放到 0.82 倍画布、套圆角砖遮罩
go run scripts/genicon/main.go -art assets/controller.png -out resources/icon.png

# B. 程序化绘制（无素材时的兜底）：深色砖 + 主色光晕 + SDF 手柄
go run scripts/genicon/main.go -out resources/icon.png
```

两种模式都会额外输出 16/32/128 预览到 `build/icons/`，方便检查小尺寸可辨认性。
素材模式的留边比例按 macOS 图标规范（可见砖块 ≈ 画布 82%）。

> 注意：`assets/controller.png` 来自第三方图库（ipaslogo CDN），
> 自用没问题；如果要公开发布或商用，需要先确认该素材的授权。

## 5.1 全屏与任务栏（历史）

- 窗口：`Frameless: true` + `FullScreen: true`，`BackgroundColor: #05060a` 防白闪
- 需要更硬的控制时：`Screen.Displays()[i].Bounds`（含任务栏；`WorkArea` 不含）+
  `SetAlwaysOnTop` / `SetSkipTaskbar`
- 逃生口：`win.NativeHandle()` 拿 `HWND`，必要时自己调 Win32 `SetWindowPos`
- **M4 必须在 Windows 真机验收**：全屏是否盖住任务栏

---

## 6. 里程碑

| M | 内容 | 状态 |
|---|---|---|
| **M0** | 脚手架、PS5 首页（卡片/主色背景/提示条/时钟）、焦点导航、手柄帧通道、Steam 扫描 + 解析测试 | ✅ 完成 |
| **M1** | 封面走 `art://` ✅、主色从封面提取 ✅、CDN 图落地离线缓存 ✅、游玩时长与分类 ✅、游戏退出自动回前台 ✅ | ✅ 完成 |
| **M2** | 详情页竖向导航 ✅、设置持久化到 Go ✅、虚拟手柄 E2E（待办） | 进行中 |
| **M3** | 手动添加游戏 ✅（`Dialog.Open` 选 exe/app）、移除 ✅、搜索 ✅、收藏置顶 ✅ | ✅ 完成 |
| **M4** | `mygo build -platform windows/amd64` ✅、图标 ✅、全屏盖任务栏保险 ✅、NSIS 安装包（待定：需 makensis）、Windows 真机验收 | 进行中 |
| **M5** | Epic Provider ✅、WeGame | 进行中 |

---

## 7. 测试

```bash
bun run typecheck                      # tsc（.ts）
CGO_ENABLED=0 go test ./...            # VDF/ACF 解析、扫描、服务层
go vet ./...
bun run dev                            # mygo dev：Vite + 开发版应用
```

- `internal/steam/*_test.go`：用 `t.TempDir()` 造双库假目录（装好/下载中/无封面），
  断言只收装好的、按上次游玩排序、封面路径解析
- `app_test.go`：空库退回 demo、缓存与 refresh、真实库识别、未知 key 与 demo 条目拒绝启动、
  手动添加（取消/重复/路径失效）、移除（连收藏一起清、不动文件）、手动游戏启动与拒卸载
- `internal/manual/*_test.go`：路径哈希稳定性、文件名推导、可执行校验、记录转换
- `internal/epic/*_test.go`：只收 games 类目（DLC/引擎/开发版/半损安装都要滤掉）、
  坏 JSON 不影响其他清单、按标题稳定排序、两个协议 URL、库指纹随清单变化
- `app_test.go` 里另有`TestGatePadStates` / `TestPadStatesFollowsFocus`（失焦不推手柄）、
  `TestLaunchRefusesWhileStarting` / `TestLaunchRefusesWhileGameRunning`（防双开）
- ⚠️ 测试里务必 `t.Setenv("LAUNCHER_NO_STORE_ASSETS", "1")`：否则重扫会去
  `store.steampowered.com` 拉图，直连超时会让 `go test` 挂几分钟（CPU 时间为 0，很像卡死）
- 浏览器里打开 `http://localhost:5173` 可脱离应用调样式（store 会自动用本地样例数据）
- 截图证据：`build/shots/`（gitignored）

**真窗口验证**（WebView 里的实际情况，浏览器看不出来）：

```bash
# 1) 让应用可被外部驱动（e2e 标签，发布产物不含）
go build -tags e2e -o /tmp/app .
# 2) 跑起来并指定假 Steam 库
LAUNCHER_STEAM_DIR=/tmp/fakesteam MYGO_AUTOMATION=/tmp/app.sock /tmp/app
```

然后连 Unix socket 发 JSON 行：`windows`（看全屏是否真的 1920×1080）、
`eval`（断言 `img.naturalWidth > 0`、`--accent` 值）、`capture`（存 PNG 看真实渲染）。
`mygo build -debug` 产出的应用也保留这套能力。

---

## 8. 风险与对策

| 风险 | 对策 |
|---|---|
| WKWebView / WebView2 渲染差异 | 关键动效双端各验；`color-mix` 等新特性降级时仍有底色 |
| Windows 全屏盖不住任务栏 | `Bounds` 而非 `WorkArea` + `SetAlwaysOnTop`；不行走 `NativeHandle()` |
| 全屏请求被忽略 | `FullScreen` 选项在窗口上屏前可能不生效 → `OnReadyToShow` 里 `Show()` 后 `SetFullScreen(true)`（已验证 macOS 生效） |
| 选中卡片被轨道裁切 | `.rail` 上下留白要容得下放大与上浮；左右用 `mask-image` 渐隐而不是硬裁 |
| PS 手柄在 Windows 非 XInput | 提示安装 Steam Input；文档写明支持 XInput 类手柄 |
| 反作弊 / DRM | 只走 `steam://`，绝不直连 exe |
| 封面版权 | 只读取本机 Steam 缓存 + 官方 CDN，不内置、不再分发 |
| 离线时没有 CDN 兜底 | 已解决：CDN 图会下载到本地缓存，第二次启动离线可用 |
| 进程探测误判 | 按安装目录匹配可执行文件路径；Steam 在库目录里跑运行库时可能误判为「在运行」，属可接受误差 |
| Steam 未运行 | `steam://` 会自动拉起客户端，首个启动较慢 → 界面显示「正在启动」 |
| vue-tsc 与 TypeScript 7 不兼容 | 目前 `tsc` 只检查 `.ts`；若要检查 SFC 需把 `typescript` 换到 5.x（待批准） |

---

## 9. 目录约定

```
app.go / main.go            服务绑定 + 窗口
internal/library/           Game / Artwork / Provider / 演示数据
internal/steam/             VDF 解析、ACF 解析、库扫描、平台根目录
internal/pad/               手柄帧、方向量化、签名去重
internal/launch/            启动外部程序（darwin/linux/windows 各自实现）
src/                        Vue 前端（components / composables / stores / lib）
docs/PLAN.md                本文件
```
