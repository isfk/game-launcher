<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from "vue";
import Backdrop from "./components/Backdrop.vue";
import DetailDrawer from "./components/DetailDrawer.vue";
import GameCard from "./components/GameCard.vue";
import HintBar from "./components/HintBar.vue";
import LaunchOverlay from "./components/LaunchOverlay.vue";
import LibraryGrid from "./components/LibraryGrid.vue";
import SearchBar from "./components/SearchBar.vue";
import SettingsSheet from "./components/SettingsSheet.vue";
import TopBar from "./components/TopBar.vue";
import { usePad, type PadIntent } from "./composables/usePad";
import { accentOf, formatLastPlayed, formatPlaytime, formatSize, providerLabel } from "./lib/format";
import { glyphsFor } from "./lib/glyphs";
import { padKind, usingPad } from "./lib/input";
import { sfx } from "./lib/sfx";
import { isMyGo } from "mygo-runtime";
import { events, Launcher } from "./mygo";
import { useLibrary } from "./stores/library";

const lib = useLibrary();

/**
 * 开机画面至少留够 2 秒。
 *
 * 库读得太快时（演示数据眨眼就好）「一闪而过」看着像闪屏故障，
 * 主机上开机动画本来就有固定节拍 —— 宁可按慢的来，也不要一眨眼就跳过去。
 */
const BOOT_MIN_MS = 2000;
const bootAt = performance.now();

/** dismissBoot 把 index.html 里那层开机画面淡出，动画结束再摘掉节点。 */
function dismissBoot() {
  const el = document.getElementById("boot");
  if (!el) return;
  el.classList.add("out");
  window.setTimeout(() => el.remove(), 520);
}

const railEl = ref<HTMLElement | null>(null);
const trackEl = ref<HTMLElement | null>(null);
const shift = ref(0);

/**
 * 大库窗口化渲染：只挂载焦点附近这些卡片。
 *
 * 为什么必须做：几百部游戏时，每张卡都是一个 <img> + 渐变 + 事件，
 * 全渲染会让滚动卡顿、内存飙升（Steam 库里三四百部很常见）。
 * 半径取 12，是因为屏幕上一屏最多看到 7 张左右，多出来的用于
 * 滑动时提前 آماده，不会看到空白。
 *
 * 注意：占位元素和卡片同宽同高、都是 track 的直接子节点，
 * 所以下标映射和 `track.children[index]` 的取法都不受影响。
 */
const RENDER_RADIUS = 12;
function inRenderWindow(i: number) {
  return Math.abs(i - lib.index) <= RENDER_RADIUS;
}

/**
 * centerFocused 把当前焦点卡滑到轨道中间。
 *
 * 用 transform 而不是 scrollIntoView：滚动容器会独立合成，
 * 在 Chromium 里会把背景光晕切出一条硬边，而且两端的居中控制不准。
 */
function centerFocused() {
  const rail = railEl.value;
  const track = trackEl.value;
  if (!rail || !track) return;

  const card = track.children[lib.index];
  if (!(card instanceof HTMLElement)) return;

  const railWidth = rail.clientWidth;
  if (!railWidth) return; // 还没布局，等下次 resize / 焦点变化

  // track 是 offsetParent，所以 offsetLeft 已经含了 track 的 padding；
  // 用 offsetWidth 而不是 getBoundingClientRect，避免卡片放大动画干扰测量。
  const target = card.offsetLeft - (railWidth - card.offsetWidth) / 2;
  const maxShift = Math.max(0, track.scrollWidth - railWidth);
  shift.value = -Math.min(Math.max(target, 0), maxShift);
}

const detailOpen = ref(false);
// 默认按手柄提示（Xbox 键位）：玩家动了键盘才切成按键提示，
// 再动手柄又切回来。
// 真实手柄的键位体系，决定提示条画 A/B/X/Y 还是 ✕○□△ 还是任天堂的排布
// 玩家先用过键盘之后，就别再拿「接了手柄」覆盖他的输入方式
let keyboardUsed = false;
const settingsOpen = ref(false);

// 卸载的二次确认：进入后底部换成确认条，按 Esc / 取消返回。
// 请求在飞时 busy 为 true，按钮禁用，避免连点发两次。
const searchOpen = ref(false);
/** 全屏网格（「全部游戏」）：几百部游戏时用它扫读，比横着按右方向快得多 */
const gridOpen = ref(false);

const uninstallAsk = ref(false);

// 两个面板的竖向焦点：App 只负责把按键翻译成「上/下/确认」，
// 具体焦点停在哪一项由面板自己管（见 composables/usePanelNav.ts）。
const detailRef = ref<{ move: (d: number) => boolean; activate: () => boolean; reset: () => void } | null>(null);
const settingsRef = ref<{
  move: (d: number) => boolean;
  page: (dir: number) => boolean;
  activate: () => boolean;
  reset: () => void;
} | null>(null);
const gridRef = ref<{
  move: (dx: number, dy: number) => boolean;
  activate: () => boolean;
  launch: () => boolean;
  openDetail: () => boolean;
  favorite: () => boolean;
  reset: (startKey?: string) => void;
  stepCard: (dir: number) => boolean;
  stepPlatform: (dir: number) => boolean;
  startMove: () => boolean;
  finishMove: () => boolean;
  cancelMove: () => boolean;
  isMoving: () => boolean;
} | null>(null);
/** 网格里是不是正在「调整顺序」（提示条和按键分发都要跟着变） */
const gridMoving = ref(false);

/** 手柄长按判定阈值：短按 Y = 详情，按住不放才是「调整顺序」 */
const PAD_LONG_PRESS = 500;
let northTimer: number | undefined;

