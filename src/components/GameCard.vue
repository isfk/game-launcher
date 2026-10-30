<script setup lang="ts">
import { computed, onBeforeUpdate, onUpdated, ref, watch } from "vue";
import { accentOf, providerLabel, vivid } from "../lib/format";
import ProviderIcon from "./ProviderIcon.vue";
import type { Game } from "../mygo";

const props = defineProps<{
  game: Game;
  focused: boolean;
  favorite: boolean;
  /** 平台图标位置：bl（默认，和名称同一行）/ tl / tr / br */
  iconPos?: string;
}>();

const emit = defineEmits<{ pick: []; activate: [] }>();

const accent = computed(() => accentOf(props.game));
const cover = computed(() => props.game.artwork?.cover ?? "");
const coverBroken = ref(false);

// 换游戏时重置加载状态，免得上一张的失败影响下一部。
watch(
  () => props.game.key,
  () => {
    coverBroken.value = false;
  },
);
const accentVivid = computed(() => vivid(accent.value));
const badge = computed(() => providerLabel(props.game.provider));

/** 贴角模式：除了「左下」以外的三个位置都画到封面角上。 */
const pos = computed(() => props.iconPos ?? "bl");
// 只有上面两个角是贴封面的；左下 / 右下都在「游戏名那一行」里（左端 / 右端），
// 这样「下」就是字面意义的下：跟标题同一条基线。
const corner = computed(() => pos.value === "tl" || pos.value === "tr");
const posClass = computed(() => `icon-${pos.value}`);

/**
 * 换图标位置时的移动动画（FLIP）。
 *
 * 为什么不直接在 CSS 里 transition top/left：换角落时锚点会从 left 变成 right，
 * 那是两个不同的属性，浏览器不会插值，元素只会瞬间跳过去。
 * 所以这里在更新前量一次位置，更新后再量一次，用 Web Animations 从旧位置
 * 「补」到新位置——四个角之间、以及「同行 ↔ 贴角」之间都能滑。
 */
// 这个 ref 可能指向组件实例（ProviderIcon）而不是 DOM 节点，取矩形前要拆一层
// 三个会「动」的元素共用一个 FLIP：图标、游戏名、收藏星标。
// 换位置时它们的布局位置都会变，各自从旧位置滑到新位置，才不会有一个跳、
// 另一个滑的割裂感。ref 可能指向组件实例（ProviderIcon）而不是 DOM 节点，
// 所以取矩形前统一用 node() 拆一层。
const iconEl = ref<unknown>(null);
const nameEl = ref<HTMLElement | null>(null);
const starEl = ref<unknown>(null);

type Box = DOMRect | null;
const prev: { icon: Box; name: Box; star: Box } = { icon: null, name: null, star: null };

/** node 把「组件实例 / DOM 节点 / 空」统一成 DOM 节点。 */
function node(target: unknown): HTMLElement | null {
  if (!target) return null;
  if (target instanceof HTMLElement) return target;
  const el = (target as { $el?: unknown }).$el;
  return el instanceof HTMLElement ? el : null;
}

/** 尊重系统的「减少动态效果」设置。 */
function motionOK() {
  return !window.matchMedia("(prefers-reduced-motion: reduce)").matches;
}

const FLIP_MS = 420;
const FLIP_EASE = "cubic-bezier(.22, 1, .36, 1)";

/** flip 让元素从 before 记录的位置滑动到现在的位置。 */
function flip(target: unknown, before: Box) {
  const el = node(target);
  if (!el || !before || !motionOK()) return;

  const after = el.getBoundingClientRect();
  const dx = before.left - after.left;
  const dy = before.top - after.top;
  if (!dx && !dy) return; // 没动就别白跑一个动画

  el.animate([{ transform: `translate(${dx}px, ${dy}px)` }, { transform: "translate(0, 0)" }], {
    duration: FLIP_MS,
    easing: FLIP_EASE,
  });
}

function measure() {
  prev.icon = node(iconEl.value)?.getBoundingClientRect() ?? null;
  prev.name = nameEl.value?.getBoundingClientRect() ?? null;
  prev.star = node(starEl.value)?.getBoundingClientRect() ?? null;
}

onBeforeUpdate(measure);

onUpdated(() => {
  flip(iconEl.value, prev.icon);
  flip(nameEl.value, prev.name);
  flip(starEl.value, prev.star);
  measure(); // 记下新位置，下次切换时当「旧位置」用
});
</script>

<template>
  <article
    class="card"
    :class="[{ focused, favorite }, posClass]"
    :style="{ '--accent': accent, '--accent-vivid': accentVivid }"
    @click="emit('pick')"
    @dblclick="emit('activate')"
  >
    <div class="art">
      <img v-if="cover && !coverBroken" :src="cover" alt="" @error="coverBroken = true" />
      <template v-if="!cover || coverBroken">
        <div class="art-fill" />
        <span class="art-glyph">{{ game.title.slice(0, 1) }}</span>
      </template>
      <div class="sheen" />
      <!-- 贴角模式：角标做成一个 flex 容器，几个元素共用同一套内边距与对齐，
           换角落时只有位置变，间距和垂直居中都不动 -->
      <div v-if="corner" class="cluster">
        <span v-if="favorite && pos === 'tr'" ref="starEl" class="star-row" title="已收藏">★</span>
        <ProviderIcon ref="iconEl" class="badge" :provider="game.provider" :title="badge" />
      </div>
    </div>

    <div class="label">
      <!-- 默认（左下）：图标 + 游戏名一行，平台信息交给图标表达
           （悬停有全名，详情面板的 eyebrow 也还写着平台） -->
      <ProviderIcon v-if="pos === 'bl'" ref="iconEl" class="label-icon" :provider="game.provider" :title="badge" />
      <span ref="nameEl" class="name">{{ game.title }}</span>
      <ProviderIcon v-if="pos === 'br'" ref="iconEl" class="label-icon end" :provider="game.provider" :title="badge" />
    </div>

    <!-- 右上角的星标已经和平台图标排进行内，这里不再重复画 -->
    <span v-if="favorite && !(corner && pos === 'tr')" ref="starEl" class="star" title="已收藏">★</span>
    <span v-if="game.running" class="running" title="正在运行">● 运行中</span>
  </article>
