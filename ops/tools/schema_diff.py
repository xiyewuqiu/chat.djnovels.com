#!/usr/bin/env python3
"""Generate a column-compatibility migration between two MySQL schemas.

The IM engine and the business layer share one database and several tables,
but each ships its own DDL. Whichever schema is applied first wins because both
use CREATE TABLE IF NOT EXISTS, so any column the other side expects is silently
missing at runtime ("Unknown column ...").

This tool reads both schema files, finds shared tables, and emits an idempotent
SQL script that adds the columns the first schema lacks.

Usage:
    python tools/schema_diff.py \
        --base chatapi.djnovels.com/sql/imserver.sql \
        --overlay chat.djnovels.com/docs/appbusiness.sql \
        --out deploy/sql/03-business-compat.sql
"""

import argparse
import re


def parse_tables(path):
    text = open(path, encoding="utf-8", errors="replace").read()
    tables = {}
    pattern = re.compile(
        r"CREATE TABLE(?:\s+IF NOT EXISTS)?\s+`([^`]+)`\s*\((.*?)\)\s*ENGINE",
        re.S | re.I,
    )
    for match in pattern.finditer(text):
        name, body = match.group(1), match.group(2)
        columns = []
        for line in body.splitlines():
            line = line.strip().rstrip(",")
            column = re.match(r"`([^`]+)`\s+(\S.*)$", line)
            if column:
                columns.append((column.group(1), column.group(2).strip()))
        tables[name] = columns
    return tables


HEADER = """-- 自动生成，请勿手工编辑。重新生成：
--   python tools/schema_diff.py --base chatapi.djnovels.com/sql/imserver.sql \\
--        --overlay chat.djnovels.com/docs/appbusiness.sql --skip users.status \\
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
"""

FOOTER = """
DROP PROCEDURE IF EXISTS im_add_column;
"""


def sql_literal(value):
    return "'" + value.replace("\\", "\\\\").replace("'", "''") + "'"


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--base", required=True, help="schema that is applied first")
    parser.add_argument("--overlay", required=True, help="schema applied second")
    parser.add_argument("--out", required=True)
    parser.add_argument(
        "--skip",
        default="",
        help="comma separated table.column pairs to leave out, for columns that "
        "an in-app migration already adds (e.g. users.status is added by "
        "commons/dbcommons/sqls/20251115.sql)",
    )
    args = parser.parse_args()

    base = parse_tables(args.base)
    overlay = parse_tables(args.overlay)
    skipped = {item.strip() for item in args.skip.split(",") if item.strip()}

    shared = sorted(set(base) & set(overlay))
    statements = []
    summary = []

    for table in shared:
        existing = {name for name, _ in base[table]}
        for name, definition in overlay[table]:
            if name in existing:
                continue
            if "{}.{}".format(table, name) in skipped:
                continue
            statements.append(
                "CALL im_add_column({}, {}, {});".format(
                    sql_literal(table), sql_literal(name), sql_literal(definition)
                )
            )
            summary.append((table, name))

    with open(args.out, "w", encoding="utf-8", newline="\n") as handle:
        handle.write(HEADER)
        handle.write("\n")
        for statement in statements:
            handle.write(statement + "\n")
        handle.write(FOOTER)

    print("shared tables:", len(shared))
    print("missing columns:", len(statements))
    for table, column in summary:
        print("  {}.{}".format(table, column))


if __name__ == "__main__":
    main()
