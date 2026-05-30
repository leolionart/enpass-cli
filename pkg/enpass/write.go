package enpass

import (
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
)

// EntryData holds the data for creating or updating an entry
type EntryData struct {
	Title    string
	Username string
	Password string
	URL      string
	Notes    string
	Category string
}

// CreateEntry creates a new password entry in the vault
func (v *Vault) CreateEntry(entry *EntryData) (string, error) {
	if v.db == nil {
		return "", errors.New("vault is not initialized")
	}

	if entry.Title == "" {
		return "", errors.New("title is required")
	}

	// Generate UUID
	entryUUID := uuid.New().String()
	now := time.Now().Unix()

	// Set defaults
	category := entry.Category
	if category == "" {
		category = "login"
	}

	// Start transaction
	tx, err := v.db.Begin()
	if err != nil {
		return "", errors.Wrap(err, "could not begin transaction")
	}
	defer tx.Rollback()

	// Encrypt password first to get the key
	var encryptedValue string
	var itemKey []byte
	if entry.Password != "" {
		var err error
		encryptedValue, itemKey, err = EncryptValue(entry.Password, entryUUID)
		if err != nil {
			return "", errors.Wrap(err, "could not encrypt password")
		}
	}

	// Insert into item table (key is stored here, not in itemfield)
	_, err = tx.Exec(`
		INSERT INTO item (
			uuid, created_at, meta_updated_at, field_updated_at, title, subtitle,
			note, trashed, deleted, category, icon, template, last_used, key, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, 0, 0, ?, ?, ?, 0, ?, ?)
	`, entryUUID, now, now, now, entry.Title, entry.Username,
		entry.Notes, category, defaultLoginIcon, "login.default", itemKey, now)
	if err != nil {
		return "", errors.Wrap(err, "could not insert item")
	}

	// Insert password field (value only, key is in item table)
	if entry.Password != "" {
		if err := insertEntryField(tx, entryUUID, "", encryptedValue, entry.Password, 1, "password", 2, "no", 0, now); err != nil {
			return "", errors.Wrap(err, "could not insert password field")
		}
	}

	// Insert username field (not encrypted)
	if entry.Username != "" {
		if err := insertEntryField(tx, entryUUID, "", entry.Username, entry.Username, 0, "username", 1, "", -1, now); err != nil {
			return "", errors.Wrap(err, "could not insert username field")
		}
	}

	// Insert URL field (not encrypted)
	if entry.URL != "" {
		if err := insertEntryField(tx, entryUUID, "", entry.URL, entry.URL, 0, "url", 3, "", -1, now); err != nil {
			return "", errors.Wrap(err, "could not insert URL field")
		}
	}

	if err := tx.Commit(); err != nil {
		return "", errors.Wrap(err, "could not commit transaction")
	}

	v.logger.WithField("uuid", entryUUID).Debug("created entry")
	return entryUUID, nil
}

const defaultLoginIcon = `{"fav":"","image":{"file":"misc/login"},"type":1,"uuid":""}`

func insertEntryField(tx *sql.Tx, entryUUID, label, value, hashValue string, sensitive int, fieldType string, order int, initial string, strength int, now int64) error {
	fieldUID, err := nextItemFieldUID(tx)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`
		INSERT INTO itemfield (
			item_uuid, item_field_uid, label, value, deleted, sensitive, historical,
			type, form_id, updated_at, value_updated_at, orde, wearable, history,
			initial, hash, strength, algo_version, expiry, excluded, pwned_check_time, extra
		) VALUES (?, ?, ?, ?, 0, ?, 1, ?, '', ?, ?, ?, 0, '', ?, ?, ?, 1, 0, 0, 0, '')
	`, entryUUID, fieldUID, label, value, sensitive, fieldType, now, now, order, initial, sha1Hex(hashValue), strength)
	return err
}

