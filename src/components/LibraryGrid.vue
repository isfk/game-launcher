<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from "vue";
import GameCard from "./GameCard.vue";
import { useLibrary } from "../stores/library";
import { sfx } from "../lib/sfx";
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

// ---- 调整顺序（移动某一张卡）----
//
// 为什么是「拿起—攞位—放下」而不是拖拽：这套界面是手柄优先的，手柄没有指针，
// 但方向键人人都有。鼠标玩家也能点标题栏的「调整顺序」进来，用同一套逻辑。
const moving = ref(false);
/** 移动模式下的临时顺序（确定才写回 setting，取消就丢掉） */
const preview = ref<Game[]>([]);
/** 正在搬的那张卡的 key */
const moveKey = ref("");

/** 界面要显示的那份顺序：移动中看本地的预览，平时看 store 的 */
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
 * cancelFades 掐掉卡片上还在跑的 FLIP 动画。
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

  const before = snapshot();
  const next = [...list];
  const [card] = next.splice(from, 1);
  if (!card) return false;
  next.splice(to, 0, card);
  preview.value = next;
  cursor.value = to; // 焦点跟着卡片走；watch 会把它滚进视口
  void animateFrom(before);
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
  cursor.value = i >= 0 ? i : 0;
  scrollTo(cursor.value, true);
}

/** move 处理二维移动：平时走光标，移动模式下搬卡片。 */
function move(dx: number, dy: number): boolean {
  if (moving.value) return moveCard(dx, dy);

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
  cursor.value = i;
}

/** activate 选中当前这部并把焦点带回首页（移动模式下 = 完成）。 */
function activate(): boolean {
  if (moving.value) return finishMove();
  const game = props.games[cursor.value];
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
  const game = props.games[cursor.value];
  if (!game) return false;
  emit("launch", game.key);
  return true;
}

function openDetail(): boolean {
  const game = props.games[cursor.value];
  if (!game) return false;
  emit("detail", game.key);
  return true;
}

function favorite(): boolean {
  const game = props.games[cursor.value];
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
  startMove,
  finishMove,
  cancelMove,
  isMoving,
});

</script>

<template>
  <div class="library" @click.self="emit('close')">
    <header class="head">
      <div>
        <h2>全部游戏</h2>
        <p class="sub">
          {{ shown.length }} 部 · 每行 {{ COLS }} 部
          <template v-if="moving">
            <span class="sep">·</span>
            <span class="moving-tip">搬动中，方向键换位置</span>
            <button class="action primary" @click="finishMove()">完成</button>
            <button class="action" @click="cancelMove()">取消</button>
          </template>
          <template v-else>
            <span class="sep">·</span>
            <span class="drag-tip">拖动卡片即可调整顺序</span>
            <button class="action" @click="startMove()">用方向键调</button>
          </template>
        </p>
      </div>
      <div class="legend">
        <template v-if="moving">
          <span>↑↓←→ 挪动</span>
          <span>Enter 放下</span>
          <span>Esc 取消</span>
        </template>
        <template v-else>
          <span>拖动卡片 调整顺序</span>
          <span>↑↓←→ 移动</span>
          <span>Enter 启动</span>
          <span>Y 详情</span>
          <span>X 收藏</span>
          <span>M 调整顺序</span>
          <span>/ 搜索</span>
          <span>S 设置</span>
          <span>Esc 关闭</span>
        </template>
        <button class="x" aria-label="关闭" @click="emit('close')">✕</button>
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
          :focused="i === cursor"
          :favorite="isFavorite(game.key)"
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
  animation: fade 0.24s var(--ease-out);
}

.head {
  position: relative;
  z-index: 1;
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  margin-bottom: 2.2vh;
}

h2 {
  margin: 0;
  font-size: 1.6rem;
  letter-spacing: -0.01em;
}

.sub {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.45rem;
  margin: 0.35rem 0 0;
  font-size: 0.78rem;
  color: var(--faint);
  font-variant-numeric: tabular-nums;
}

.sep {
  opacity: 0.5;
}

/* 标题栏里的小动作：调整顺序 / 完成 / 取消 */
.action {
  padding: 0.2em 0.7em;
  border: 1px solid rgba(255, 255, 255, 0.12);
  border-radius: 999px;
  font-size: 0.74rem;
  color: var(--muted);
  background: rgba(255, 255, 255, 0.06);
  transition:
    color 0.2s var(--ease-out),
    background 0.2s var(--ease-out);
}

.action:hover {
  color: #fff;
  background: rgba(255, 255, 255, 0.14);
}

.action.primary {
  color: #10131a;
  background: #e9ecf2;
  border-color: transparent;
}

.action.primary:hover {
  background: #fff;
}

.drag-tip {
  color: rgba(255, 255, 255, 0.5);
}

.moving-tip {
  color: rgba(255, 255, 255, 0.72);
}

.legend {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  align-items: center;
  gap: 0.4rem 1rem;
  font-size: 0.72rem;
  color: var(--faint);
}

.x {
  display: grid;
  place-items: center;
  width: 1.7rem;
  height: 1.7rem;
  border-radius: 50%;
  font-size: 0.75rem;
  color: var(--muted);
  background: rgba(255, 255, 255, 0.08);
}

.x:hover {
  color: #fff;
  background: rgba(255, 255, 255, 0.18);
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
