-- 租户数据清单（干跑）：只统计，不删除任何数据。
--
-- 用途：在停用或清理某个租户之前，先看清它在库里到底占了多少行、分布在哪几张表。
-- 表清单从 information_schema 动态发现（所有带 app_key 列的表），不硬编码，
-- 这样上游新增表也不会被漏掉。
--
-- 用法一：交互式，先设变量再执行
--   mysql -uroot -p im_db
--   SET @app_key = 'dz';
--   source ops/sql/tenant-inventory.sql
--
-- 用法二：一条命令
--   mysql -uroot -p im_db -e "SET @app_key='dz'; source ops/sql/tenant-inventory.sql"
--
-- 注意：这是只读脚本。脚本本身不含任何 DELETE / DROP / UPDATE。

SET SESSION group_concat_max_len = 1024 * 1024;

SET @app_key = IFNULL(@app_key, '');

-- 拼一条 UNION ALL 的统计语句：每张表一行 (table_name, rows_of_tenant)
SELECT GROUP_CONCAT(
         CONCAT('SELECT ''', TABLE_NAME, ''' AS table_name, COUNT(*) AS rows_of_tenant FROM `',
                TABLE_NAME, '` WHERE app_key = ', QUOTE(@app_key))
         ORDER BY TABLE_NAME
         SEPARATOR ' UNION ALL '
       )
  INTO @inventory_sql
  FROM information_schema.COLUMNS
 WHERE TABLE_SCHEMA = DATABASE()
   AND COLUMN_NAME = 'app_key'
   AND @app_key <> '';

-- 没给 app_key 时返回一行显式错误，避免"静默 0 行"被误解成"这个租户没有数据"
SET @inventory_sql = IF(
  @app_key = '',
  'SELECT ''ERROR: set @app_key first'' AS table_name, -1 AS rows_of_tenant',
  IF(@inventory_sql IS NULL,
     'SELECT ''(no table with app_key column found)'' AS table_name, 0 AS rows_of_tenant WHERE 0',
     CONCAT('SELECT table_name, rows_of_tenant FROM (', @inventory_sql,
            ') AS t WHERE rows_of_tenant > 0 ORDER BY rows_of_tenant DESC')));

PREPARE inventory_stmt FROM @inventory_sql;
EXECUTE inventory_stmt;
DEALLOCATE PREPARE inventory_stmt;
