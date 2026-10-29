<script setup lang="ts">
import { onMounted, ref } from "vue";

const props = defineProps<{
  title: string;
  status: "starting" | "failed";
  message?: string;
}>();

defineEmits<{ close: [] }>();

const dots = ref("");

onMounted(() => {
  if (props.status !== "starting") return;
  const timer = window.setInterval(() => {
    dots.value = dots.value.length >= 3 ? "" : `${dots.value}.`;
  }, 420);
  // 组件卸载时清掉，避免计时器乱跑
  return () => window.clearInterval(timer);
});
</script>

<template>
  <div class="overlay" :class="status">
    <div class="inner">
      <template v-if="status === 'starting'">
        <div class="ring" />
        <h2>{{ title }}</h2>
        <p class="line">正在启动{{ dots }}</p>
        <p class="tip">交给 Steam 客户端接管，进度请留意 Steam 窗口</p>
      </template>

      <template v-else>
        <div class="mark">!</div>
        <h2>{{ title }}</h2>
        <p class="line">启动失败</p>
        <p class="tip">{{ message }}</p>
        <button class="btn" @click="$emit('close')">返回（○ / Esc）</button>
      </template>
    </div>
  </div>
</template>

<style scoped>
.overlay {
  position: absolute;
  inset: 0;
  display: grid;
  place-items: center;
  background: radial-gradient(80% 80% at 50% 45%, rgba(6, 8, 14, 0.86), rgba(2, 3, 6, 0.97));
  animation: fade 0.28s var(--ease-out);
  z-index: var(--z-launch);
}

.inner {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.6rem;
  text-align: center;
  max-width: 40rem;
  padding: 0 2rem;
}

h2 {
  margin: 0.8rem 0 0;
  font-size: 2rem;
  letter-spacing: -0.01em;
}

.line {
  margin: 0;
  color: var(--fg);
  font-size: 0.95rem;
}

.tip {
  margin: 0.2rem 0 0;
  color: var(--faint);
  font-size: 0.82rem;
}

.ring {
  width: 3.2rem;
  height: 3.2rem;
  border-radius: 50%;
  border: 2px solid rgba(255, 255, 255, 0.16);
  border-top-color: #fff;
  animation: spin 0.9s linear infinite;
}

.mark {
  display: grid;
  place-items: center;
  width: 3.2rem;
  height: 3.2rem;
  border-radius: 50%;
  font-size: 1.6rem;
  font-weight: 700;
  color: #ffd7d7;
  background: rgba(224, 90, 90, 0.22);
  border: 1px solid rgba(224, 90, 90, 0.5);
}

.btn {
  margin-top: 1.1rem;
  padding: 0.6em 1.5em;
  border-radius: 999px;
  font-size: 0.86rem;
  background: rgba(255, 255, 255, 0.12);
}

.btn:hover {
  background: rgba(255, 255, 255, 0.2);
}

@keyframes fade {
  from {
    opacity: 0;
  }
}

@keyframes spin {
  to {
    transform: rotate(1turn);
  }
}
</style>
