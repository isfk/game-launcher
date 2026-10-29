<script setup lang="ts">
import { useClock } from "../composables/useClock";

defineProps<{ source: string; count: number }>();

const clock = useClock();
</script>

<template>
  <header class="topbar">
    <div class="chip">
      <span class="dot" :class="source === 'steam' ? 'ok' : 'warn'" />
      <span>{{ source === "steam" ? `Steam · ${count} 部` : "演示数据" }}</span>
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
  gap: 1.1rem;
  padding: 2.2vh 3vw;
  font-size: 0.82rem;
  color: var(--muted);
}

.chip {
  display: inline-flex;
  align-items: center;
  gap: 0.5em;
  padding: 0.35em 0.8em;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.06);
  border: 1px solid rgba(255, 255, 255, 0.08);
}

.dot {
  width: 0.45em;
  height: 0.45em;
  border-radius: 50%;
}

.dot.ok {
  background: #57c08a;
  box-shadow: 0 0 8px #57c08a;
}

.dot.warn {
  background: #e0b154;
  box-shadow: 0 0 8px #e0b154;
}

.who {
  display: inline-flex;
  align-items: center;
  gap: 0.55em;
}

.avatar {
  display: grid;
  place-items: center;
  width: 1.9em;
  height: 1.9em;
  border-radius: 50%;
  font-size: 0.9em;
  font-weight: 700;
  color: #fff;
  background: linear-gradient(140deg, #4a6cd4, #8a4ad4);
}

.name {
  color: var(--fg);
  font-weight: 600;
}

.clock {
  font-variant-numeric: tabular-nums;
  font-size: 1.05rem;
  color: var(--fg);
  letter-spacing: 0.02em;
}
</style>
