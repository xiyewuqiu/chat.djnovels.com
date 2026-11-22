package services

import (
	"context"
	"sort"
	"strconv"
	"strings"

	apimodels "github.com/juggleim/jugglechat-server/apis/models"
	"github.com/juggleim/jugglechat-server/commons/appinfos"
	"github.com/juggleim/jugglechat-server/commons/dbcommons"
	"github.com/juggleim/jugglechat-server/commons/errs"
	"github.com/juggleim/jugglechat-server/storages/dbs"
	dbmodels "github.com/juggleim/jugglechat-server/storages/models"
)

const (
	// purgeBatchSize 是单表每批删除的行数。太小则来回太多趟，太大则会长时间持锁。
	purgeBatchSize = 1000
	// purgeMaxRowsPerCall 是单次调用最多删除的行数。
	//
	// 一次 HTTP 请求里删几十万行，超时和锁等待都不可控；所以到这里就停，
	// 把 remaining 交回调用方，由它决定是否继续调。
	purgeMaxRowsPerCall = 10000
)

// tenantMetaTables 是租户"本体"所在的表，必须最后删。
//
// 顺序不能反：先删数据、再删本体，中途失败时租户还在，可以继续重试；
// 反过来就是"租户没了、数据还在库里"，既查不到也清不掉。
var tenantMetaTables = map[string]struct{}{
	"apps":    {},
	"appexts": {},
}

// PurgeTenant 清退一个租户，删除它在库里带 app_key 的全部数据，最后删租户本体。
//
// 三条硬约束，缺一条都不允许执行：
//  1. ConfirmAppKey 必须与 AppKey 完全一致（防手滑删错租户）；
//  2. 租户必须已经停用（app_status != 0）——活跃租户的数据删一半比不删更糟；
//  3. 只处理带 app_key 列的表，且表名过白名单（与数据清单同一套口径）。
//
// 单次最多删 purgeMaxRowsPerCall 行，没删完就返回 remaining_rows，
// 调用方需要再调一次。这样大租户不会把一次请求拖到超时。
func PurgeTenant(ctx context.Context, req *apimodels.PurgeTenantReq) (errs.IMErrorCode, *apimodels.PurgeTenantResp) {
	appkey := strings.TrimSpace(req.AppKey)
	if appkey == "" {
		return errs.IMErrorCode_APP_APPKEY_REQUIRED, nil
	}
	if strings.TrimSpace(req.ConfirmAppKey) != appkey {
		// 确认串不匹配一律当参数非法：这条路径上没有"差不多对"的余地。
		return errs.IMErrorCode_APP_REQ_BODY_ILLEGAL, nil
	}

	app, err := dbs.AppInfoDao{}.FindByAppkey(appkey)
	if err != nil || app == nil {
		return errs.IMErrorCode_APP_NOT_EXISTED, nil
	}
	if app.AppStatus == int(dbs.AppStatus_Normal) {
		return errs.IMErrorCode_APP_PURGE_BLOCKED, nil
	}

	tables, err := listTenantTables()
	if err != nil {
		return errs.IMErrorCode_APP_DEFAULT, nil
	}
	ordered := orderTablesForPurge(tables)

	resp := &apimodels.PurgeTenantResp{AppKey: appkey, DryRun: req.DryRun}

	if req.DryRun {
		for _, table := range ordered {
			count, err := countTenantRows(appkey, table)
			if err != nil {
				return errs.IMErrorCode_APP_DEFAULT, nil
			}
			if count > 0 {
				resp.Tables = append(resp.Tables, apimodels.TenantTableCount{TableName: table, RowCount: count})
				resp.ProcessedRows += count
			}
		}
		return errs.IMErrorCode_SUCCESS, resp
	}

	budget := int64(purgeMaxRowsPerCall)
	for _, table := range ordered {
		deleted, left, err := purgeTable(appkey, table, budget)
		if err != nil {
			// 中途失败：已删的不回滚，但租户本体还在，修好原因后可以接着调。
			return errs.IMErrorCode_APP_DEFAULT, nil
		}
		budget -= deleted
		resp.ProcessedRows += deleted
		resp.RemainingRows += left
		if deleted > 0 {
			resp.Tables = append(resp.Tables, apimodels.TenantTableCount{TableName: table, RowCount: deleted})
		}
	}

	// 租户本体是否已经删掉？以 apps 表实际查询为准，不看预算——预算用完和删完是两回事。
	remainingApp, err := countTenantRows(appkey, "apps")
	if err != nil {
		return errs.IMErrorCode_APP_DEFAULT, nil
	}
	if remainingApp == 0 {
		resp.Purged = true
		appinfos.InvalidateAppInfoCache(appkey)
		RecordAdminAudit(ctx, req.OperatorId, dbmodels.AdminAuditAction_AppPurge, appkey,
			auditSummary(resp))
	}
	return errs.IMErrorCode_SUCCESS, resp
}

// purgeTable 分批删除单表里该租户的数据，返回（已删行数，剩余行数）。
func purgeTable(appkey, table string, budget int64) (int64, int64, error) {
	var deleted int64
	for budget > 0 {
		batch := int64(purgeBatchSize)
		if budget < batch {
			batch = budget
		}
		res := dbcommons.GetDb().Exec("DELETE FROM `"+table+"` WHERE app_key = ? LIMIT ?", appkey, batch)
		if res.Error != nil {
			return deleted, 0, res.Error
		}
		if res.RowsAffected == 0 {
			break // 这张表已经清干净
		}
		deleted += res.RowsAffected
		budget -= res.RowsAffected
		if res.RowsAffected < batch {
			break
		}
	}

	// 只有"预算耗尽"才需要回读剩余量；正常删完就是 0。
	if budget <= 0 {
		left, err := countTenantRows(appkey, table)
		if err != nil {
			return deleted, 0, err
		}
		return deleted, left, nil
	}
	return deleted, 0, nil
}

func countTenantRows(appkey, table string) (int64, error) {
	var count int64
	err := dbcommons.GetDb().Raw("SELECT COUNT(*) FROM `"+table+"` WHERE app_key = ?", appkey).Scan(&count).Error
	return count, err
}

// orderTablesForPurge 把租户本体表排到最后，其余按表名排序保证结果稳定。
func orderTablesForPurge(tables []string) []string {
	data := make([]string, 0, len(tables))
	meta := make([]string, 0, len(tenantMetaTables))
	for _, table := range tables {
		if _, isMeta := tenantMetaTables[table]; isMeta {
			meta = append(meta, table)
			continue
		}
		data = append(data, table)
	}
	sort.Strings(data)
	// appexts 先于 apps：配置是租户的附属物，本体最后走。
	sort.Slice(meta, func(i, j int) bool {
		return meta[i] == "appexts" && meta[j] == "apps"
	})
	return append(data, meta...)
}

// auditSummary 把清退结果压成一行审计备注。
//
// remaining 始终写出来：清退可能分多次调用，只有 visto remaining=0 才能证明
// "这个租户的数据真的清干净了"——审计的价值就在这里。
func auditSummary(resp *apimodels.PurgeTenantResp) string {
	return "purged,processed=" + strconv.FormatInt(resp.ProcessedRows, 10) +
		",remaining=" + strconv.FormatInt(resp.RemainingRows, 10)
}
