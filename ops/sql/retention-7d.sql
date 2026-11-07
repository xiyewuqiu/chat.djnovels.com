-- 把一个租户的消息保留期设为 7 天。
--
-- 使用 INSERT IGNORE + UPDATE 而不是 ON DUPLICATE KEY UPDATE 的别名语法，
-- 这样在 MySQL 5.7 / 8.0 / 8.4 上行为一致，不会因为版本差异报语法错。
--
-- 三个键必须同时设置。只设其中一部分会出现「离线队列 10 分钟就清空、
-- 历史却留 180 天」的错位，用户会以为消息丢了。
--
-- 键名规则：引擎用反射把结构体字段转成 snake_case
-- （chatapi.djnovels.com/services/commonservices/appinfocache.go:197），
-- 因此 HisMsgSaveDay -> his_msg_save_day。
--
-- 执行（宿主机直接连 MySQL，不使用容器）：
--   mysql -uroot -p im_db < ops/sql/retention-7d.sql
-- 把第 18 行的 REPLACE_WITH_APP_KEY 换成实际租户 appkey。
--
-- 引擎对 app 配置有 5–10 分钟缓存，执行后请重启 im-server 立即生效。

SET @app_key = 'REPLACE_WITH_APP_KEY';

-- 1) 缺失则插入（值留空，随后由 UPDATE 统一覆盖）
INSERT IGNORE INTO `appexts` (`app_key`, `app_item_key`, `app_item_value`) VALUES
  (@app_key, 'his_msg_save_day',          ''),
  (@app_key, 'offline_msg_save_time',     ''),
  (@app_key, 'offline_cmd_msg_save_time', '');

-- 2) 统一覆盖为 7 天（离线队列单位是分钟：7 * 24 * 60 = 10080）
UPDATE `appexts` SET `app_item_value` = '7'
  WHERE `app_key` = @app_key AND `app_item_key` = 'his_msg_save_day';
UPDATE `appexts` SET `app_item_value` = '10080'
  WHERE `app_key` = @app_key AND `app_item_key` = 'offline_msg_save_time';
UPDATE `appexts` SET `app_item_value` = '10080'
  WHERE `app_key` = @app_key AND `app_item_key` = 'offline_cmd_msg_save_time';

-- 3) 回查：应返回三行，值依次为 7 / 10080 / 10080
SELECT `app_item_key`, `app_item_value`
FROM `appexts`
WHERE `app_key` = @app_key
  AND `app_item_key` IN ('his_msg_save_day', 'offline_msg_save_time', 'offline_cmd_msg_save_time');
