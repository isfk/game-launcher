<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from "vue";
import Backdrop from "./components/Backdrop.vue";
import DetailDrawer from "./components/DetailDrawer.vue";
import GameCard from "./components/GameCard.vue";
import HintBar from "./components/HintBar.vue";
import LaunchOverlay from "./components/LaunchOverlay.vue";
import SearchBar from "./components/SearchBar.vue";
import SettingsSheet from "./components/SettingsSheet.vue";
import TopBar from "./components/TopBar.vue";
import { usePad, type PadIntent } from "./composables/usePad";
import { accentOf, formatLastPlayed, formatPlaytime, formatSize, providerLabel } from "./lib/format";
import { glyphsFor } from "./lib/glyphs";
import { sfx } from "./lib/sfx";
import { isMyGo } from "mygo-runtime";
import { events, Launcher } from "./mygo";
import { useLibrary } from "./stores/library";

const lib = useLibrary();

const railEl = ref<HTMLElement | null>(null);
const trackEl = ref<HTMLElement | null>(null);
const shift = ref(0);

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
const usingPad = ref(true);
// 真实手柄的键位体系，决定提示条画 A/B/X/Y 还是 ✕○□△ 还是任天堂的排布
const padKind = ref<"xbox" | "playstation" | "switch">("xbox");
// 玩家先用过键盘之后，就别再拿「接了手柄」覆盖他的输入方式
let keyboardUsed = false;
const settingsOpen = ref(false);

// 卸载的二次确认：进入后底部换成确认条，按 Esc / 取消返回。
// 请求在飞时 busy 为 true，按钮禁用，避免连点发两次。
const searchOpen = ref(false);

const uninstallAsk = ref(false);

// 两个面板的竖向焦点：App 只负责把按键翻译成「上/下/确认」，
// 具体焦点停在哪一项由面板自己管（见 composables/usePanelNav.ts）。
const detailRef = ref<{ move: (d: number) => boolean; activate: () => boolean; reset: () => void } | null>(null);
const settingsRef = ref<{ move: (d: number) => boolean; activate: () => boolean; reset: () => void } | null>(null);
const uninstalling = ref(false);
let uninstallTimer: number | undefined;
const launching = ref<{ title: string; status: "starting" | "failed"; message?: string } | null>(null);

