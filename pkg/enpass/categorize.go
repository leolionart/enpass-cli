package enpass

import (
	"encoding/json"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/pkg/errors"
)

type InfraCategoryPlan struct {
	Title  string `json:"title"`
	UUID   string `json:"uuid,omitempty"`
	Icon   string `json:"icon,omitempty"`
	Exists bool   `json:"exists"`
}

type InfraCategorizationItem struct {
	UUID               string   `json:"uuid"`
	Title              string   `json:"title"`
	Login              string   `json:"login"`
	CurrentCategory    string   `json:"current_category"`
	TargetCategory     string   `json:"target_category"`
	TargetCategoryUUID string   `json:"target_category_uuid,omitempty"`
	Tags               []string `json:"tags,omitempty"`
	Reason             string   `json:"reason"`
}

type InfraCategorizationReport struct {
	GeneratedAt int64                     `json:"generated_at"`
	Categories  []InfraCategoryPlan       `json:"categories"`
	Counts      map[string]int            `json:"counts"`
	Items       []InfraCategorizationItem `json:"items"`
}

type InfraApplyResult struct {
	BackupPath        string         `json:"backup_path,omitempty"`
	CategoriesCreated []Category     `json:"categories_created"`
	Moved             []CategoryMove `json:"moved"`
}

type CategoryMove struct {
	UUID           string `json:"uuid"`
	Title          string `json:"title"`
	FromCategory   string `json:"from_category"`
	ToCategory     string `json:"to_category"`
	ToCategoryUUID string `json:"to_category_uuid"`
}

var defaultInfraCategoryPlans = []InfraCategoryPlan{
	{Title: "Password"},
}

var privateAddressPattern = regexp.MustCompile(`(?i)(^|[^0-9])((10|127)\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}|172\.(1[6-9]|2[0-9]|3[0-1])\.[0-9]{1,3}\.[0-9]{1,3}|192\.168\.[0-9]{1,3}\.[0-9]{1,3}|localhost)([^0-9]|$)`)
var publicAddressPattern = regexp.MustCompile(`(?i)(^|[^0-9])([0-9]{1,3}\.){3}[0-9]{1,3}([^0-9]|$)`)

func (v *Vault) BuildInfraCategorizationReport() (InfraCategorizationReport, error) {
	if v.db == nil {
		return InfraCategorizationReport{}, errors.New("vault is not initialized")
	}

	report := InfraCategorizationReport{
		GeneratedAt: time.Now().Unix(),
		Counts:      map[string]int{},
	}

	categories, err := v.ListCategories()
	if err != nil {
		return report, err
	}

	categoryByTitle := map[string]Category{}
	for _, category := range categories {
		if category.Deleted {
			continue
		}
		categoryByTitle[strings.ToLower(category.Title)] = category
	}

	for _, plan := range defaultInfraCategoryPlans {
		if category, ok := categoryByTitle[strings.ToLower(plan.Title)]; ok {
			plan.UUID = category.UUID
			plan.Icon = category.Icon
			plan.Exists = true
		}
		report.Categories = append(report.Categories, plan)
	}

	cards, err := v.GetEntries("password", nil)
	if err != nil {
		return report, err
	}
	sort.SliceStable(cards, func(i, j int) bool {
		return strings.ToLower(cards[i].Title) < strings.ToLower(cards[j].Title)
	})

	for _, card := range cards {
		if card.IsTrashed() || card.IsDeleted() || strings.ToLower(card.Category) != "login" {
			continue
		}
		fields, err := v.GetPublicFields(card.UUID)
		if err != nil {
			return report, err
		}
		target, tags, reason := classifyInfraItem(card, fields)
		if target == "" {
			continue
		}

		item := InfraCategorizationItem{
			UUID:            card.UUID,
			Title:           card.Title,
			Login:           card.Subtitle,
			CurrentCategory: card.Category,
			TargetCategory:  target,
			Tags:            tags,
			Reason:          reason,
		}
		if category, ok := categoryByTitle[strings.ToLower(target)]; ok {
			item.TargetCategoryUUID = category.UUID
		}
		report.Items = append(report.Items, item)
		report.Counts[target]++
	}

	return report, nil
}

func LoadInfraCategorizationReport(path string) (InfraCategorizationReport, error) {
	var report InfraCategorizationReport
	data, err := os.ReadFile(path)
	if err != nil {
		return report, errors.Wrap(err, "could not read categorization report")
	}
	if err := json.Unmarshal(data, &report); err != nil {
		return report, errors.Wrap(err, "could not parse categorization report")
	}
	return report, nil
}

