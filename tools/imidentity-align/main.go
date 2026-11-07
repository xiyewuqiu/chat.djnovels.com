// imidentity-align —— 把引擎侧身份对齐到业务账号。
//
// 背景：自营引擎最初建号时用的是随机 11 位 user_id，业务账号只落在 login_account 上。
// 结果是同一个用户在业务库叫 zhangsan、在 IM 里叫 M_5kJw96BaX，客服号/群成员/会话对端
// 全部对不上号。修正后（services.IssueUserToken）新账号的 user_id 直接就是业务账号，
// 但历史行仍带着随机 id —— 本工具负责把历史一次性搬过去，让
//
//	users.user_id == users.login_account == 业务账号
//
// 在全库成立，并同步改写所有引用这些 id 的表、私聊会话 id 与二进制消息体。
//
// 用法（必须在业务层仓库根目录执行：配置路径 conf/config.yml 是相对路径）：
//
//	go run ./tools/imidentity-align                     # 干跑，只打印将要发生什么
//	go run ./tools/imidentity-align -apply              # 真正写入（数据变更在单事务内）
//	go run ./tools/imidentity-align -verify-map m.tsv   # 事后验收：还有没有旧 id 残留
//
// 幂等：跑完一遍后旧 id 不再存在，再跑一遍是空操作。
//
// 注意：映射表 zz_im_identity_map_align 的 CREATE/DROP 属于 DDL，MySQL 会隐式提交，
// 因此它必须建在事务之外 —— 否则"单事务、失败全回滚"是一句空话。
package main

import (
	"bufio"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/juggleim/jugglechat-server/commons/configures"
	"github.com/juggleim/jugglechat-server/commons/dbcommons"
)

const (
	defaultAppKey = "dz.djnovels.com"
	mapTable      = "zz_im_identity_map_align"
)

// col 一个表 + 列。
type col struct {
	table  string
	column string
}

// idColumns：列值就是**一个**引擎 user id。值不在映射表里就不动
// （群 id、机器/消息 id 天然免疫，不会被误伤）。
var idColumns = []col{
	{"assistant_prompts", "user_id"},
	{"banuserrules", "user_id"},
	{"banusers", "user_id"},
	{"bc_hismsgs", "sender_id"},
	{"binddevices", "user_id"},
	{"blocks", "user_id"},
	{"blocks", "block_user_id"},
	{"brdinboxmsgs", "sender_id"},
	{"clientlogs", "user_id"},
	{"cmdinboxmsgs", "user_id"},
	{"cmdinboxmsgs", "target_id"},
	{"cmdinboxmsgs_split", "user_id"},
	{"cmdinboxmsgs_split", "target_id"},
	{"cmdsendboxmsgs", "user_id"},
	{"cmdsendboxmsgs", "target_id"},
	{"cmdsendboxmsgs_split", "user_id"},
	{"cmdsendboxmsgs_split", "target_id"},
	{"conversations", "user_id"},
	{"conversations", "target_id"},
	{"convertagrels", "user_id"},
	{"convertagrels", "target_id"},
	{"device_login_qrs", "user_id"},
	{"device_revoke_jobs", "user_id"},
	{"favoritemsgs", "user_id"},
	{"favoritemsgs", "sender_id"},
	{"favoritemsgs", "receiver_id"},
	{"feedbacks", "user_id"},
	{"friendapplications", "recipient_id"},
	{"friendapplications", "sponsor_id"},
	{"friendrels", "user_id"},
	{"friendrels", "friend_id"},
	{"gc_hismsgs", "sender_id"},
	{"gc_hismsgs", "receiver_id"},
	{"globalconvers", "sender_id"},
	{"globalconvers", "target_id"},
	{"groupadmins", "admin_id"},
	{"groupinfos", "creator_id"},
	{"groupmemberexts", "member_id"},
	{"groupmembers", "member_id"},
	{"g_delhismsgs", "user_id"},
	{"g_hismsgs", "sender_id"},
	{"g_hismsgs", "receiver_id"},
	{"g_portionrels", "user_id"},
	{"inboxmsgs", "user_id"},
	{"inboxmsgs", "target_id"},
	{"inboxmsgs_split", "user_id"},
	{"inboxmsgs_split", "target_id"},
	{"mentionmsgs", "user_id"},
	{"mentionmsgs", "sender_id"},
	{"mentionmsgs", "target_id"},
	{"mergedmsgs", "from_id"},
	{"mergedmsgs", "target_id"},
	{"msgexsets", "user_id"},
	{"msgexts", "user_id"},
	{"p_delhismsgs", "user_id"},
	{"p_delhismsgs", "target_id"},
	{"p_hismsgs", "sender_id"},
	{"p_hismsgs", "receiver_id"},
	{"pushtokens", "user_id"},
	{"qrcoderecords", "user_id"},
	{"readinfos", "member_id"},
	{"rtcmembers", "member_id"},
	{"rtcmembers", "inviter_id"},
	{"rtcrooms", "owner_id"},
	{"sendboxmsgs", "user_id"},
	{"sendboxmsgs", "target_id"},
	{"sendboxmsgs_split", "user_id"},
	{"sendboxmsgs_split", "target_id"},
	{"s_hismsgs", "sender_id"},
	{"s_hismsgs", "receiver_id"},
	{"telebotrels", "user_id"},
	{"telebots", "user_id"},
	{"topmsgs", "user_id"},
	{"useractivities", "user_id"},
	{"usercleantimes", "user_id"},
	{"usercleantimes", "target_id"},
	{"userconverconfs", "user_id"},
	{"userconvertags", "user_id"},
	{"userexts", "user_id"},
	{"usersubrels", "user_id"},
	{"user_devices", "user_id"},
}

