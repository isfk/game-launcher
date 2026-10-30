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
| `Library(ctx, refresh)` | 扫各平台的库；一个都没扫到就返回演示数据（`source: "demo"`） |
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

## 4.2.1 开机画面（index.html 里那层）

WebView 从加载页面到 Vue 挂载之间有一段空窗（实测第一帧后还有 ~0.66s 的主线程阻塞：
解析 JS 包 + Vue 首帧）。这段时间如果什么都不画，就是「黑屏闪一下」。

所以开机画面写在 `index.html` 的 `<style>` 里，不依赖任何 JS 包：

- 图标 logo（用 `public/logo.png`，保留图标自带的圆角方块，不裁圆、不加外圈描边）
  + 2.6s 呼吸（`scale(.975↔1.025)`）
- `GAME LAUNCHER` 0.4em 字距，1.9s 呼吸
- 扫光进度条（1.35s）
- 全部动画都是 transform / opacity，会被提到合成层

三个关键点：

1. **动画不靠 JS 起跑**。早先试过「双 rAF 加 `.go` 类」，但窗口被遮挡时 rAF 不触发，
   动画就永远不开始 —— 一律直接用 CSS animation。
2. **挂载时把时间轴拨回 0**（`App.vue` 的 `onMounted`）：主线程堵着的时候 CSS 动画的时间
   照样在走，等空出来会「跳」到半圈；拨回 0 之后第一帧就是第 0 帧，看不出跳。
3. **撤画面要等库读完**（`Promise.allSettled([load(), loadSettings()])`），否则会先闪一下空库
   再蹦出卡片；最短展示 2s，库读太快也不会「一闪而过」。

## 4.2.2 胶囊底色统一

`--capsule-bg` / `--capsule-bg-strong` / `--capsule-line` / `--capsule-shadow` 四个 token
管住所有圆形/胶囊状底板：平台统计、底部提示条、右上头像、网格与设置的关闭键。
以前每处各写 `rgba(10,12,18,.34)` 之类的深色，压在封面上像一块墨；现在统一成浅色玻璃，
深浅只由 hover 的 `-strong` 区分。

## 4.3 键位表

**提示条默认显示手柄键位（Xbox）**：没接手柄也按 Xbox 画，因为 XInput 最常见。
接上手柄后按真实设备画 —— Xbox 用 A/B/X/Y，PS 用 ✕○□△，任天堂用 B/A/Y/X
（识别靠设备名，见 `internal/pad.KindOf`，认不出来一律按 Xbox）。
玩家按了键盘才切到按键提示，再动手柄又切回来。

手柄（北通等 XInput 设备，走 mygo 的 gamepad 插件统一 Xbox/PS/任天堂布局）：

| 操作 | 按键（south = 下方那颗） | Xbox 显示 | PS 显示 | Switch 显示 |
|---|---|---|---|---|
| 切换游戏（首页） | lb / rb | LB | L1 | L |
| 切换平台（网格） | lb / rb | RB | R1 | R |
| 全部游戏 | back | View | Create | − |
| 设置面板翻页 | lb / rb | LB / RB | L1 / R1 | L / R |
| 详情（首页；十字键上也开） | north | Y | △ | X |
| 面板内选焦点 | 十字键 / 左摇杆 上下 | — | — | — |
| 确认（焦点项） | south | A | ✕ | B |
| 详情页 | north | Y | △ | X |
| 收藏 | west | X | □ | Y |
| 返回 / 关闭 | east | B | ○ | A |
| 设置 | start | Menu | Options | + |
| 全屏切换 | guide | Guide | PS | Home |

键盘：

| 操作 | 按键 | 备注 |
|---|---|---|
| 切换游戏 | ← → | 详情展开时也可用 |
| 面板内选焦点 | ↑ ↓ | 设置里左右也能选 |
| 启动 / 确认 | Enter / Space | 面板里 = 确认焦点项 |
| 详情 | **Y** | 对位手柄 Y / △；首页额外认 ↑（网格里 ↑ 要留给移动）|
| 收藏 | X | 对位手柄 □ / X |
| 卸载（详情内） | U | |
| 设置 | S | 网格里不生效（设置面板在首页那层）|
| 搜索 | / | 同上 |
| 全部游戏 | G | 网格里 G = 关闭 |
| 返回 / 关闭 | Esc | 首页不做事（全屏只从设置里切）|
| 全屏切换 | F | 只在首页/详情 |
| 重新扫描库 | R | 只在首页/详情 |

