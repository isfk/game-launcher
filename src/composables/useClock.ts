import { onMounted, onUnmounted, ref } from "vue";

/** useClock 返回一个每 10 秒刷新一次的 "HH:MM"。 */
export function useClock() {
  const time = ref(format(new Date()));
  let timer: number | undefined;

  onMounted(() => {
    timer = window.setInterval(() => {
      time.value = format(new Date());
    }, 10_000);
  });

  onUnmounted(() => {
    if (timer !== undefined) window.clearInterval(timer);
  });

  return time;
}

function format(d: Date): string {
  const hh = String(d.getHours()).padStart(2, "0");
  const mm = String(d.getMinutes()).padStart(2, "0");
  return `${hh}:${mm}`;
}
