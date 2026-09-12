package db

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

// RunMigrations выполняет все SQL миграции из папки migrationDir
// Ожидает, что файлы имеют формат: YYYYMMDDHHMMSS_description.up.sql
func RunMigrations(db *sqlx.DB, migrationDir string, log *zap.Logger) error {
	log.Info("Starting migrations scan", zap.String("dir", migrationDir))

	// 1. Читаем список файлов в директории
	entries, err := os.ReadDir(migrationDir)
	if err != nil {
		return fmt.Errorf("не удалось прочитать директорию миграций %s: %w", migrationDir, err)
	}

	// 2. Фильтруем только .up.sql файлы
	var migrationFiles []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".up.sql") {
			migrationFiles = append(migrationFiles, name)
		}
	}

	if len(migrationFiles) == 0 {
		log.Warn("No migration files found")
		return nil
	}

	// 3. Сортируем файлы по имени (важно для порядка применения!)
	sort.Strings(migrationFiles)
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY, checksum TEXT NOT NULL, applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP)`); err != nil {
		return fmt.Errorf("create migration ledger: %w", err)
	}

	log.Info("Found migration files", zap.Any("files", migrationFiles))

	// 4. Выполняем миграции по очереди
	for _, fileName := range migrationFiles {
		filePath := filepath.Join(migrationDir, fileName)

		log.Info("Applying migration", zap.String("file", fileName))

		// Читаем содержимое файла
		content, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("ошибка чтения файла миграции %s: %w", fileName, err)
		}

		checksum := fmt.Sprintf("%x", sha256.Sum256(content))
		var stored string
		err = db.Get(&stored, `SELECT checksum FROM schema_migrations WHERE name = ?`, fileName)
		if err == nil {
			if stored != checksum {
				return fmt.Errorf("migration %s changed after application", fileName)
			}
			continue
		}
		if err != sql.ErrNoRows {
			return fmt.Errorf("read migration ledger: %w", err)
		}
		// Apply the schema and ledger atomically. Existing idempotent upstream schema can be adopted.
		tx, err := db.Beginx()
		if err != nil {
			return err
		}
		query := string(content)
		_, err = tx.Exec(query)
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("ошибка выполнения миграции %s: %w", fileName, err)
		}
		if _, err = tx.Exec(`INSERT INTO schema_migrations (name, checksum) VALUES (?, ?)`, fileName, checksum); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}

		log.Info("Migration applied successfully", zap.String("file", fileName))
	}

	log.Info("All migrations completed successfully")
	return nil
}