### 「全部游戏」网格（独立一页）

网格不是「首页的简化版」：首页能做的事它都能做，只多一个「移动」、把「全部游戏」换成「关闭」。

| 操作 | 手柄 | 键盘 | 备注 |
|---|---|---|---|
| 移动光标 | 十字键 / 左摇杆 | ↑ ↓ ← → | |
| 换平台 | LT / RT | 鼠标点胶囊 | 在「全部 → 各平台」之间轮转，光标回第一部；平台行里 ←→ 也是换它，且**焦点落到哪颗就筛哪颗** |
| 换卡片 | LB / RB | ↑↓←→ | 肩键永远只换卡片（平台行上按也先收回焦点），方向键是按格子走 |
| 确认平台 | A | Enter / Space | 应用这颗平台 + 选中第一部 + 焦点回卡片区（**不会去启动上一张卡**）|
| 启动 | A / ✕ / B | Enter / Space | **启动成功才收起网格**，失败（演示数据、文件没了）就留在网格里看原因 |
| 详情 | Y / △ / X | Y | 打开详情才收起网格（详情抽屉住在首页那层）|
| 收藏 | X / □ / Y | X | 光标不动，星标就地亮 |
| 设置 | Menu / Options / + | S | 设置面板提到 `.app` 根上，盖在网格上开 |
| 搜索 | — | `/` | 选中结果就把**网格光标**移过去（不是偷偷换首页焦点）|
| 调整顺序 | — | M | 见下一节；鼠标直接拖卡片也行 |
| 全屏 | —（guide 不用）| F | |
| 重新扫描 | — | R | 列表就地刷新，网格不收 |
| 关闭 | B / ○ / A | Esc / G | |

换筛选（扳机 / 点胶囊 / 平台行 ←→）之后列表会整块变：留下来的卡片用 FLIP 滑到新位置
（300ms，和拖动同一套），而不是「唰」地闪一下重排。触发条件是**卡片 key 序列变化**，
所以光标移动、收藏星标这类不改变成员集合的更新不会白跑动画。

**「没有选中卡片」是一个真状态，不能当没有。** 焦点上到平台那行时，卡片的高亮是灭的
（一屏只有一处焦点），这时按确认键不能去动「光标上一次停的那部」——那是玩家看不见的目标。
所以动作键一律先过 `focusedGame()`：不在卡片区、或者没有卡片，就返回空，
`launch` / 详情 / 收藏全都不执行；平台行的确认键单独走 `confirmTab()`，落到「选中第一部」。

### 调整游戏顺序（自定义排序）

想自己摆位置时，「全部游戏」里可以直接把卡片挪到想去的地方；落地后排序切成 `manual`，
顺序存成 `settings.Order`（完整的 key 列表）。三条入口是同一个状态机（`preview` → `setManualOrder`），
不各写一套：

| 入口 | 操作 | 落地 |
|---|---|---|
| 鼠标 | 按住卡片直接拖 | 松手即落盘 |
| 键盘 | 网格里按 `M`，或点标题栏的「用方向键调」| 方向键挪、Enter 放下、Esc 取消 |
| 手柄 | 网格里**长按 Y**（≥500ms） | 方向键挪、A 放下、B 退出；短按 Y 仍是详情 |

手柄没有指针，所以它用长按：`Y` 起手先挂一个 500ms 的计时器，
按住够久就进搬动模式，按没够就松开 —— 那还是原来的「详情」。
入口只有这三条，设置面板里**不再有**「全部游戏 / 平台筛选 / 调整游戏顺序」——
那些行只是把已经能做的事又列一遍，属于冗余；手柄进网格改走 `View`（`back`）键，
换平台走网格里的 LB / RB。

