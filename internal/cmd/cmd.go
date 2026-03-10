package cmd

import (
	"context"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gcmd"

	"lakeside/internal/controller/assistant"
	"lakeside/internal/controller/hello"
	"lakeside/internal/controller/itsm"
	"lakeside/internal/openapi"
)

var (
	Main = gcmd.Command{
		Name:  "main",
		Usage: "main",
		Brief: "start http server",
		Func: func(ctx context.Context, parser *gcmd.Parser) (err error) {
			s := g.Server()
			openAPIPath := g.Cfg().MustGet(ctx, "server.openapiPath", "/api.json").String()
			// 在输出 /api.json 前补充业务示例，保持接口定义与示例都集中在 api 包。
			s.BindHookHandler(openAPIPath, ghttp.HookBeforeServe, func(r *ghttp.Request) {
				openapi.PatchServerExamples(s)
			})
			s.Group("/", func(group *ghttp.RouterGroup) {
				group.Middleware(ghttp.MiddlewareHandlerResponse)
				group.Bind(
					assistant.NewV1(),
					hello.NewV1(),
					itsm.NewV1(),
				)
			})
			s.SetPort(8011)
			s.Run()
			return nil
		},
	}
)
