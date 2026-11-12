package services

import (
	"context"
	"crypto/subtle"
	"fmt"
	"strings"

	"github.com/juggleim/jugglechat-server/commons/ctxs"
	utils "github.com/juggleim/jugglechat-server/commons/tools"
	"github.com/juggleim/jugglechat-server/storages"

	"golang.org/x/crypto/bcrypt"
)

const (
	// passwordHashPrefix 让新旧两种摘要可以共存：带前缀的是 bcrypt，不带的按历史
	// SHA1 理解。上线后不需要一次性迁移全量用户，登录成功时逐个升级即可。
	passwordHashPrefix = "bcrypt$"
	// bcrypt 只处理前 72 字节，超出部分会被截断（与 bcrypt 自身行为一致）。
	bcryptMaxBytes = 72
)

// HashPassword 生成可入库的密码摘要。
func HashPassword(plain string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(truncatePassword(plain)), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return passwordHashPrefix + string(hashed), nil
}

// VerifyPassword 校验密码，同时接受 bcrypt 与历史 SHA1 摘要。
func VerifyPassword(stored, plain string) bool {
	if stored == "" || plain == "" {
		return false
	}
	if strings.HasPrefix(stored, passwordHashPrefix) {
		expected := strings.TrimPrefix(stored, passwordHashPrefix)
		return bcrypt.CompareHashAndPassword([]byte(expected), []byte(truncatePassword(plain))) == nil
	}
	// 历史摘要：40 位十六进制，无盐。等用户下次登录成功后会被升级。
	return subtle.ConstantTimeCompare([]byte(stored), []byte(utils.SHA1(plain))) == 1
}

// PasswordNeedsRehash 报告存量摘要是否需要在登录成功后升级为 bcrypt。
func PasswordNeedsRehash(stored string) bool {
	return stored != "" && !strings.HasPrefix(stored, passwordHashPrefix)
}

// UpgradePasswordHash 把历史 SHA1 摘要就地升级为 bcrypt。失败只记日志：
// 登录已经成功，不能因为升级失败把用户挡在门外。
func UpgradePasswordHash(ctx context.Context, userId, plain string) {
	appkey := ctxs.GetAppKeyFromCtx(ctx)
	if appkey == "" || userId == "" {
		return
	}
	hashed, err := HashPassword(plain)
	if err != nil {
		fmt.Println("[password] hash for upgrade failed:", err)
		return
	}
	if err := storages.NewUserStorage().UpdatePass(appkey, userId, hashed); err != nil {
		fmt.Println("[password] upgrade stored hash failed:", err)
	}
}

func truncatePassword(plain string) string {
	if len(plain) > bcryptMaxBytes {
		return plain[:bcryptMaxBytes]
	}
	return plain
}