function cancelNorthTimer() {
  if (northTimer !== undefined) {
    window.clearTimeout(northTimer);
    northTimer = undefined;
  }
}
const uninstalling = ref(false);
let uninstallTimer: number | undefined;
const launching = ref<{ title: string; status: "starting" | "failed"; message?: string } | null>(null);

const overlayOpen = computed(
  () =>
    detailOpen.value ||
    settingsOpen.value ||
    searchOpen.value ||
    gridOpen.value ||
    launching.value !== null,
);

// logo 图加载失败就退回文字标题
const logoBroken = ref(false);
const logoUrl = computed(() => {
  const game = lib.selected;
  if (!game || logoBroken.value) return "";
  return game.artwork?.logo ?? "";
});
const meta = computed(() => {
  const g = lib.selected;
  if (!g) return [];
  return [
    providerLabel(g.provider),
    formatLastPlayed(g.lastPlayed),
    formatPlaytime(g.playtimeMinutes),
    formatSize(g.sizeBytes),
    ...(g.tags ?? []),
  ].filter(Boolean);
});

/** 就地展开后才出现的两条技术信息：平时不需要，排查时才看。 */
const facts = computed(() => {
  const g = lib.selected;
  if (!g) return [];
  return [
    ["AppID", g.appId ?? "—"],
    ["安装位置", g.installDir ?? "—"],
  ] as const;
});

/** 展开区里按钮上的键位：跟着当前输入方式走。 */
const keys = computed(() => {
  const g = glyphsFor(padKind.value);
  if (usingPad.value) return { launch: g.confirm.text, favorite: g.favorite.text, close: g.back.text };
  return { launch: "Enter", favorite: "X", close: "Esc" };
});

const favorite = computed(() => lib.isFavorite(lib.selected?.key));

// 底部的小提示（游戏启动 / 退出时冒一下）
const toast = ref("");
let toastTimer: number | undefined;

function showToast(text: string) {
  toast.value = text;
  if (toastTimer !== undefined) window.clearTimeout(toastTimer);
  toastTimer = window.setTimeout(() => {
    toast.value = "";
  }, 4000);
}

// ---- 动作 ----

function move(delta: number) {
  // 详情是「页」不是「弹窗」：展开时左右仍可切卡片，内容跟着换。
  // 设置和启动浮层是真的模态，才拦导航。
  if (settingsOpen.value || launching.value || uninstallAsk.value) return;
  if (lib.move(delta)) sfx.move();
}

async function activate() {
  if (overlayOpen.value) return;
  const game = lib.selected;
  if (!game) return;
  await startGame(game);
}

/**
 * startGame 是「真的要启动一部」的唯一入口：首页和网格都走它。
 *
 * onLaunched 只在启动真的成功之后调 —— 网格靠它在成功后收起自己。
 * 失败时要留在原地（网格里就继续待在网格里），不能先把人挑走再报错，
 * 那看起来就像「按一下 Enter 就莫名回了首页」。
 */
async function startGame(game: { key: string; title: string }, onLaunched?: () => void) {
  if (!game.key || launching.value) return;

  sfx.confirm();
  launching.value = { title: game.title, status: "starting" };
  try {
    await Launcher.launch(game.key);
    onLaunched?.();
    // 启动成功就先让「正在启动」停一下：游戏客户端抢焦点后这个窗口自然会退到后面。
    window.setTimeout(() => {
      if (launching.value?.status === "starting") launching.value = null;
    }, 1500);
  } catch (e) {
    sfx.error();
    launching.value = {
      title: game.title,
      status: "failed",
      message: e instanceof Error ? e.message : String(e),
    };
  }
}

function back() {
  if (launching.value) {
    launching.value = null;
    return;
  }
  if (searchOpen.value) {
    closeSearch();
    return;
  }
  if (uninstallAsk.value) {
    cancelUninstall();
    return;
  }
  if (settingsOpen.value) {
    settingsOpen.value = false;
    return;
  }
  if (detailOpen.value) {
    detailOpen.value = false;
    return;
  }

  // 首页已经有层层浮层可关，剩下的一层就是首页本身：这里什么也不做。
  // 尤其是**不要在这里退全屏**——以前按 B / Esc 会掉出全屏，
  // 而全屏开关只应留在设置里（键盘另有 F 快捷键）。
}

async function toggleFullScreen() {
  sfx.confirm();
  await Launcher.toggleFullScreen();
}

/** hideCurrent 只在启动器里藏起当前这部（客户端和磁盘都不动）。 */
async function hideCurrent() {
  const game = lib.selected;
  if (!game) return;
  sfx.back();
  detailOpen.value = false;
  showToast(`已隐藏「${game.title}」· 可在设置 → 恢复隐藏的游戏里找回`);
  await lib.toggleHidden(game.key, true);
}

/** openFolder 交给系统文件管理器打开安装目录。 */
async function openFolder() {
  const game = lib.selected;
  if (!game) return;
  try {
    await Launcher.openFolder(game.key);
    sfx.confirm();
  } catch (e) {
    sfx.error();
    showToast(`打开目录失败：${e instanceof Error ? e.message : String(e)}`);
  }
}

async function rescan() {
  sfx.confirm();
  await lib.load(true);
}

// 详情是「左栏就地展开」，不是新的一层：开合只是切换一个状态位。
function openDetail() {
  if (overlayOpen.value || !lib.selected) return;
  if (detailOpen.value) {
    detailOpen.value = false;
    sfx.back();
    return;
  }
  sfx.confirm();
  detailOpen.value = true;
  detailRef.value?.reset();
}

