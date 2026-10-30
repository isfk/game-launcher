# Provider 图标

放这里，文件名就是 provider id，组件会自动加载（`ProviderIcon.vue` 用
`import.meta.glob` 扫描本目录）：

| 文件名 | 平台 |
|---|---|
| `steam.svg` / `steam.png` | Steam |
| `epic.svg` / `epic.png` | Epic Games |
| `wegame.svg` / `wegame.png` | 腾讯 WeGame |
| `manual.svg` / `manual.png` | 手动添加 |
| `demo.svg` / `demo.png` | 演示数据 |

建议：

- **SVG 优先**（任意尺寸都清晰）；PNG 也行，128×128 或更大、带透明底
- 显示尺寸只有 **14–18px**（卡片角标 / 详情 eyebrow / 顶栏），所以图案要简单
- 自带圆角底块最好（和现在的胶囊徽章风格一致），透明底纯色标也行
- 没放文件的平台会退回组件里画的 fallback 图标
