<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from "vue";
import GameCard from "./GameCard.vue";
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

const emit = defineEmits<{ close: []; select: [key: string]; focus: [key: string] }>();

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

onUnmounted(() => observer?.disconnect());

watch(() => props.games.length, () => nextTick(measureCards));

const cursor = ref(0);

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
  scrollTo(cursor.value);
  const game = props.games[cursor.value];
  if (game) emit("focus", game.key);
});
watch(
  () => props.games.length,
  (n) => {
    if (cursor.value >= n) cursor.value = Math.max(0, n - 1);
  },
);

/** reset 打开时调用：把光标放到首页当前那部上，并居中显示。 */
function reset(startKey?: string) {
  const i = startKey ? props.games.findIndex((g) => g.key === startKey) : -1;
  cursor.value = i >= 0 ? i : 0;
  scrollTo(cursor.value, true);
}

/** move 处理二维移动：左右走一位，上下走一列；到头就停，不绕回。 */
function move(dx: number, dy: number): boolean {
  const total = props.games.length;
  if (!total) return false;

  const next = cursor.value + dx + dy * COLS;
  if (next < 0 || next >= total) return false;
  cursor.value = next;
  return true;
}

/** pick 鼠标落点：把焦点挪到这格，再交给 activate。 */
function pick(i: number) {
  cursor.value = i;
}

/** activate 选中当前这部并把焦点带回首页。 */
function activate(): boolean {
  const game = props.games[cursor.value];
  if (!game) return false;
  emit("select", game.key);
  return true;
}

defineExpose({ move, activate, reset });

</script>

<template>
  <div class="library" @click.self="emit('close')">
    <header class="head">
      <div>
        <h2>全部游戏</h2>
        <p class="sub">{{ games.length }} 部 · 每行 {{ COLS }} 部</p>
      </div>
      <div class="legend">
        <span>↑↓←→ 移动</span>
        <span>Enter 选中</span>
        <span>Esc 关闭</span>
        <button class="x" aria-label="关闭" @click="emit('close')">✕</button>
      </div>
    </header>

    <!-- 每格就是首页那张 GameCard：同一个组件，样式/角标/星标/坏图兜底全都一样 -->
    <div ref="gridEl" class="grid" :style="{ '--cols': COLS, '--card-w': `${cardW}px` }">
      <GameCard
        v-for="(game, i) in games"
        :key="game.key"
        :game="game"
        :focused="i === cursor"
        :favorite="isFavorite(game.key)"
        :icon-pos="pos"
        class="cell"
        @pick="pick(i)"
        @activate="
          pick(i);
          activate();
        "
      />
    </div>

    <p v-if="!games.length" class="empty">这个筛选下没有游戏</p>
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
  margin: 0.35rem 0 0;
  font-size: 0.78rem;
  color: var(--faint);
  font-variant-numeric: tabular-nums;
}

.legend {
  display: flex;
  align-items: center;
  gap: 1rem;
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
  /* 尺寸由 --card-w 决定（GameCard 自己的 .card 宽度），这里不再干预 */
  flex: 0 0 auto;
  /* 焦点放大/上抬沿用 GameCard 的默认值，和首页完全一致 */
  --focus-scale: 1.04;
  --focus-lift: -0.4vh;
  /* 网格里卡片只隔 1vh，选中垫子收紧一点，别糊到邻居身上 */
  --plate-inset: -0.9rem -0.55rem -0.7rem;
  /* 光晕和首页同一个半径（GameCard 的默认 96px），两页观感一致 */
  --card-glow: 96px;
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