function closeDetail() {
  if (!detailOpen.value) return;
  uninstallAsk.value = false;
  detailOpen.value = false;
  sfx.back();
}

function openGrid() {
  if (overlayOpen.value) return;
  sfx.confirm();
  gridOpen.value = true;
  gridRef.value?.reset(lib.selected?.key);
}

function closeGrid() {
  if (!gridOpen.value) return;
  cancelNorthTimer();
  // 搬动中直接关掉等于默默丢弃这次调整：先按取消处理
  if (gridRef.value?.isMoving()) gridRef.value.cancelMove();
  sfx.back();
  gridOpen.value = false;
}

/** 在网格里选中一部：关掉网格并把首页焦点落到它身上。 */
function pickFromGrid(key: string) {
  sfx.confirm();
  gridOpen.value = false;
  lib.focusKey(key);
}

/**
 * 网格里直接启动。
 *
 * 网格**不先关**：启动失败（演示数据、文件被删、客户端没装）时人就该待在原地
 * 看清原因，而不是先被挑回首页再报错 —— 那看起来就像「按 Enter 就回首页」。
 * 只有真的启动成功了才收起网格（startGame 的 onLaunched）。
 */
async function launchFromGrid(key: string) {
  const game = lib.games.find((g) => g.key === key) ?? lib.selected;
  if (!game) return;
  lib.focusKey(key);
  await startGame(game, () => {
    gridOpen.value = false;
  });
}

/** 网格里看详情：确实能开才收起网格（没选中就留在原位）。 */
function detailFromGrid(key: string) {
  lib.focusKey(key);
  if (!lib.selected) return;
  gridOpen.value = false;
  openDetail();
}

/** 网格里收藏：光标不离开网格，星标就地亮起来。 */
function favoriteFromGrid(key: string) {
  if (lib.toggleFavorite(key)) sfx.confirm();
}

/** 搜索结果的落点：网格开着就挪网格光标，否则换首页焦点。 */
function pickFromSearch(key: string) {
  if (gridOpen.value) gridRef.value?.reset(key);
  else lib.focusKey(key);
}

/**
 * startGridMove 进入「调整顺序」。
 *
 * 三种入口共用：标题栏按钮（鼠标）、键盘 M、设置面板里的「调整游戏顺序」
 * （手柄玩家的路子 —— 手柄没有指针，但 Menu 键人人都有）。
 */
async function startGridMove() {
  if (!lib.games.length) return;
  if (!gridOpen.value) {
    openGrid();
    await nextTick();
  }
  if (gridRef.value?.startMove()) sfx.confirm();
}

function openSearch() {
  // 网格里也能搜：它是一整页，不该比首页少东西。
  // 只有真正叠在上面的东西（启动浮层 / 详情）才拦。
  if (launching.value || detailOpen.value) return;
  sfx.confirm();
  searchOpen.value = true;
}

function closeSearch() {
  if (!searchOpen.value) return;
  sfx.back();
  searchOpen.value = false;
}

function openSettings() {
  // 同上：网格里按 S / Menu 也要能开设置，拦的只是更上层的浮层
  if (launching.value || detailOpen.value) return;
  if (settingsOpen.value) return;
  sfx.confirm();
  settingsOpen.value = true;
  settingsRef.value?.reset();
}

function toggleFavorite() {
  if (overlayOpen.value) return;
  if (lib.toggleFavorite()) sfx.confirm();
}

/** 点「卸载」：只进确认态，不动任何东西。8 秒没动静自动撤销，免得一直停在危险态。 */
function askUninstall() {
  if (!detailOpen.value || uninstalling.value) return;
  sfx.confirm();
  uninstallAsk.value = true;
  if (uninstallTimer !== undefined) window.clearTimeout(uninstallTimer);
  uninstallTimer = window.setTimeout(() => {
    uninstallAsk.value = false;
  }, 8000);
}

function cancelUninstall() {
  if (!uninstallAsk.value) return;
  sfx.back();
  uninstallAsk.value = false;
  if (uninstallTimer !== undefined) window.clearTimeout(uninstallTimer);
}

/**
 * confirmDestructive 执行二次确认后的动作。
 *
 * 两种游戏两种命运：
 * - Steam 游戏：交给客户端卸载（它会再问一次），卡片的消失靠库指纹事件
 * - 手动添加的游戏：只从库里摘掉这条记录，磁盘上的文件一个字都不动
 */
async function confirmDestructive() {
  const game = lib.selected;
  if (!game || uninstalling.value) return;

  uninstalling.value = true;
  uninstallAsk.value = false;
  if (uninstallTimer !== undefined) window.clearTimeout(uninstallTimer);

  try {
    if (game.provider === "manual") {
      await lib.removeManualGame(game.key);
      sfx.back();
      showToast(`已从启动器移除「${game.title}」（磁盘上的文件没动）`);
    } else {
      const title = await Launcher.uninstall(game.key);
      sfx.confirm();
      showToast(`已请 Steam 卸载「${title}」· 完成后会自动从库里消失`);
    }
    detailOpen.value = false;
  } catch (e) {
    sfx.error();
    showToast(`操作失败：${e instanceof Error ? e.message : String(e)}`);
  } finally {
    uninstalling.value = false;
  }
}

/** 详情页里点「启动」：先收起详情，再走正常的启动流程。 */
function launchFromDetail() {
  detailOpen.value = false;
  void activate();
}

/** 详情页里点「收藏」：详情页本身就是浮层，这里不能走 toggleFavorite 的守卫。 */
function favoriteFromDetail() {
  if (lib.toggleFavorite()) sfx.confirm();
}

// ---- 手柄 ----