const overlayOpen = computed(
  () => detailOpen.value || settingsOpen.value || searchOpen.value || launching.value !== null,
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

  sfx.confirm();
  launching.value = { title: game.title, status: "starting" };
  try {
    await Launcher.launch(game.key);
    // 启动成功就先让「正在启动」停一下：Steam 抢焦点后这个窗口自然会退到后面。
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

function openSearch() {
  if (overlayOpen.value) return;
  sfx.confirm();
  searchOpen.value = true;
}

function closeSearch() {
  if (!searchOpen.value) return;
  sfx.back();
  searchOpen.value = false;
}

function openSettings() {
  if (overlayOpen.value) return;
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

  // 搜索是键盘场景：手柄这边只允许「返回」把它关掉
  if (searchOpen.value) {
    if (intent.kind === "button" && intent.button === "east") closeSearch();
    return;
  }

  if (intent.kind === "nav") {
    // 面板打开时上下（设置里再加左右）在面板内部移动焦点；左右在详情页留给卡片带
    if (settingsOpen.value) {
      if (intent.dir === "up" || intent.dir === "left") settingsRef.value?.move(-1);
      else settingsRef.value?.move(1);
      return;
    }
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
    return;
  }

  switch (intent.button) {
    case "south":
      // 面板打开时确认的是「当前焦点的那个按钮」，不是无脑启动
      if (uninstallAsk.value) void confirmDestructive();
      else if (settingsOpen.value) settingsRef.value?.activate();
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
    case "start":
      if (detailOpen.value) detailOpen.value = false;
      else openSettings();
      break;
    // guide（手柄中间的 Xbox/PS 键）以前直接切全屏，等于又一个
    // 「按一下就掉出全屏」的坑。这里不用它：全屏只留在设置里（键盘另有 F）。
  }
}

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

  // 搜索框自己处理输入：这里只兜住「怎么退出」，别抢方向键
  if (searchOpen.value) {
    if (e.key === "Escape" || e.key === "Enter") {
      e.preventDefault();
      closeSearch();
    }
    return;
  }

  // 详情页是一整页内容，键盘操作按页面语义走：
  // Enter 启动、X 收藏、Esc 返回。
  // 设置是个模态面板：所有导航键都归它，别漏到底下的首页
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
      case "Enter":
      case " ":
        e.preventDefault();
        settingsRef.value?.activate();
        break;
      case "Escape":
      case "Backspace":
        e.preventDefault();
        back();
        break;
      case "s":
      case "S":
        back();
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
  window.addEventListener("keydown", onKeydown);
  window.addEventListener("mousemove", onMousemove);
  window.addEventListener("resize", centerFocused);
  void lib.load().then(centerFocused);
  // 设置（音效开关、收藏）存在后端，启动时读一次并把音效同步过去
  void lib.loadSettings();

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
  <div class="home" :class="{ expanded: detailOpen || settingsOpen }">
    <Backdrop :game="lib.selected" />
    <TopBar :breakdown="lib.providerCounts" />

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
          演示数据 · 没找到 Steam 游戏库（可用 <code>LAUNCHER_STEAM_DIR</code> 指定）
        </p>

      </template>
    </section>

    <section ref="railEl" class="rail" aria-label="游戏">
      <div ref="trackEl" class="track" :style="{ transform: `translate3d(calc(${shift}px + var(--rail-nudge)), 0, 0)` }">
        <GameCard
          v-for="(game, i) in lib.games"
          :key="game.key"
          :game="game"
          :focused="i === lib.index"
          :favorite="lib.favorites.has(game.key)"
          :icon-pos="lib.iconPos"
          @pick="lib.focusKey(game.key)"
          @activate="activate"
        />
      </div>
    </section>

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
      @confirm-uninstall="confirmDestructive"
      @cancel-uninstall="cancelUninstall"
    />

    <SearchBar
      v-if="searchOpen"
      :games="lib.games"
      @pick="lib.focusKey"
      @close="closeSearch"
    />

    <HintBar
      :pad="usingPad"
      :kind="padKind"
      :context="settingsOpen ? 'settings' : detailOpen ? 'detail' : 'home'"
    />

    <Transition name="toast">
      <p v-if="toast" class="toast">{{ toast }}</p>
    </Transition>

    <SettingsSheet
      ref="settingsRef"
      :open="settingsOpen"
      @close="settingsOpen = false"
      @rescan="rescan"
      @toggle-fullscreen="toggleFullScreen"
      @added="showToast('已添加 1 部游戏（手动添加的排在库里最前面）')"
      @failed="showToast"
    />
    <LaunchOverlay
      v-if="launching"
      :title="launching.title"
      :status="launching.status"
      :message="launching.message"
      @close="launching = null"
    />
  </div>
</template>

<style scoped>
.home {
  position: relative;
  height: 100%;
  overflow: hidden;
}

/*
 * 标题块贴着卡片带定位而不是按 top: 24vh —— 退出全屏后窗口变矮，
 * vh 跟着变，标签就会和卡片带错位。改成从底部往上算，关系就稳定了。
 */
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

/*
 * 卡片带自己的压暗层。滚动容器在 Chromium 里会独立合成，
 * 不加这一层，背景光晕会在卡片带顶边留下一条硬边。
 */
.rail::before {
  content: "";
  position: absolute;
  inset: -6vh 0 -12vh 0;
  z-index: -1;
  pointer-events: none;
  background: linear-gradient(
    180deg,
    rgba(5, 6, 10, 0) 0%,
    rgba(5, 6, 10, 0.45) 55%,
    rgba(5, 6, 10, 0.82) 100%
  );
}


</style>
