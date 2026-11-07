-- GoSuxin V1.0.0: additive and idempotent application metadata migration.
CREATE TABLE IF NOT EXISTS `gf_app_meta` (
  `meta_key` varchar(64) NOT NULL,
  `meta_value` varchar(255) NOT NULL,
  `updated_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`meta_key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
INSERT INTO `gf_app_meta` (`meta_key`, `meta_value`) VALUES ('app_version', '1.0.0')
ON DUPLICATE KEY UPDATE `meta_value` = VALUES(`meta_value`);