function onPadIntent(intent: PadIntent) {
  usingPad.value = true;
  document.body.classList.add("using-pad");

  // 层次从「最上面那层」往下判：启动浮层 → 设置 → 搜索 → 网格 → 详情/首页。
  // 这个顺序就是 z-index 的顺序，不能反过来（否则网格会把设置面板的按键吃掉）

  // 启动浮层是模态：只认「关掉它」
  if (launching.value) {
    if (intent.kind === "button" && (intent.button === "east" || intent.button === "south")) {
      launching.value = null;
    }
    return;
  }

  if (settingsOpen.value) {
    if (intent.kind === "nav") {
      if (intent.dir === "up" || intent.dir === "left") settingsRef.value?.move(-1);
      else settingsRef.value?.move(1);
      return;
    }
    if (intent.kind === "button") {
      if (intent.button === "south") settingsRef.value?.activate();
      else if (intent.button === "lb") settingsRef.value?.page(-1);
      else if (intent.button === "rb") settingsRef.value?.page(1);
      else if (intent.button === "east" || intent.button === "start") settingsOpen.value = false;
    }
    return;
  }

  // 搜索是键盘场景：手柄这边只允许「返回」把它关掉
  if (searchOpen.value) {
    if (intent.kind === "button" && intent.button === "east") closeSearch();
    return;
  }

  // 网格：方向键走格子，A 启动、Y 详情、X 收藏、Menu 设置、B 关闭
  if (gridOpen.value) {
    if (intent.kind === "nav") {
      const dx = intent.dir === "left" ? -1 : intent.dir === "right" ? 1 : 0;
      const dy = intent.dir === "up" ? -1 : intent.dir === "down" ? 1 : 0;
      if (gridRef.value?.move(dx, dy)) sfx.move();
      return;
    }
    if (intent.kind === "button") {
      // 调整顺序时只剩三件事：A 放下、B 退出、方向键挪位置
      if (gridMoving.value) {
        if (intent.button === "south") {
          if (gridRef.value?.finishMove()) sfx.confirm();
        } else if (intent.button === "east") {
          if (gridRef.value?.cancelMove()) sfx.back();
        }
        return;
      }
      if (intent.button === "south") gridRef.value?.launch();
      else if (intent.button === "lb" || intent.button === "rb") {
        // 肩键 = 上一张 / 下一张（和方向键左右同义，但不会跑去平台行）
        if (gridRef.value?.stepCard(intent.button === "lb" ? -1 : 1)) sfx.move();
      } else if (intent.button === "lt" || intent.button === "rt") {
        // 扳机 = 换平台（「全部 → 各平台」轮转）
        if (gridRef.value?.stepPlatform(intent.button === "lt" ? -1 : 1)) sfx.move();
      } else if (intent.button === "north") {
        // Y 起手不立刻开详情：短按要能开详情、长按要能进搬动模式，
        // 只能等「松开」或者「按住够久」来定夺
        cancelNorthTimer();
        northTimer = window.setTimeout(() => {
          northTimer = undefined;
          void startGridMove();
        }, PAD_LONG_PRESS);
      } else if (intent.button === "west") gridRef.value?.favorite();
      else if (intent.button === "start") openSettings();
      else if (intent.button === "east") closeGrid();
    }
    if (intent.kind === "release" && intent.button === "north" && !gridMoving.value) {
      const wasPending = northTimer !== undefined;
      cancelNorthTimer();
      if (wasPending) gridRef.value?.openDetail(); // 短按 Y：详情
    }
    return;
  }

  if (intent.kind === "nav") {
    // 详情里上下在面板内部选按钮，左右留给卡片带
    if (detailOpen.value) {
      if (!uninstallAsk.value && (intent.dir === "up" || intent.dir === "down")) {
        if (detailRef.value?.move(intent.dir === "up" ? -1 : 1)) sfx.move();
        return;
      }
      if (intent.dir === "left") move(-1);
      else if (intent.dir === "right") move(1);
      return;
    }
    if (intent.dir === "left") move(-1);
    else if (intent.dir === "right") move(1);
    // 首页上方向：和键盘 ↑ 一样开详情（网格里上方向要留给自己移动，
    // 那边走 Y / △ —— 两边都一样的是「Y 键就是详情」）
    else if (intent.dir === "up") openDetail();
    return;
  }

  switch (intent.button) {
    case "south":
      // 面板打开时确认的是「当前焦点的那个按钮」，不是无脑启动
      if (uninstallAsk.value) void confirmDestructive();
      else if (detailOpen.value) detailRef.value?.activate();
      else void activate();
      break;
    case "east":
      back();
      break;
    case "west":
      if (detailOpen.value) favoriteFromDetail();
      else toggleFavorite();
      break;
    case "north":
      openDetail();
      break;
    case "back":
      openGrid();
      break;
    case "lb":
      move(-1);
      break;
    case "rb":
      move(1);
      break;
    case "start":
      if (detailOpen.value) detailOpen.value = false;
      else openSettings();
      break;
    // guide（手柄中间的 Xbox/PS 键）以前直接切全屏，等于又一个
    // 「按一下就掉出全屏」的坑。这里不用它：全屏只留在设置里（键盘另有 F）。
  }
}

// e2e / 调试用：真手柄没法在自动化里造，直接把一条意图灌进分发器
(window as unknown as { __padIntent?: (i: PadIntent) => void }).__padIntent = onPadIntent;

usePad(onPadIntent, (kind) => {
  if (kind === "playstation" || kind === "switch" || kind === "xbox") padKind.value = kind;
});

// ---- 键盘 ----

