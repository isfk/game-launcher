<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useClock } from "../composables/useClock";
import ProviderIcon from "./ProviderIcon.vue";

/**
 * TopBar 是右上角的状态区：本机库有哪些平台、各多少部，再加账号与时钟。
 *
 * 排版思路：不做「胶囊里塞一行字」。平台按组排开，组间用 1px 发丝线分隔，
 * 平台名走小字号 + 大字距（当标签用），数量用等宽数字加重——
 * 视线先抓到数字，再看到平台，最后才是容器本身（几乎看不见的磨砂底）。
 */
const props = defineProps<{
  /** [{ id: "steam", count: 2 }, ...]，空数组表示只有演示数据 */
  breakdown: Array<{ id: string; count: number }>;
  /** 当前生效的平台筛选，空串 = 全部 */
  active?: string;
  /** 昵称（后端没自定义时是系统账户名） */
  name: string;
  /** 头像的 art:// 地址，空串表示用昵称首字兜底 */
  avatar?: string;
}>();

const emit = defineEmits<{ filter: [id: string]; profile: [] }>();

const clock = useClock();

/** 头像首字：昵称的第一个字（中文就是第一个字，英文大写）。 */
const initial = computed(() => ([...props.name.trim()][0] ?? "?").toUpperCase());

// 图片坏了就退回首字：自定义头像删了、格式奇怪，都不该在右上角留个碎图
const avatarBroken = ref(false);
watch(
  () => props.avatar,
  () => {
    avatarBroken.value = false;
  },
);
const avatarUrl = computed(() => (avatarBroken.value ? "" : (props.avatar ?? "")));

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
        <button
          v-for="item in breakdown"
          :key="item.id"
          class="src"
          :class="{ on: props.active === item.id }"
          :title="props.active === item.id ? '点击显示全部' : `只看 ${labelOf(item.id)}`"
          @click="emit('filter', item.id)"
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

    <button class="who" type="button" :title="`${props.name} · 点击改昵称和头像`" @click="emit('profile')">
      <img v-if="avatarUrl" class="avatar-img" :src="avatarUrl" alt="" @error="avatarBroken = true" />
      <span v-else class="avatar">{{ initial }}</span>
      <span class="name">{{ props.name }}</span>
    </button>

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

/* ---- 账号 ----
/*
 * 头像 + 昵称装进一颗胶囊里，和左边平台统计是同一套材质：
 * 没接自定义资料时左边那组可能整块透明（.flat），这时胶囊就是
 * 右上角唯一一块「有厚度」的东西，它得自己立得住。
 * 高度对齐平台的 1.95rem，两行视线才是平的。
 */
.who {
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
  height: 2.05rem;
  padding: 0 0.8rem 0 0.2rem;
  border: 1px solid rgba(255, 255, 255, 0.06);
  border-radius: 999px;
  background: rgba(10, 12, 18, 0.34);
  box-shadow:
    0 8px 26px rgba(0, 0, 0, 0.3),
    inset 0 1px 0 rgba(255, 255, 255, 0.04);
  backdrop-filter: blur(16px) saturate(1.15);
  color: inherit;
  font: inherit;
  cursor: pointer;
  transition:
    background 0.2s var(--ease-out),
    border-color 0.2s var(--ease-out);
}

.who:hover {
  background: rgba(18, 21, 30, 0.52);
  border-color: rgba(255, 255, 255, 0.12);
}

.avatar,
.avatar-img {
  display: grid;
  place-items: center;
  flex: 0 0 auto;
  width: 1.6rem;
  height: 1.6rem;
  border-radius: 50%;
  font-size: 0.72rem;
  font-weight: 700;
  color: #fff;
  background: linear-gradient(140deg, #4a6cd4, #8a4ad4);
  box-shadow:
    0 0 0 1px rgba(255, 255, 255, 0.14),
    0 4px 14px rgba(0, 0, 0, 0.3);
}

/* 自定义头像：裁成圆，和首字兜底一样大，换过去不会跳一下 */
.avatar-img {
  object-fit: cover;
  background: rgba(255, 255, 255, 0.08);
}

.name {
  /* 昵称最长 24 字：不给个上限会一路挤到时钟跟前，
     长名字宁可省略号，也不要让时钟跟着跳位置 */
  max-width: 12rem;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
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
