package services

import (
	"context"
	"strings"

	"github.com/juggleim/jugglechat-server/commons/ctxs"
	"github.com/juggleim/jugglechat-server/commons/errs"
	utils "github.com/juggleim/jugglechat-server/commons/tools"
	"github.com/juggleim/jugglechat-server/storages"
	dbModels "github.com/juggleim/jugglechat-server/storages/models"
)

// MaxUserIdLen 引擎 users.user_id 的列宽（varchar(32)）。
const MaxUserIdLen = 32

// IsValidUserId 校验引擎 user id 能否安全落库。
//
// 与业务层 pkg/validator.IsIMIdentifier 同源（非空、≤32 字节、无控制字符/空白/#//\），
// 并额外拒绝 ':'：私聊 conver_id 的编码是 "<uidA>:<uidB>"
// （引擎 services/commonservices.GetConversationId），id 里带冒号会把一条会话劈成两条。
//
// 业务层签名前已经过滤过一遍，这里仍要再拦一道：/jim/internal/token 是服务端接口，
// 任何拿到服务密钥的调用方都能直接调，不能假设上游一定会校验。
func IsValidUserId(id string) bool {
	if id == "" || len(id) > MaxUserIdLen {
		return false
	}
	for i := 0; i < len(id); i++ {
		switch b := id[i]; {
		case b <= 0x1F || b == 0x7F: // 控制字符（含 \n \r \t）
			return false
		case b == 0x20: // 空白
			return false
		case b == '#', b == '/', b == '\\', b == ':':
			return false
		}
	}
	return true
}

// IssueUserToken 为业务账号签发 IM 凭据（服务端到服务端）。
//
// 身份铁律：**引擎 user_id ≡ 业务账号（account）**。
// 账号就是业务侧的稳定用户标识（业务后端 resolveIMIdentifier 产出），引擎不再自己
// 生成随机 id —— 否则同一个用户在业务库里叫 `zhangsan`、在 IM 里叫 `M_5kJw96BaX`，
// 客服、群成员、会话对端全都对不上号，第三方按账号联调也无从下手。
//
// 语义是"确保存在 + 对齐资料 + 签发"，不是"注册"：
//   - 账号已存在：复用该行，并按业务侧最新资料对齐昵称/头像（业务数据是唯一事实源）
//   - 账号不存在：以账号本身作为 user_id 建一条最小记录，口令设为随机值
//     （该账号永远不走口令登录，口令只是满足表结构的非空约束）
//
// 这样调用方在注册、登录、换设备时都能安全重放，不需要维护
// "IM 用户是否已开通"的状态机，也不会把不可撤销的初始化错误留在库里。
func IssueUserToken(ctx context.Context, account, nickname, avatar string) (userId, token string, code errs.IMErrorCode) {
	appkey := ctxs.GetAppKeyFromCtx(ctx)
	account = strings.TrimSpace(account)
	if appkey == "" || !IsValidUserId(account) {
		return "", "", errs.IMErrorCode_APP_REQ_BODY_ILLEGAL
	}

	nickname = strings.TrimSpace(nickname)
	avatar = strings.TrimSpace(avatar)

	storage := storages.NewUserStorage()
	user, err := storage.FindByAccount(appkey, account)
	if err != nil {
		return "", "", errs.IMErrorCode_APP_DEFAULT
	}

	if user == nil {
		userId = account
		displayName := nickname
		if displayName == "" {
			// 业务侧没给昵称时回落成账号本身，避免引擎侧出现无名用户
			displayName = account
		}
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
		// 对齐业务侧资料：只在真的变了才写，避免每次登录都产生一次无意义 UPDATE。
		// 空值不落库（Update 的语义就是"空 = 不改"），业务侧清空昵称/头像不在本链路支持。
		nickChanged := nickname != "" && nickname != user.Nickname
		avatarChanged := avatar != "" && avatar != user.UserPortrait
		if nickChanged || avatarChanged {
			if err = storage.Update(appkey, userId, nickname, avatar); err != nil {
				return "", "", errs.IMErrorCode_APP_DEFAULT
			}
		}
	}

	token = GenerateToken(appkey, userId)
	if token == "" {
		return "", "", errs.IMErrorCode_APP_DEFAULT
	}
	return userId, token, errs.IMErrorCode_SUCCESS
}
