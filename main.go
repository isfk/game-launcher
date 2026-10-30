package main

import (
	"context"
	"log"
	"path/filepath"
	"runtime"

	"github.com/egoist/mygo"

	"game-launcher/internal/artcache"
	"game-launcher/internal/asset"
	"game-launcher/internal/config"
	"game-launcher/internal/storeart"
)

func main() {
	// 页面能调用的后端（见 app.go），`mygo generate` 会据此生成 src/mygo.ts。
	launcher := newLauncher()
	mygo.Bind(launcher)

	// 封面 / hero 图的自定义协议，必须在建窗口之前注册。
	if err := mygo.Protocol.Handle(asset.Scheme, artStore.Handler()); err != nil {
		log.Printf("注册 %s:// 协议失败（封面将显示为占位色块）: %v", asset.Scheme, err)
	}

	mygo.App.WhenReady(func() {
		// 封面 / hero 的本地缓存目录：放在系统的 cache 目录下，
		// 第二次启动就能离线秒开，也能从封面取主色。
		if dir, err := mygo.App.Path(mygo.PathCache); err == nil {
			launcher.setCache(artcache.New(filepath.Join(dir, "artwork")))
			// 商店图查询结果单独放一处，别和图片混在一起
			launcher.setStoreArt(storeart.New(filepath.Join(dir, "storeassets")))
		} else {
			log.Printf("取缓存目录失败，图片将只用 CDN: %v", err)
		}

		// 设置的落盘位置：PathUserData 是「配置目录 / 应用名」，首次使用自动创建。
		if dir, err := mygo.App.Path(mygo.PathUserData); err == nil {
			store := config.NewStore(dir)
			log.Printf("设置文件: %s", store.Path())
			launcher.setConfig(store)
		} else {
			log.Printf("取配置目录失败，设置将不会保存: %v", err)
		}

		win := mygo.NewWindow(mygo.WindowOptions{
			Title: "Game Launcher",
			URL:   "/",
			// 主机式启动器：没有标题栏、深色打底（避免加载时白闪）。
			Frameless:       true,
			BackgroundColor: "#05060a",
			// 退出全屏后回到这个尺寸，而不是最小尺寸（否则布局会挤成一团）
			Width:     1600,
			Height:    900,
			MinWidth:  1024,
			MinHeight: 640,
		})

		launcher.attach(win)

		// 在 Steam 里装/删游戏后，首页自己更新，不用重启应用。
		ctx, cancel := context.WithCancel(context.Background())
		go launcher.WatchLibrary(ctx)
		win.OnClose(func(*mygo.CloseEvent) { cancel() })

		// 手柄是全局轮询的，跟窗口有没有焦点无关：游戏里按 A，启动器照样读得到。
		// 记下前台状态，失焦时后端就不再把手柄帧推给界面（见 Launcher.padStates）。
		win.OnFocus(func() { launcher.SetFocused(true) })
		win.OnBlur(func() { launcher.SetFocused(false) })

		// Windows 的任务栏本身是 topmost 的：窗口只铺满显示器还不够，
		// 还得自己也 topmost 才能盖住它。但失焦时必须撤掉，
		// 否则游戏起来后会被这个窗口压在下面。
		if runtime.GOOS == "windows" {
			win.SetAlwaysOnTop(true)
			win.OnFocus(func() { win.SetAlwaysOnTop(true) })
			win.OnBlur(func() { win.SetAlwaysOnTop(false) })
		}

		// 全屏要等窗口真的上屏之后才切：macOS 的原生全屏是「把窗口搬进
		// 一个 Space」，窗口还没显示时这个请求会被忽略（Windows 上是
		// 边框全屏，盖住任务栏）。
		win.OnReadyToShow(func() {
			win.Show()
			win.Focus()
			win.SetFullScreen(true)
		})
	})

	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