`usePad` 因此会多发一种 `release` 意图：只发按下的话，长按和短按根本分不出来。

手感上的三个约束（都是踩过坑才加的）：

- **插位按网格几何算**（列宽、行高、间距、滚动量），不去量正在做动画的卡片 ——
  指针停在两格之间时，量动画中的 rect 会判成 A、B、A、B，卡片就左右抖。
- **换格位有 20% 死区**：指针必须真正挪进目标格才换位，格线上来回晃不会触发重排。
- **旁边的卡片用 FLIP 滑过去**（拖动期间每档 300ms），松手时被拖的那张再用 300ms 滑回格位，
  而不是「啪」地跳过去；动画的起点取「卡片此刻的视觉位置」，所以连着换十几格也是一路滑。

数据层的两条兜底：`manual` 排序下收藏不再置顶（否则「把收藏挪下去」会被一直弹回顶部），
没写进 `Order` 的新游戏排在最后；`SetOrder([])` 清掉自定义顺序并回落到最近游玩，
`SetSort` 遇到「轮到 manual 却没有顺序」也自己回落，不让界面停在一个排不出来的档。

**顺序也统一**：提示条按「导航 → 肩键 → 启动 → 详情 → 收藏 → 设置 → 其余（全部游戏 /
全屏 / 刷新）→ 返回」排列，首页与网格一模一样，只换首项的措辞。

**一个键只有一个意思**：A 永远是启动、Y 永远是详情、X 永远是收藏、B/Esc 永远是返回。
首页的 ↑ 只是个额外便利（对着「往上看详情」），网格里 ↑ 是移动，所以两边都认 Y；
手柄十字键上在首页也开详情（与键盘 ↑ 对齐），在网格里则是向上移动一格。

提示条会跟着最近一次输入切换，并且按层次换内容（`HintBar` 的 `context`：
`home` / `detail` / `settings` / `grid` / `move` / `search`）——
首页能左右翻卡片、详情里能上下选按钮，挂同一排提示等于没提示。

**输入方式本身也是共享状态**：`usingPad` / `padKind` 放在 `src/lib/input.ts`，
不再由 `App.vue` 持有再往下传 —— 因为除了提示条，**按钮上的字**也要跟着换：
网格、设置、搜索的关闭按钮在手柄模式下直接画返回键（Xbox `B` / PS `○` / Switch `A`），
键鼠模式下才画 `✕`。画一个 `✕` 给手柄玩家看，等于让他猜「哪个键是 ✕」。

键位符号表在 `src/lib/glyphs.ts`（纯函数，可直接 `bun -e` 验证）：
`prev` / `next`（肩键：LB·RB / L1·R1 / L·R）、`view`（View / Create / −）也在里面。

**`.vue` 里的 script 改动 tsc 查不到**：`bun run typecheck` 只跑 `tsc --noEmit`，
SFC 里的 `<script setup>` 不参与；`vue-tsc` 那份装的是老版本，跟当前 TypeScript 不兼容（`./lib/tsc` 已经被 exports 挡掉）。
所以改了 `.vue` 的 script 就必须真跑一遍界面 —— 踩过的坑：`SearchBar` 里用了 `glyphsFor` 却没 import，
编译期毫无反应，运行期整个搜索框渲染不出来（`searchOpen` 是 true，DOM 里没有 `.search`）。

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

## 4.41.1 演示数据也按真库的形排（三平台）

本机一个平台都没扫到时，界面不能是一排空白：`library.DemoGames()` 给 8 部游戏，
**覆盖 Steam 4 / Epic 2 / WeGame 2**。

- **Provider 写真平台名**（`steam` / `epic` / `wegame`），不是 `"demo"`：顶栏统计、
  平台图标、点平台筛选全都按真库的样子跑，能看出真实版式
- 靠新字段 `Game.Demo` 把它标成假的：`Launch` / `Uninstall` / `OpenFolder`
  进门第一件事就是看这一位，绝不去拉一个假的 `steam://` 协议
