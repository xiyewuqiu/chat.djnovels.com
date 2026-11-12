package services

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	apimodels "github.com/juggleim/jugglechat-server/apis/models"
	"github.com/juggleim/jugglechat-server/commons/appinfos"
	"github.com/juggleim/jugglechat-server/commons/errs"
	utils "github.com/juggleim/jugglechat-server/commons/tools"
	"github.com/juggleim/jugglechat-server/storages/dbs"
	dbmodels "github.com/juggleim/jugglechat-server/storages/models"
)

const (
	tenantDefaultRetentionDays = 7
	tenantMinRetentionDays     = 1
	tenantMaxRetentionDays     = 30
	tenantSecretLength         = 32
	tenantKeyLength            = 16
)

var tenantAppKeyRegexp = regexp.MustCompile(`^[a-zA-Z0-9_-]{6,32}$`)

// CreateTenant 开通租户：写 apps + 默认保留期配置（appexts），并让应用缓存立刻失效。
// 业务层与引擎共用同一个库，所以写一次两边都生效。
func CreateTenant(ctx context.Context, req *apimodels.CreateTenantReq) (errs.IMErrorCode, *apimodels.CreateTenantResp) {
	appName := strings.TrimSpace(req.AppName)
	if appName == "" {
		return errs.IMErrorCode_APP_REQ_BODY_ILLEGAL, nil
	}
	// 引擎只认 private/alone/public 三种类型，越界值会让新租户直接不可用。
	if req.AppType < int(dbs.AppType_Private) || req.AppType > int(dbs.AppType_Public) {
		return errs.IMErrorCode_APP_REQ_BODY_ILLEGAL, nil
	}
	days := normalizeRetentionDays(req.RetentionDays)

	appKey := strings.TrimSpace(req.AppKey)
	if appKey != "" && !tenantAppKeyRegexp.MatchString(appKey) {
		return errs.IMErrorCode_APP_REQ_BODY_ILLEGAL, nil
	}
	appSecret := utils.RandStr(tenantSecretLength)
	appSecureKey := utils.RandStr(tenantSecretLength)

	storage := dbs.AppInfoDao{}
	if appKey == "" {
		// 生成冲突概率极低，但仍按"生成 → 查重"处理，避免静默覆盖别人的租户。
		for i := 0; i < 3; i++ {
			candidate := strings.ToLower(utils.RandStr(tenantKeyLength))
			if existing, err := storage.FindByAppkey(candidate); err != nil || existing == nil {
				appKey = candidate
				break
			}
		}
		if appKey == "" {
			return errs.IMErrorCode_APP_DEFAULT, nil
		}
	} else if existing, err := storage.FindByAppkey(appKey); err == nil && existing != nil {
		return errs.IMErrorCode_APP_EXISTED, nil
	}

	item := dbs.AppInfoDao{
		AppName:      appName,
		AppKey:       appKey,
		AppSecret:    appSecret,
		AppSecureKey: appSecureKey,
		AppType:      req.AppType,
		CreatedTime:  time.Now(),
		UpdatedTime:  time.Now(),
	}
	if err := storage.Create(item); err != nil {
		return errs.IMErrorCode_APP_DEFAULT, nil
	}

	extDao := dbs.AppExtDao{}
	for key, value := range tenantExtValues(days) {
		if err := extDao.Upsert(appKey, key, value); err != nil {
			// 应用已经建出来了，保留它并如实报错：调用方可以用 info 接口核对后补齐配置。
			appinfos.InvalidateAppInfoCache(appKey)
			return errs.IMErrorCode_APP_DEFAULT, nil
		}
	}
	appinfos.InvalidateAppInfoCache(appKey)

	return errs.IMErrorCode_SUCCESS, &apimodels.CreateTenantResp{
		AppKey:        appKey,
		AppSecret:     appSecret,
		AppSecureKey:  appSecureKey,
		AppName:       appName,
		AppType:       req.AppType,
		RetentionDays: days,
	}
}

// GetTenantInfo 读取租户概览；密钥永不出现在返回里。
func GetTenantInfo(ctx context.Context, appkey string) (errs.IMErrorCode, *apimodels.TenantInfo) {
	appkey = strings.TrimSpace(appkey)
	if appkey == "" {
		return errs.IMErrorCode_APP_APPKEY_REQUIRED, nil
	}
	app, err := dbs.AppInfoDao{}.FindByAppkey(appkey)
	if err != nil || app == nil {
		return errs.IMErrorCode_APP_NOT_EXISTED, nil
	}
	info := &apimodels.TenantInfo{
		AppKey:        app.AppKey,
		AppName:       app.AppName,
		AppType:       app.AppType,
		AppStatus:     app.AppStatus,
		RetentionDays: tenantDefaultRetentionDays,
		CreatedTime:   app.CreatedTime.UnixMilli(),
	}
	if ext, err := (dbs.AppExtDao{}).Find(appkey, "his_msg_save_day"); err == nil && ext != nil {
		if days, err := strconv.Atoi(ext.AppItemValue); err == nil && days > 0 {
			info.RetentionDays = days
		}
	}
	return errs.IMErrorCode_SUCCESS, info
}

