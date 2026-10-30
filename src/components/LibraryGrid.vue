<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from "vue";
import GameCard from "./GameCard.vue";
import ProviderStats from "./ProviderStats.vue";
import { useLibrary } from "../stores/library";
import { sfx } from "../lib/sfx";
import { glyphsFor } from "../lib/glyphs";
import { padKind, usingPad } from "../lib/input";
import type { Game } from "../mygo";

/**
 * LibraryGrid 是「全部游戏」的全屏网格。
 *
 * 为什么需要它：首页那条横向带适合「顺着翻几部」，但几百部游戏时
 * 只能一步一步按右方向，扫读体验极差。网格一次能看到 18 部，
 * 上下左右直接跳，配合搜索/筛选才是逛大库的方式。
 *
 * 焦点是画出来的（和高亮环），和首页/侧栏一致；键盘和手柄都落到
 * 同一个 cursor 上，避免双焦点。
 */
const props = defineProps<{
  games: Game[];
  /** 打开时从哪部开始（一般是首页当前选中的那部） */
  startKey?: string;
  /** 平台图标位置，和首页共用同一个设置 */
  iconPos?: string;
  /** 已收藏的 key，星标要和首页一致 */
  favorites?: Set<string>;

}>();

/** 和首页同一套位置语义：上两个角贴封面，下两个角在名称那一行。 */
const pos = computed(() => props.iconPos ?? "bl");

function isFavorite(key: string) {
  return props.favorites?.has(key) ?? false;
}

const emit = defineEmits<{
  close: [];
  /** 选中：把焦点带回首页（鼠标点击走这条） */
  select: [key: string];
  /** 直接启动 / 看详情 / 收藏：手柄 A / Y / X，键盘 Enter / Y / X */
  launch: [key: string];
  detail: [key: string];
  favorite: [key: string];
  focus: [key: string];
  /** 进出「调整顺序」模式（外层据此换提示条、拦按键） */
  moving: [on: boolean];
}>();

const lib = useLibrary();

/** 每行固定 6 部（主机 UI 的常规密度，不做自定义）。 */
const COLS = 6;

/*
 * 不分页（主机 UI 的通用做法）：所有游戏一次性铺进网格，超出高度就纵向滚动。
 * 卡片宽度 = 列宽，行高由卡片自己的 2:3 比例决定，所以只要量一次列宽。
 */
const gridEl = ref<HTMLElement | null>(null);
const cardW = ref(200);
let observer: ResizeObserver | undefined;

function measureCards() {
  const el = gridEl.value;
  if (!el) return;
  const cs = getComputedStyle(el);
  const gapX = parseFloat(cs.columnGap) || 0;
  const padX = (parseFloat(cs.paddingLeft) || 0) + (parseFloat(cs.paddingRight) || 0);
  const colW = (el.clientWidth - padX - gapX * (COLS - 1)) / COLS;
  cardW.value = Math.max(60, Math.floor(colW));
}

onMounted(() => {
  if (gridEl.value) {
    observer = new ResizeObserver(measureCards);
    observer.observe(gridEl.value);
  }
  measureCards();
});

onUnmounted(() => {
  observer?.disconnect();
  window.removeEventListener("pointermove", onPointerMove);
  window.removeEventListener("pointerup", onPointerUp);
  window.removeEventListener("pointercancel", onPointerUp);
  document.body.classList.remove("dragging-card");
});

watch(() => props.games.length, () => nextTick(measureCards));

const cursor = ref(0);

/**
 * 焦点落在哪一区：「卡片」还是「平台 tab」。
 *
 * 主机 UI 的老规矩：一屏里只有一处焦点。第一行卡片按 ↑ 上到平台那行，
 * 点平台胶囊是鼠标的事，焦点该回卡片区，别两处一起亮。
 */
const zone = ref<"cards" | "tabs">("cards");

/**
 * 关闭按钮上的字：手柄画 B / ○ / A，键鼠画 ✕。
 *
 * 关掉这页的方式本来就跟「返回」同一个键，按钮上直接写那个键，
 * 比写一个 ✕ 更省事 —— 手柄玩家不用猜哪个键是 ✕。
 */
const closeCap = computed(() => (usingPad.value ? glyphsFor(padKind.value).back.text : "✕"));
/** 平台 tab 里的光标 */
const tabIndex = ref(0);