- **封面借 Steam CDN**（`cdn.cloudflare.steamstatic.com/steam/apps/<appid>/library_600x900.jpg`）：
  挑的是 Steam 上有同一部游戏的作品，图对得上名字；`AppID` 只给 Steam 那批填，
  其他平台留空（Epic 是 catalog id，混填会让「商店图兜底」查到别的游戏）
- 顺手带上了体积 / 游玩时长 / 分类标签，详情页的 chips 也是真的

前端 `stores/library.ts` 的 `browserDemo()` 跟着改同样一份数据，
浏览器里调样式时看到的三平台顶栏和真机一致。
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

## 4.38 全屏只从设置里切（手柄 B / Esc 不再掉出全屏）

**现象**：Windows 上全屏时按手柄 B，界面掉出全屏。`back()` 的兜底分支
（关无可关时）会调 `leaveFullScreen()`，而手柄 **B(east)** 和键盘 **Esc** 都走 `back()`。

同一类坑还有第二个：**Guide 键**（手柄中间的 Xbox/PS 键）原先直接调
`toggleFullScreen()`，一样是「按一下就掉出全屏」。现在这个绑定也去掉了。

**修法**：`back()` 只管「关浮层」——启动遮罩 / 搜索 / 卸载确认 / 设置 / 详情，
全关了之后就什么也不做，**不再碰全屏**；Guide 键不再绑全屏。
全屏只剩两个入口：

- 设置中心的「全屏」那一行
- 键盘 `F` 快捷键（提示条里仍然写着）

提示条也跟着改：首页不再显示「B 返回 / Esc 返回」（那两下现在什么也不做），
详情和设置里还保留「收起 / 返回」。

## 4.39 WeGame Provider（`internal/wegame`）

WeGame 没有公开清单格式（不像 Steam 的 appmanifest、Epic 的 `.item`），
它把游戏直接铺在 `rail_apps` 目录下：**一个子目录 = 一款游戏**。
所以这里只能靠目录结构推断，规则都写在 `pickExec` 里。

**目录**：`%ProgramFiles(x86)%\WeGame\rail_apps`、`%ProgramFiles%\WeGame\rail_apps`、
`%ProgramFiles%\Tencent\WeGame\rail_apps`（可用 `LAUNCHER_WEGAME_DIR` 覆盖；
非 Windows 直接返回空，WeGame 只有 Windows 客户端）。

**过滤**：`rail_apps` 里躺的不只有游戏，还有 WeGame 自己的组件目录
（`wegame` / `tgp` / `tenio` / `plugins` / `redist` / `directx` / `download` …），
这些都跳过；没有 `.exe` 的目录也跳过（还没下载完）。

**挑主程序**（`pickExec`）：

1. 目录树里找 `.exe`，深度不超过 3 层（够覆盖 `bin/`、`Binaries/Win64/`）
2. 跳过卸载器/安装包/上报器（`unins*`、`vcredist*`、`dxsetup`、`crash*` …）
3. 与目录同名的优先（`LOL\LOL.exe` 这种最常见）
4. 其次路径最浅，最后体积最大

**启动**：直接跑找到的 exe（`Game.Exec`，`json:"-"` 不发给页面）。
WeGame 的 `tencent://` 协议需要它内部的游戏 id，没有稳定来源，拼错只会静默失败。

**卸载**：不支持——由 WeGame 客户端管理，详情面板不显示卸载按钮，
误调 `Uninstall` 会提示「请在 WeGame 客户端里卸载」。

## 4.39.1 游戏管理：隐藏 / 排序 / 筛选 / 打开目录

