package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"

	"github.com/Sheyiyuan/GoDoIt/core/buildinfo"
	"github.com/Sheyiyuan/GoDoIt/gui/bridge"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--build-info" {
		if err := json.NewEncoder(os.Stdout).Encode(buildinfo.Read()); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	root, err := bridge.ResolveRoot()
	if err != nil {
		panic(fmt.Sprintf("解析 gdit 根目录失败：%v", err))
	}
	app := bridge.NewApp(root)

	err = wails.Run(&options.App{
		Title:     "GoDoIt",
		Width:     1180,
		Height:    760,
		MinWidth:  900,
		MinHeight: 620,
		Frameless: true,
		Mac:       &mac.Options{TitleBar: mac.TitleBarHidden()},
		AssetServer: &assetserver.Options{
			Assets:  assets,
			Handler: bridge.IconHandler(app),
		},
		BackgroundColour: &options.RGBA{R: 245, G: 247, B: 249, A: 1},
		OnStartup:        bridge.Startup(app),
		OnBeforeClose:    bridge.BeforeClose(app),
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("启动 GoDoIt GUI 失败：", err.Error())
	}
}
