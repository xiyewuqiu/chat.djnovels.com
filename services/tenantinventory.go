package services

import (
	"context"
	"sort"
	"strings"

	apimodels "github.com/juggleim/jugglechat-server/apis/models"
	"github.com/juggleim/jugglechat-server/commons/dbcommons"
	"github.com/juggleim/jugglechat-server/commons/errs"
	"github.com/juggleim/jugglechat-server/storages/dbs"
)

// TenantDataInventory 统计某租户在库里各表的行数（干跑，只读）。
//
// 为什么表清单要动态发现：引擎与业务层共用一个库、上百张表，靠手写清单维护必然漏表；
// 漏一张表的后果是清理时以为删干净了，实际留有残渣——这种事故最难查。
// 所以从 information_schema 取「所有带 app_key 列的表」，上游新增表也会自动纳入。
//
// 为什么先做干跑而不是直接删：删除不可逆。先让人看清体量与分布，
// 确认这是要动的那个租户、确认没有意外的巨表，再谈真删。
func TenantDataInventory(ctx context.Context, appkey string) (errs.IMErrorCode, *apimodels.TenantInventoryResp) {
	appkey = strings.TrimSpace(appkey)
	if appkey == "" {
		return errs.IMErrorCode_APP_APPKEY_REQUIRED, nil
	}
	app, err := dbs.AppInfoDao{}.FindByAppkey(appkey)
	if err != nil || app == nil {
		return errs.IMErrorCode_APP_NOT_EXISTED, nil
	}

	tables, err := listTenantTables()
	if err != nil {
		return errs.IMErrorCode_APP_DEFAULT, nil
	}

	resp := &apimodels.TenantInventoryResp{AppKey: appkey}
	for _, table := range tables {
		var count int64
		// 表名来自 information_schema，且已过 isSafeTableName 白名单校验，才允许拼进 SQL。
		if err := dbcommons.GetDb().Raw("SELECT COUNT(*) FROM `"+table+"` WHERE app_key = ?", appkey).Scan(&count).Error; err != nil {
			// 单表失败不静默：清单不全比报错更危险，会让人得出"没有数据"的错误结论。
			return errs.IMErrorCode_APP_DEFAULT, nil
		}
		if count > 0 {
			resp.Tables = append(resp.Tables, apimodels.TenantTableCount{TableName: table, RowCount: count})
			resp.TotalRows += count
		}
	}

	sort.Slice(resp.Tables, func(i, j int) bool {
		if resp.Tables[i].RowCount == resp.Tables[j].RowCount {
			return resp.Tables[i].TableName < resp.Tables[j].TableName
		}
		return resp.Tables[i].RowCount > resp.Tables[j].RowCount
	})
	return errs.IMErrorCode_SUCCESS, resp
}

// listTenantTables 返回库里所有带 app_key 列的表名。
func listTenantTables() ([]string, error) {
	var names []string
	err := dbcommons.GetDb().
		Table("information_schema.COLUMNS").
		Where("TABLE_SCHEMA = DATABASE() AND COLUMN_NAME = ?", "app_key").
		Order("TABLE_NAME").
		Pluck("TABLE_NAME", &names).Error
	if err != nil {
		return nil, err
	}
	safe := make([]string, 0, len(names))
	for _, name := range names {
		if isSafeTableName(name) {
			safe = append(safe, name)
		}
	}
	return safe, nil
}

// isSafeTableName 只允许小写字母、数字与下划线。
//
// 表名来自 information_schema，本身是可信来源；但它是唯一被拼进 SQL 的标识符，
// 所以在拼接前再收一道。几行代码换掉一次"某天某张表名带反引号"的事故，划算。
func isSafeTableName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		isLower := r >= 'a' && r <= 'z'
		isDigit := r >= '0' && r <= '9'
		if !isLower && !isDigit && r != '_' {
			return false
		}
	}
	return true
}
