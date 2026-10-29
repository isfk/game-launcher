<script setup lang="ts">
/**
 * PanelShell 是右侧浮层的公共外壳（详情、设置都用它）。
 *
 * 为什么右侧而不是居中弹窗：居中弹窗会遮住卡片带、把卡片和操作切开，
 * 还得额外管一层 z-index 和焦点陷阱；右侧滑出的面板让左侧内容仍然可见，
 * 一张面板里塞得下更多信息，视觉上也更像主机系统。
 *
 * 左侧一道 18rem 宽的横向蒙版（越往左越透明）让面板和首页背景衔接，
 * 所以不用边框，也没有「一刀切」的分界线。
 * 面板自己的全幅封面需要在组件内部再压一道窄蒙版，那是详情页的事。
 *
 * 结构只有三块，由调用方自己排版：header（可选）/ 默认插槽（正文）/ footer（可选）。
 */
defineProps<{
  /** 展开（滑入）/ 收起（滑出屏幕外） */
  open?: boolean;
  /** 主色，写进 --accent 供子元素取用 */
  accent?: string;
  /** 无障碍名称 */
  label?: string;
}>();
</script>

<template>
  <aside
    class="panel"
    :class="{ open }"
    :style="accent ? { '--accent': accent } : undefined"
    :aria-hidden="open ? undefined : 'true'"
    :aria-label="label"
  >
    <slot />
  </aside>
</template>

<style scoped>
.panel {
  position: absolute;
  top: 0;
  right: 0;
  bottom: 0;
  z-index: var(--z-detail);
  width: var(--drawer-w, min(28rem, 32vw));
  display: flex;
  flex-direction: column;
  background:
    radial-gradient(120% 70% at 100% 0%, color-mix(in oklab, var(--accent, #3a4a6a) 16%, transparent), transparent 62%),
    linear-gradient(180deg, rgba(10, 12, 18, 0.82) 0%, rgba(8, 10, 15, 0.94) 100%);
  backdrop-filter: blur(26px) saturate(1.1);
  box-shadow: -40px 0 120px rgba(0, 0, 0, 0.45);
  transform: translate3d(100%, 0, 0);
  transition: transform 0.42s var(--ease-out);
  will-change: transform;
}

.panel.open {
  transform: translate3d(0, 0, 0);
}

/*
 * 外侧蒙版：越往左越透明，让浮层融进首页背景。
 *
 * 只在展开时显示 —— 面板收起后它被 translateX(100%) 挪到屏幕外，
 * 但这道蒙版还在屏幕里（在面板左边 18rem），会压在另一个浮层上。
 */
.panel::before {
  content: "";
  position: absolute;
  top: 0;
  bottom: 0;
  left: -18rem;
  width: 18rem;
  pointer-events: none;
  opacity: 0;
  transition: opacity 0.32s var(--ease-out);
  background: linear-gradient(
    90deg,
    rgba(8, 10, 15, 0) 0%,
    rgba(8, 10, 15, 0.14) 26%,
    rgba(8, 10, 15, 0.34) 50%,
    rgba(8, 10, 15, 0.58) 72%,
    rgba(8, 10, 15, 0.78) 88%,
    rgba(8, 10, 15, 0.84) 100%
  );
}

.panel.open::before {
  opacity: 1;
}
</style>
