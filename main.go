package main

import (
	_ "lakeside/internal/packed"

	"github.com/gogf/gf/v2/os/gctx"

	"lakeside/internal/cmd"
)

func main() {
	cmd.Main.Run(gctx.GetInitCtx())
}