/**
 * 平台行的可选项：0 号是「全部」，之后跟着各平台。
 *
 * 顺序必须和 ProviderStats 画出来的胶囊一致 —— 下标就是焦点下标，
 * 少一颗「全部」的话，肩键能轮到的档位和方向键能挑到的档位就对不上了。
 */
const tabIds = computed(() => ["", ...lib.providerCounts.map((it) => it.id)]);

// ---- 调整顺序（拿起 / 挪动 / 放下）----
//
// 手柄和键盘走这条状态机；鼠标拖拽共用同一个 preview，
// 落地都走 lib.setManualOrder，不额外记一套状态。
const moving = ref(false);
/** 移动模式下的临时顺序（确定才写回设置，取消就丢掉） */
const preview = ref<Game[]>([]);
/** 正在搬的那张卡的 key */
const moveKey = ref("");

/** 界面要显示的那份顺序：移动中看本地预览，平时看 store 的 */
const shown = computed(() => (moving.value ? preview.value : props.games));

/** 按 key 记下每张卡当前的位置（FLIP 的「旧位置」）。 */
function snapshot(skip?: string): Map<string, DOMRect> {
  const map = new Map<string, DOMRect>();
  const box = gridEl.value;
  if (!box) return map;
  for (const el of box.children) {
    if (!(el instanceof HTMLElement)) continue;
    const key = el.dataset.key;
    if (key && key !== skip) map.set(key, el.getBoundingClientRect());
  }
  return map;
}

/**
 * cancelCellAnims 掐掉卡片上还在跑的 FLIP 动画。
 *
 * 必须在「量完旧位置」之后、改数据之前做：拖动时每隔几十毫秒就重排一次，
 * 上一次动画常常还没跑完。先量到它此刻的视觉位置（旧位置），再掐掉动画，
 * 等 DOM 重排完从那个位置重新滑过去 —— 位置接得上，也不会两份动画叠着打架。
 */
function cancelCellAnims(skip?: string) {
  const box = gridEl.value;
  if (!box) return;
  for (const el of box.children) {
    if (!(el instanceof HTMLElement) || el.dataset.key === skip) continue;
    for (const anim of el.getAnimations()) anim.cancel();
  }
}

/**
 * FLIP：先让每张卡回到旧位置，再动画回新位置，于是重排看起来是「滑过去」。
 *
 * 动画打在包裹卡片的 .cell 上，不打在 .card 上 —— 卡片自己有
 * `transform: scale(...)` 的焦点样式，两边会互相覆盖。
 */
async function animateFrom(before: Map<string, DOMRect>, skip?: string, duration = 300) {
  await nextTick();
  const box = gridEl.value;
  if (!box) return;
  if (!motionOK()) return;

  for (const el of box.children) {
    if (!(el instanceof HTMLElement)) continue;
    if (el.dataset.key === skip) continue; // 被拖的那张跟着指针走，别抢它的 transform
    const prev = before.get(el.dataset.key ?? "");
    if (!prev) continue;
    const now = el.getBoundingClientRect();
    const dx = prev.left - now.left;
    const dy = prev.top - now.top;
    if (Math.abs(dx) < 1 && Math.abs(dy) < 1) continue;

    el.animate?.(
      [{ transform: `translate(${dx}px, ${dy}px)` }, { transform: "translate(0, 0)" }],
      { duration, easing: "cubic-bezier(.22, .61, .36, 1)" },
    );
  }
}

/** startMove 进入「调整顺序」：从当前光标那部开始搬。 */
function startMove(): boolean {
  if (moving.value || !props.games.length) return false;
  const game = props.games[cursor.value];
  if (!game) return false;

  preview.value = [...props.games];
  moveKey.value = game.key;
  moving.value = true;
  emit("moving", true);
  return true;
}

/** moveCard 把正在搬的卡往前/后挤一格，其余卡平滑让位。 */
function moveCard(dx: number, dy: number): boolean {
  const list = preview.value;
  const from = list.findIndex((g) => g.key === moveKey.value);
  const to = from + dx + dy * COLS;
  if (from < 0 || to < 0 || to >= list.length || to === from) return false;

  const before = snapshot(moveKey.value);
  cancelCellAnims(moveKey.value);

  const next = [...list];
  const [card] = next.splice(from, 1);
  if (!card) return false;
  next.splice(to, 0, card);
  preview.value = next;
  cursor.value = to; // 焦点跟着卡片走；watch 会把它滚进视口
  void animateFrom(before, moveKey.value, 300);
  return true;
}

