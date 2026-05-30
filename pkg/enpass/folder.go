package enpass

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
)

type Folder struct {
	UUID       string `json:"uuid"`
	Title      string `json:"title"`
	Icon       string `json:"icon"`
	UpdatedAt  int64  `json:"updated_at"`
	Deleted    bool   `json:"deleted"`
	ParentUUID string `json:"parent_uuid"`
}

type FolderWriteResult struct {
	Folder  Folder `json:"folder"`
	Created bool   `json:"created"`
}

type FolderAssignment struct {
	ItemUUID   string `json:"item_uuid"`
	ItemTitle  string `json:"item_title,omitempty"`
	FolderUUID string `json:"folder_uuid"`
	Folder     string `json:"folder"`
	Created    bool   `json:"created"`
}

type InfraTagApplyResult struct {
	BackupPath     string             `json:"backup_path,omitempty"`
	FoldersCreated []Folder           `json:"folders_created"`
	Assigned       []FolderAssignment `json:"assigned"`
}

func (v *Vault) ListFolders() ([]Folder, error) {
	if v.db == nil {
		return nil, errors.New("vault is not initialized")
	}
	if !v.tableExists("folder") {
		return nil, errors.New("folder table not found")
	}

	rows, err := v.db.Query(`
		SELECT uuid, title, icon, updated_at, deleted, coalesce(parent_uuid, '')
		FROM folder
		ORDER BY deleted, lower(title), uuid
	`)
	if err != nil {
		return nil, errors.Wrap(err, "could not retrieve folders")
	}
	defer rows.Close()

	var folders []Folder
	for rows.Next() {
		var folder Folder
		var deleted int64
		if err := rows.Scan(&folder.UUID, &folder.Title, &folder.Icon, &folder.UpdatedAt, &deleted, &folder.ParentUUID); err != nil {
			return nil, errors.Wrap(err, "could not read folder")
		}
		folder.Deleted = deleted != 0
		folders = append(folders, folder)
	}
	if err := rows.Err(); err != nil {
		return nil, errors.Wrap(err, "error iterating folders")
	}
	return folders, nil
}

func (v *Vault) CreateFolder(title string, icon string, parentUUID string) (FolderWriteResult, error) {
	if v.db == nil {
		return FolderWriteResult{}, errors.New("vault is not initialized")
	}
	if !v.tableExists("folder") {
		return FolderWriteResult{}, errors.New("folder table not found")
	}

	title = strings.TrimSpace(title)
	if title == "" {
		return FolderWriteResult{}, errors.New("folder title is required")
	}
	if icon == "" {
		icon = "1008"
	}

	folders, err := v.ListFolders()
	if err != nil {
		return FolderWriteResult{}, err
	}
	normalized := strings.ToLower(title)
	for _, folder := range folders {
		if !folder.Deleted && strings.ToLower(folder.Title) == normalized && folder.ParentUUID == parentUUID {
			return FolderWriteResult{Folder: folder, Created: false}, nil
		}
	}

	folder := Folder{
		UUID:       uuid.New().String(),
		Title:      title,
		Icon:       icon,
		UpdatedAt:  time.Now().Unix(),
		ParentUUID: parentUUID,
	}
	_, err = v.db.Exec(`
		INSERT INTO folder (uuid, title, icon, updated_at, deleted, parent_uuid, extra)
		VALUES (?, ?, ?, ?, 0, ?, '')
	`, folder.UUID, folder.Title, folder.Icon, folder.UpdatedAt, folder.ParentUUID)
	if err != nil {
		return FolderWriteResult{}, errors.Wrap(err, "could not create folder")
	}
	return FolderWriteResult{Folder: folder, Created: true}, nil
}

func (v *Vault) RenameFolder(folderUUID string, title string) (Folder, error) {
	if v.db == nil {
		return Folder{}, errors.New("vault is not initialized")
	}

	title = strings.TrimSpace(title)
	if title == "" {
		return Folder{}, errors.New("folder title is required")
	}

	result, err := v.db.Exec(`
		UPDATE folder
		SET title = ?, updated_at = ?
		WHERE uuid = ? AND deleted = 0
	`, title, time.Now().Unix(), folderUUID)
	if err != nil {
		return Folder{}, errors.Wrap(err, "could not rename folder")
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return Folder{}, errors.New("folder not found")
	}

	folders, err := v.ListFolders()
	if err != nil {
		return Folder{}, err
	}
	for _, folder := range folders {
		if folder.UUID == folderUUID {
			return folder, nil
		}
	}
	return Folder{}, errors.New("renamed folder not found")
}

