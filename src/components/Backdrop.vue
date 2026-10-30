<script setup lang="ts">
import { ref, watch } from "vue";
import { accentOf, vivid } from "../lib/format";
import type { Game } from "../mygo";

const props = defineProps<{ game?: Game }>();

// 交叉淡入：TransitionGroup 需要旧的那一层继续留在 DOM 里播 leave，
// 所以这里只保留最新一部，旧的由动画自己走完再从列表消失。
const shown = ref<Game[]>([]);
// 记录加载失败的 hero 地址：失败就只剩主色光晕。
// 记的是地址而不是 key —— 后台补到新的大图后，地址变了就该重新试一次。
const heroFailed = ref<Record<string, string>>({});

/** 没有 hero 大图时拿封面顶上（模糊放大），每部游戏就都有背景了。 */
/** 这个地址是不是刚刚加载失败过。 */
function failed(key: string, src: string): boolean {
  return !!src && heroFailed.value[key] === src;
}

function heroOf(game: Game): string {
  const art = game.artwork;
  if (!art) return "";
  return art.hero || art.cover || "";
}
watch(
  () => props.game,
  (next) => {
    if (!next) return;
    if (shown.value.some((s) => s.key === next.key)) return;
    shown.value = [next];
  },
  { immediate: true },
);
</script>

<template>
  <div class="backdrop">
    <!-- duration 是兜底：万一把 CSS 过渡关掉了（系统「减少动态效果」、
         或者截图时注入 transition:none），transitionend 不会来，
         不显式给时长旧的那一层就会一直挂在 DOM 里。 -->
    <TransitionGroup name="scene" :duration="550">
      <div v-for="g in shown" :key="g.key" class="scene" :style="{ '--accent': accentOf(g), '--accent-vivid': vivid(accentOf(g)) }">
        <img
          v-if="heroOf(g) && !failed(g.key, heroOf(g))"
          class="hero"
          :class="{ 'is-cover': !g.artwork?.hero }"
          :src="heroOf(g)"
          alt=""
          @error="heroFailed[g.key] = heroOf(g)"
        />
        <div class="glow" />
      </div>
    </TransitionGroup>
    <div class="grain" />
  </div>
</template>

<style scoped>
.backdrop {
  position: absolute;
  inset: 0;
  overflow: hidden;
  background: #05060a;
}

.scene {
  position: absolute;
  inset: 0;
}

.hero {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  object-fit: cover;
  transform: scale(1.04);
  filter: blur(1px) saturate(1.06) brightness(0.8);
}

/* 用 2:3 封面顶替 16:9 hero 时，糊得更狠一点，别看出是竖图 */
.hero.is-cover {
  transform: scale(1.35);
  filter: blur(26px) saturate(1.1) brightness(0.62);
}

/* 主色光斑：没有封面时，这就是画面的全部气质。
   两层不同色相（主色 + 偏冷偏移色）重叠，才有 PS5 那种双色光晕感。 */
.glow {
  position: absolute;
  inset: -14%;
  background:
    radial-gradient(
      62% 56% at 16% 58%,
      color-mix(in oklab, var(--accent-vivid) 100%, transparent) 0%,
      transparent 66%
    ),
    radial-gradient(
      44% 48% at 85% 12%,
      color-mix(in oklab, var(--accent-vivid) 62%, #31407e) 0%,
      transparent 76%
    );
  /*
   * 有大图时这层只是「染色」，不能盖住图：原来 opacity:1 会把整张图糊成一片纯色。
   * 没有大图时再让它撒开（下面 :has 规则），那时它就是画面的全部气质。
   */
  opacity: 0.42;
  animation: drift 26s ease-in-out infinite alternate;
}


/* 细颗粒：压掉渐变的色带，带一点胶片味（内联 SVG，不打包图片） */
.grain {
  position: absolute;
  inset: 0;
  opacity: 0.03;
  pointer-events: none;
  background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='160' height='160'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.85' numOctaves='2'/%3E%3C/filter%3E%3Crect width='160' height='160' filter='url(%23n)'/%3E%3C/svg%3E");
}


@keyframes drift {
  from {
    transform: scale(1) translate3d(0, 0, 0);
  }
  to {
    transform: scale(1.09) translate3d(1.5%, -1.5%, 0);
  }
}

.scene-enter-active,
.scene-leave-active {
  transition: opacity 0.55s var(--ease-out);
}

.scene-enter-from,
.scene-leave-to {
  opacity: 0;
}
</style>