func (v *Vault) ApplyInfraCategorizationReport(report InfraCategorizationReport) (InfraApplyResult, error) {
	result := InfraApplyResult{}
	categoryByTitle := map[string]Category{}

	categories, err := v.ListCategories()
	if err != nil {
		return result, err
	}
	for _, category := range categories {
		if !category.Deleted {
			categoryByTitle[strings.ToLower(category.Title)] = category
		}
	}

	for _, plan := range report.Categories {
		if plan.Title == "" {
			continue
		}
		if _, ok := categoryByTitle[strings.ToLower(plan.Title)]; ok {
			continue
		}
		created, err := v.CreateCategory(plan.Title, plan.Icon)
		if err != nil {
			return result, err
		}
		categoryByTitle[strings.ToLower(created.Category.Title)] = created.Category
		if created.Created {
			result.CategoriesCreated = append(result.CategoriesCreated, created.Category)
		}
	}

	for _, item := range report.Items {
		category, ok := categoryByTitle[strings.ToLower(item.TargetCategory)]
		if !ok {
			return result, errors.Errorf("target category %q not found", item.TargetCategory)
		}

		current, err := v.GetEntryByUUID(item.UUID)
		if err != nil {
			return result, errors.Wrapf(err, "could not read entry %s", item.UUID)
		}
		if current.IsTrashed() || current.IsDeleted() {
			return result, errors.Errorf("entry %q is trashed or deleted", item.Title)
		}
		if current.Category != item.CurrentCategory {
			return result, errors.Errorf("entry %q category changed from %q to %q; regenerate dry-run", item.Title, item.CurrentCategory, current.Category)
		}

		if err := v.UpdateEntryCategory(item.UUID, category.UUID); err != nil {
			return result, errors.Wrapf(err, "could not move entry %s", item.UUID)
		}

		result.Moved = append(result.Moved, CategoryMove{
			UUID:           item.UUID,
			Title:          item.Title,
			FromCategory:   item.CurrentCategory,
			ToCategory:     item.TargetCategory,
			ToCategoryUUID: category.UUID,
		})
	}

	return result, nil
}

func classifyInfraItem(card Card, fields []PublicField) (string, []string, string) {
	values := []string{card.Title, card.Subtitle, card.Note}
	for _, field := range fields {
		values = append(values, field.Label, field.Type, field.Value)
	}
	text := strings.ToLower(strings.Join(values, " "))
	title := strings.ToLower(card.Title)
	login := strings.ToLower(strings.TrimSpace(card.Subtitle))

	if containsAny(text, "router", "modem", "openwrt", "mikrotik", "zte", "ruijie", "wireguard", "gateway") ||
		containsAny(title, "wi-fi", "wifi", " ap") {
		return "Password", []string{"infra", "network"}, "network device keyword"
	}
	if containsAny(text, "phpmyadmin", "mysql", "mariadb", "postgres", "mongodb", "redis", "influxdb") ||
		containsAny(title, " db", "database") {
		return "Password", []string{"infra", "database"}, "database/admin keyword"
	}
	if login == "root" || login == "ubuntu" || strings.Contains(text, "ssh://") ||
		containsAny(title, "ssh", "server", "vps", "proxmox", "pbs", "idrac", "ilo") {
		return "Password", []string{"infra", "server"}, "server or host-login keyword"
	}
	if containsAny(text, "cloudflare", "cpanel", "directadmin", "hosting", "hostinger", "bkdata", "vps") {
		return "Password", []string{"infra", "hosting-cloud"}, "hosting/cloud keyword"
	}
	if privateAddressPattern.MatchString(text) ||
		containsAny(text, "home assistant", "homeassistant", "portainer", "jellyfin", "frigate", "scrypted", "dockge", "runtipi", "tautulli", "audiobookshelf", "paperless", "filebrowser", "n8n") {
		return "Password", []string{"infra", "home-lab"}, "private address or self-hosted app keyword"
	}
	if publicAddressPattern.MatchString(text) && containsAny(text, ":8006", ":8007", ":9000", ":9443", ":2222", ":2083") {
		return "Password", []string{"infra", "hosting-cloud"}, "public server panel address"
	}

	return "", nil, ""
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}
