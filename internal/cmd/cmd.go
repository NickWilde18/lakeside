package cmd

import (
	"context"
	"os"
	"strings"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gcmd"

	"lakeside/internal/controller/agent"
	"lakeside/internal/controller/hello"
	"lakeside/internal/controller/itsm"
	"lakeside/internal/openapi"
	"lakeside/internal/service/agentplatform"
)

var (
	Main = gcmd.Command{
		Name:  "main",
		Usage: "main",
		Brief: "start http server",
		Func: func(ctx context.Context, parser *gcmd.Parser) (err error) {
			validateStartupConfig(ctx)
			mode := strings.ToLower(strings.TrimSpace(os.Getenv("MODE")))
			if mode == "" {
				mode = strings.ToLower(strings.TrimSpace(g.Cfg().MustGet(ctx, "server.mode", "all").String()))
			}
			switch mode {
			case "api":
				startHTTPServer(ctx)
			case "worker":
				if err := agentplatform.GetService(ctx).StartWorkers(ctx); err != nil {
					g.Log().Fatalf(ctx, "start agent workers failed: %v", err)
				}
				g.Log().Info(ctx, "agent worker started, mode=worker")
				select {}
			case "all":
				if err := agentplatform.GetService(ctx).StartWorkers(ctx); err != nil {
					g.Log().Fatalf(ctx, "start agent workers failed: %v", err)
				}
				startHTTPServer(ctx)
			default:
				g.Log().Fatalf(ctx, "invalid MODE=%s, expected api|worker|all", mode)
			}
			return nil
		},
	}
)

func startHTTPServer(ctx context.Context) {
	s := g.Server()
	openAPIPath := g.Cfg().MustGet(ctx, "server.openapiPath", "/api.json").String()
	// 在输出 /api.json 前补充业务示例，保持接口定义与示例都集中在 api 包。
	s.BindHookHandler(openAPIPath, ghttp.HookBeforeServe, func(r *ghttp.Request) {
		openapi.PatchServerExamples(s)
	})
	s.Group("/", func(group *ghttp.RouterGroup) {
		group.Middleware(ghttp.MiddlewareHandlerResponse)
		group.Bind(
			agent.NewV1(),
			hello.NewV1(),
			itsm.NewV1(),
		)
	})
	addr := strings.TrimSpace(g.Cfg().MustGet(ctx, "server.address", ":8011").String())
	if addr != "" {
		s.SetAddr(addr)
	}
	s.Run()
}
