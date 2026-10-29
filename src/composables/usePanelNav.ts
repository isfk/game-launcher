import { computed, ref, watch } from "vue";

/**
 * NavItem 是面板里一个可聚焦的动作。
 *
 * key 用来在模板里标记 `focused`（`nav.isFocused('launch')`），
 * activate 是这个动作被确认时真正要干的事。
 */
export interface NavItem {
  key: string;
  activate: () => void;
}

/**
 * usePanelNav 处理「面板内部」的竖向焦点移动。
 *
 * 为什么不让浏览器 DOM 焦点来做：这套界面是给手柄设计的，焦点是**画出来的**
 * （accent 描边 + 抬起），不是系统焦点环；而且键盘和手柄两条输入要落到同一个
 * 焦点状态上，DOM 焦点在多面板切换时还会被 WebView 抢走。
 *
 * 焦点状态是一个下标，越界自动回到第一个 —— 面板里的动作数量会随游戏变
 * （有没有卸载按钮、有没有手柄信息），不能假设它是固定的。
 */
export function usePanelNav(items: () => NavItem[]) {
  const index = ref(0);
  const list = computed(items);

  // 动作数量变了（换了游戏、设置项多了少了）就把越界的焦点拉回来
  watch(list, () => {
    if (index.value >= list.value.length) index.value = 0;
  });

  function move(delta: number) {
    const count = list.value.length;
    if (count === 0) return false;
    index.value = (index.value + delta + count) % count;
    return true;
  }

  function focusTo(i: number) {
    if (i >= 0 && i < list.value.length) index.value = i;
  }

  function activate() {
    const item = list.value[index.value];
    if (!item) return false;
    item.activate();
    return true;
  }

  function reset() {
    index.value = 0;
  }

  function isFocused(key: string) {
    return list.value[index.value]?.key === key;
  }

  return { index, list, move, focusTo, activate, reset, isFocused };
}