| 功能 | 实现 |
|---|---|
| **隐藏（仅启动器内）** | `Settings.Hidden []string` + `SetHidden(key, bool)`。不碰客户端、不碰磁盘。详情面板「隐藏」按钮；设置里「隐藏的游戏」段：顶部「全部恢复」，下面**每个被隐藏的游戏单独一行**（图标+标题+平台 + 自己的「恢复」），可以只放回其中一部 |
| **排序** | 只剩两档：`recent`（默认，最近玩过在前）与 `manual`（拖出来的顺序，存 `Settings.Order`）。字段 / 方向那套对启动器没意义，想按名字找用搜索。`recent` 下**收藏置顶**；`manual` 下不置顶（否则「把收藏挪下去」会被弹回顶部）|
| **平台筛选** | 「全部游戏」标题行那三颗胶囊：可点（再点一次取消）、手柄 `LB / RB` 轮转、方向键 ↑ 上到胶囊行精确挑。首页不再显示统计 |
| **打开安装目录** | `OpenFolder(key)` → 交给系统文件管理器（`openURL(installDir)`），目录不存在会报错。详情面板「安装位置」那一行即可点击 |

**架构改动**：store 里把「后端给的完整列表」`allGames` 和「视图列表」`games(computed)` 分开。
隐藏 / 筛选 / 排序全是视图层的事——数据不动，取消后立刻还原；
`reconcileFocus(keepKey)` 负责让焦点跟着数据走（藏掉的正好是当前这部时顺延一位）。
副作用：`ensureAccent` / `applyArtwork` / `setRunning` 改成写 `allGames`，
因为 `games` 是 computed，不能就地 mutate。

## 4.40.1 顶栏平台统计（不做「胶囊塞一行字」）

右上角原来是 `▣▣▣ Steam + Epic + WeGame · 6 部` 一颗胶囊，信息糊成一团。
现在按组排开：

```
▣ STEAM 2 │ ▣ EPIC 2 │ ◔ WEGAME 2      Ⓜ shen   10:04
```

- **组间发丝线**（1px、0.9rem 高、只在中间画），组内不再用分隔符
- **平台名当标签用**：0.6rem、`letter-spacing: .16em`、大写、42% 白
- **数量**：0.78rem / 600 字重 / `tabular-nums`——视线先抓数字，再看平台
- **容器几乎看不见**：磨砂底 `rgba(10,12,18,.34)` + 模糊 + 1px 内描边，
  只有演示数据时整块透明（`.flat`）
- 账号（头像 + 昵称）也是一颗同材质的胶囊：头像 1.6rem 圆片 + 首字兜底，
  昵称最长 12rem 后省略，不让长名字把时钟挤得跳位；点它进设置的资料区

数字来源是前端 `providerCounts`（按 `Game.provider` 聚合，顺序固定
steam → epic → wegame → manual），后端不用改。

## 4.40.2 账号资料：昵称 + 头像（不再写死 Master）

右上角那个 `Master` 是写死在前端的假名字，跟玩家没关系。现在它真的是「你」：

| 部分 | 实现 |
|---|---|
| **昵称** | `Settings.Name`（`json:"name,omitempty"`）。空串 = 没自定义，后端回落成系统账户名（`USER` / `USERNAME` / `LOGNAME`，取不到才用「玩家」）。`config.NormalizeName` 去首尾空白、换行/制表压成空格、超 24 字截断 |
| **头像** | `Settings.Avatar` 存**本机绝对路径**，但页面拿到的是 `art://` URL（`Profile.Avatar`）。没头像时是空串，界面画「昵称首字 + 渐变」兜底 |
| **接口** | `Profile()` / `SetNickname(name)` / `PickAvatar()` / `ClearAvatar()`，都返回同一个 `Profile{name, avatar}` |
| **入口** | 点右上角那个圆头像 → 打开设置面板最上面的「昵称 / 头像」两行 |

**为什么不记住原图路径**：原图通常躺在下载目录，过两天被清理掉头像就白了。
`config.Store.SaveAvatar` 把图片复制进 `<配置目录>/avatars/<内容散列>.<ext>`：

- 按**内容**嗅探类型（`http.DetectContentType`），只收 PNG / JPEG / WebP / GIF，
  上限 8 MB —— 把 `.txt` 改名成 `.png` 糊弄不过去
- 文件名用 `sha256` 前 16 位：同一张图反复选不会多出副本；换了图就是新文件，
  `art://` 的 URL 跟着变，浏览器缓存不会拿出旧头像（`art://` 的 id 是按路径算的）
