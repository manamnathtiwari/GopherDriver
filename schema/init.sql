CREATE DATABASE IF NOT EXISTS gopherdrive;
USE gopherdrive;

CREATE TABLE IF NOT EXISTS files (
    id        VARCHAR(36)  PRIMARY KEY,
    hash      VARCHAR(64)  NOT NULL DEFAULT '',
    size      BIGINT       NOT NULL DEFAULT 0,
    status    VARCHAR(20)  NOT NULL DEFAULT 'pending',
    file_path VARCHAR(512) NOT NULL,
    created_at TIMESTAMP   DEFAULT CURRENT_TIMESTAMP,
    metadata   JSON
);