// normalizeRetentionDays 钳制到 1–30 天；0 视为未填，回落默认 7 天。
func normalizeRetentionDays(days int) int {
	if days <= 0 {
		return tenantDefaultRetentionDays
	}
	if days < tenantMinRetentionDays {
		return tenantMinRetentionDays
	}
	if days > tenantMaxRetentionDays {
		return tenantMaxRetentionDays
	}
	return days
}

// tenantExtValues 生成租户默认保留期配置。
// 消息保留按天，离线队列按分钟：7 天 = 10080 分钟，与 ops 文档口径一致。
func tenantExtValues(days int) map[string]string {
	minutes := days * 24 * 60
	return map[string]string{
		"his_msg_save_day":          strconv.Itoa(days),
		"offline_msg_save_time":     strconv.Itoa(minutes),
		"offline_cmd_msg_save_time": strconv.Itoa(minutes),
	}
}

// UpdateTenantStatus 启用/停用租户。引擎在 GetAppInfo 里直接拒绝 app_status != 0 的应用，
// 所以停用是真停；但引擎侧有 5 分钟缓存，最晚 5 分钟后全量生效。
func UpdateTenantStatus(ctx context.Context, req *apimodels.UpdateTenantStatusReq) errs.IMErrorCode {
	appkey := strings.TrimSpace(req.AppKey)
	if appkey == "" {
		return errs.IMErrorCode_APP_APPKEY_REQUIRED
	}
	if !validTenantStatus(req.AppStatus) {
		return errs.IMErrorCode_APP_REQ_BODY_ILLEGAL
	}
	storage := dbs.AppInfoDao{}
	app, err := storage.FindByAppkey(appkey)
	if err != nil || app == nil {
		return errs.IMErrorCode_APP_NOT_EXISTED
	}
	if err := storage.UpdateStatus(appkey, req.AppStatus); err != nil {
		return errs.IMErrorCode_APP_DEFAULT
	}
	appinfos.InvalidateAppInfoCache(appkey)
	RecordAdminAudit(ctx, req.OperatorId, dbmodels.AdminAuditAction_AppStatus, appkey, tenantStatusText(req.AppStatus))
	return errs.IMErrorCode_SUCCESS
}

// RotateTenantKeys 轮换密钥。
// 轮换 app_secure_key 会让该租户所有已签发的用户令牌失效（令牌用这把钥匙加密），
// 所有人必须重新登录；这是有意为之的安全动作，调用方要自己判断窗口。
func RotateTenantKeys(ctx context.Context, req *apimodels.RotateTenantKeysReq) (errs.IMErrorCode, *apimodels.RotateTenantKeysResp) {
	appkey := strings.TrimSpace(req.AppKey)
	if appkey == "" {
		return errs.IMErrorCode_APP_APPKEY_REQUIRED, nil
	}
	scope, ok := normalizeRotateScope(req.Scope)
	if !ok {
		return errs.IMErrorCode_APP_REQ_BODY_ILLEGAL, nil
	}
	storage := dbs.AppInfoDao{}
	app, err := storage.FindByAppkey(appkey)
	if err != nil || app == nil {
		return errs.IMErrorCode_APP_NOT_EXISTED, nil
	}

	resp := &apimodels.RotateTenantKeysResp{AppKey: appkey}
	appSecret, appSecureKey := "", ""
	if scope == rotateScopeSecret || scope == rotateScopeBoth {
		appSecret = utils.RandStr(tenantSecretLength)
		resp.AppSecret = appSecret
	}
	if scope == rotateScopeSecureKey || scope == rotateScopeBoth {
		appSecureKey = utils.RandStr(tenantSecretLength)
		resp.AppSecureKey = appSecureKey
	}
	if err := storage.UpdateKeys(appkey, appSecret, appSecureKey); err != nil {
		return errs.IMErrorCode_APP_DEFAULT, nil
	}
	appinfos.InvalidateAppInfoCache(appkey)
	RecordAdminAudit(ctx, req.OperatorId, dbmodels.AdminAuditAction_AppRotate, appkey, scope)
	return errs.IMErrorCode_SUCCESS, resp
}

const (
	rotateScopeSecret    = "secret"
	rotateScopeSecureKey = "secure_key"
	rotateScopeBoth      = "both"
)

// normalizeRotateScope 空值默认只换 app_secret（影响面最小）。
func normalizeRotateScope(scope string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "", rotateScopeSecret:
		return rotateScopeSecret, true
	case rotateScopeSecureKey:
		return rotateScopeSecureKey, true
	case rotateScopeBoth:
		return rotateScopeBoth, true
	}
	return "", false
}

// validTenantStatus 只认 0 正常 / 1 封禁 / 2 过期，与引擎 AppStatus 常量一致。
func validTenantStatus(status int) bool {
	return status >= int(dbs.AppStatus_Normal) && status <= int(dbs.AppStatus_Expire)
}

func tenantStatusText(status int) string {
	switch dbs.AppStatus(status) {
	case dbs.AppStatus_Normal:
		return "正常"
	case dbs.AppStatus_Block:
		return "封禁"
	case dbs.AppStatus_Expire:
		return "过期"
	}
	return "未知"
}