function onKeydown(e: KeyboardEvent) {
  const w = window as unknown as Record<string, unknown>;
  w.__keys = [...((w.__keys as string[]) ?? []), `${e.key}|${e.code}|${e.isTrusted}`];
  // 只按了修饰键（⌘/Shift/Ctrl）不算「改用键盘」，
  // 否则切个窗口就把键位图换掉了。
  if (!e.key || e.key === "Shift" || e.key === "Control" || e.key === "Alt" || e.key === "Meta") return;

  keyboardUsed = true;
  usingPad.value = false;
  document.body.classList.remove("using-pad");

  // 层次跟 z-index 一致，从最上那层往下判：
  // 启动浮层 → 设置 → 搜索 → 网格 → 详情 → 首页。

  // 启动浮层：只认「关掉它」，别让按键漏到下面的网格/首页
  if (launching.value) {
    if (["Escape", "Backspace", "Enter", " "].includes(e.key)) {
      e.preventDefault();
      launching.value = null;
    }
    return;
  }

  // 设置是个模态面板：所有导航键都归它，别漏到底下的页面
  if (settingsOpen.value) {
    switch (e.key) {
      case "ArrowUp":
      case "ArrowLeft":
        e.preventDefault();
        settingsRef.value?.move(-1);
        break;
      case "ArrowDown":
      case "ArrowRight":
        e.preventDefault();
        settingsRef.value?.move(1);
        break;
      case "PageUp":
        e.preventDefault();
        settingsRef.value?.page(-1);
        break;
      case "PageDown":
        e.preventDefault();
        settingsRef.value?.page(1);
        break;
      case "Enter":
      case " ":
        e.preventDefault();
        settingsRef.value?.activate();
        break;
      case "Escape":
      case "Backspace":
      case "s":
      case "S":
        e.preventDefault();
        settingsOpen.value = false;
        break;
    }
    return;
  }

  // 搜索框自己处理输入：这里只兜住「怎么退出」，别抢方向键
  if (searchOpen.value) {
    if (e.key === "Escape" || e.key === "Enter") {
      e.preventDefault();
      closeSearch();
    }
    return;
  }

  // 网格是一整页：键位与首页同一套（启动 / 详情 / 收藏 / 设置 / 搜索 / 全屏 / 刷新），
  // 只多一个「移动」和「关闭」，少一个「全部游戏」（网格里 G 就是关闭）。
  if (gridOpen.value) {
    const step = (dx: number, dy: number) => {
      if (!gridRef.value?.move(dx, dy)) return;
      e.preventDefault();
      sfx.move(); // 只有「移动」用这个音，动作键各自发自己的声音
    };

    // 调整顺序：方向键挪卡片，Enter 放下，Esc 取消；别的键先不接
    if (gridMoving.value) {
      switch (e.key) {
        case "ArrowLeft":
          step(-1, 0);
          break;
        case "ArrowRight":
          step(1, 0);
          break;
        case "ArrowUp":
          step(0, -1);
          break;
        case "ArrowDown":
          step(0, 1);
          break;
        case "Enter":
        case " ":
          e.preventDefault();
          if (gridRef.value?.finishMove()) sfx.confirm();
          break;
        case "Escape":
        case "Backspace":
          e.preventDefault();
          if (gridRef.value?.cancelMove()) sfx.back();
          break;
      }
      return;
    }

    switch (e.key) {
      case "ArrowLeft":
        step(-1, 0);
        break;
      case "ArrowRight":
        step(1, 0);
        break;
      case "ArrowUp":
        step(0, -1);
        break;
      case "ArrowDown":
        step(0, 1);
        break;
      case "Enter":
      case " ":
        e.preventDefault();
        gridRef.value?.launch();
        break;
      case "y":
      case "Y":
        e.preventDefault();
        gridRef.value?.openDetail();
        break;
      case "x":
      case "X":
        e.preventDefault();
        gridRef.value?.favorite();
        break;
      case "s":
      case "S":
        e.preventDefault();
        openSettings();
        break;
      case "m":
      case "M":
        e.preventDefault();
        void startGridMove();
        break;
      case "/":
        e.preventDefault();
        openSearch();
        break;
      case "f":
      case "F":
        e.preventDefault();
        void toggleFullScreen();
        break;
      case "r":
      case "R":
        e.preventDefault();
        void rescan();
        break;
      case "Escape":
      case "Backspace":
      case "g":
      case "G":
        e.preventDefault();
        closeGrid();
        break;
    }
    return;
  }

  if (detailOpen.value) {
    // 确认态是一个独立的小上下文：只认 Enter / Esc，别让方向键把卡片换掉
    if (uninstallAsk.value) {
      switch (e.key) {
        case "Enter":
          e.preventDefault();
          void confirmDestructive();
          break;
        case "Escape":
        case "Backspace":
          e.preventDefault();
          cancelUninstall();
          break;
      }
      return;
    }

    switch (e.key) {
      case "u":
      case "U":
        askUninstall();
        break;
      case "ArrowUp":
      case "ArrowDown":
        // 详情面板内：上下在「启动 / 收藏 / 卸载 / 收起」之间移动
        e.preventDefault();
        if (detailRef.value?.move(e.key === "ArrowUp" ? -1 : 1)) sfx.move();
        break;
      case "ArrowLeft":
      case "ArrowRight":
        // 左右仍然是换卡片：卡片带没被抽屉挡住，内容跟着换
        e.preventDefault();
        move(e.key === "ArrowLeft" ? -1 : 1);
        break;
      case "Enter":
      case " ":
        e.preventDefault();
        detailRef.value?.activate();
        break;
      case "x":
      case "X":
        favoriteFromDetail();
        break;
      case "Escape":
      case "Backspace":
        e.preventDefault();
        closeDetail();
        break;
    }
    return;
  }

  switch (e.key) {
    case "ArrowLeft":
      e.preventDefault();
      move(-1);
      break;
    case "ArrowRight":
      e.preventDefault();
      move(1);
      break;
    case "Enter":
    case " ":
      e.preventDefault();
      void activate();
      break;
    case "Escape":
      e.preventDefault();
      back();
      break;
    case "ArrowUp":
    case "y":
    case "Y":
      // 详情：Y 是手柄 Y / △ 的对位键，↑ 是首页额外送的顺手键。
      // 网格里 ↑ 被移动占着，只能走 Y；两边都认 Y 才是一致的。
      e.preventDefault();
      openDetail();
      break;
    case "x":
    case "X":
      // 对应手柄的 □ 收藏
      toggleFavorite();
      break;
    case "s":
    case "S":
      // 对应手柄的 Options 设置
      openSettings();
      break;
    case "/":
      e.preventDefault();
      openSearch();
      break;
    case "g":
    case "G":
      e.preventDefault();
      openGrid();
      break;
    case "f":
    case "F":
      void toggleFullScreen();
      break;
    case "r":
    case "R":
      void rescan();
      break;
  }
}

