import { ref } from "vue";

/**
 * 最近一次输入来自手柄吗？
 *
 * 提示条要按它切文案，按钮上的键位也要按它画 —— 所以放在模块里共享，
 * 而不是从 App 一层层往下传：面板、网格、详情都可能要用。
 */
export const usingPad = ref(true);

/** 认出来的手柄体系，决定画 A/B/X/Y 还是 ✕○□△ */
export const padKind = ref<"xbox" | "playstation" | "switch">("xbox");
