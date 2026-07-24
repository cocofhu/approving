-- sandbox-gateway MySQL 初始化
-- 用法（root）：
--   mysql -uroot -p < deploy/mysql/init.sql
-- 表结构与 GORM AutoMigrate(models.Sandbox) 对齐；应用启动仍会 AutoMigrate，
-- 本脚本便于 DBA 预置库与权限。

-- ---------------------------------------------------------------------------
-- 1) 库
-- ---------------------------------------------------------------------------
CREATE DATABASE IF NOT EXISTS `sandbox_gateway`
  DEFAULT CHARACTER SET utf8mb4
  DEFAULT COLLATE utf8mb4_unicode_ci;

-- ---------------------------------------------------------------------------
-- 2) 用户（请先改密码再执行）
-- ---------------------------------------------------------------------------
-- 生产请把 'CHANGE_ME_STRONG_PASSWORD' 换成强密码。
CREATE USER IF NOT EXISTS 'sandbox_gateway'@'%' IDENTIFIED BY 'CHANGE_ME_STRONG_PASSWORD';

GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, ALTER, INDEX, DROP, REFERENCES
  ON `sandbox_gateway`.* TO 'sandbox_gateway'@'%';

FLUSH PRIVILEGES;

-- ---------------------------------------------------------------------------
-- 3) 表（沙箱元数据）
-- ---------------------------------------------------------------------------
USE `sandbox_gateway`;

CREATE TABLE IF NOT EXISTS `sandboxes` (
  `id`         VARCHAR(64)  NOT NULL,
  `name`       VARCHAR(255) NOT NULL DEFAULT '',
  `status`     VARCHAR(32)  NOT NULL DEFAULT '',
  `image`      VARCHAR(512) NOT NULL DEFAULT '',
  `namespace`  VARCHAR(255) NOT NULL DEFAULT '',
  `error`      TEXT         NULL,
  `cpu_cores`  DOUBLE       NOT NULL DEFAULT 0,
  `memory_mb`  BIGINT       NOT NULL DEFAULT 0,
  `disk_gi`    BIGINT       NOT NULL DEFAULT 0,
  `env`        TEXT         NULL,
  `endpoints`  TEXT         NULL,
  `labels`     TEXT         NULL,
  `created_at` DATETIME(3)  NULL,
  `updated_at` DATETIME(3)  NULL,
  PRIMARY KEY (`id`),
  KEY `idx_sandboxes_name` (`name`),
  KEY `idx_sandboxes_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
