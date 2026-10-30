<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { accentOf, providerLabel, vivid } from "../lib/format";
import ProviderIcon from "./ProviderIcon.vue";
import type { Game } from "../mygo";

const props = defineProps<{
  game: Game;
  focused: boolean;
  favorite: boolean;
}>();

const emit = defineEmits<{ pick: []; activate: [] }>();

const accent = computed(() => accentOf(props.game));
const cover = computed(() => props.game.artwork?.cover ?? "");
const coverBroken = ref(false);

// 换游戏时重置加载状态，免得上一张的失败影响下一部。
watch(
  () => props.game.key,
  () => {
    coverBroken.value = false;
  },
);
const accentVivid = computed(() => vivid(accent.value));
const badge = computed(() => providerLabel(props.game.provider));
</script>

<template>
  <article
    class="card"
    :class="{ focused, favorite }"
    :style="{ '--accent': accent, '--accent-vivid': accentVivid }"
    @click="emit('pick')"
    @dblclick="emit('activate')"
  >
    <div class="art">
      <img v-if="cover && !coverBroken" :src="cover" alt="" @error="coverBroken = true" />
      <template v-if="!cover || coverBroken">
        <div class="art-fill" />
        <span class="art-glyph">{{ game.title.slice(0, 1) }}</span>
      </template>
      <div class="sheen" />
    </div>

    <div class="label">
      <span class="name">{{ game.title }}</span>
      <span class="provider">
        <ProviderIcon :provider="game.provider" />
        {{ badge }}
      </span>
    </div>

    <span v-if="favorite" class="star" title="已收藏">★</span>
    <span v-if="game.running" class="running" title="正在运行">● 运行中</span>
  </article>
</template>

<style scoped>
.card {
  position: relative;
  flex: 0 0 auto;
  width: var(--card-w, 22vh);
  aspect-ratio: 2 / 3;
  border-radius: 12px;
  overflow: hidden;
  opacity: 0.5;
  transform: scale(0.92);
  filter: saturate(0.85);
  transition:
    transform 0.34s var(--ease-spring),
    opacity 0.3s var(--ease-out),
    filter 0.3s var(--ease-out),
    box-shadow 0.34s var(--ease-out);
  will-change: transform, opacity;
  background: #0b0d14;
}

.card.focused {
  opacity: 1;
  transform: scale(1.08) translateY(-0.8vh);
  filter: saturate(1.1);
  box-shadow:
    0 0 0 2px rgba(255, 255, 255, 0.95),
    0 26px 64px rgba(0, 0, 0, 0.6),
    0 0 120px color-mix(in oklab, var(--accent-vivid) 72%, transparent);
  z-index: 2;
}

.art {
  position: absolute;
  inset: 0;
}

.art img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.art-fill {
  position: absolute;
  inset: 0;
  background:
    radial-gradient(130% 90% at 16% 6%, color-mix(in oklab, var(--accent-vivid) 92%, #fff 8%), transparent 66%),
    linear-gradient(155deg, color-mix(in oklab, var(--accent-vivid) 78%, #0a0c13) 0%, #0a0c13 84%);
}

.art-glyph {
  position: absolute;
  inset: 0;
  display: grid;
  place-items: center;
  font-size: calc(var(--card-w, 22vh) * 0.52);
  font-weight: 700;
  color: rgba(255, 255, 255, 0.16);
  text-shadow: 0 2px 24px rgba(0, 0, 0, 0.4);
}

.sheen {
  position: absolute;
  inset: 0;
  background: linear-gradient(200deg, rgba(255, 255, 255, 0.16) 0%, transparent 42%, rgba(0, 0, 0, 0.45) 100%);
}

.label {
  position: absolute;
  inset: auto 0 0 0;
  display: flex;
  flex-direction: column;
  gap: 0.15em;
  padding: 0.9em 0.85em 0.75em;
  background: linear-gradient(0deg, rgba(3, 4, 8, 0.92) 12%, transparent 100%);
}

.name {
  font-size: 0.86rem;
  font-weight: 600;
  line-height: 1.25;
  overflow: hidden;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
}

.provider {
  display: inline-flex;
  align-items: center;
  gap: 0.32em;
  font-size: 0.68rem;
  letter-spacing: 0.04em;
  color: var(--faint);
}

.running {
  position: absolute;
  top: 0.55em;
  left: 0.6em;
  padding: 0.15em 0.5em;
  border-radius: 999px;
  font-size: 0.62rem;
  letter-spacing: 0.02em;
  color: #eafff2;
  background: rgba(24, 160, 100, 0.86);
  box-shadow: 0 2px 10px rgba(0, 0, 0, 0.35);
}

.star {
  position: absolute;
  top: 0.5em;
  right: 0.6em;
  font-size: 0.85rem;
  color: #ffd479;
  text-shadow: 0 1px 6px rgba(0, 0, 0, 0.6);
}
</style>
