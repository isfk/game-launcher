<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from "vue";

/**
 * SearchBar 是首页顶部的搜索框。
 *
 * 为什么是「跳转」而不是「过滤」：卡片带的下标就是焦点下标，过滤会把
 * 玩家正在看的那张卡抽走、还会让 ←→ 的邻居关系突然变掉。这里输入即跳转，
 * 卡片带保持原样，玩家仍然能前后翻。
 */
const props = defineProps<{ games: Array<{ key: string; title: string }> }>();

const emit = defineEmits<{ close: []; pick: [key: string] }>();

const query = ref("");
const inputEl = ref<HTMLInputElement | null>(null);

const matches = computed(() => {
  const q = query.value.trim().toLowerCase();
  if (!q) return [];
  return props.games.filter((g) => g.title.toLowerCase().includes(q));
});

const hint = computed(() => {
  if (!query.value.trim()) return "";
  if (!matches.value.length) return "没有匹配的游戏";
  return `${matches.value.length} 个匹配 · Enter 选定`;
});

onMounted(async () => {
  await nextTick();
  inputEl.value?.focus();
});

// 输入即跳转：把焦点移到第一个匹配项（store 里 index 就是焦点）
watch(matches, (list) => {
  if (list[0]) emit("pick", list[0].key);
});

function onKeydown(e: KeyboardEvent) {
  if (e.key === "Escape") {
    e.preventDefault();
    emit("close");
    return;
  }
  if (e.key === "Enter") {
    e.preventDefault();
    emit("close");
  }
}

defineExpose({ focus: () => inputEl.value?.focus() });
</script>

<template>
  <div class="search" @keydown.stop>
    <span class="icon" aria-hidden="true">🔍</span>
    <input
      ref="inputEl"
      v-model="query"
      class="input"
      type="text"
      placeholder="搜索游戏名…"
      spellcheck="false"
      autocomplete="off"
      @keydown="onKeydown"
    />
    <span v-if="hint" class="hint">{{ hint }}</span>
    <button class="close" aria-label="关闭搜索" @click="emit('close')">✕</button>
  </div>
</template>

<style scoped>
.search {
  position: absolute;
  z-index: var(--z-sheet);
  top: 4vh;
  left: 50%;
  transform: translateX(-50%);
  display: flex;
  align-items: center;
  gap: 0.6rem;
  width: min(34rem, 56vw);
  padding: 0.55rem 0.9rem;
  border-radius: 999px;
  border: 1px solid rgba(255, 255, 255, 0.14);
  background: rgba(12, 14, 20, 0.92);
  backdrop-filter: blur(22px);
  box-shadow: 0 24px 60px rgba(0, 0, 0, 0.55);
  animation: drop 0.22s var(--ease-out);
}

.icon {
  font-size: 0.85rem;
  opacity: 0.6;
}

.input {
  flex: 1 1 auto;
  min-width: 0;
  border: 0;
  outline: 0;
  background: transparent;
  color: var(--fg);
  font-size: 0.95rem;
  font-family: inherit;
}

.input::placeholder {
  color: var(--faint);
}

.hint {
  flex: 0 0 auto;
  font-size: 0.72rem;
  color: var(--faint);
}

.close {
  flex: 0 0 auto;
  display: grid;
  place-items: center;
  width: 1.6rem;
  height: 1.6rem;
  border-radius: 50%;
  font-size: 0.7rem;
  color: var(--muted);
  background: rgba(255, 255, 255, 0.08);
}

.close:hover {
  color: #fff;
  background: rgba(255, 255, 255, 0.18);
}

@keyframes drop {
  from {
    opacity: 0;
    transform: translate(-50%, -8px);
  }
}
</style>