func nextItemFieldUID(tx *sql.Tx) (int64, error) {
	var fieldUID int64
	if err := tx.QueryRow("SELECT COALESCE(MAX(item_field_uid), 0) + 1 FROM itemfield").Scan(&fieldUID); err != nil {
		return 0, errors.Wrap(err, "could not allocate item field uid")
	}
	return fieldUID, nil
}

func sha1Hex(value string) string {
	if value == "" {
		return ""
	}
	sum := sha1.Sum([]byte(value))
	return hex.EncodeToString(sum[:])
}

// UpdateEntry updates an existing entry in the vault
func (v *Vault) UpdateEntry(entryUUID string, updates *EntryData) error {
	if v.db == nil {
		return errors.New("vault is not initialized")
	}

	now := time.Now().Unix()

	// Start transaction
	tx, err := v.db.Begin()
	if err != nil {
		return errors.Wrap(err, "could not begin transaction")
	}
	defer tx.Rollback()

	// Update item table if title, notes, or category changed
	if updates.Title != "" || updates.Notes != "" || updates.Category != "" {
		query := "UPDATE item SET field_updated_at = ?"
		args := []interface{}{now}

		if updates.Title != "" {
			query += ", title = ?"
			args = append(args, updates.Title)
		}
		if updates.Notes != "" {
			query += ", note = ?"
			args = append(args, updates.Notes)
		}
		if updates.Category != "" {
			query += ", category = ?"
			args = append(args, updates.Category)
		}

		query += " WHERE uuid = ?"
		args = append(args, entryUUID)

		_, err = tx.Exec(query, args...)
		if err != nil {
			return errors.Wrap(err, "could not update item")
		}
	}

	// Update username in item.subtitle and itemfield
	if updates.Username != "" {
		_, err = tx.Exec("UPDATE item SET subtitle = ?, field_updated_at = ? WHERE uuid = ?",
			updates.Username, now, entryUUID)
		if err != nil {
			return errors.Wrap(err, "could not update subtitle")
		}

		// Update or insert username field
		result, err := tx.Exec("UPDATE itemfield SET value = ? WHERE item_uuid = ? AND type = ?",
			updates.Username, entryUUID, "username")
		if err != nil {
			return errors.Wrap(err, "could not update username field")
		}
		rowsAffected, _ := result.RowsAffected()
		if rowsAffected == 0 {
			if err := insertEntryField(tx, entryUUID, "", updates.Username, updates.Username, 0, "username", 1, "", -1, now); err != nil {
				return errors.Wrap(err, "could not insert username field")
			}
		}
	}

	// Update password (encrypted) - key is stored in item table
	if updates.Password != "" {
		encryptedValue, itemKey, err := EncryptValue(updates.Password, entryUUID)
		if err != nil {
			return errors.Wrap(err, "could not encrypt password")
		}

		// Update key in item table
		_, err = tx.Exec("UPDATE item SET key = ?, field_updated_at = ? WHERE uuid = ?",
			itemKey, now, entryUUID)
		if err != nil {
			return errors.Wrap(err, "could not update item key")
		}

		// Update password value in itemfield
		result, err := tx.Exec("UPDATE itemfield SET value = ? WHERE item_uuid = ? AND type = ?",
			encryptedValue, entryUUID, "password")
		if err != nil {
			return errors.Wrap(err, "could not update password field")
		}
		rowsAffected, _ := result.RowsAffected()
		if rowsAffected == 0 {
			if err := insertEntryField(tx, entryUUID, "", encryptedValue, updates.Password, 1, "password", 2, "no", 0, now); err != nil {
				return errors.Wrap(err, "could not insert password field")
			}
		}
	}

	// Update URL
	if updates.URL != "" {
		result, err := tx.Exec("UPDATE itemfield SET value = ? WHERE item_uuid = ? AND type = ?",
			updates.URL, entryUUID, "url")
		if err != nil {
			return errors.Wrap(err, "could not update URL field")
		}
		rowsAffected, _ := result.RowsAffected()
		if rowsAffected == 0 {
			if err := insertEntryField(tx, entryUUID, "", updates.URL, updates.URL, 0, "url", 3, "", -1, now); err != nil {
				return errors.Wrap(err, "could not insert URL field")
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, "could not commit transaction")
	}

	v.logger.WithField("uuid", entryUUID).Debug("updated entry")
	return nil
}

// TrashEntry moves an entry to the trash
func (v *Vault) TrashEntry(entryUUID string) error {
	if v.db == nil {
		return errors.New("vault is not initialized")
	}

	now := time.Now().Unix()
	result, err := v.db.Exec("UPDATE item SET trashed = 1, field_updated_at = ? WHERE uuid = ?", now, entryUUID)
	if err != nil {
		return errors.Wrap(err, "could not trash entry")
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return errors.New("entry not found")
	}

	v.logger.WithField("uuid", entryUUID).Debug("trashed entry")
	return nil
}

// RestoreEntry restores an entry from the trash
func (v *Vault) RestoreEntry(entryUUID string) error {
	if v.db == nil {
		return errors.New("vault is not initialized")
	}

	now := time.Now().Unix()
	result, err := v.db.Exec("UPDATE item SET trashed = 0, field_updated_at = ? WHERE uuid = ?", now, entryUUID)
	if err != nil {
		return errors.Wrap(err, "could not restore entry")
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return errors.New("entry not found")
	}

	v.logger.WithField("uuid", entryUUID).Debug("restored entry")
	return nil
}

// DeleteEntry permanently deletes an entry from the vault
func (v *Vault) DeleteEntry(entryUUID string) error {
	if v.db == nil {
		return errors.New("vault is not initialized")
	}

	// Start transaction
	tx, err := v.db.Begin()
	if err != nil {
		return errors.Wrap(err, "could not begin transaction")
	}
	defer tx.Rollback()

	// Delete from itemfield first (foreign key constraint)
	_, err = tx.Exec("DELETE FROM itemfield WHERE item_uuid = ?", entryUUID)
	if err != nil {
		return errors.Wrap(err, "could not delete item fields")
	}

	// Delete from item
	result, err := tx.Exec("DELETE FROM item WHERE uuid = ?", entryUUID)
	if err != nil {
		return errors.Wrap(err, "could not delete item")
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return errors.New("entry not found")
	}

	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, "could not commit transaction")
	}

	v.logger.WithField("uuid", entryUUID).Debug("deleted entry")
	return nil
}