- 旧文件故意不删：「清除头像」只把 `Settings.Avatar` 置空，复制件留在原地；
  换回来时 URL 不变，而乱删文件的风险比几十 KB 大得多

**交互细节**：昵称是「就地进输入框」，不另弹一层——`Enter` 保存、`Esc` 取消，
失焦也算保存（`Esc` 先改状态再 `blur`，避免取消又被当成保存）。
输入框上 `@keydown.stop`，否则 `App.vue` 的全局键位会把打字当成快捷键。
头像坏图（文件被手删了、格式怪）在 `TopBar` 里 `@error` 退回首字兜底。

## 4.39.2 大库（几百部游戏）怎么办

Steam 库里三四百部很常见，原来所有卡片一次性渲染：每张卡都是一次 `<img>`
加渐变和事件，滚动会卡、内存也顶不住。

**窗口化渲染**（`App.vue` 的 `inRenderWindow`）：只挂载焦点前后各 12 张，
其余位置用同宽同高的 `.card-slot` 占位。半径 12 是因为一屏最多看到 7 张，
多出来的用于滑动时提前准备，不会看到空白。

- 占位元素同样是 `.track` 的直接子节点 → `track.children[index]` 的下标映射不变，
  `centerFocused()` 一行都不用改
- 卡片图加 `loading="lazy" decoding="async"`，离屏的图不下载、不占解码内存

**位置指示**：库超过 12 部时，卡片带下方出现一条发丝轨道 + 一小段滑块
（`left` 按 `index / (count-1)` 算），一眼知道自己翻到哪儿了。

实测 200 部假库：初始只挂载 **13** 张卡、187 个占位；连按 30 次右方向后
挂载 **25** 张、焦点到第 31 部、滑块在 12% 位置，`.track` 子节点仍是 200。

## 4.40 provider 彩色图标

`src/components/ProviderIcon.vue` 在卡片角标、详情 eyebrow、顶栏来源处各显示一个小图标。

图标文件放 `src/assets/providers/<provider>.svg|png|webp`，组件用
`import.meta.glob(..., { eager: true, query: "?url" })` 扫出来：
**加文件即生效**，不用改代码；目录里没有对应文件时退回手写的内联 SVG 兜底。

**位置可配**（设置 →「平台图标位置」，`settings.json` 的 `iconPos`）：

| 值 | 位置 | 表现 |
|---|---|---|
| `bl`（默认） | 左下 | **和游戏名同一行**，图标在名称左侧 |
| `br` | 右下 | **和游戏名同一行**，图标在名称右侧（名称占满剩余宽度，图标自然靠右） |
| `tl` | 左上 | 封面左上角贴角徽章 |
| `tr` | 右上 | 封面右上角，和收藏星标排成一行（星标在左、图标在右） |

**角标对齐**：贴角模式下不是一个元素一个偏移，而是一个 flex 容器 `.cluster`
（`align-items: center` + `gap`），四个角只改 `top/left/right/bottom` 且统一用
**rem** 而不是 em —— em 会跟着图标 font-size 缩放，四角留白就会算得不一样。
右上角同时有收藏星标时，两者在同一个容器里排成一行：**星标在左、平台图标在右**。
星标单独显示时（其他三个位置）位置要跟这里对齐：cluster 顶边 0.55rem、内部按
1.05rem 徽章垂直居中 → 中心落在 1.075rem；星标高 0.9rem，所以 `top: 0.625rem`，
两种状态中心都正好 1.075rem，切换时星标不跳、字号也一致（0.9rem）。

**切换动画**：用 FLIP——`onBeforeUpdate` 量一次位置、`onUpdated` 再量一次，
用 Web Animations 从旧位置补到新位置（420ms，`cubic-bezier(.22,1,.36,1)`）。
**三个元素一起补**：平台图标、游戏名、收藏星标。它们换位置时布局都会挪，
只补图标会出现「一个滑、两个跳」的割裂感；位移为 0 的直接跳过，不白跑动画。
不在 CSS 里 transition `top/left` 的原因：换角落时锚点会从 `left` 变成 `right`，
是两个不同属性，浏览器不会插值，元素只会瞬移。内联 ↔ 贴角也能滑，因为两种
位置共用同一个 template ref。系统开了「减少动态效果」时直接跳过动画。

