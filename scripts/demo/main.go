// Explicit local-only synthetic fixture. Never reads the upstream data directory.
package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"go.uber.org/zap"
	"mitm-departament/internal/db"
	"mitm-departament/pkg/hasher"
	"os"
	"path/filepath"
	"time"
)

func main() {
	if err := seed(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func seed() error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	if filepath.VolumeName(root) != "D:" {
		return fmt.Errorf("demo requires D: workspace")
	}
	path := filepath.Join(root, ".local", "data", "demo.db")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		return fmt.Errorf("refusing to seed existing database")
	}
	conn, err := db.New(path, zap.NewNop())
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := db.RunMigrations(conn, filepath.Join(root, "internal", "db", "migration"), zap.NewNop()); err != nil {
		return err
	}
	tx, err := conn.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	accounts := []map[string]string{}
	for _, role := range []string{"admin", "staff"} {
		secret := make([]byte, 24)
		if _, err := rand.Read(secret); err != nil {
			return err
		}
		password := base64.RawURLEncoding.EncodeToString(secret)
		hash, err := hasher.NewHasher().GenerateHash(password)
		if err != nil {
			return err
		}
		email := role + "@demo.invalid"
		if _, err := tx.Exec(`INSERT INTO users (id, full_name, password, role, email, is_active) VALUES (?, ?, ?, ?, ?, 1)`, map[string]string{"admin": "11111111-1111-4111-8111-111111111111", "staff": "22222222-2222-4222-8222-222222222222"}[role], "Демо · "+role, hash, role, email); err != nil {
			return err
		}
		accounts = append(accounts, map[string]string{"email": email, "password": password, "role": role})
	}
	for i, room := range []string{"Учебная лаборатория", "Приборная", "Реактивная"} {
		if _, err := tx.Exec(`INSERT INTO keys (key_number, room_description, notes) VALUES (?, ?, ?)`, fmt.Sprintf("ДЕМО-%02d", i+1), room, "Синтетическая демонстрационная запись"); err != nil {
			return err
		}
	}
	for i, item := range []struct{ kind, name, location string }{{"equipment", "Спектрофотометр", "Приборная"}, {"inventory", "Шкаф лабораторный", "Учебная лаборатория"}, {"raw_material", "Натрий хлорид", "Реактивная"}, {"other", "Набор мерной посуды", "Учебная лаборатория"}} {
		if _, err := tx.Exec(`INSERT INTO inventory (type,name,location,description,inventory_number,responsible_id) VALUES (?, ?, ?, ?, ?, '11111111-1111-4111-8111-111111111111')`, item.kind, item.name, item.location, "Демонстрационная запись", fmt.Sprintf("DEMO-%03d", i+1)); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`INSERT INTO articles (title, details, status, created_by) VALUES ('Демонстрационный план исследования', 'Синтетическая запись для знакомства с интерфейсом', 'planned', '11111111-1111-4111-8111-111111111111')`); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO article_authors (article_id, user_id, name) VALUES (1, '11111111-1111-4111-8111-111111111111', 'Демо · admin')`); err != nil {
		return err
	}
	for i, title := range []string{"Открытый научный семинар", "Проверка лабораторного оборудования"} {
		if _, err := tx.Exec(`INSERT INTO events (creator_id,title,location,description,start_time,is_public) VALUES ('11111111-1111-4111-8111-111111111111', ?, 'Учебная лаборатория', 'Демонстрационное событие', ?, ?)`, title, time.Now().AddDate(0, 0, i+2).UTC(), i == 0); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	content, err := json.MarshalIndent(accounts, "", "  ")
	if err != nil {
		return err
	}
	credentials := filepath.Join(root, ".local", "demo-credentials.json")
	if err := os.WriteFile(credentials, content, 0600); err != nil {
		return err
	}
	fmt.Println("Synthetic demo created. Credentials: " + credentials)
	return nil
}
