<script setup lang="ts">
import { computed } from "vue";
import { useClock } from "../composables/useClock";
import ProviderIcon from "./ProviderIcon.vue";

const props = defineProps<{ source: string; count: number }>();

const clock = useClock();

/** source 可能是多平台拼起来的（"steam+epic+wegame"），拆开逐个显示图标。 */
const sources = computed(() =>
  props.source === "demo" || !props.source ? [] : props.source.split("+").filter(Boolean),
);

/** 后端的 source 是多平台拼起来的，这里翻成人话。 */
const label = computed(() => {
  if (props.source === "demo" || !props.source) return "演示数据";
  const names: Record<string, string> = { steam: "Steam", epic: "Epic", wegame: "WeGame", manual: "手动添加" };
  const parts = sources.value.map((id) => names[id] ?? id);
  return `${parts.join(" + ")} · ${props.count} 部`;
});

/** 只有演示数据时点个黄灯；真扫到东西就是绿的。 */
const ok = computed(() => props.source !== "demo" && !!props.source);
</script>

<template>
  <header class="topbar">
    <div class="chip">
      <span class="icons">
        <ProviderIcon v-for="id in sources" :key="id" :provider="id" size="1.05em" />
        <span v-if="!sources.length" class="dot" :class="ok ? 'ok' : 'warn'" />
      </span>
      <span>{{ label }}</span>
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

.icons {
  display: inline-flex;
  align-items: center;
  gap: 0.22em;
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
