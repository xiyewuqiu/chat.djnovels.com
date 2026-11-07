-- 自动生成，请勿手工编辑。重新生成：
--   python tools/schema_diff.py --base chatapi.djnovels.com/sql/imserver.sql \
--        --overlay chat.djnovels.com/docs/appbusiness.sql --skip users.status \
--        --out deploy/sql/03-business-compat.sql
--
-- 背景：引擎与业务层共用同一个库，但各自带一份 DDL，都用 CREATE TABLE IF NOT EXISTS。
-- 谁先建表谁赢，另一侧需要的列就会静默缺失，运行时报 Unknown column。
-- 本脚本把「业务层需要、而引擎 schema 里没有」的列补上。可重复执行。
--
-- users.status 被排除：业务层迁移 commons/dbcommons/sqls/20251115.sql 已经负责添加。

DROP PROCEDURE IF EXISTS im_add_column;

DELIMITER $$

CREATE PROCEDURE im_add_column(IN tbl VARCHAR(64), IN col VARCHAR(64), IN def TEXT)
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.COLUMNS
        WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = tbl AND COLUMN_NAME = col
    ) THEN
        SET @ddl = CONCAT('ALTER TABLE `', tbl, '` ADD COLUMN `', col, '` ', def);
        PREPARE stmt FROM @ddl;
        EXECUTE stmt;
        DEALLOCATE PREPARE stmt;
    END IF;
END$$

DELIMITER ;

CALL im_add_column('accountapprels', 'account', 'varchar(20) DEFAULT ''''');
CALL im_add_column('groupinfos', 'creator_id', 'varchar(64) DEFAULT NULL');

DROP PROCEDURE IF EXISTS im_add_column;
