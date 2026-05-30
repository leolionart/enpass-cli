package enpass

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
)

type Category struct {
	UUID      string `json:"uuid"`
	Title     string `json:"title"`
	Icon      string `json:"icon"`
	UpdatedAt int64  `json:"updated_at"`
	Deleted   bool   `json:"deleted"`
	BuiltIn   bool   `json:"built_in"`
}

type CategoryWriteResult struct {
	Category Category `json:"category"`
	Created  bool     `json:"created"`
}

var builtInCategories = []Category{
	{UUID: "login", Title: "Login", BuiltIn: true},
	{UUID: "password", Title: "Password", BuiltIn: true},
	{UUID: "creditcard", Title: "Credit Card", BuiltIn: true},
	{UUID: "finance", Title: "Finance", BuiltIn: true},
	{UUID: "identity", Title: "Identity", BuiltIn: true},
	{UUID: "license", Title: "License", BuiltIn: true},
	{UUID: "computer", Title: "Computer", BuiltIn: true},
	{UUID: "travel", Title: "Travel", BuiltIn: true},
}

func (v *Vault) ListCategories() ([]Category, error) {
	if v.db == nil {
		return nil, errors.New("vault is not initialized")
	}

	categories := append([]Category{}, builtInCategories...)
	if !v.tableExists("category") {
		return categories, nil
	}

	rows, err := v.db.Query(`
		SELECT uuid, title, icon, updated_at, deleted
		FROM category
		ORDER BY deleted, lower(title), uuid
	`)
	if err != nil {
		return nil, errors.Wrap(err, "could not retrieve categories")
	}
	defer rows.Close()

	for rows.Next() {
		var category Category
		var deleted int64
		if err := rows.Scan(&category.UUID, &category.Title, &category.Icon, &category.UpdatedAt, &deleted); err != nil {
			return nil, errors.Wrap(err, "could not read category")
		}
		category.Deleted = deleted != 0
		categories = append(categories, category)
	}
	if err := rows.Err(); err != nil {
		return nil, errors.Wrap(err, "error iterating categories")
	}

	return categories, nil
}

func (v *Vault) ResolveCategory(category string) (string, error) {
	category = strings.TrimSpace(category)
	if category == "" {
		return "login", nil
	}

	categories, err := v.ListCategories()
	if err != nil {
		return "", err
	}

	normalized := strings.ToLower(category)
	var matches []Category
	for _, candidate := range categories {
		if candidate.Deleted {
			continue
		}
		if strings.ToLower(candidate.UUID) == normalized || strings.ToLower(candidate.Title) == normalized {
			matches = append(matches, candidate)
		}
	}

	if len(matches) == 1 {
		return matches[0].UUID, nil
	}
	if len(matches) > 1 {
		return "", errors.Errorf("multiple categories match %q", category)
	}

	if _, err := uuid.Parse(category); err == nil {
		return category, nil
	}

	return "", errors.Errorf("category %q not found", category)
}

func (v *Vault) CreateCategory(title string, icon string) (CategoryWriteResult, error) {
	if v.db == nil {
		return CategoryWriteResult{}, errors.New("vault is not initialized")
	}

	title = strings.TrimSpace(title)
	if title == "" {
		return CategoryWriteResult{}, errors.New("category title is required")
	}

	categories, err := v.ListCategories()
	if err != nil {
		return CategoryWriteResult{}, err
	}

	normalized := strings.ToLower(title)
	for _, category := range categories {
		if !category.Deleted && strings.ToLower(category.Title) == normalized {
			return CategoryWriteResult{Category: category, Created: false}, nil
		}
	}

	if !v.tableExists("category") {
		return CategoryWriteResult{}, errors.New("category table not found")
	}

	category := Category{
		UUID:      uuid.New().String(),
		Title:     title,
		Icon:      icon,
		UpdatedAt: time.Now().Unix(),
		Deleted:   false,
		BuiltIn:   false,
	}

	_, err = v.db.Exec(`
		INSERT INTO category (uuid, title, icon, updated_at, deleted, extra)
		VALUES (?, ?, ?, ?, 0, '')
	`, category.UUID, category.Title, category.Icon, category.UpdatedAt)
	if err != nil {
		return CategoryWriteResult{}, errors.Wrap(err, "could not create category")
	}

	return CategoryWriteResult{Category: category, Created: true}, nil
}

func (v *Vault) DeleteCategory(category string) (Category, error) {
	if v.db == nil {
		return Category{}, errors.New("vault is not initialized")
	}

	resolved, err := v.ResolveCategory(category)
	if err != nil {
		return Category{}, err
	}

	categories, err := v.ListCategories()
	if err != nil {
		return Category{}, err
	}

	var target Category
	for _, candidate := range categories {
		if candidate.UUID == resolved {
			target = candidate
			break
		}
	}
	if target.UUID == "" {
		return Category{}, errors.Errorf("category %q not found", category)
	}
	if target.BuiltIn {
		return Category{}, errors.Errorf("cannot delete built-in category %q", target.Title)
	}

	result, err := v.db.Exec(`
		UPDATE category
		SET deleted = 1, updated_at = ?
		WHERE uuid = ? AND deleted = 0
	`, time.Now().Unix(), target.UUID)
	if err != nil {
		return Category{}, errors.Wrap(err, "could not delete category")
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return Category{}, errors.New("category already deleted")
	}

	target.Deleted = true
	return target, nil
}

func (v *Vault) UpdateEntryCategory(entryUUID string, category string) error {
	if v.db == nil {
		return errors.New("vault is not initialized")
	}

	result, err := v.db.Exec(`
		UPDATE item
		SET category = ?, field_updated_at = ?, updated_at = ?
		WHERE uuid = ? AND deleted = 0
	`, category, time.Now().Unix(), time.Now().Unix(), entryUUID)
	if err != nil {
		return errors.Wrap(err, "could not update entry category")
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return errors.New("entry not found")
	}

	return nil
}

func (v *Vault) tableExists(name string) bool {
	var found string
	err := v.db.QueryRow(`
		SELECT name
		FROM sqlite_master
		WHERE type = 'table' AND name = ?
	`, name).Scan(&found)
	return err == nil
}