/** finishMove 确定：把这份顺序交给 store（它会落盘并把排序切到「自定义」）。 */
function finishMove(): boolean {
  if (!moving.value) return false;
  const keys = preview.value.map((g) => g.key);
  const key = moveKey.value;
  moving.value = false;
  preview.value = [];
  emit("moving", false);

  // 先乐观放好光标：store 里那份顺序马上就是这个顺序
  const i = props.games.findIndex((g) => g.key === key);
  if (i >= 0) cursor.value = i;
  void lib.setManualOrder(keys);
  return true;
}

/** cancelMove 取消：什么都不写，光标回到原来那部。 */
function cancelMove(): boolean {
  if (!moving.value) return false;
  const key = moveKey.value;
  moving.value = false;
  preview.value = [];
  emit("moving", false);
  const i = props.games.findIndex((g) => g.key === key);
  if (i >= 0) cursor.value = i;
  return true;
}

function isMoving() {
  return moving.value;
}

// ---- 鼠标 / 触摸：直接拖着卡片换位置 ----
//
// 为什么还要一条拖拽路径：手柄和键盘走「拿起—挪—放下」，
// 鼠标玩家的第一反应却是「把卡片拖过去」。两条路共用同一个 preview，
// 落地也走同一个 setManualOrder，不额外记状态，也就不会两套逻辑打架。

/** 拖动阈值：小于这个距离当成点击（正常选卡片），免得手一抖就进搬动模式 */
const DRAG_SLOP = 6;

type Drag = {
  key: string;
  el: HTMLElement | null;
  pointerId: number;
  /** 按下点相对卡片左上角的偏移 —— 拖动时保持这个抓取点，卡不会跳到指针下面 */
  grabX: number;
  grabY: number;
  startX: number;
  startY: number;
  /** 当前施加的位移（用来从「含 transform 的 rect」反推出布局位置） */
  dx: number;
  dy: number;
  /** 最后一帧的指针位置：重排后要按它重新贴一次，不然会闪一格 */
  px: number;
  py: number;
  active: boolean;
  /** 真的换过位置吗？没换过就不算调整，别顺手把排序改成自定义 */
  changed: boolean;
};

let drag: Drag | null = null;
/** 正在拖的那张卡（要有响应性，卡片才会上浮） */
const dragKey = ref("");
/** 拖完那一下的 click 要吃掉，否则会当成「选中这张卡」 */
let swallowClick = false;

function onPointerDown(e: PointerEvent, i: number) {
  if (e.button !== 0 || !e.isPrimary) return;
  const cell = e.currentTarget as HTMLElement | null;
  const game = shown.value[i];
  if (!cell || !game) return;

  const r = cell.getBoundingClientRect();
  drag = {
    key: game.key,
    el: cell,
    pointerId: e.pointerId,
    grabX: e.clientX - r.left,
    grabY: e.clientY - r.top,
    startX: e.clientX,
    startY: e.clientY,
    dx: 0,
    dy: 0,
    px: e.clientX,
    py: e.clientY,
    active: false,
    changed: false,
  };
  window.addEventListener("pointermove", onPointerMove);
  window.addEventListener("pointerup", onPointerUp);
  window.addEventListener("pointercancel", onPointerUp);
}

/**
 * syncDrag 让被拖的卡贴住指针。
 *
 * 位移直接用「指针 - 抓取点 - 这张卡现在应有的格位」，格位是算出来的
 * （slotRect），不量卡片自己的 rect：卡片正带着上一帧的 transform，
 * 量它再反推容易越算越偏，而换格位、滚动之后算出来的格位总是准的。
 */
function syncDrag(d: Drag) {
  if (!d.el) return;
  const i = preview.value.findIndex((g) => g.key === d.key);
  const slot = i >= 0 ? slotRect(i) : null;
  if (!slot) return;
  d.dx = d.px - d.grabX - slot.left;
  d.dy = d.py - d.grabY - slot.top;
  d.el.style.transform = `translate(${d.dx}px, ${d.dy}px)`;
}

