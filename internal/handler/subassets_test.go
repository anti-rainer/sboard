package handler

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"
)

// 验证两级 fs.Sub 能读到 assets 下文件（这是 StaticFS 改自实现后的核心假设）。
func TestSubAssetsReadable(t *testing.T) {
	root := fstest.MapFS{
		"web/index.html":      &fstest.MapFile{Data: []byte("<html></html>")},
		"web/assets/app.css":  &fstest.MapFile{Data: []byte("body{}")},
		"web/assets/app.js":   &fstest.MapFile{Data: []byte("var a=1")},
	}
	webFS, err := fs.Sub(root, "web")
	if err != nil {
		t.Fatalf("fs.Sub(web) 失败: %v", err)
	}
	assetsFS, err := fs.Sub(webFS, "assets")
	if err != nil {
		t.Fatalf("fs.Sub(assets) 失败: %v", err)
	}
	data, err := fs.ReadFile(assetsFS, "app.css")
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if string(data) != "body{}" {
		t.Fatalf("内容不符: %q", data)
	}
}

// 验证 gin 的 *filepath 参数能正确解析出文件名。
func TestGinWildcardParam(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	var got string
	r.GET("/assets/*filepath", func(c *gin.Context) {
		got = c.Param("filepath")
		c.Status(200)
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/assets/app.css", nil)
	r.ServeHTTP(w, req)

	if got != "/app.css" {
		t.Fatalf("filepath = %q，期望 /app.css", got)
	}
	if w.Code != 200 {
		t.Fatalf("状态码 = %d", w.Code)
	}
}
