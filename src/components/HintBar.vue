<script setup lang="ts">
import { computed } from "vue";
import { glyphsFor } from "../lib/glyphs";

/**
 * HintBar 是底部提示条。
 *
 * pad 为真时显示手柄键位图，否则显示键盘按键；kind 决定键位体系
 * （xbox / playstation / switch，见 lib/glyphs.ts）。
 *
 * context 让提示跟着当前层次走：首页、详情面板、设置面板能做的事不一样，
 * 一直挂着同一排提示等于没提示。
 */
const props = defineProps<{
  pad: boolean;
  kind?: "xbox" | "playstation" | "switch";
  context?: "home" | "detail" | "settings";
}>();

const glyphs = computed(() => glyphsFor(props.kind));
const context = computed(() => props.context ?? "home");

interface Hint {
  /** 键帽文字 */
  cap: string;
  /** 键色 class（手柄用） */
  tone?: string;
  label: string;
}

const hints = computed<Hint[]>(() => {
  const g = glyphs.value;

  if (props.pad) {
    if (context.value === "detail") {
      return [
        { cap: "↑↓", label: "选择" },
        { cap: g.confirm.text, tone: g.confirm.tone, label: "确认" },
        { cap: g.favorite.text, tone: g.favorite.tone, label: "收藏" },
        { cap: g.back.text, tone: g.back.tone, label: "收起" },
      ];
    }
    if (context.value === "settings") {
      return [
        { cap: "↑↓", label: "选择" },
        { cap: g.confirm.text, tone: g.confirm.tone, label: "确认" },
        { cap: g.back.text, tone: g.back.tone, label: "返回" },
      ];
    }
    return [
      { cap: g.confirm.text, tone: g.confirm.tone, label: "启动" },
      { cap: g.detail.text, tone: g.detail.tone, label: "详情" },
      { cap: g.favorite.text, tone: g.favorite.tone, label: "收藏" },
      { cap: g.back.text, tone: g.back.tone, label: "返回" },
      { cap: g.menu, tone: "pill", label: "设置" },
    ];
  }

  if (context.value === "detail") {
    return [
      { cap: "↑↓", label: "选择" },
      { cap: "←→", label: "换游戏" },
      { cap: "Enter", label: "确认" },
      { cap: "X", label: "收藏" },
      { cap: "U", label: "卸载" },
      { cap: "Esc", label: "收起" },
    ];
  }
  if (context.value === "settings") {
    return [
      { cap: "↑↓", label: "选择" },
      { cap: "Enter", label: "确认" },
      { cap: "Esc", label: "返回" },
    ];
  }
  return [
    { cap: "←→", label: "选择" },
    { cap: "↑", label: "详情" },
    { cap: "Enter", label: "启动" },
    { cap: "X", label: "收藏" },
    { cap: "S", label: "设置" },
    { cap: "/", label: "搜索" },
    { cap: "Esc", label: "返回" },
    { cap: "F", label: "全屏" },
    { cap: "R", label: "刷新" },
  ];
});
</script>

<template>
  <footer class="hints">
    <div class="group" :class="pad ? 'pad-hints' : 'key-hints'">
      <span v-for="hint in hints" :key="hint.cap + hint.label" class="item">
        <i v-if="hint.tone === 'pill'" class="pill">{{ hint.cap }}</i>
        <i v-else-if="pad" class="glyph" :class="hint.tone">{{ hint.cap }}</i>
        <kbd v-else>{{ hint.cap }}</kbd>
        {{ hint.label }}
      </span>
    </div>
  </footer>
</template>

<style scoped>
.hints {
  position: absolute;
  z-index: var(--z-hints);
  left: 0;
  right: 0;
  bottom: 3.2vh;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: center;
  gap: 1.4rem;
  font-size: 0.82rem;
  color: var(--muted);
  pointer-events: none;
  transition: right 0.42s var(--ease-out);
}

.item {
  display: inline-flex;
  align-items: center;
  gap: 0.5em;
}

kbd {
  display: inline-grid;
  place-items: center;
  min-width: 1.5em;
  padding: 0.1em 0.4em;
  border-radius: 6px;
  font-family: inherit;
  font-size: 0.8em;
  color: var(--fg);
  background: rgba(255, 255, 255, 0.1);
  border: 1px solid rgba(255, 255, 255, 0.12);
}

.glyph {
  display: grid;
  place-items: center;
  min-width: 1.55em;
  height: 1.55em;
  padding: 0 0.25em;
  border-radius: 999px;
  font-size: 0.78em;
  font-style: normal;
  letter-spacing: 0;
  color: #06070b;
  background: #e9ecf2;
}

/* Xbox 键色：A 绿、B 红、X 蓝、Y 黄 */
.xb-green {
  background: #5cbf4a;
}

.xb-red {
  background: #e0574f;
}

.xb-blue {
  background: #3f8ee0;
}

.xb-yellow {
  background: #e6c341;
}

/* PlayStation：✕ 蓝、○ 红、□ 粉、△ 绿 */
.ps-blue {
  background: #7f9bea;
}

.ps-red {
  background: #e8695f;
}

.ps-pink {
  background: #e884b4;
}

.ps-green {
  background: #6ed3a1;
}

/* Switch：黑白键帽，符号用深灰 */
.sw {
  background: #e9ecf2;
  color: #2b2f3a;
}

.pill {
  padding: 0.15em 0.6em;
  border-radius: 999px;
  font-size: 0.78em;
  font-style: normal;
  color: #06070b;
  background: #e9ecf2;
}

.group {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: center;
  gap: 1.4rem;
}

/* 默认（键盘）只看按键提示，摸到手柄就切回键位图 */
.key-hints {
  color: var(--faint);
}
</style>