function onPointerMove(e: PointerEvent) {
  const d = drag;
  if (!d || !d.el || e.pointerId !== d.pointerId) return;

  if (!d.active) {
    if (Math.hypot(e.clientX - d.startX, e.clientY - d.startY) < DRAG_SLOP) return;
    // 越过阈值：进入搬动模式，但起点改用「按下时」的位置重新算，手感才连续
    if (!moving.value) {
      preview.value = [...props.games];
      moveKey.value = d.key;
      moving.value = true;
      emit("moving", true);
    }
    d.active = true;
    d.changed = false;
    dragKey.value = d.key;
    cursor.value = Math.max(0, preview.value.findIndex((g) => g.key === d.key));
    document.body.classList.add("dragging-card");
  }

  e.preventDefault();
  d.px = e.clientX;
  d.py = e.clientY;
  syncDrag(d);
  reorderTo(e.clientX, e.clientY);
  autoScroll(e.clientY);
}

/** slotRect 第 i 格在屏幕上的位置（纯几何，不量 DOM，所以动画中途也准）。 */
function slotRect(i: number) {
  const box = gridEl.value;
  if (!box) return null;
  const cs = getComputedStyle(box);
  const padL = parseFloat(cs.paddingLeft) || 0;
  const padT = parseFloat(cs.paddingTop) || 0;
  const gapX = parseFloat(cs.columnGap) || 0;
  const gapY = parseFloat(cs.rowGap) || 0;
  const w = cardW.value;
  const h = w * 1.5;
  const r = box.getBoundingClientRect();
  const col = i % COLS;
  const row = Math.floor(i / COLS);
  const left = r.left + padL + col * (w + gapX);
  const top = r.top + padT + row * (h + gapY) - box.scrollTop;
  return { left, top, width: w, height: h, right: left + w, bottom: top + h };
}

/**
 * indexAt 指针落在第几格。
 *
 * 为什么不用「最近的卡片中心」：那些卡片正在为上一次重排做 FLIP 动画，
 * 量到的位置并不是它们真正的格位；指针停在两格中间时会来回判成 A、B、A、B，
 * 于是卡片一帧一帧地左右弹 —— 就是肉眼看到的抖动。
 * 按网格几何（列宽、行高、间距、滚动量）算出来是稳定值，不跟着动画飘。
 */
function indexAt(x: number, y: number): number {
  const box = gridEl.value;
  if (!box || !preview.value.length) return -1;

  const cs = getComputedStyle(box);
  const padL = parseFloat(cs.paddingLeft) || 0;
  const padT = parseFloat(cs.paddingTop) || 0;
  const gapX = parseFloat(cs.columnGap) || 0;
  const gapY = parseFloat(cs.rowGap) || 0;
  const w = cardW.value;
  const h = w * 1.5; // 和 grid-auto-rows 一致（卡片是 2:3）

  const r = box.getBoundingClientRect();
  const relX = x - r.left - padL;
  const relY = y - r.top - padT + box.scrollTop;

  const col = Math.min(COLS - 1, Math.max(0, Math.floor(relX / (w + gapX))));
  const row = Math.max(0, Math.floor(relY / (h + gapY)));
  return Math.min(preview.value.length - 1, row * COLS + col);
}

/**
 * reorderTo 指针挪到哪一格，就把拖着的卡插到那一格（其余卡滑过去让位）。
 *
 * 两个细节决定手感：
 * 1) 换位要越过目标格 20% 的死区 —— 指针正好压在两格之间的线上时，
 *    差一个像素就会判成左边、再一个像素又判回右边，卡片就左右弹个不停。
 * 2) 滑位动画从「卡片此刻的视觉位置」接着跑，不是从格位重来，
 *    所以哪怕连着换十几格，看起来也是一路滑过去，而不是每格闪一下。
 */
function reorderTo(x: number, y: number) {
  const d = drag;
  if (!d) return;

  const target = indexAt(x, y);
  const from = preview.value.findIndex((g) => g.key === d.key);
  if (target < 0 || from < 0 || target === from) return;

  // 死区：没真正挪进目标格就不动，避免在边界上来回抖
  const slot = slotRect(target);
  if (!slot) return;
  const mx = slot.width * 0.2;
  const my = slot.height * 0.2;
  if (x < slot.left + mx || x > slot.right - mx) return;
  if (y < slot.top + my || y > slot.bottom - my) return;

  const before = snapshot(d.key);
  cancelCellAnims(d.key);

  const next = [...preview.value];
  const [card] = next.splice(from, 1);
  if (!card) return;
  next.splice(target, 0, card);
  preview.value = next;
  cursor.value = target;
  d.changed = true;

  void nextTick(() => {
    if (drag === d) {
      syncDrag(d);
      void animateFrom(before, d.key, 300);
    }
  });
}

