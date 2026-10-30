<script setup lang="ts">
import { useClock } from "../composables/useClock";
import ProviderIcon from "./ProviderIcon.vue";

/**
 * TopBar 是右上角的状态区：本机库有哪些平台、各多少部，再加账号与时钟。
 *
 * 排版思路：不做「胶囊里塞一行字」。平台按组排开，组间用 1px 发丝线分隔，
 * 平台名走小字号 + 大字距（当标签用），数量用等宽数字加重——
 * 视线先抓到数字，再看到平台，最后才是容器本身（几乎看不见的磨砂底）。
 */
defineProps<{
  /** [{ id: "steam", count: 2 }, ...]，空数组表示只有演示数据 */
  breakdown: Array<{ id: string; count: number }>;
}>();

const clock = useClock();

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
  <header class="topbar">
    <div class="sources" :class="{ flat: !breakdown.length }">
      <template v-if="breakdown.length">
        <span v-for="item in breakdown" :key="item.id" class="src">
          <ProviderIcon :provider="item.id" size="0.95rem" />
          <span class="label">{{ labelOf(item.id) }}</span>
          <span class="num">{{ item.count }}</span>
        </span>
      </template>
      <span v-else class="src demo">
        <i class="dot" />
        <span class="label">演示数据</span>
      </span>
    </div>

    <div class="who">
      <span class="avatar">M</span>
      <span class="name">Master</span>
    </div>

    <time class="clock">{{ clock }}</time>
  </header>
</template>

<style scoped>
.topbar {
  position: absolute;
  z-index: var(--z-topbar);
  transition: opacity 0.3s var(--ease-out);
  top: 0;
  right: 0;
  display: flex;
  align-items: center;
  gap: 1.15rem;
  padding: 2.2vh 3vw;
}

/* ---- 平台与数量 ---- */
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
  padding: 0 0.75rem;
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

/* ---- 账号 ---- */
.who {
  display: inline-flex;
  align-items: center;
  gap: 0.55rem;
}

.avatar {
  display: grid;
  place-items: center;
  width: 1.7rem;
  height: 1.7rem;
  border-radius: 50%;
  font-size: 0.78rem;
  font-weight: 700;
  color: #fff;
  background: linear-gradient(140deg, #4a6cd4, #8a4ad4);
  box-shadow:
    0 0 0 1px rgba(255, 255, 255, 0.14),
    0 6px 18px rgba(0, 0, 0, 0.35);
}

.name {
  font-size: 0.78rem;
  font-weight: 600;
  letter-spacing: 0.02em;
  color: rgba(255, 255, 255, 0.82);
}

/* ---- 时钟 ---- */
.clock {
  font-variant-numeric: tabular-nums;
  font-size: 1.05rem;
  font-weight: 500;
  letter-spacing: 0.04em;
  color: rgba(255, 255, 255, 0.92);
}
</style>
