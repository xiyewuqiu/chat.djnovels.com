package routers

import (
	"testing"

	"github.com/gin-gonic/gin"
)

// 新加的账号找回与群发现入口必须真的挂上路由；漏挂时接口存在但客户端 404。
func TestFeatureRoutesAreRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	eng := gin.New()
	Route(eng, "jim")

	registered := map[string]bool{}
	for _, route := range eng.Routes() {
		registered[route.Method+" "+route.Path] = true
	}

	want := []string{
		"POST /jim/users/resetpass/send",
		"POST /jim/users/resetpass",
		"POST /jim/groups/discover",
		"POST /jim/groups/setpublic",
		"POST /jim/login",
		"POST /jim/sms/send",
		"POST /jim/groups/mygroups/search",
	}
	for _, route := range want {
		if !registered[route] {
			t.Fatalf("route not registered: %s", route)
		}
	}
}