/** autoScroll 拖到网格上下边缘时自动滚（不然大库没法往远处搬）。 */
function autoScroll(clientY: number) {
  const box = gridEl.value;
  const d = drag;
  if (!box || !d) return;
  const r = box.getBoundingClientRect();
  const edge = 70;
  const before = box.scrollTop;
  if (clientY < r.top + edge) box.scrollTop -= Math.min(20, (r.top + edge - clientY) / 3);
  else if (clientY > r.bottom - edge) box.scrollTop += Math.min(20, (clientY - (r.bottom - edge)) / 3);
  // 滚完布局变了，卡片得重新贴一次指针
  if (box.scrollTop !== before) syncDrag(d);
}

/**
 * settleDrag 松手时让卡片自己滑回格位。
 *
 * 直接把 transform 清掉是「啪」地跳过去，很突兀 —— 手指（鼠标）只是松开了，
 * 卡片还应该有一下收尾的动量。这里从当前偏移滑到 0，落点就是它该在的格子。
 */
function settleDrag(d: Drag) {
  const el = d.el;
  if (!el) return;
  const { dx, dy } = d;
  el.style.transform = ""; // 先交给动画，别让内联样式盖住关键帧
  if ((!dx && !dy) || !motionOK()) return;

  el.animate([{ transform: `translate(${dx}px, ${dy}px)` }, { transform: "translate(0, 0)" }], {
    duration: 300,
    easing: "cubic-bezier(.2, .72, .3, 1)",
  });
}

/** motionOK 尊重系统的「减少动态效果」。 */
function motionOK() {
  return !window.matchMedia("(prefers-reduced-motion: reduce)").matches;
}

function onPointerUp(e: PointerEvent) {
  const d = drag;
  if (!d) return;
  if (e.type !== "pointercancel" && e.pointerId !== d.pointerId) return;

  window.removeEventListener("pointermove", onPointerMove);
  window.removeEventListener("pointerup", onPointerUp);
  window.removeEventListener("pointercancel", onPointerUp);
  drag = null;
  dragKey.value = "";
  document.body.classList.remove("dragging-card");

  if (!d.active) return; // 只是点了一下：交给 click 走原来的选中逻辑

  settleDrag(d);
  // pointerup 之后紧跟着是 click：那一下不算「选中」，稍后自动放开
  swallowClick = true;
  window.setTimeout(() => {
    swallowClick = false;
  }, 400);

  if (d.changed) {
    // 拖拽的结束点在组件内部，确认音就地播
    if (finishMove()) sfx.confirm();
  } else {
    cancelMove(); // 抖了一下但没换位置：退出搬动模式，不写盘
  }
}

/**
 * 把某张卡滚进视口。
 *
 * 只动 scrollTop，不用 scrollIntoView：后者连水平方向也会滚，
 * 焦点在首/末列时会把网格整体推歪，看起来就像「卡片被裁掉了」。
 */
function scrollTo(i: number, center = false) {
  void nextTick(() => {
    const box = gridEl.value;
    const card = box?.children[i] as HTMLElement | undefined;
    if (!box || !card) return;

    const br = box.getBoundingClientRect();
    const cr = card.getBoundingClientRect();
    const pad = 12; // 给焦点环和 1.04 的放大留点余量

    if (center) {
      box.scrollTop += cr.top - br.top - (box.clientHeight - cr.height) / 2;
      return;
    }
    if (cr.top < br.top + pad) box.scrollTop += cr.top - br.top - pad;
    else if (cr.bottom > br.bottom - pad) box.scrollTop += cr.bottom - br.bottom + pad;
  });
}

// 焦点一动就保证可见，同时把「当前游戏」通知给外层：
// 整页的背景大图跟着光标换（不然一直是打开网格前那一部）
/*
 * 换筛选、重新扫描之后列表会整块变：留下的卡片用 FLIP 滑到新位置，
 * 而不是「唰」地闪一下重排。用 key 序列当触发条件 —— 顺序或成员变了才演，
 * 光标移动、收藏星标这类不改变成员集合的更新不掺和。
 */
watch(
  () => props.games.map((g) => g.key).join("|"),
  () => {
    if (moving.value) return; // 搬动模式有自己的 FLIP，别抢
    const before = snapshot();
    cancelCellAnims();
    void animateFrom(before);
  },
);

watch(cursor, () => {
  if (!drag?.active) scrollTo(cursor.value);
  const game = shown.value[cursor.value];
  if (game) emit("focus", game.key);
});
watch(
  () => props.games.length,
  (n) => {
    if (cursor.value >= n) cursor.value = Math.max(0, n - 1);
  },
);

