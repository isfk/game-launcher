<script setup lang="ts">
import ProviderIcon from "./ProviderIcon.vue";

/**
 * ProviderStats 是「有哪些平台、各多少部」那组胶囊。
 *
 * 原来挂在首页右上角；现在挪到「全部游戏」的标题行里，和关闭按钮同一行 ——
 * 在网格里换平台筛选才是真的有事可做（首页只有一条卡片带，筛选意义不大）。
 *
 * 排版思路照旧：不做「胶囊里塞一行字」，平台按组排开，组间用 1px 发丝线分隔，
 * 平台名走小字号 + 大字距（当标签用），数量用等宽数字加重 ——
 * 视线先抓到数字，再看到平台，最后才是容器本身（几乎看不见的磨砂底）。
 */
const props = defineProps<{
  /** [{ id: "steam", count: 2 }, ...]，空数组表示只有演示数据 */
  items: Array<{ id: string; count: number }>;
  /** 当前生效的平台筛选，空串 = 全部 */
  active?: string;
  /** 焦点落在第几颗胶囊上（-1 / 不给 = 没有焦点，鼠标点的时候也走这个） */
  focused?: number;
}>();

const emit = defineEmits<{ pick: [id: string] }>();

const names: Record<string, string> = {
  steam: "Steam",
  epic: "Epic",
  wegame: "WeGame",
  manual: "手动",
};

function labelOf(id: string) {
  return names[id] ?? id;
}
</script>

<template>
  <div class="sources" :class="{ flat: !props.items.length }">
    <template v-if="props.items.length">
      <button
        v-for="(item, i) in props.items"
        :key="item.id"
        class="src"
        :class="{ on: props.active === item.id, focus: props.focused === i }"
        :title="props.active === item.id ? '点击显示全部' : `只看 ${labelOf(item.id)}`"
        @click="emit('pick', item.id)"
      >
        <ProviderIcon :provider="item.id" size="0.95rem" />
        <span class="label">{{ labelOf(item.id) }}</span>
        <span class="num">{{ item.count }}</span>
      </button>
    </template>
    <span v-else class="src demo">
      <i class="dot" />
      <span class="label">演示数据</span>
    </span>
  </div>
</template>

<style scoped>
.sources {
  display: inline-flex;
  align-items: center;
  height: 1.95rem;
  padding: 0 0.15rem;
  border-radius: 999px;
  background: rgba(10, 12, 18, 0.34);
  border: 1px solid rgba(255, 255, 255, 0.06);
  box-shadow:
    0 8px 26px rgba(0, 0, 0, 0.3),
    inset 0 1px 0 rgba(255, 255, 255, 0.04);
  backdrop-filter: blur(16px) saturate(1.15);
}

/* 只有演示数据时不需要那圈底，留个极简标签就够 */
.sources.flat {
  background: transparent;
  border-color: transparent;
  box-shadow: none;
  backdrop-filter: none;
}

.src {
  position: relative;
  display: inline-flex;
  align-items: center;
  gap: 0.4rem;
  height: 100%;
  padding: 0 0.75rem;
  border: 0;
  border-radius: 999px;
  background: transparent;
  color: inherit;
  font: inherit;
  cursor: pointer;
  transition:
    background 0.2s var(--ease-out),
    opacity 0.2s var(--ease-out);
}

.src:hover {
  background: rgba(255, 255, 255, 0.06);
}

/* 选中的平台：亮起来 + 数量换成主色感的白 */
.src.on {
  background: rgba(255, 255, 255, 0.1);
}

/*
 * 手柄 / 键盘的焦点环：和卡片焦点同一套语言（白环 + 一点外扩），
 * 只是贴在这个小胶囊上，所以用 ring 而不是 inset 阴影。
 */
.src.focus {
  background: rgba(255, 255, 255, 0.16);
  box-shadow: 0 0 0 2px rgba(255, 255, 255, 0.9);
  z-index: 1;
}

.src.on .label {
  color: rgba(255, 255, 255, 0.72);
}

/* 组间发丝线：只在中间画，两端不画 */
.src:not(:last-child)::after {
  content: "";
  position: absolute;
  right: 0;
  top: 50%;
  width: 1px;
  height: 0.9rem;
  transform: translateY(-50%);
  background: rgba(255, 255, 255, 0.09);
}

.label {
  font-size: 0.6rem;
  font-weight: 500;
  letter-spacing: 0.16em;
  text-transform: uppercase;
  color: rgba(255, 255, 255, 0.42);
}

.num {
  font-size: 0.78rem;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  letter-spacing: 0.02em;
  color: rgba(255, 255, 255, 0.9);
}

.dot {
  width: 0.4rem;
  height: 0.4rem;
  border-radius: 50%;
  background: #e0b154;
  box-shadow: 0 0 10px rgba(224, 177, 84, 0.7);
}
</style>