设置里是一个轮转按钮（点击 / Enter 依次切换），后端 `SetIconPos` 用
`config.NormalizeIconPos` 收敛非法值：手改坏的 `settings.json` 不会让界面
停在一个没有对应样式的值上。卡片和收藏角标冲突时由 CSS 让位
（`icon-tl` 把「运行中」下移、`icon-tr` 把星标下移）。

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

## 4.41 全部游戏网格：复用首页卡片、列数档位、焦点环不裁切

- **卡片直接复用 `GameCard` 组件**：之前网格自己写了一套 `.tile/.cover/.label`，
  抄着抄着就歪了——`.meta` 被写成 `flex-direction: column`（图标跑到名字上面）、
  徽章/字号比首页小一圈、封面为保 2:3 居中收缩而名称行按整列宽排版（整体左移）。
  现在网格只负责摆格子，每格渲染同一个 `GameCard`，只喂一个 `--card-w`。
  实测两边绝对像素完全一致：图标 14.8px、名称 13.76px、星标 15.6px、
  label 内边距 `14.4 13.6 12`、名称左偏移 37px、星标 top 10px / right 8.8px。
- **卡片宽度由容器算**：`cardW = min(列宽, 行高 / 1.5)`，卡片是 2:3 的
  `GameCard`，所以既不横向挤也不纵向溢出；`ResizeObserver` 跟着窗口变。
- **每行几部**：`config.GridCols` 取 `4 / 5 / 6 / 8`（非法值收敛到 6，加载时归一化）。
  行数按列数定：4/5 列两行、6/8 列三行。入口三处：网格里按 `D` 轮转、
  设置面板「每行几部」、副标题实时显示当前列数。
- **焦点卡顶部被切**：`.grid` 的 `overflow: hidden` 会裁掉向外画的 2px 焦点环，
  行高 `1fr` 平分后本来就不溢出，于是放开裁切、只留 `padding-block: 2px`。
- **换列数不改网格宽度**：列用 `minmax(0, 1fr)` 始终铺满整行，只让卡片变窄；
  行数 = 竖着能叠几行（2~4 行），卡片宽 = `min(列宽, 行高 / 1.5)`。
- **焦点放大收敛**：`--focus-scale` / `--focus-lift` 变量化，首页与网格统一 `1.04 / -0.4vh`
  （原来 1.08 在网格里会压到邻居、顶到副标题）。
- **网格上/下可跨页**：光标走二维，最后一行再按下就翻到下一页同一列，
  第一行再按上回到上一页同一列；只有越过总条数才不响应（原来是整段禁止跨页）。

## 4.42 「全部游戏」改为独立页面 + 卡片复用 + 视觉定稿

### 结构
- **不再是浮层**：原来 `.library` 是 `position: absolute; inset: 0; z-index: 40` 的弹层，
  改成和首页平级的一页 —— `App.vue` 里 `.home` 用 `v-show="!gridOpen"` 让位，
  `<LibraryGrid>` 和 `<HintBar>` 提到 `.app` 下。`.library` 因此变成
  `position: relative; height: 100%`，不再有自己的底色。
- **设置 / 搜索 / toast / 启动浮层也提到 `.app` 根上**：它们本来挂在 `.home` 里，
  网格一开就被 `v-show` 整层藏起来 —— 于是「网格里按 S、`/` 没反应」
  「添加游戏成功的提示看不见」「Enter 启动失败先被挑回首页再报错」都是同一个原因。
  现在这四层和网格平级，靠 z-index 叠：启动浮层(60) > toast(80，总在最上) > 设置(40) > 网格(1)。
  `App.vue` 的键盘/手柄分发也改成**同一顺序**从最上层往下判：
  启动浮层 → 设置 → 搜索 → 网格 → 详情 → 首页。
- **背景只有一个**：`<Backdrop>` 放在最外层 `.app`，首页和网格共用同一张
  「大图 + 主色光晕 + 颗粒」。谁都不再自己铺背景。