// 移动中库变了（装了新游戏、后台重扫）：拿新的列表当底，别让预览停在旧快照上
watch(
  () => props.games,
  (list) => {
    if (!moving.value) return;
    const keys = new Set(list.map((g) => g.key));
    const kept = preview.value.filter((g) => keys.has(g.key));
    const known = new Set(kept.map((g) => g.key));
    preview.value = [...kept, ...list.filter((g) => !known.has(g.key))];
  },
);

/** reset 打开时调用：把光标放到首页当前那部上，并居中显示。 */
function reset(startKey?: string) {
  const i = startKey ? props.games.findIndex((g) => g.key === startKey) : -1;
  zone.value = "cards";
  cursor.value = i >= 0 ? i : 0;
  scrollTo(cursor.value, true);
}

/**
 * move 处理二维移动。
 *
 * 三种情形：搬卡片、在平台 tab 行里走、在卡片网格里走。
 * 从第一行往上就进 tab 行 —— 「往上 = 更外层」这套方向感，主机上都这么写。
 */
function move(dx: number, dy: number): boolean {
  if (moving.value) return moveCard(dx, dy);

  const tabs = tabIds.value.length;

  if (zone.value === "tabs") {
    if (dy > 0) {
      // 往下回卡片
      zone.value = "cards";
      return true;
    }
    if (dx && tabs) {
      tabIndex.value = Math.min(tabs - 1, Math.max(0, tabIndex.value + dx));
      // 焦点落在哪颗就筛哪颗：站在平台行上看得到下面列表的变化，
      // 不用再按一次确认（按了也只是确认同一颗）
      applyTab();
      return true;
    }
    return false;
  }

  // 卡片区：最上面一行再往上，就上到平台那一行（焦点落在当前筛选对应的那颗）
  if (dy < 0 && tabs && cursor.value < COLS) {
    const at = tabIds.value.indexOf(lib.platformFilter);
    tabIndex.value = at >= 0 ? at : 0;
    zone.value = "tabs";
    return true;
  }

  const total = props.games.length;
  if (!total) return false;

  const next = cursor.value + dx + dy * COLS;
  if (next < 0 || next >= total) return false;
  cursor.value = next;
  return true;
}

/** pick 鼠标落点：把焦点挪到这格；移动模式下点谁就搬谁。 */
function pick(i: number) {
  // 拖完紧跟的那一下 click 不是「选中」，别把光标又弹回去
  if (swallowClick) {
    swallowClick = false;
    return;
  }
  const game = shown.value[i];
  if (moving.value && game) moveKey.value = game.key;
  zone.value = "cards";
  cursor.value = i;
}

/**
 * stepCard 肩键换卡片：LB / RB 永远只干这件事，不管焦点在哪一行。
 *
 * 方向键是「按格子走」（所以 ↑ 能上到平台行），肩键是「上一张 / 下一张」，
 * 语义不同：在平台行上按肩键，先把焦点收回来再去换卡。
 */
function stepCard(dir: number): boolean {
  if (moving.value) return moveCard(dir, 0);
  if (zone.value === "tabs") zone.value = "cards";
  return move(dir, 0);
}

/**
 * stepPlatform 扳机换平台：LT / RT 在「全部 → 各平台」之间轮转。
 *
 * 和 tab 行是同一份状态：肩键直接改筛选，方向键上移是精确挑那颗。
 * 主机上两种习惯都有，一起给，谁也不碍谁。
 */
function stepPlatform(dir: number): boolean {
  const ids = tabIds.value;
  if (ids.length < 2) return false;
  const at = Math.max(0, ids.indexOf(lib.platformFilter));
  const next = ids[(at + dir + ids.length) % ids.length] ?? "";
  lib.setPlatformFilter(next === lib.platformFilter ? "" : next);
  zone.value = "cards";
  cursor.value = 0;
  return true;
}

/**
 * focusedGame 只在这块屏幕上真的有一张卡被选中时才返回它。
 *
 * 焦点跑到平台那行时，卡片的焦点环是灭的 —— 那就不算选中任何东西。
 * 这时候按 Enter / Y / X 还去动「上一次光标停的那部」，玩家会觉得
 * 自己按了个不该有反应的东西。
 */
function focusedGame(): Game | null {
  if (moving.value || zone.value !== "cards") return null;
  return props.games[cursor.value] ?? null;
}