function onMousemove() {
  usingPad.value = false;
  document.body.classList.remove("using-pad");
}

// 卸载 / 手动删库之后选中的游戏可能整个不见了，详情不能停在空壳上
watch(
  () => lib.selected,
  (game) => {
    if (!game && detailOpen.value) {
      detailOpen.value = false;
      uninstallAsk.value = false;
    }
  },
);

watch(
  () => [lib.index, lib.games.length],
  () => void nextTick(centerFocused),
);

// 音效开关以 store 为准（设置面板改了、或者启动时从磁盘读回来，都走这里）
watch(
  () => lib.soundEnabled,
  (on) => sfx.setEnabled(on),
  { immediate: true },
);

// 焦点换了才去后端要主色（从封面提取，后端有缓存）
watch(
  () => lib.selected?.key,
  (key) => {
    // 换了一部游戏，上一部没确认完的卸载该作废，焦点也回到「启动」
    uninstallAsk.value = false;
    detailRef.value?.reset();
    logoBroken.value = false;
    void lib.ensureAccent(key);
  },
  { immediate: true },
);

// 后端事件：图片缓存好了 → 就地换图；游戏跑起来 / 退出 → 卡片状态 + 提示
const unsubscribers: Array<() => void> = [];

onMounted(() => {
  // 开机画面在「解析 JS 包」那段（实测 0.6~0.8s）是静止的：CSS 动画的时间轴照样在走，
  // 等主线程空出来会直接跳到半圈。这里把它拨回 0 —— 挂载后的第一帧就是动画的第 0 帧，
  // 看不出跳跃，也不会卡在静止画面。
  document.getElementById("boot")?.getAnimations({ subtree: true }).forEach((a) => {
    a.currentTime = 0;
  });

  // 临时调试钩子：自动化里直接看几个开关的真实状态
  (window as unknown as Record<string, unknown>).__state = () => ({
    searchOpen: searchOpen.value,
    gridOpen: gridOpen.value,
    settingsOpen: settingsOpen.value,
    detailOpen: detailOpen.value,
    usingPad: usingPad.value,
  });
  window.addEventListener("keydown", onKeydown);
  window.addEventListener("mousemove", onMousemove);
  window.addEventListener("resize", centerFocused);
  // 开机画面一直留到「库真的读完」再撤：中间那段是扫 Steam / Epic / WeGame，
  // 提前撤掉就会先闪一下空库、再蹦出卡片，比直接黑屏还难看。
  // allSettled 是因为扫描失败也得撤画面 —— 不然玩家卡在环形进度上出不来。
  void Promise.allSettled([lib.load(), lib.loadSettings()]).then(() => {
    centerFocused();
    const left = Math.max(0, BOOT_MIN_MS - (performance.now() - bootAt));
    window.setTimeout(dismissBoot, left);
  });

  if (isMyGo()) {
    // 接着手柄就直接显示 Xbox 键位，不用等玩家先按一下
    void Launcher.padNames()
      .then((names) => {
        if (names.length && !keyboardUsed) usingPad.value = true;
      })
      .catch(() => undefined);

    unsubscribers.push(events.artworkReady.on((update) => lib.applyArtwork(update)));
    unsubscribers.push(
      events.libraryChanged.on((result) => {
        const grew = result.games.length - lib.games.length;
        lib.apply(result.games, result.source, result.roots);
        if (grew > 0) showToast(`发现 ${grew} 部新游戏`);
        void nextTick(centerFocused);
      }),
    );
    unsubscribers.push(
      events.gameRunning.on((state) => {
        lib.setRunning(state.key, state.running);
        showToast(state.running ? `正在运行：${state.title}` : `${state.title} 已退出`);
      }),
    );
  }
});

onUnmounted(() => {
  window.removeEventListener("keydown", onKeydown);
  window.removeEventListener("mousemove", onMousemove);
  window.removeEventListener("resize", centerFocused);
  for (const off of unsubscribers) off();
  if (toastTimer !== undefined) window.clearTimeout(toastTimer);
});
</script>

