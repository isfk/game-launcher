<script setup lang="ts">
import { computed } from "vue";

/**
 * ProviderIcon 是平台的小角标图标。
 *
 * 图标文件放在 `src/assets/providers/<provider>.svg|png`，这里用
 * `import.meta.glob` 扫出来用——好处是**加文件即生效**，不用改代码；
 * 目录里没有对应文件时退回下面手写的内联 SVG（保底能看，但不追求像）。
 */
const props = defineProps<{
  provider: string;
  /** 边长，默认 1em，跟着字号走 */
  size?: string;
}>();

// ?url + eager：直接拿到打包后的地址字符串，图片不内联进 JS
const files = import.meta.glob("../assets/providers/*.{svg,png,webp}", {
  eager: true,
  query: "?url",
  import: "default",
}) as Record<string, string>;

const box = computed(() => props.size ?? "1em");

/** 按 provider 找图标：优先 svg，其次 png / webp。 */
const src = computed(() => {
  for (const ext of ["svg", "png", "webp"]) {
    const hit = files[`../assets/providers/${props.provider}.${ext}`];
    if (hit) return hit;
  }
  return "";
});
</script>

<template>
  <img
    v-if="src"
    class="provider-icon"
    :src="src"
    :style="{ width: box, height: box }"
    :alt="provider"
    draggable="false"
  />

  <svg
    v-else-if="provider === 'steam'"
    class="provider-icon"
    :style="{ width: box, height: box }"
    viewBox="0 0 16 16"
    aria-hidden="true"
  >
    <circle cx="8" cy="8" r="7.5" fill="#1b2838" />
    <circle cx="8" cy="8" r="2.6" fill="none" stroke="#66c0f4" stroke-width="1.1" />
    <circle cx="10.6" cy="5.6" r="1.15" fill="#66c0f4" />
    <circle cx="5.2" cy="10.4" r="1.15" fill="#66c0f4" />
    <path d="M6.1 9.5 8.9 6.6" stroke="#66c0f4" stroke-width="1.1" stroke-linecap="round" />
  </svg>

  <svg
    v-else-if="provider === 'epic'"
    class="provider-icon"
    :style="{ width: box, height: box }"
    viewBox="0 0 16 16"
    aria-hidden="true"
  >
    <rect width="16" height="16" rx="3.5" fill="#121215" />
    <path d="M5.6 11.6V3.4h4.1v1.5H7.3v1.6h2.2V8H7.3v2.1h2.4v1.5z" fill="#f2f2f2" />
  </svg>

  <svg
    v-else-if="provider === 'wegame'"
    class="provider-icon"
    :style="{ width: box, height: box }"
    viewBox="0 0 16 16"
    aria-hidden="true"
  >
    <rect width="16" height="16" rx="3.5" fill="#2a6bff" />
    <path
      d="M3.4 5.3 4.9 10.7 6.6 6.6 8 10.7 9.6 5.3"
      fill="none"
      stroke="#fff"
      stroke-width="1.25"
      stroke-linecap="round"
      stroke-linejoin="round"
    />
    <path d="M11.4 5.3v5.4" stroke="#fff" stroke-width="1.25" stroke-linecap="round" />
  </svg>

  <svg
    v-else-if="provider === 'manual'"
    class="provider-icon"
    :style="{ width: box, height: box }"
    viewBox="0 0 16 16"
    aria-hidden="true"
  >
    <rect width="16" height="16" rx="3.5" fill="#2b3a34" />
    <path d="M8 5v6M5 8h6" stroke="#4ade80" stroke-width="1.4" stroke-linecap="round" />
  </svg>

  <svg v-else class="provider-icon" :style="{ width: box, height: box }" viewBox="0 0 16 16" aria-hidden="true">
    <rect width="16" height="16" rx="3.5" fill="#2a2d35" />
    <circle cx="8" cy="8" r="2.2" fill="#8a8f99" />
  </svg>
</template>

<style scoped>
.provider-icon {
  display: inline-block;
  flex: 0 0 auto;
  vertical-align: -0.12em;
  border-radius: 0.22em;
  object-fit: contain;
  user-select: none;
}
</style>