// converIdColumns：私聊会话 id 的编码是 "min(uidA,uidB):max(uidA,uidB)"
// （引擎 services/commonservices.GetConversationId），改写后必须**重新排序**，
// 否则引擎再也找不到这条会话。群会话 id 就是群 id，单段无冒号，天然不动。
var converIdColumns = []col{
	{"bc_hismsgs", "conver_id"},
	{"convercleantimes", "conver_id"},
	{"converconfs", "conver_id"},
	{"gc_hismsgs", "conver_id"},
	{"globalconvers", "conver_id"},
	{"g_portionrels", "conver_id"},
	{"g_hismsgs", "conver_id"},
	{"p_hismsgs", "conver_id"},
	{"rtcrooms", "conver_id"},
	{"s_hismsgs", "conver_id"},
	{"topmsgs", "conver_id"},
	{"userconverconfs", "conver_id"},
}

// blobColumns：二进制消息体里内嵌着 sender/receiver 的原型字段。
// 只改列、不改体内字节的话，客户端解回来的发送者仍是一个已经不存在的 id。
var blobColumns = []col{
	{"bc_hismsgs", "msg_body"},
	{"brdinboxmsgs", "msg_body"},
	{"cmdinboxmsgs", "msg_body"},
	{"cmdinboxmsgs_split", "msg_body"},
	{"cmdsendboxmsgs", "msg_body"},
	{"cmdsendboxmsgs_split", "msg_body"},
	{"conversations", "conver_exts"},
	{"conversations", "latest_msg"},
	{"favoritemsgs", "msg_body"},
	{"gc_hismsgs", "msg_body"},
	{"grpsnapshots", "snapshot"},
	{"g_hismsgs", "msg_body"},
	{"g_hismsgs", "msg_ext"},
	{"g_hismsgs", "msg_exset"},
	{"inboxmsgs", "msg_body"},
	{"inboxmsgs_split", "msg_body"},
	{"mergedmsgs", "msg_body"},
	{"p_hismsgs", "msg_body"},
	{"p_hismsgs", "msg_ext"},
	{"p_hismsgs", "msg_exset"},
	{"sendboxmsgs", "msg_body"},
	{"sendboxmsgs_split", "msg_body"},
	{"s_hismsgs", "msg_body"},
}