<template>
  <!-- 最外层：整页共享的那张大图就铺在这一层，首页和网格都透过去 -->
  <div class="app" :style="{ '--accent-glow': accentOf(lib.selected) }">
    <Backdrop :game="lib.selected" />

    <div v-show="!gridOpen" class="home" :class="{ expanded: detailOpen || settingsOpen }">
    <TopBar :name="lib.profileName" :avatar="lib.profileAvatar" @profile="openSettings" />

    <section class="meta" :style="{ '--accent': accentOf(lib.selected) }">
      <p v-if="lib.loading" class="status">正在扫描游戏库…</p>
      <p v-else-if="lib.error" class="status error">{{ lib.error }}</p>
      <p v-else-if="!lib.selected" class="status">游戏库是空的</p>

      <template v-else>
        <img v-if="logoUrl" class="logo" :src="logoUrl" alt="" @error="logoBroken = true" />
        <h1 v-else class="title">{{ lib.selected.title }}</h1>
        <ul class="chips">
          <li v-for="item in meta" :key="item">{{ item }}</li>
        </ul>

        <p v-if="lib.isDemo" class="note">
          演示数据 · 没扫到 Steam / Epic / WeGame 的库
          （可用 <code>LAUNCHER_STEAM_DIR</code> / <code>LAUNCHER_EPIC_DIR</code> /
          <code>LAUNCHER_WEGAME_DIR</code> 指定）
        </p>

      </template>
    </section>

    <section ref="railEl" class="rail" aria-label="游戏">
      <div ref="trackEl" class="track" :style="{ transform: `translate3d(calc(${shift}px + var(--rail-nudge)), 0, 0)` }">
        <template v-for="(game, i) in lib.games" :key="game.key">
          <GameCard
            v-if="inRenderWindow(i)"
            :game="game"
            :focused="i === lib.index"
            :favorite="lib.favorites.has(game.key)"
            :icon-pos="lib.iconPos"
            :recent="game.key === lib.lastPlayedKey"
            @pick="lib.focusKey(game.key)"
            @activate="activate"
          />
          <!-- 占位：顶住轨道宽度，但不创建图片和事件 -->
          <div v-else class="card-slot" aria-hidden="true" />
        </template>
      </div>
    </section>

    <!-- 库超过一屏时给一条发丝进度线：知道自己排到第几个，比空翻好 -->
    <div v-if="lib.games.length > 12" class="rail-progress" aria-hidden="true">
      <span
        class="thumb"
        :style="{ left: `calc(${(lib.index / Math.max(1, lib.games.length - 1)) * 100}% - 3%)` }"
      />
    </div>

    <DetailDrawer
      ref="detailRef"
      :game="lib.selected"
      :open="detailOpen"
      :favorite="favorite"
      :pad="usingPad"
      :kind="padKind"
      :confirming="uninstallAsk"
      :busy="uninstalling"
      @close="closeDetail"
      @launch="launchFromDetail"
      @favorite="favoriteFromDetail"
      @uninstall="askUninstall"
      @hide="hideCurrent"
      @open-folder="openFolder"
      @confirm-uninstall="confirmDestructive"
      @cancel-uninstall="cancelUninstall"
    />

    </div>

    <Transition name="page">
      <LibraryGrid
        v-if="gridOpen"
        ref="gridRef"
        :games="lib.games"
        :start-key="lib.selected?.key"
        :icon-pos="lib.iconPos"
        :favorites="lib.favorites"
        @close="closeGrid"
        @focus="lib.focusKey"
        @select="pickFromGrid"
        @launch="launchFromGrid"
        @detail="detailFromGrid"
        @favorite="favoriteFromGrid"
        @moving="gridMoving = $event"
      />
    </Transition>

    <!-- 下面这三层提到 .app 根上：首页和网格都要用。
         以前它们挂在 .home 里（`v-show="!gridOpen"`），网格一开就整个被藏起来，
         于是「网格里按 S / / 没反应」「保存成功的 toast 看不见」都是同一个原因。 -->
    <SearchBar
      v-if="searchOpen"
      :games="lib.games"
      @pick="pickFromSearch"
      @close="closeSearch"
    />

    <SettingsSheet
      ref="settingsRef"
      :open="settingsOpen"
      @close="settingsOpen = false"
      @rescan="rescan"
      @toggle-fullscreen="toggleFullScreen"
      @added="showToast('已添加 1 部游戏（手动添加的排在库里最前面）')"
      @failed="showToast"
    />

    <Transition name="toast">
      <p v-if="toast" class="toast">{{ toast }}</p>
    </Transition>

    <!-- 启动浮层放在最外层：网格里按 Enter 也要能看到它，
         启动失败时人就该待在网格里，而不是被甩回首页 -->
    <LaunchOverlay
      v-if="launching"
      :title="launching.title"
      :status="launching.status"
      :message="launching.message"
      @close="launching = null"
    />

    <HintBar
      :pad="usingPad"
      :kind="padKind"
      :context="
        searchOpen
          ? 'search'
          : gridMoving
            ? 'move'
            : settingsOpen
              ? 'settings'
              : gridOpen
                ? 'grid'
                : detailOpen
                  ? 'detail'
                  : 'home'
      "
    />

  </div>

</template>

<style scoped>
/* 最外层容器：整页共享的背景（大图 + 光晕 + 暗角）铺在这里 */
.app {
  position: relative;
  height: 100%;
  isolation: isolate;
}

/*
 * 进出「全部游戏」：整页淡入淡出 + 一点点位移/缩放。
 *
 * 用 <Transition> 而不是组件里的 CSS animation：离场也要有动画，
 * 而 v-if 一卸载元素就没了，animation 根本没机会播。
 */
.page-enter-active,
.page-leave-active {
  transition:
    opacity 0.22s var(--ease-out),
    transform 0.26s var(--ease-out);
}

