package services

import (
	"context"

	"github.com/juggleim/jugglechat-server/commons/ctxs"
	"github.com/juggleim/jugglechat-server/commons/errs"
	utils "github.com/juggleim/jugglechat-server/commons/tools"
	"github.com/juggleim/jugglechat-server/storages"
	dbModels "github.com/juggleim/jugglechat-server/storages/models"
)

// IssueUserToken 为业务账号签发 IM 凭据（服务端到服务端）。
//
// 语义是"确保存在 + 签发"，不是"注册"：
//   - 账号已存在：直接复用，绝不重置资料或口令
//   - 账号不存在：建一条最小用户记录，口令设为随机值（该账号永远不走口令登录）
//
// 这样调用方在注册、登录、换设备时都能安全重放，不需要维护
// "IM 用户是否已开通"的状态机，也不会把不可撤销的初始化错误留在库里。
func IssueUserToken(ctx context.Context, account, nickname, avatar string) (userId, token string, code errs.IMErrorCode) {
	appkey := ctxs.GetAppKeyFromCtx(ctx)
	if appkey == "" || account == "" {
		return "", "", errs.IMErrorCode_APP_REQ_BODY_ILLEGAL
	}

	storage := storages.NewUserStorage()
	user, err := storage.FindByAccount(appkey, account)
	if err != nil {
		return "", "", errs.IMErrorCode_APP_DEFAULT
	}

	if user == nil {
		userId = utils.GenerateUUIDShort11()
		displayName := nickname
		if displayName == "" {
			displayName = account
		}
		// 随机口令：该账号永远走服务端签发，口令只是满足表结构的非空约束。
		// 用随机值而非派生值，避免"密钥轮换导致全量账号登录失效"这类不可逆故障。
		err = storage.Create(dbModels.User{
			UserId:       userId,
			Nickname:     displayName,
			UserPortrait: avatar,
			LoginAccount: account,
			LoginPass:    utils.SHA1(utils.GenerateUUID().String()),
			AppKey:       appkey,
		})
		if err != nil {
			return "", "", errs.IMErrorCode_APP_DEFAULT
		}
	} else {
		userId = user.UserId
	}

	token = GenerateToken(appkey, userId)
	if token == "" {
		return "", "", errs.IMErrorCode_APP_DEFAULT
	}
	return userId, token, errs.IMErrorCode_SUCCESS
}