type pair struct {
	old string
	new string
}

func main() {
	appkey := flag.String("appkey", defaultAppKey, "租户 app_key")
	apply := flag.Bool("apply", false, "真正写入（默认只干跑）")
	mapOut := flag.String("map-out", "identity-align-map.tsv", "写入旧 id→新 id 映射文件（留空则不写）")
	verifyMap := flag.String("verify-map", "", "验收模式：读取映射文件，报告库里是否还有旧 id 残留")
	flag.Parse()

	connect()

	if *verifyMap != "" {
		pairs, err := readMapFile(*verifyMap)
		if err != nil {
			log.Fatalf("读取映射文件失败: %v", err)
		}
		bad, err := scanOldIDs(db(), *appkey, pairs)
		if err != nil {
			log.Fatalf("验收失败: %v", err)
		}
		if len(bad) > 0 {
			fmt.Printf("❌ 仍有旧 id 残留（%d 处）：\n", len(bad))
			for _, b := range bad {
				fmt.Println("   " + b)
			}
			os.Exit(1)
		}
		fmt.Println("✅ 验收通过：库里已找不到任何旧 id（列 / 私聊会话 id / 消息体）")
		return
	}

	pairs, unmapped, err := loadMapping(db(), *appkey)
	if err != nil {
		log.Fatalf("加载身份映射失败: %v", err)
	}
	fmt.Printf("🔎 app_key=%s\n", *appkey)
	if len(pairs) == 0 {
		fmt.Println("✅ 没有需要对齐的账号：user_id 已经等于 login_account")
		return
	}
	fmt.Printf("   待对齐账号 %d 个\n", len(pairs))
	for _, p := range pairs {
		fmt.Printf("     %s → %s\n", p.old, p.new)
	}
	if len(unmapped) > 0 {
		fmt.Printf("   ⏭️ 跳过（没有业务账号，保持原样）%d 个：%s\n", len(unmapped), strings.Join(unmapped, ", "))
	}
	if !*apply {
		fmt.Println("\n🧪 干跑结束。确认上面没有异常后加 -apply 真正写入。")
		return
	}

	if *mapOut != "" {
		if err := writeMapFile(*mapOut, pairs); err != nil {
			log.Fatalf("写映射文件失败: %v", err)
		}
		fmt.Printf("   📝 映射已留存：%s（验收用：-verify-map %s）\n", *mapOut, *mapOut)
	}

	if err := align(db(), *appkey, pairs); err != nil {
		log.Fatalf("❌ 对齐失败（数据变更已回滚）: %v", err)
	}
	fmt.Println("\n🎉 对齐完成。")
}

var conn *sql.DB

func connect() {
	if err := configures.InitConfigures(); err != nil {
		log.Fatalf("读取 conf/config.yml 失败（请在业务层仓库根目录执行）: %v", err)
	}
	if err := dbcommons.InitMysql(); err != nil {
		log.Fatalf("连接 MySQL 失败: %v", err)
	}
	db, err := dbcommons.GetDb().DB()
	if err != nil {
		log.Fatalf("获取连接池失败: %v", err)
	}
	conn = db
}

func db() *sql.DB { return conn }