/** applyTab 让焦点所在的那颗平台胶囊生效，并把光标落到第一部。 */
function applyTab() {
  const id = tabIds.value[tabIndex.value];
  if (id === undefined) return;
  // 已经在「全部」上再确认一次还是全部，不需要 toggle 掉
  if (id === "" && lib.platformFilter === "") return;
  lib.setPlatformFilter(id);
  cursor.value = 0;
}

/**
 * confirmTab 平台行上的「确认」：应用筛选、选中第一部、焦点回到卡片区。
 *
 * 没有这一步的话，站在平台行按确认会去启动「上一次光标停的那部」——
 * 屏幕上根本没有卡片高亮，玩家会觉得按到了不存在的东西。
 */
function confirmTab(): boolean {
  if (tabIds.value.length < 2) return false;
  applyTab();
  zone.value = "cards";
  return true;
}

/** activate 选中当前这部并把焦点带回首页（移动模式下 = 完成）。 */
function activate(): boolean {
  if (moving.value) return finishMove();

  if (zone.value === "tabs") return confirmTab();

  const game = focusedGame();
  if (!game) return false;
  emit("select", game.key);
  return true;
}

/**
 * 网格里也能做事，不只「选中就回家」。
 *
 * 浏览大库时看到一部想玩的，不该被退回首页再按一次启动：
 * 这三下就是首页那三下（A 启动 / Y 详情 / X 收藏），只是光标在网格里。
 */
function launch(): boolean {
  // 站在平台行上没有选中卡片：确认键落到「选中第一部」上，不去开旧的游戏
  if (zone.value === "tabs") return confirmTab();

  const game = focusedGame();
  if (!game) return false;
  emit("launch", game.key);
  return true;
}

function openDetail(): boolean {
  const game = focusedGame();
  if (!game) return false;
  emit("detail", game.key);
  return true;
}

function favorite(): boolean {
  const game = focusedGame();
  if (!game) return false;
  emit("favorite", game.key);
  return true;
}

defineExpose({
  move,
  activate,
  launch,
  openDetail,
  favorite,
  reset,
  stepCard,
  stepPlatform,
  startMove,
  finishMove,
  cancelMove,
  isMoving,
});

</script>

<template>
  <div class="library" @click.self="emit('close')">
    <header class="head">
      <h2>
        全部游戏
        <span class="count">{{ shown.length }}</span>
      </h2>
      <div class="head-right">
        <ProviderStats
          :items="lib.providerCounts"
          :active="lib.platformFilter"
          :focused="zone === 'tabs' ? tabIndex : -1"
          with-all
          :total="lib.providerCounts.reduce((n, it) => n + it.count, 0)"
          @pick="lib.setPlatformFilter"
        />
        <button
          class="x"
          aria-label="关闭"
          :title="`关闭（${closeCap}）`"
          @click="emit('close')"
        >
          {{ closeCap }}
        </button>
      </div>
    </header>

    <!--
      每格是一个 .cell 包裹（不是卡片自己）：FLIP 动画打在包裹上，
      否则会和卡片自己的 `transform: scale()` 焦点样式互相覆盖。
      卡片本身仍然是首页那个 GameCard，样式/角标/星标全都一样。
    -->
    <div ref="gridEl" class="grid" :style="{ '--cols': COLS, '--card-w': `${cardW}px` }">
      <div
        v-for="(game, i) in shown"
        :key="game.key"
        class="cell"
        :class="{ dragging: game.key === dragKey }"
        :data-key="game.key"
        @pointerdown="onPointerDown($event, i)"
      >
        <GameCard
          :game="game"
          :focused="zone === 'cards' && i === cursor"
          :favorite="isFavorite(game.key)"
          :recent="game.key === lib.lastPlayedKey"
          :icon-pos="pos"
          :class="{ lifting: moving && game.key === moveKey }"
          @pick="pick(i)"
          @activate="
            pick(i);
            activate();
          "
        />
      </div>
    </div>

    <p v-if="!shown.length" class="empty">这个筛选下没有游戏</p>
  </div>
</template>

<style scoped>
.library {
  /* 这是一页（和首页平级），不是浮层：正常占满高度，背景就是最外层那张大图 */
  position: relative;
  z-index: 1;
  height: 100%;
  display: flex;
  flex-direction: column;
  padding: 4.5vh 4vw 3vh;
}