</template>

<style scoped>
.card {
  position: relative;
  flex: 0 0 auto;
  width: var(--card-w, 22vh);
  aspect-ratio: 2 / 3;
  border-radius: 12px;
  overflow: hidden;
  opacity: 0.5;
  transform: scale(0.92);
  filter: saturate(0.85);
  transition:
    transform 0.34s var(--ease-spring),
    opacity 0.3s var(--ease-out),
    filter 0.3s var(--ease-out),
    box-shadow 0.34s var(--ease-out);
  will-change: transform, opacity;
  background: #0b0d14;
}

.card.focused {
  opacity: 1;
  transform: scale(1.08) translateY(-0.8vh);
  filter: saturate(1.1);
  box-shadow:
    0 0 0 2px rgba(255, 255, 255, 0.95),
    0 26px 64px rgba(0, 0, 0, 0.6),
    0 0 120px color-mix(in oklab, var(--accent-vivid) 72%, transparent);
  z-index: 2;
}

.art {
  position: absolute;
  inset: 0;
}

.art img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.art-fill {
  position: absolute;
  inset: 0;
  background:
    radial-gradient(130% 90% at 16% 6%, color-mix(in oklab, var(--accent-vivid) 92%, #fff 8%), transparent 66%),
    linear-gradient(155deg, color-mix(in oklab, var(--accent-vivid) 78%, #0a0c13) 0%, #0a0c13 84%);
}

.art-glyph {
  position: absolute;
  inset: 0;
  display: grid;
  place-items: center;
  font-size: calc(var(--card-w, 22vh) * 0.52);
  font-weight: 700;
  color: rgba(255, 255, 255, 0.16);
  text-shadow: 0 2px 24px rgba(0, 0, 0, 0.4);
}

.sheen {
  position: absolute;
  inset: 0;
  background: linear-gradient(200deg, rgba(255, 255, 255, 0.16) 0%, transparent 42%, rgba(0, 0, 0, 0.45) 100%);
}

.label {
  position: absolute;
  inset: auto 0 0 0;
  display: flex;
  align-items: flex-start;
  gap: 0.42em;
  padding: 0.9em 0.85em 0.75em;
  background: linear-gradient(0deg, rgba(3, 4, 8, 0.92) 12%, transparent 100%);
}

/* 图标对齐到标题的第一行，不要跟着两行标题垂直居中 */
.label-icon {
  margin-top: 0.16em;
  font-size: 0.86rem;
}

/* 右下：名称占满剩余宽度，图标自然被推到行尾 */
.label-icon.end {
  margin-left: 0.3em;
}

.name {
  flex: 1 1 auto;
  min-width: 0;
  font-size: 0.86rem;
  font-weight: 600;
  line-height: 1.25;
  overflow: hidden;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
}

.provider {
  display: inline-flex;
  align-items: center;
  gap: 0.32em;
  font-size: 0.68rem;
  letter-spacing: 0.04em;
  line-height: 1;
  color: var(--faint);
}

.running {
  position: absolute;
  top: 0.55em;
  left: 0.6em;
  padding: 0.15em 0.5em;
  border-radius: 999px;
  font-size: 0.62rem;
  letter-spacing: 0.02em;
  color: #eafff2;
  background: rgba(24, 160, 100, 0.86);
  box-shadow: 0 2px 10px rgba(0, 0, 0, 0.35);
}

.star {
  position: absolute;
  top: 0.5em;
  right: 0.6em;
  font-size: 0.85rem;
  color: #ffd479;
  text-shadow: 0 1px 6px rgba(0, 0, 0, 0.6);
}

/* ---- 贴角模式的平台图标 ---- */
/*
 * 角标是一个 flex 容器：换角落时只有 top/left/right/bottom 变，
 * 内部间距（gap）、垂直居中、图标字号都不动。
 * 内边距统一用 rem 而不是 em —— em 会跟着图标的 font-size 缩放，
 * 四个角算出来的留白就会不一样，看起来就没对齐。
 */
.cluster {
  position: absolute;
  z-index: 2;
  display: flex;
  align-items: center;
  gap: 0.4rem;
}

.icon-tl .cluster {
  top: 0.55rem;
  left: 0.55rem;
}

.icon-tr .cluster {
  top: 0.55rem;
  right: 0.55rem;
}

.badge {
  font-size: 1.05rem;
  filter: drop-shadow(0 2px 6px rgba(0, 0, 0, 0.55));
}

/* 行内的收藏星标：跟着 flex 走，不再自己绝对定位 */
.star-row {
  font-size: 0.9rem;
  line-height: 1;
  color: #ffd479;
  text-shadow: 0 1px 6px rgba(0, 0, 0, 0.6);
}

/* 图标占了左上角：把「运行中」往下让开，别叠在一起 */
.icon-tl .running {
  top: 2.1rem;
  left: 0.55rem;
}
</style>