.page-enter-from,
.page-leave-to {
  opacity: 0;
  transform: translateY(1.6vh) scale(0.985);
}

@media (prefers-reduced-motion: reduce) {
  .page-enter-active,
  .page-leave-active {
    transition: none;
  }
}

.app > :deep(.backdrop) {
  z-index: 0;
}


.home {
  position: relative;
  z-index: 1; /* 背景在最外层 .app，内容整体抬一层 */
  height: 100%;
  overflow: hidden;
}

/*
 * 标题块贴着卡片带定位而不是按 top: 24vh —— 退出全屏后窗口变矮，
 * vh 跟着变，标签就会和卡片带错位。改成从底部往上算，关系就稳定了。
 */
/* 窗口化渲染的占位：只占位，不画东西（半径够大，正常视线里看不到） */
.card-slot {
  flex: 0 0 auto;
  width: var(--card-w, 22vh);
  aspect-ratio: 2 / 3;
  border-radius: 12px;
}

/* 大库的位置指示：一条极细的轨道 + 一小段滑块 */
.rail-progress {
  position: absolute;
  z-index: 3;
  left: 50%;
  bottom: calc(11vh - 1.6rem);
  width: min(26rem, 30vw);
  height: 2px;
  transform: translateX(-50%);
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.1);
}

.rail-progress .thumb {
  position: absolute;
  top: 0;
  width: 6%;
  height: 100%;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.65);
  box-shadow: 0 0 10px rgba(255, 255, 255, 0.35);
  transition: left 0.28s var(--ease-out);
}

.meta {
  position: absolute;
  left: 6.5vw;
  bottom: calc(11vh + var(--card-h) + 6.5rem);
  width: min(34rem, 42vw);
  z-index: 3;
}

.logo {
  display: block;
  max-width: 100%;
  max-height: 9rem;
  margin-bottom: 0.4rem;
  object-fit: contain;
  object-position: left center;
  filter: drop-shadow(0 6px 24px rgba(0, 0, 0, 0.55));
}

.title {
  margin: 0;
  font-size: clamp(1.8rem, 3.4vw, 3.1rem);
  font-weight: 700;
  letter-spacing: -0.02em;
  line-height: 1.08;
  text-shadow: 0 4px 30px rgba(0, 0, 0, 0.55);
}

.chips {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
  margin: 1rem 0 0;
  padding: 0;
  list-style: none;
  font-size: 0.8rem;
  color: var(--muted);
}

.chips li {
  padding: 0.28em 0.85em;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.07);
  border: 1px solid rgba(255, 255, 255, 0.08);
}

.note {
  margin: 1rem 0 0;
  font-size: 0.78rem;
  color: var(--faint);
}

.note code {
  padding: 0.1em 0.4em;
  border-radius: 4px;
  background: rgba(255, 255, 255, 0.08);
}

.status {
  margin: 0;
  font-size: 1rem;
  color: var(--muted);
}

.status.error {
  color: #ff9d9d;
}

.toast {
  z-index: var(--z-toast);
  position: absolute;
  left: 50%;
  bottom: 9vh;
  transform: translateX(-50%);
  margin: 0;
  padding: 0.6em 1.4em;
  border-radius: 999px;
  font-size: 0.86rem;
  color: var(--fg);
  background: rgba(12, 14, 20, 0.86);
  border: 1px solid rgba(255, 255, 255, 0.12);
  box-shadow: 0 12px 40px rgba(0, 0, 0, 0.5);
  backdrop-filter: blur(12px);
  pointer-events: none;
}

.toast-enter-active,
.toast-leave-active {
  transition:
    opacity 0.24s var(--ease-out),
    transform 0.24s var(--ease-out);
}

.toast-enter-from,
.toast-leave-to {
  opacity: 0;
  transform: translateX(-50%) translateY(8px);
}

.home {
  /* 卡片带的基准高度：标题块和轨道都从这里推位置 */
  --card-h: clamp(16rem, 34vh, 26rem);
  /* 右侧信息栏宽度；展开时轨道按它的一半让位 */
  --drawer-w: min(28rem, 32vw);
  --rail-nudge: 0px;
}

/* 侧栏占掉右边：轨道按侧栏的一半左移，卡片看起来仍在剩余区域居中 */
.home.expanded {
  --rail-nudge: calc(var(--drawer-w) / -2);
}

/* 展开时右上角状态让位，别压在面板上 */
.home.expanded :deep(.topbar) {
  opacity: 0;
  pointer-events: none;
}

/* 底部提示条也往左收，保持相对剩余区域的居中 */
.home.expanded :deep(.hints) {
  right: var(--drawer-w);
}

.rail {
  --card-w: calc(var(--card-h) * 2 / 3);

  position: absolute;
  left: 0;
  right: 0;
  bottom: 11vh;
  display: flex;
  align-items: flex-end;
  gap: 1.1vw;
  /* 上下留白要容得下选中卡片的放大与上浮，否则顶部会被裁切 */
  padding: 3rem 0 2rem;
  overflow: hidden;
  z-index: var(--z-rail);
  isolation: isolate;
  /* 左右不硬裁，改成渐隐：轨道滑动时卡片从边缘化开，像主机系统那样 */
  -webkit-mask-image: linear-gradient(
    90deg,
    transparent 0,
    #000 5%,
    #000 95%,
    transparent 100%
  );
  mask-image: linear-gradient(90deg, transparent 0, #000 5%, #000 95%, transparent 100%);
}

.track {
  position: relative;
  display: flex;
  align-items: flex-end;
  gap: 1.1vw;
  padding-inline: 6vw;
  transition: transform 0.46s var(--ease-out);
  will-change: transform;
}



</style>