// GetEntryByUUID retrieves a single entry by its UUID (including trashed)
func (v *Vault) GetEntryByUUID(entryUUID string) (*Card, error) {
	if v.db == nil {
		return nil, errors.New("vault is not initialized")
	}

	row := v.db.QueryRow(`
		SELECT item.uuid, itemfield.type, item.created_at, item.field_updated_at, item.title,
		       item.subtitle, item.note, item.trashed, item.deleted, item.category,
		       itemfield.label, itemfield.value, item.key, item.usage_count, item.last_used, itemfield.sensitive, item.icon
		FROM item
		INNER JOIN itemfield ON item.uuid = itemfield.item_uuid
		WHERE item.uuid = ? AND itemfield.sensitive = 1
		LIMIT 1
	`, entryUUID)

	var card Card
	err := row.Scan(
		&card.UUID, &card.Type, &card.CreatedAt, &card.UpdatedAt, &card.Title,
		&card.Subtitle, &card.Note, &card.Trashed, &card.Deleted, &card.Category,
		&card.Label, &card.value, &card.itemKey, &card.UsageCount, &card.LastUsed, &card.Sensitive, &card.Icon,
	)
	if err != nil {
		return nil, errors.Wrap(err, "could not retrieve entry")
	}

	card.RawValue = card.value
	return &card, nil
}