.head {
  position: relative;
  z-index: 1;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  margin-bottom: 2.2vh;
}

/* 平台胶囊和关闭按钮同一行，都和标题的视觉中心对齐 */
.head-right {
  display: flex;
  align-items: center;
  gap: 0.7rem;
}

h2 {
  display: flex;
  align-items: baseline;
  gap: 0.55rem;
  margin: 0;
  font-size: 1.6rem;
  letter-spacing: -0.01em;
}

/* 总数跟在大字右边：不另起一行，也不再占一条副标题 */
.count {
  font-size: 0.9rem;
  font-weight: 500;
  color: rgba(255, 255, 255, 0.45);
  font-variant-numeric: tabular-nums;
  letter-spacing: 0.02em;
}

/* 关闭：贴着标题那一行，比标题的视觉高度再大一号，点起来不费劲 */
.x {
  flex: 0 0 auto;
  display: grid;
  place-items: center;
  width: 2.1rem;
  height: 2.1rem;
  border-radius: 50%;
  font-size: 0.95rem;
  line-height: 1;
  color: rgba(255, 255, 255, 0.7);
  background: var(--capsule-bg);
  border: 1px solid var(--capsule-line);
  transition:
    color 0.2s var(--ease-out),
    background 0.2s var(--ease-out),
    transform 0.2s var(--ease-out);
}

.x:hover {
  color: #fff;
  background: rgba(255, 255, 255, 0.18);
  transform: scale(1.05);
}

.x:active {
  transform: scale(0.96);
}

.grid::-webkit-scrollbar {
  display: none;
}

/* 网格本体：只负责摆格子，卡片内部一切样式都来自 GameCard */
.grid {
  position: relative;
  z-index: 1;
  /* basis 0：高度严格等于「剩余空间」，不被内容撑大（否则测行数会自我循环） */
  flex: 1 1 0%;
  min-height: 0;
  display: grid;
  /* 网格始终铺满整行：换列数只让卡片变窄，块宽不动 */
  grid-template-columns: repeat(var(--cols, 6), minmax(0, 1fr));
  /* 行高跟着卡片走（不在 JS 里算行数），全部游戏直接铺下去 */
  grid-auto-rows: calc(var(--card-w, 200px) * 1.5);
  gap: 1vh 0.55vw;
  justify-items: center;
  align-content: start;
  /* 不分页：多出来的部分纵向滚动，滚动条藏起来（主机 UI 不留系统控件） */
  overflow-y: auto;
  overflow-x: hidden;
  overscroll-behavior: contain;
  scrollbar-width: none;
  /*
   * 顶部要留够：焦点卡会放大到 1.08 并上抬 0.8vh，不留白就会顶到标题和副标题上。
   * 左右/底部留 2px 给焦点环（环是往外画的）。
   */
  /*
   * 左右留出「焦点卡放大 + 焦点环」的空间：滚动容器在水平方向也会裁切，
   * 不留白的话第一列/最后一列被激活时会被切掉一条边。
   */
  /*
   * 左右必须比选中卡外那圈光晕（box-shadow 的 96px + 放大 1.04 的溢出）更宽，
   * 否则滚动容器（overflow 会同时裁水平方向）会把第一列/最后一列的光晕切掉——
   * 首页是自由浮动的、没有裁切，两边观感就会不一致。
   */
  padding: 2.4vh 5.5vw 3vh;
}

.cell {
  /* 包裹层只负责占位和 FLIP：宽度跟着卡片走，卡片本体仍是 GameCard */
  width: var(--card-w, 200px);
  /* 焦点放大/上抬沿用 GameCard 的默认值，和首页完全一致 */
  --focus-scale: 1.04;
  --focus-lift: -0.4vh;
  /* 网格里卡片只隔 1vh，选中垫子收紧一点，别糊到邻居身上 */
  --plate-inset: -0.9rem -0.55rem -0.7rem;
  /* 光晕和首页同一个半径（GameCard 的默认 96px），两页观感一致 */
  --card-glow: 96px;
}

/* 正被鼠标拖着的那一格：浮到最上面，指针不再落回它自己身上 */
.cell.dragging {
  z-index: 6;
  cursor: grabbing;
  pointer-events: none;
}

:global(body.dragging-card) {
  cursor: grabbing;
  user-select: none;
}

.empty {
  margin: auto;
  color: var(--faint);
  font-size: 0.85rem;
}

@keyframes fade {
  from {
    opacity: 0;
  }
}
</style>
