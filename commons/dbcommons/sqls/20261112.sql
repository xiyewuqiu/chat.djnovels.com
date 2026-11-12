CREATE TABLE IF NOT EXISTS reports (
  id BIGINT NOT NULL AUTO_INCREMENT,
  app_key VARCHAR(64) COLLATE utf8mb4_bin NOT NULL,
  reporter_id VARCHAR(64) COLLATE utf8mb4_bin NOT NULL,
  target_type TINYINT NOT NULL DEFAULT 1,
  target_id VARCHAR(64) COLLATE utf8mb4_bin NOT NULL DEFAULT '',
  target_owner_id VARCHAR(64) COLLATE utf8mb4_bin NOT NULL DEFAULT '',
  msg_id VARCHAR(64) COLLATE utf8mb4_bin NOT NULL DEFAULT '',
  reason_type TINYINT NOT NULL DEFAULT 0,
  description VARCHAR(500) NOT NULL DEFAULT '',
  status TINYINT NOT NULL DEFAULT 0,
  handler_id VARCHAR(64) COLLATE utf8mb4_bin NOT NULL DEFAULT '',
  handle_result VARCHAR(500) NOT NULL DEFAULT '',
  created_time DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_time DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  KEY idx_report_status (app_key, status, id),
  KEY idx_report_reporter (app_key, reporter_id, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
