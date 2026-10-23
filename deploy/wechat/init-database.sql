-- Run in the existing MySQL instance used by golang-8467.
-- This creates only the application database, not a cloud database instance.
-- Existing databases, tables and data are preserved. No credentials are included.
CREATE DATABASE IF NOT EXISTS `english_adventure`
  CHARACTER SET utf8mb4;

-- Must return one row before deploying the application.
SELECT SCHEMA_NAME
FROM INFORMATION_SCHEMA.SCHEMATA
WHERE SCHEMA_NAME = 'english_adventure';