### 卡片
- **直接复用首页的 `GameCard`**。之前网格自己抄了一套 `.tile/.cover/.meta`，抄歪了：
  `.meta` 被写成 `flex-direction: column`（图标跑到名字上面）、字号小一圈、
  封面为保 2:3 居中收缩而名称行按整列宽排版（整体左移）。
  现在网格只负责摆格子，喂一个 `--card-w`。实测两边绝对像素一致：
  图标 14.8px、名称 13.76px、星标 15.6px、label 内边距 `14.4 13.6 12`。
- **不分页**：一次铺完所有游戏，纵向滚动，光标只做二维移动。

### 列数
- 固定**每行 6 部**，不再可配（删掉 `config.GridCols`、`SetGridCols`、设置项、`D` 键）。
- 网格始终铺满整行（`repeat(6, minmax(0, 1fr))`），换窗口只改卡片尺寸，不改网格宽度。

### 视觉
- **选中卡外圈**：5px，贴在卡片**外侧**，`inset: -5px` + `mask` 掏空中间；
  用的是**竖向渐变透明**（`rgba(255,255,255,.62) → .28 → .06`），不是实心白。
- **主色光晕**：`box-shadow: 0 0 var(--card-glow, 96px)`，首页与网格都是 96px；
  网格左右留白 `5.5vw`（≈106px）确保光晕不被滚动容器裁切。
- **未选中卡**：`opacity: .8` + `scale(.92)` + `saturate(.92) brightness(.94)`
  （0.5 太透，看着像玻璃）。
- **删掉所有整页/整排的半透明层**：`Backdrop` 的 `.wash`、`.vignette`，
  首页卡片带那条 `.rail::before`。压暗只由卡片自己的 `.label` 承担。
- **卡片光泽 `.sheen` 调薄**：`white .16 / black .45` → `.05 / .26`，
  原来是给整张封面盖了一层灰罩，颜色发闷。
- **平台图标加深色圆底**：图标是纯白（实测 2523 个不透明像素全是 `(255,255,255)`），
  压在金/雪这类亮封面上会「看不见」；`.label-icon` / `.badge` 加
  `rgba(0,0,0,.42)` 圆底 + 阴影。
- **`.card` 改 `overflow: visible`**（要往外画焦点环），裁切下沉到 `.art`；
  `.label` 因此要自己收 `border-radius: 0 0 12px 12px`，否则它的直角会盖住封面下沿圆角。

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
| **M2** | 详情页竖向导航 ✅、设置持久化到 Go ✅、虚拟手柄 E2E ✅ | ✅ 完成 |
| **M3** | 手动添加游戏 ✅（`Dialog.Open` 选 exe/app）、移除 ✅、搜索 ✅、收藏置顶 ✅ | ✅ 完成 |
| **M4** | `mygo build -platform windows/amd64` ✅、图标 ✅、全屏盖任务栏保险 ✅、NSIS 安装包（待定：需 makensis）、Windows 真机验收 | 进行中 |
| **M5** | Epic Provider ✅、WeGame Provider ✅ | ✅ 完成 |

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

### 虚拟手柄 E2E（`scripts/e2e-pad.py`）

没有真实手柄也能测手柄逻辑：脚本往 `window.__padQueue` 推帧，
`usePad` 用**和真实手柄完全相同的管线**处理（边沿检测、方向连发都跑到），
生产环境里这个队列永远是 undefined，所以那段代码不会真的做事。

覆盖 9 项：方向移动、按住连发、Y 开详情、B 收详情、X 收藏、
Options 开设置、设置内竖向导航、B 关设置、键位体系跟随手柄类型。

```bash
GOFLAGS=-tags=e2e mygo build -debug -skip-dmg -skip-notarize
LAUNCHER_STEAM_DIR=/tmp/fakesteam MYGO_AUTOMATION=/tmp/gl.sock \
  "build/darwin-arm64/Game Launcher.app/Contents/MacOS/Game Launcher" &
python3 scripts/e2e-pad.py /tmp/gl.sock      # 退出码 0 = 全过
```

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