// loadMapping 从 users 表本身推导映射：login_account 就是业务账号。
//
// 只处理"有业务账号且与当前 user_id 不同"的行；没有 login_account 的行
// （手工建的机器人/客服/烟测残留）原样保留并单独报告，避免误伤。
func loadMapping(db *sql.DB, appkey string) ([]pair, []string, error) {
	rows, err := db.Query("SELECT user_id, login_account FROM users WHERE app_key = ? ORDER BY id", appkey)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	type raw struct {
		userID  string
		account sql.NullString
	}
	var (
		all   []raw
		taken = map[string]bool{}
	)
	for rows.Next() {
		var r raw
		if err := rows.Scan(&r.userID, &r.account); err != nil {
			return nil, nil, err
		}
		all = append(all, r)
		taken[r.userID] = true
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	var (
		pairs    []pair
		unmapped []string
		seenNew  = map[string]bool{}
	)
	for _, r := range all {
		account := strings.TrimSpace(r.account.String)
		if !r.account.Valid || account == "" {
			unmapped = append(unmapped, r.userID+"(无账号)")
			continue
		}
		if account == r.userID {
			continue // 已经对齐
		}
		if !validUserID(account) {
			return nil, nil, fmt.Errorf("账号 %q 不能作为引擎 user_id（超长或含非法字符）", account)
		}
		if seenNew[account] {
			return nil, nil, fmt.Errorf("两个账号都指向同一个新 id %q，拒绝迁移", account)
		}
		// 目标 id 已被别的行占用 → 改名会撞唯一键 uniq_userid(app_key,user_id)
		if taken[account] {
			return nil, nil, fmt.Errorf("目标 id %q 已被另一条用户记录占用，拒绝迁移", account)
		}
		seenNew[account] = true
		pairs = append(pairs, pair{old: r.userID, new: account})
	}
	return pairs, unmapped, nil
}

// validUserID 与 services.IsValidUserId 同规则：引擎 user_id 的硬约束。
func validUserID(id string) bool {
	if id == "" || len(id) > 32 {
		return false
	}
	for i := 0; i < len(id); i++ {
		switch b := id[i]; {
		case b <= 0x1F || b == 0x7F, b == 0x20, b == '#', b == '/', b == '\\', b == ':':
			return false
		}
	}
	return true
}

func writeMapFile(path string, pairs []pair) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	for _, p := range pairs {
		if _, err := fmt.Fprintf(w, "%s\t%s\n", p.old, p.new); err != nil {
			return err
		}
	}
	return w.Flush()
}

func readMapFile(path string) ([]pair, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var pairs []pair
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("映射文件格式不对：%q", line)
		}
		pairs = append(pairs, pair{old: parts[0], new: parts[1]})
	}
	return pairs, sc.Err()
}

