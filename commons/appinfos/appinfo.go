package appinfos

import (
	"sync"
	"time"

	"github.com/juggleim/jugglechat-server/commons/caches"
	"github.com/juggleim/jugglechat-server/commons/emailengines"
	"github.com/juggleim/jugglechat-server/commons/smsengines"
	"github.com/juggleim/jugglechat-server/commons/transengines"
	"github.com/juggleim/jugglechat-server/storages/dbs"
)

var appCache *caches.LruCache
var appLock *sync.RWMutex

func init() {
	appCache = caches.NewLruCacheWithAddReadTimeout("appcaches", 1000, nil, 5*time.Minute, 5*time.Minute)
	appLock = &sync.RWMutex{}
}

var PostMode_Normal int = 0
var PostMode_Friend int = 1

type AppInfo struct {
	AppName      string
	AppKey       string
	AppSecret    string
	AppSecureKey string
	AppStatus    int

	SmsEngine   smsengines.ISmsEngine
	MailEngine  emailengines.IEmailEngine
	TransEngine transengines.ITransEngine
	PostMode    int // 0:normal；1:friend

	ExtMap map[string]interface{}
}

func GetAppInfo(appkey string) (*AppInfo, bool) {
	if obj, exist := appCache.Get(appkey); exist {
		return obj.(*AppInfo), true
	} else {
		appLock.Lock()
		defer appLock.Unlock()
		if obj, exist := appCache.Get(appkey); exist {
			return obj.(*AppInfo), true
		} else {
			dao := &dbs.AppInfoDao{}
			app, err := dao.FindByAppkey(appkey)
			if err == nil && app != nil {
				info := &AppInfo{
					AppName:      app.AppName,
					AppKey:       app.AppKey,
					AppSecret:    app.AppSecret,
					AppSecureKey: app.AppSecureKey,
					AppStatus:    app.AppStatus,
					ExtMap:       make(map[string]interface{}),
				}
				appCache.Add(appkey, info)
				return info, true
			}
			return nil, false
		}
	}
}

// InvalidateAppInfoCache 让新建或改过配置的租户立刻生效，不必等缓存自然过期。
func InvalidateAppInfoCache(appkey string) {
	if appCache != nil {
		appCache.Remove(appkey)
	}
}

func GetAppLock() *sync.RWMutex {
	return appLock
}

var notExist interface{} = struct{}{}

func (app *AppInfo) GetExt(key string) (bool, interface{}) {
	if val, ok := app.ExtMap[key]; ok {
		if val == notExist {
			return false, nil
		}
		return true, val
	} else {
		appLock.Lock()
		defer appLock.Unlock()
		extDao := dbs.AppExtDao{}
		exts, err := extDao.FindByItemKeys(app.AppKey, []string{key})
		if err == nil {
			for _, ext := range exts {
				if ext.AppItemKey == key {
					app.ExtMap[key] = ext.AppItemValue
					return true, ext.AppItemValue
				}
			}
		}
		app.ExtMap[key] = notExist
		return false, nil
	}
}

// ExtServiceKey 是租户级服务密钥在 appexts 里的键名。
//
// 放在 appexts 而不是 apps：引擎只认 apps 的固定列，加列会破坏跟版；
// appexts 本来就是给业务层放配置用的键值表，两边读同一张库。
const ExtServiceKey = "service_key"

// GetServiceKey 读取租户级服务密钥，走 LRU 缓存（5 分钟；开通与轮换时会主动失效）。
//
// ok=false 表示该租户尚未下发服务密钥（存量租户）。调用方必须据此拒绝请求，
// 绝不能回落到平台级密钥——回落一次，租户隔离就没了。
//
// DB 不可用时 GetAppInfo 会 panic（dbcommons 未初始化或连接不可用）。鉴权入口不能
// 因此炸成 500：拿不到密钥就等于校验失败，fail closed。
func GetServiceKey(appkey string) (key string, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			key, ok = "", false
		}
	}()
	app, found := GetAppInfo(appkey)
	if !found {
		return "", false
	}
	extFound, val := app.GetExt(ExtServiceKey)
	if !extFound {
		return "", false
	}
	key, isString := val.(string)
	if !isString || key == "" {
		return "", false
	}
	return key, true
}

func (app *AppInfo) GetExtByCreator(key string, creator func(val string) interface{}) (bool, interface{}) {
	if val, ok := app.ExtMap[key]; ok {
		if val == notExist {
			return false, nil
		}
		return true, val
	} else {
		appLock.Lock()
		defer appLock.Unlock()
		extDao := dbs.AppExtDao{}
		exts, err := extDao.FindByItemKeys(app.AppKey, []string{key})
		if err == nil {
			for _, ext := range exts {
				if ext.AppItemKey == key {
					obj := creator(ext.AppItemValue)
					app.ExtMap[key] = obj
					return true, obj
				}
			}
		}
		app.ExtMap[key] = notExist
		return false, nil
	}
}