func (v *Vault) DeleteFolder(folder string) (Folder, error) {
	if v.db == nil {
		return Folder{}, errors.New("vault is not initialized")
	}

	folders, err := v.ListFolders()
	if err != nil {
		return Folder{}, err
	}

	normalized := strings.ToLower(strings.TrimSpace(folder))
	var target Folder
	for _, candidate := range folders {
		if candidate.Deleted {
			continue
		}
		if strings.ToLower(candidate.UUID) == normalized || strings.ToLower(candidate.Title) == normalized {
			target = candidate
			break
		}
	}
	if target.UUID == "" {
		return Folder{}, errors.Errorf("folder %q not found", folder)
	}

	now := time.Now().Unix()
	tx, err := v.db.Begin()
	if err != nil {
		return Folder{}, errors.Wrap(err, "could not begin transaction")
	}
	defer tx.Rollback()

	if _, err := tx.Exec("UPDATE folder SET deleted = 1, updated_at = ? WHERE uuid = ?", now, target.UUID); err != nil {
		return Folder{}, errors.Wrap(err, "could not delete folder")
	}
	if _, err := tx.Exec("UPDATE folder_items SET deleted = 1, updated_at = ? WHERE folder_uuid = ?", now, target.UUID); err != nil {
		return Folder{}, errors.Wrap(err, "could not delete folder items")
	}
	if err := tx.Commit(); err != nil {
		return Folder{}, errors.Wrap(err, "could not commit folder delete")
	}

	target.Deleted = true
	target.UpdatedAt = now
	return target, nil
}

func (v *Vault) AddEntryToFolder(itemUUID string, itemTitle string, folder Folder) (FolderAssignment, error) {
	if v.db == nil {
		return FolderAssignment{}, errors.New("vault is not initialized")
	}
	if !v.tableExists("folder_items") {
		return FolderAssignment{}, errors.New("folder_items table not found")
	}

	var deleted int64
	err := v.db.QueryRow(`
		SELECT deleted
		FROM folder_items
		WHERE folder_uuid = ? AND item_uuid = ?
	`, folder.UUID, itemUUID).Scan(&deleted)
	if err == nil && deleted == 0 {
		return FolderAssignment{ItemUUID: itemUUID, ItemTitle: itemTitle, FolderUUID: folder.UUID, Folder: folder.Title, Created: false}, nil
	}

	_, err = v.db.Exec(`
		INSERT INTO folder_items (folder_uuid, item_uuid, updated_at, deleted, extra)
		VALUES (?, ?, ?, 0, '')
	`, folder.UUID, itemUUID, time.Now().Unix())
	if err != nil {
		return FolderAssignment{}, errors.Wrap(err, "could not assign folder")
	}
	return FolderAssignment{ItemUUID: itemUUID, ItemTitle: itemTitle, FolderUUID: folder.UUID, Folder: folder.Title, Created: true}, nil
}

func (v *Vault) ApplyInfraTagsFromReport(report InfraCategorizationReport) (InfraTagApplyResult, error) {
	result := InfraTagApplyResult{}
	folderByTitle := map[string]Folder{}

	for _, folder := range mustListFolders(v) {
		if !folder.Deleted {
			folderByTitle[strings.ToLower(folder.Title)] = folder
		}
	}

	for _, item := range report.Items {
		for _, tag := range item.Tags {
			folder, ok := folderByTitle[strings.ToLower(tag)]
			if !ok {
				created, err := v.CreateFolder(tag, "", "")
				if err != nil {
					return result, err
				}
				folder = created.Folder
				folderByTitle[strings.ToLower(tag)] = folder
				if created.Created {
					result.FoldersCreated = append(result.FoldersCreated, folder)
				}
			}

			assignment, err := v.AddEntryToFolder(item.UUID, item.Title, folder)
			if err != nil {
				return result, err
			}
			result.Assigned = append(result.Assigned, assignment)
		}
	}

	return result, nil
}

func mustListFolders(v *Vault) []Folder {
	folders, err := v.ListFolders()
	if err != nil {
		return nil
	}
	return folders
}