func align(db *sql.DB, appkey string, pairs []pair) error {
	existing, err := tableSet(db)
	if err != nil {
		return err
	}

	// 映射表建在事务之外：DDL 隐式提交，混进事务会让"失败全回滚"失效。
	if err := buildMapTable(db, pairs); err != nil {
		return err
	}
	defer dropMapTable(db) //nolint:errcheck // 收尾，失败不影响数据正确性

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // 提交后是空操作

	// 1) 普通 user id 列
	for _, c := range idColumns {
		if !existing[c.table] {
			continue
		}
		res, err := tx.Exec(fmt.Sprintf(
			"UPDATE `%s` t JOIN `%s` m ON t.`%s` = m.old_id SET t.`%s` = m.new_id WHERE t.app_key = ?",
			c.table, mapTable, c.column, c.column), appkey)
		if err != nil {
			return fmt.Errorf("改写 %s.%s 失败: %w", c.table, c.column, err)
		}
		report(c, res)
	}

	// 2) 私聊 conver_id：先把两段各自换成新 id，再按引擎的规则重新排序
	for _, c := range converIdColumns {
		if !existing[c.table] {
			continue
		}
		res, err := tx.Exec(fmt.Sprintf(
			"UPDATE `%s` t"+
				" LEFT JOIN `%s` m1 ON m1.old_id = SUBSTRING_INDEX(t.`%s`, ':', 1)"+
				" LEFT JOIN `%s` m2 ON m2.old_id = SUBSTRING_INDEX(t.`%s`, ':', -1)"+
				" SET t.`%s` = CONCAT("+
				"   COALESCE(m1.new_id, SUBSTRING_INDEX(t.`%s`, ':', 1)), ':',"+
				"   COALESCE(m2.new_id, SUBSTRING_INDEX(t.`%s`, ':', -1)))"+
				" WHERE t.app_key = ? AND t.`%s` LIKE '%%:%%' AND (m1.old_id IS NOT NULL OR m2.old_id IS NOT NULL)",
			c.table, mapTable, c.column, mapTable, c.column, c.column, c.column, c.column, c.column), appkey)
		if err != nil {
			return fmt.Errorf("改写 %s.%s 失败: %w", c.table, c.column, err)
		}
		sortRes, err := tx.Exec(fmt.Sprintf(
			"UPDATE `%s` SET `%s` = CONCAT("+
				" LEAST(SUBSTRING_INDEX(`%s`, ':', 1), SUBSTRING_INDEX(`%s`, ':', -1)), ':',"+
				" GREATEST(SUBSTRING_INDEX(`%s`, ':', 1), SUBSTRING_INDEX(`%s`, ':', -1)))"+
				" WHERE app_key = ? AND `%s` LIKE '%%:%%'",
			c.table, c.column, c.column, c.column, c.column, c.column, c.column), appkey)
		if err != nil {
			return fmt.Errorf("重排 %s.%s 失败: %w", c.table, c.column, err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			fmt.Printf("   %-28s 改写 %d 行\n", c.table+"."+c.column, n)
		}
		_ = sortRes
	}

	// 3) 二进制消息体：里面的 sender/receiver 是同一个 id，必须一起换
	if err := rewriteBlobs(tx, appkey, pairs, existing); err != nil {
		return err
	}

	// 4) 最后搬 users 自己：其它表的引用都已经指到新 id 上了
	res, err := tx.Exec("UPDATE `users` u JOIN `"+mapTable+"` m ON u.user_id = m.old_id SET u.user_id = m.new_id WHERE u.app_key = ?", appkey)
	if err != nil {
		return fmt.Errorf("改写 users.user_id 失败: %w", err)
	}
	report(col{"users", "user_id"}, res)

	// 5) 验收：任何一处还残留旧 id 都算失败，直接回滚
	bad, err := scanOldIDs(tx, appkey, pairs)
	if err != nil {
		return err
	}
	if len(bad) > 0 {
		return fmt.Errorf("仍有旧 id 残留：%s", strings.Join(bad, "; "))
	}
	return tx.Commit()
}

func report(c col, res sql.Result) {
	n, err := res.RowsAffected()
	if err != nil || n == 0 {
		return
	}
	fmt.Printf("   %-28s %d 行\n", c.table+"."+c.column, n)
}

type querier interface {
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

func tableSet(q querier) (map[string]bool, error) {
	rows, err := q.Query("SELECT table_name FROM information_schema.tables WHERE table_schema = DATABASE()")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	set := map[string]bool{}
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		set[t] = true
	}
	return set, rows.Err()
}

// scanOldIDs 逐列确认旧 id 已经彻底消失（列值、私聊会话 id 的两段、消息体内嵌字段）。
func scanOldIDs(q querier, appkey string, pairs []pair) ([]string, error) {
	var bad []string
	olds := make([]string, 0, len(pairs))
	placeholders := make([]string, 0, len(pairs))
	args := make([]any, 0, len(pairs)+2)
	for _, p := range pairs {
		olds = append(olds, p.old)
		placeholders = append(placeholders, "?")
		args = append(args, p.old)
	}
	in := strings.Join(placeholders, ",")

	existing, err := tableSet(q)
	if err != nil {
		return nil, err
	}

	for _, c := range idColumns {
		if !existing[c.table] {
			continue
		}
		var n int64
		query := fmt.Sprintf("SELECT COUNT(*) FROM `%s` WHERE app_key = ? AND `%s` IN (%s)", c.table, c.column, in)
		if err := q.QueryRow(query, append([]any{appkey}, args...)...).Scan(&n); err != nil {
			return nil, err
		}
		if n > 0 {
			bad = append(bad, fmt.Sprintf("%s.%s=%d", c.table, c.column, n))
		}
	}

	for _, c := range converIdColumns {
		if !existing[c.table] {
			continue
		}
		var n int64
		query := fmt.Sprintf(
			"SELECT COUNT(*) FROM `%s` WHERE app_key = ? AND (SUBSTRING_INDEX(`%s`, ':', 1) IN (%s) OR SUBSTRING_INDEX(`%s`, ':', -1) IN (%s))",
			c.table, c.column, in, c.column, in)
		if err := q.QueryRow(query, append([]any{appkey}, append(append([]any{}, args...), args...)...)...).Scan(&n); err != nil {
			return nil, err
		}
		if n > 0 {
			bad = append(bad, fmt.Sprintf("%s.%s=%d", c.table, c.column, n))
		}
		// 排序不变量：私聊会话 id 必须是 min:max，否则引擎按 conver_id 找不到会话
		var unsorted int64
		query = fmt.Sprintf(
			"SELECT COUNT(*) FROM `%s` WHERE app_key = ? AND `%s` LIKE '%%:%%' AND `%s` <> CONCAT("+
				"LEAST(SUBSTRING_INDEX(`%s`, ':', 1), SUBSTRING_INDEX(`%s`, ':', -1)), ':',"+
				"GREATEST(SUBSTRING_INDEX(`%s`, ':', 1), SUBSTRING_INDEX(`%s`, ':', -1)))",
			c.table, c.column, c.column, c.column, c.column, c.column, c.column)
		if err := q.QueryRow(query, appkey).Scan(&unsorted); err != nil {
			return nil, err
		}
		if unsorted > 0 {
			bad = append(bad, fmt.Sprintf("%s.%s 未按 min:max 排序=%d", c.table, c.column, unsorted))
		}
	}

	var userLeft int64
	if err := q.QueryRow("SELECT COUNT(*) FROM `users` WHERE app_key = ? AND user_id IN ("+in+")", append([]any{appkey}, args...)...).Scan(&userLeft); err != nil {
		return nil, err
	}
	if userLeft > 0 {
		bad = append(bad, fmt.Sprintf("users.user_id=%d", userLeft))
	}

	blobBad, err := scanBlobs(q, appkey, pairs, existing)
	if err != nil {
		return nil, err
	}
	return append(bad, blobBad...), nil
}

func scanBlobs(q querier, appkey string, pairs []pair, existing map[string]bool) ([]string, error) {
	var bad []string
	for _, c := range blobColumns {
		if !existing[c.table] {
			continue
		}
		rows, err := q.Query(fmt.Sprintf(
			"SELECT id, `%s` FROM `%s` WHERE app_key = ? AND `%s` IS NOT NULL",
			c.column, c.table, c.column), appkey)
		if err != nil {
			return nil, err
		}
		hits := 0
		for rows.Next() {
			var (
				id  int64
				raw []byte
			)
			if err := rows.Scan(&id, &raw); err != nil {
				rows.Close()
				return nil, err
			}
			if _, n := replaceProtoUserIDs(raw, pairs); n > 0 {
				hits += n
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		if hits > 0 {
			bad = append(bad, fmt.Sprintf("%s.%s 内嵌 id=%d", c.table, c.column, hits))
		}
	}
	return bad, nil
}

// rewriteBlobs 把二进制消息体里内嵌的用户 id 换成新 id。
//
// 消息体是 protobuf：字符串字段编码为 <tag><长度><字节…>（长度是 varint）。
// 因此不能直接做字节替换 —— 新 id 长度不同，长度前缀不跟着改，解析端就会读歪。
// 这里只在"前一个字节恰好等于旧 id 长度、再前一个是长度delimited 的 tag"时才替换，
// 并且把长度前缀一起改掉；其余出现位置（比如正文里恰好出现同名字符串）一律不动。
func rewriteBlobs(tx *sql.Tx, appkey string, pairs []pair, existing map[string]bool) error {
	for _, c := range blobColumns {
		if !existing[c.table] {
			continue
		}
		rows, err := tx.Query(fmt.Sprintf(
			"SELECT id, `%s` FROM `%s` WHERE app_key = ? AND `%s` IS NOT NULL",
			c.column, c.table, c.column), appkey)
		if err != nil {
			return fmt.Errorf("读取 %s.%s 失败: %w", c.table, c.column, err)
		}
		type rowData struct {
			id  int64
			raw []byte
		}
		var datas []rowData
		for rows.Next() {
			var d rowData
			if err := rows.Scan(&d.id, &d.raw); err != nil {
				rows.Close()
				return err
			}
			datas = append(datas, d)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		var (
			changed int64
			hits    int
		)
		for _, d := range datas {
			next, n := replaceProtoUserIDs(d.raw, pairs)
			if n == 0 {
				continue
			}
			// 逐行复扫：替换必须把该行里所有"已成字段值"的旧 id 都换干净。
			// 漏一个就意味着客户端会解出一个不存在的发送者，宁可整笔回滚。
			if _, again := replaceProtoUserIDs(next, pairs); again != 0 {
				return fmt.Errorf("%s.%s(id=%d) 替换不完整，剩余 %d 处", c.table, c.column, d.id, again)
			}
			if _, err := tx.Exec(fmt.Sprintf("UPDATE `%s` SET `%s` = ? WHERE id = ?", c.table, c.column), next, d.id); err != nil {
				return fmt.Errorf("回写 %s.%s(id=%d) 失败: %w", c.table, c.column, d.id, err)
			}
			changed++
			hits += n
		}
		if changed > 0 {
			fmt.Printf("   %-28s %d 行 / %d 处内嵌 id\n", c.table+"."+c.column, changed, hits)
		}
	}
	return nil
}

// replaceProtoUserIDs 返回替换后的字节与替换处数。
func replaceProtoUserIDs(blob []byte, pairs []pair) ([]byte, int) {
	out := make([]byte, 0, len(blob))
	hits := 0
	for i := 0; i < len(blob); {
		matched := false
		for _, p := range pairs {
			// 形态：<tag><len(old)><old>，tag 必须是长度delimited（低 3 位 = 2）
			if i+2+len(p.old) > len(blob) {
				continue
			}
			if blob[i+1] != byte(len(p.old)) || string(blob[i+2:i+2+len(p.old)]) != p.old {
				continue
			}
			if blob[i]&0x07 != 0x02 || blob[i]>>3 == 0 {
				continue
			}
			out = append(out, blob[i], byte(len(p.new)))
			out = append(out, p.new...)
			i += 2 + len(p.old)
			hits++
			matched = true
			break
		}
		if !matched {
			out = append(out, blob[i])
			i++
		}
	}
	if hits == 0 {
		return blob, 0
	}
	return out, hits
}

// buildMapTable 建映射表并灌入数据（DDL，必须在事务外执行）。
func buildMapTable(db *sql.DB, pairs []pair) error {
	if _, err := db.Exec("DROP TABLE IF EXISTS `" + mapTable + "`"); err != nil {
		return err
	}
	if _, err := db.Exec("CREATE TABLE `" + mapTable + "` (old_id VARCHAR(64) PRIMARY KEY, new_id VARCHAR(64) NOT NULL) ENGINE=InnoDB"); err != nil {
		return err
	}
	for _, p := range pairs {
		if _, err := db.Exec("INSERT INTO `"+mapTable+"` (old_id,new_id) VALUES (?,?)", p.old, p.new); err != nil {
			return err
		}
	}
	return nil
}

func dropMapTable(db *sql.DB) error {
	_, err := db.Exec("DROP TABLE IF EXISTS `" + mapTable + "`")
	return err
}
