package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/leolionart/enpass-cli/pkg/clipboard"
	"github.com/leolionart/enpass-cli/pkg/enpass"
	"github.com/leolionart/enpass-cli/pkg/unlock"
	"github.com/miquella/ask"
	"github.com/rivo/tview"
	"github.com/sirupsen/logrus"
)

const (
	// commands
	cmdVersion         = "version"
	cmdHelp            = "help"
	cmdDryRun          = "dryrun"
	cmdList            = "list"
	cmdShow            = "show"
	cmdSearch          = "search"
	cmdGet             = "get"
	cmdCopy            = "copy"
	cmdPass            = "pass"
	cmdUi              = "ui"
	cmdCreate          = "create"
	cmdEdit            = "edit"
	cmdCategories      = "categories"
	cmdCategorizeInfra = "categorize-infra"
	cmdFolders         = "folders"
	cmdApplyInfraTags  = "apply-infra-tags"
	cmdApplyOrgTags    = "apply-org-tags"
	cmdTrash           = "trash"
	cmdRestore         = "restore"
	cmdDelete          = "delete"

	// defaults
	defaultLogLevel        = logrus.InfoLevel
	pinMinLength           = 8
	pinDefaultKdfIterCount = 100000
)

var (
	// overwritten by go build
	version = "dev"
	// set of all commands
	commands = map[string]struct{}{
		cmdVersion: {}, cmdHelp: {}, cmdDryRun: {}, cmdList: {},
		cmdShow: {}, cmdSearch: {}, cmdGet: {}, cmdCopy: {}, cmdPass: {}, cmdUi: {},
		cmdCreate: {}, cmdEdit: {}, cmdCategories: {}, cmdCategorizeInfra: {}, cmdFolders: {}, cmdApplyInfraTags: {}, cmdApplyOrgTags: {}, cmdTrash: {}, cmdRestore: {}, cmdDelete: {},
	}
)

type sortMode string

const (
	sortNone    sortMode = ""
	sortTitle   sortMode = "title"
	sortLogin   sortMode = "login"
	sortCreated sortMode = "created"
	sortUpdated sortMode = "updated"
	sortUsed    sortMode = "used"
	sortUsage   sortMode = "usage"
)

type sortFlag struct {
	mode sortMode
}

func (s *sortFlag) String() string {
	return string(s.mode)
}

func (s *sortFlag) IsBoolFlag() bool {
	return true
}

func (s *sortFlag) Set(value string) error {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "true", "title", "name":
		s.mode = sortTitle
	case "false", "none":
		s.mode = sortNone
	case "login", "username", "user":
		s.mode = sortLogin
	case "created", "created_at", "create":
		s.mode = sortCreated
	case "updated", "modified", "changed", "updated_at", "field_updated_at":
		s.mode = sortUpdated
	case "used", "last_used", "lastused", "recent":
		s.mode = sortUsed
	case "usage", "usage_count", "use_count", "uses", "count":
		s.mode = sortUsage
	default:
		return fmt.Errorf("unsupported sort %q; use title, login, created, updated, used, or usage", value)
	}
	return nil
}

func formatUnixTime(value int64) string {
	if value <= 0 {
		return ""
	}
	return time.Unix(value, 0).Format(time.RFC3339)
}

type AgentCredential struct {
	UUID       string               `json:"uuid"`
	Title      string               `json:"title"`
	Login      string               `json:"login"`
	Password   string               `json:"password,omitempty"`
	Category   string               `json:"category"`
	Label      string               `json:"label"`
	Type       string               `json:"type"`
	Trashed    bool                 `json:"trashed"`
	CreatedAt  int64                `json:"created_at"`
	UpdatedAt  int64                `json:"updated_at"`
	LastUsed   int64                `json:"last_used"`
	UsageCount int64                `json:"usage_count"`
	Fields     []enpass.PublicField `json:"fields,omitempty"`
}

type Args struct {
	command string
	// params
	filters []string
	// flags
	vaultPath        *string
	cardType         *string
	keyFilePath      *string
	logLevelStr      *string
	jsonOutput       *bool
	nonInteractive   *bool
	pinEnable        *bool
	sort             sortFlag
	trashed          *bool
	and              *bool
	clipboardPrimary *bool
	field            *string
	details          *bool
	passwordCmd      *string
	// write command flags
	title      *string
	login      *string
	password   *string
	url        *string
	notes      *string
	category   *string
	icon       *string
	dryRun     *bool
	apply      *bool
	fromDryRun *string
	force      *bool
}

func (args *Args) parse() {
	args.vaultPath = flag.String("vault", os.Getenv("ENPASS_VAULT"), "Path to your Enpass vault. Defaults to ENPASS_VAULT.")
	args.cardType = flag.String("type", "password", "The type of your card. (password, ...)")
	args.keyFilePath = flag.String("keyfile", "", "Path to your Enpass vault keyfile.")
	args.logLevelStr = flag.String("log", defaultLogLevel.String(), "The log level from debug (5) to error (1).")
	args.jsonOutput = flag.Bool("json", false, "Output data in JSON format.")
	args.nonInteractive = flag.Bool("nonInteractive", false, "Disable prompts and fail instead.")
	args.pinEnable = flag.Bool("pin", false, "Enable PIN.")
	args.and = flag.Bool("and", false, "Combines filters with AND instead of default OR.")
	flag.Var(&args.sort, "sort", "Sort list/search/show output. Optional value: title, login, created, updated, used, usage. Metadata sorts default to newest/highest first.")
	args.trashed = flag.Bool("trashed", false, "Show trashed items in the 'list' and 'show' command.")
	args.clipboardPrimary = flag.Bool("clipboardPrimary", false, "Use primary X selection instead of clipboard for the 'copy' command.")
	args.field = flag.String("field", "password", "Field to print for the 'get' command: password, login, title, uuid, category, label, type, created_at, updated_at, last_used, usage_count.")
	args.details = flag.Bool("details", false, "Include non-sensitive item fields in the 'search' command.")
	args.passwordCmd = flag.String("password-command", "", "Execute command to retrieve vault password. Also controlled by ENPASS_PASSWORD_COMMAND.")
	// write command flags
	args.title = flag.String("title", "", "Entry title (for create/edit).")
	args.login = flag.String("login", "", "Username or email (for create/edit).")
	args.password = flag.String("password", "", "Password (for create/edit). Prompts if flag present without value.")
	args.url = flag.String("url", "", "URL (for create/edit).")
	args.notes = flag.String("notes", "", "Notes (for create/edit).")
	args.category = flag.String("category", "", "Category UUID or title (for create/edit).")
	args.icon = flag.String("icon", "", "Category icon (for categories create).")
	args.dryRun = flag.Bool("dry-run", false, "Preview changes without writing.")
	args.apply = flag.Bool("apply", false, "Apply a reviewed operation.")
	args.fromDryRun = flag.String("from-dry-run", "", "Path to a JSON dry-run report to apply.")
	args.force = flag.Bool("force", false, "Skip confirmation prompts.")
	flag.Parse()
	args.command = strings.ToLower(flag.Arg(0))
	if len(flag.Args()) > 1 {
		args.filters = flag.Args()[1:]
	} else {
		args.filters = []string{}
	}
}

func prompt(logger *logrus.Logger, args *Args, msg string) string {
	if !*args.nonInteractive {
		if response, err := ask.HiddenAsk("Enter " + msg + ": "); err != nil {
			logger.WithError(err).Fatal("could not prompt for " + msg)
		} else {
			return response
		}
	}
	return ""
}

func printHelp() {
	fmt.Println("Usage: enpass-cli [flags] <command> [filters...]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  get <filter>      Print one field from one matching entry (default: password)")
	fmt.Println("  search [filter]   Search entries and output agent-friendly JSON")
	fmt.Println("  list [filter]     List entries (without passwords)")
	fmt.Println("  show [filter]     Show entries (with passwords)")
	fmt.Println("  copy <filter>     Copy password to clipboard")
	fmt.Println("  pass <filter>     Print password to stdout")
	fmt.Println("  ui                Interactive terminal UI")
	fmt.Println("  create            Create a new entry")
	fmt.Println("  edit <filter>     Edit an existing entry")
	fmt.Println("  categories        List available categories")
	fmt.Println("  categorize-infra  Dry-run or apply Login-to-infra category moves")
	fmt.Println("  folders           List or create folders used as tags")
	fmt.Println("  apply-infra-tags  Assign folders/tags from a categorization report")
	fmt.Println("  apply-org-tags    Assign organization/domain folders/tags")
	fmt.Println("  trash <filter>    Move entry to trash")
	fmt.Println("  restore <filter>  Restore entry from trash")
	fmt.Println("  delete <filter>   Permanently delete entry")
	fmt.Println("  dryrun            Test vault opening")
	fmt.Println("  version           Print version")
	fmt.Println("  help              Print this help")
	fmt.Println()
	fmt.Println("Flags:")
	flag.Usage()
}

func sortEntries(cards []enpass.Card, mode sortMode) {
	switch mode {
	case sortNone:
		return
	case sortLogin:
		sort.SliceStable(cards, func(i, j int) bool {
			left := strings.ToLower(cards[i].Subtitle)
			right := strings.ToLower(cards[j].Subtitle)
			if left == right {
				return strings.ToLower(cards[i].Title) < strings.ToLower(cards[j].Title)
			}
			return left < right
		})
	case sortCreated:
		sortNewestFirst(cards, func(card enpass.Card) int64 { return card.CreatedAt })
	case sortUpdated:
		sortNewestFirst(cards, func(card enpass.Card) int64 { return card.UpdatedAt })
	case sortUsed:
		sortNewestFirst(cards, func(card enpass.Card) int64 { return card.LastUsed })
	case sortUsage:
		sortNewestFirst(cards, func(card enpass.Card) int64 { return card.UsageCount })
	default:
		// Sort by username preserving original order.
		sort.SliceStable(cards, func(i, j int) bool {
			return strings.ToLower(cards[i].Subtitle) < strings.ToLower(cards[j].Subtitle)
		})
		// Sort by title, preserving username order.
		sort.SliceStable(cards, func(i, j int) bool {
			return strings.ToLower(cards[i].Title) < strings.ToLower(cards[j].Title)
		})
	}
}

func sortNewestFirst(cards []enpass.Card, value func(enpass.Card) int64) {
	sort.SliceStable(cards, func(i, j int) bool {
		left := value(cards[i])
		right := value(cards[j])
		if left == right {
			leftTitle := strings.ToLower(cards[i].Title)
			rightTitle := strings.ToLower(cards[j].Title)
			if leftTitle == rightTitle {
				return strings.ToLower(cards[i].Subtitle) < strings.ToLower(cards[j].Subtitle)
			}
			return leftTitle < rightTitle
		}
		return left > right
	})
}

func listEntries(logger *logrus.Logger, vault *enpass.Vault, args *Args) {
	cards, err := vault.GetEntries(*args.cardType, args.filters)
	if err != nil {
		logger.WithError(err).Fatal("could not retrieve cards")
	}
	sortEntries(cards, args.sort.mode)

	data, err := prepareCardData(cards, false, args)
	if err != nil {
		logger.WithError(err).Fatal(err.Error())
	}

	outputDataOrLog(logger, data, args)
}

func showEntries(logger *logrus.Logger, vault *enpass.Vault, args *Args) {
	cards, err := vault.GetEntries(*args.cardType, args.filters)
	if err != nil {
		logger.WithError(err).Fatal("could not retrieve cards")
	}
	sortEntries(cards, args.sort.mode)

	data, err := prepareCardData(cards, true, args)
	if err != nil {
		logger.WithError(err).Fatal(err.Error())
	}

	outputDataOrLog(logger, data, args)
}

func toAgentCredential(vault *enpass.Vault, card *enpass.Card, includePassword bool, includeDetails bool) (AgentCredential, error) {
	credential := AgentCredential{
		UUID:       card.UUID,
		Title:      card.Title,
		Login:      card.Subtitle,
		Category:   card.Category,
		Label:      card.Label,
		Type:       card.Type,
		Trashed:    card.IsTrashed(),
		CreatedAt:  card.CreatedAt,
		UpdatedAt:  card.UpdatedAt,
		LastUsed:   card.LastUsed,
		UsageCount: card.UsageCount,
	}

	if includePassword {
		decrypted, err := card.Decrypt()
		if err != nil {
			return credential, err
		}
		credential.Password = decrypted
	}

	if includeDetails {
		fields, err := vault.GetPublicFields(card.UUID)
		if err != nil {
			return credential, err
		}
		credential.Fields = fields
	}

	return credential, nil
}

func searchEntries(logger *logrus.Logger, vault *enpass.Vault, args *Args) {
	cards, err := vault.GetEntries(*args.cardType, args.filters)
	if err != nil {
		logger.WithError(err).Fatal("could not retrieve cards")
	}
	sortEntries(cards, args.sort.mode)

	credentials := make([]AgentCredential, 0, len(cards))
	for _, card := range cards {
		if card.IsTrashed() && !*args.trashed {
			continue
		}
		credential, err := toAgentCredential(vault, &card, false, *args.details)
		if err != nil {
			logger.WithError(err).Fatal("could not prepare credential")
		}
		credentials = append(credentials, credential)
	}

	outputJSON(logger, credentials)
}

func listCategories(logger *logrus.Logger, vault *enpass.Vault, args *Args) {
	if len(args.filters) > 0 && strings.ToLower(args.filters[0]) == "create" {
		createCategory(logger, vault, args)
		return
	}
	if len(args.filters) > 0 && strings.ToLower(args.filters[0]) == "delete" {
		deleteCategory(logger, vault, args)
		return
	}

	categories, err := vault.ListCategories()
	if err != nil {
		logger.WithError(err).Fatal("could not retrieve categories")
	}

	if *args.jsonOutput {
		outputJSON(logger, categories)
		return
	}

	for _, category := range categories {
		if category.Deleted && !*args.trashed {
			continue
		}
		logger.Printf("> title: %s  uuid: %s  icon: %s  builtin: %t", category.Title, category.UUID, category.Icon, category.BuiltIn)
	}
}

func createCategory(logger *logrus.Logger, vault *enpass.Vault, args *Args) {
	title := strings.TrimSpace(strings.Join(args.filters[1:], " "))
	if title == "" {
		title = strings.TrimSpace(*args.title)
	}
	if title == "" {
		logger.Fatal("category title is required")
	}

	result, err := vault.CreateCategory(title, *args.icon)
	if err != nil {
		logger.WithError(err).Fatal("could not create category")
	}

	if *args.jsonOutput {
		outputJSON(logger, result)
		return
	}
	if result.Created {
		logger.Printf("Created category: %s (UUID: %s)", result.Category.Title, result.Category.UUID)
	} else {
		logger.Printf("Category already exists: %s (UUID: %s)", result.Category.Title, result.Category.UUID)
	}
}

func deleteCategory(logger *logrus.Logger, vault *enpass.Vault, args *Args) {
	title := strings.TrimSpace(strings.Join(args.filters[1:], " "))
	if title == "" {
		title = strings.TrimSpace(*args.category)
	}
	if title == "" {
		logger.Fatal("category title or UUID is required")
	}

	category, err := vault.DeleteCategory(title)
	if err != nil {
		logger.WithError(err).Fatal("could not delete category")
	}

	if *args.jsonOutput {
		outputJSON(logger, category)
		return
	}
	logger.Printf("Deleted category: %s (UUID: %s)", category.Title, category.UUID)
}

func folders(logger *logrus.Logger, vault *enpass.Vault, args *Args) {
	if len(args.filters) > 0 && strings.ToLower(args.filters[0]) == "create" {
		createFolder(logger, vault, args)
		return
	}
	if len(args.filters) > 0 && strings.ToLower(args.filters[0]) == "rename" {
		renameFolder(logger, vault, args)
		return
	}
	if len(args.filters) > 0 && strings.ToLower(args.filters[0]) == "delete" {
		deleteFolder(logger, vault, args)
		return
	}

	folders, err := vault.ListFolders()
	if err != nil {
		logger.WithError(err).Fatal("could not retrieve folders")
	}
	if *args.jsonOutput {
		outputJSON(logger, folders)
		return
	}
	for _, folder := range folders {
		if folder.Deleted && !*args.trashed {
			continue
		}
		logger.Printf("> title: %s  uuid: %s  parent: %s", folder.Title, folder.UUID, folder.ParentUUID)
	}
}

func renameFolder(logger *logrus.Logger, vault *enpass.Vault, args *Args) {
	if len(args.filters) < 3 {
		logger.Fatal("usage: folders rename <folder-uuid> <new-title>")
	}
	folder, err := vault.RenameFolder(args.filters[1], strings.Join(args.filters[2:], " "))
	if err != nil {
		logger.WithError(err).Fatal("could not rename folder")
	}
	if *args.jsonOutput {
		outputJSON(logger, folder)
		return
	}
	logger.Printf("Renamed folder: %s (UUID: %s)", folder.Title, folder.UUID)
}

func deleteFolder(logger *logrus.Logger, vault *enpass.Vault, args *Args) {
	if len(args.filters) < 2 {
		logger.Fatal("usage: folders delete <folder-title-or-uuid>")
	}
	folder, err := vault.DeleteFolder(strings.Join(args.filters[1:], " "))
	if err != nil {
		logger.WithError(err).Fatal("could not delete folder")
	}
	if *args.jsonOutput {
		outputJSON(logger, folder)
		return
	}
	logger.Printf("Deleted folder/tag: %s (UUID: %s)", folder.Title, folder.UUID)
}

func createFolder(logger *logrus.Logger, vault *enpass.Vault, args *Args) {
	title := strings.TrimSpace(strings.Join(args.filters[1:], " "))
	if title == "" {
		title = strings.TrimSpace(*args.title)
	}
	if title == "" {
		logger.Fatal("folder title is required")
	}

	result, err := vault.CreateFolder(title, *args.icon, "")
	if err != nil {
		logger.WithError(err).Fatal("could not create folder")
	}
	if *args.jsonOutput {
		outputJSON(logger, result)
		return
	}
	if result.Created {
		logger.Printf("Created folder: %s (UUID: %s)", result.Folder.Title, result.Folder.UUID)
	} else {
		logger.Printf("Folder already exists: %s (UUID: %s)", result.Folder.Title, result.Folder.UUID)
	}
}

func applyInfraTags(logger *logrus.Logger, vault *enpass.Vault, args *Args) {
	if strings.TrimSpace(*args.fromDryRun) == "" {
		logger.Fatal("-from-dry-run is required")
	}

	report, err := enpass.LoadInfraCategorizationReport(*args.fromDryRun)
	if err != nil {
		logger.WithError(err).Fatal("could not load dry-run report")
	}

	backupPath, err := backupVaultDirectory(*args.vaultPath)
	if err != nil {
		logger.WithError(err).Fatal("could not backup vault before assigning tags")
	}

	result, err := vault.ApplyInfraTagsFromReport(report)
	if err != nil {
		logger.WithError(err).WithField("backup", backupPath).Fatal("could not assign tags")
	}
	result.BackupPath = backupPath

	if *args.jsonOutput {
		outputJSON(logger, result)
		return
	}
	logger.Printf("Backup: %s", result.BackupPath)
	for _, folder := range result.FoldersCreated {
		logger.Printf("Created folder/tag: %s (UUID: %s)", folder.Title, folder.UUID)
	}
	logger.Printf("Assigned %d folder/tag memberships", len(result.Assigned))
}

func applyOrgTags(logger *logrus.Logger, vault *enpass.Vault, args *Args) {
	backupPath, err := backupVaultDirectory(*args.vaultPath)
	if err != nil {
		logger.WithError(err).Fatal("could not backup vault before assigning org tags")
	}
	rules, foldersCreated, assignments, err := vault.ApplyDefaultOrgTags()
	if err != nil {
		logger.WithError(err).WithField("backup", backupPath).Fatal("could not assign org tags")
	}
	result := map[string]interface{}{
		"backup_path":     backupPath,
		"rules":           rules,
		"folders_created": foldersCreated,
		"assigned":        assignments,
	}
	if *args.jsonOutput {
		outputJSON(logger, result)
		return
	}
	logger.Printf("Backup: %s", backupPath)
	for _, rule := range rules {
		if rule.Assigned > 0 {
			logger.Printf("Assigned tag %s to %d entries", rule.Tag, rule.Assigned)
		}
	}
	logger.Printf("Assigned %d organization/domain tag memberships", len(assignments))
}

func categorizeInfra(logger *logrus.Logger, vault *enpass.Vault, args *Args) {
	if *args.apply {
		applyInfraCategorization(logger, vault, args)
		return
	}

	report, err := vault.BuildInfraCategorizationReport()
	if err != nil {
		logger.WithError(err).Fatal("could not build infra categorization report")
	}

	if *args.jsonOutput || *args.dryRun {
		outputJSON(logger, report)
		return
	}

	outputInfraReport(logger, report)
}

func applyInfraCategorization(logger *logrus.Logger, vault *enpass.Vault, args *Args) {
	if strings.TrimSpace(*args.fromDryRun) == "" {
		logger.Fatal("-from-dry-run is required with -apply")
	}

	report, err := enpass.LoadInfraCategorizationReport(*args.fromDryRun)
	if err != nil {
		logger.WithError(err).Fatal("could not load dry-run report")
	}

	backupPath, err := backupVaultDirectory(*args.vaultPath)
	if err != nil {
		logger.WithError(err).Fatal("could not backup vault before applying")
	}

	result, err := vault.ApplyInfraCategorizationReport(report)
	if err != nil {
		logger.WithError(err).WithField("backup", backupPath).Fatal("could not apply infra categorization")
	}
	result.BackupPath = backupPath

	if *args.jsonOutput {
		outputJSON(logger, result)
		return
	}
	logger.Printf("Backup: %s", result.BackupPath)
	for _, category := range result.CategoriesCreated {
		logger.Printf("Created category: %s (UUID: %s)", category.Title, category.UUID)
	}
	for _, move := range result.Moved {
		logger.Printf("Moved: %s  %s -> %s", move.Title, move.FromCategory, move.ToCategory)
	}
	logger.Printf("Moved %d entries", len(result.Moved))
}

func outputInfraReport(logger *logrus.Logger, report enpass.InfraCategorizationReport) {
	logger.Printf("Infra categorization dry-run: %d candidate entries", len(report.Items))
	for _, category := range report.Categories {
		status := "missing"
		if category.Exists {
			status = "exists"
		}
		logger.Printf("Category: %s (%s) uuid=%s", category.Title, status, category.UUID)
	}
	for _, item := range report.Items {
		logger.Printf("> %s  %s -> %s  reason=%s", item.Title, item.CurrentCategory, item.TargetCategory, item.Reason)
	}
}

func getEntry(logger *logrus.Logger, vault *enpass.Vault, args *Args) {
	card, err := vault.GetEntry(*args.cardType, args.filters, true)
	if err != nil {
		logger.WithError(err).Fatal("could not retrieve unique card")
	}

	credential, err := toAgentCredential(vault, card, true, *args.details)
	if err != nil {
		logger.WithError(err).Fatal("could not decrypt card")
	}

	if *args.jsonOutput {
		outputJSON(logger, credential)
		return
	}

	switch strings.ToLower(*args.field) {
	case "password", "pass", "secret":
		fmt.Println(credential.Password)
	case "login", "username", "user":
		fmt.Println(credential.Login)
	case "title":
		fmt.Println(credential.Title)
	case "uuid", "id":
		fmt.Println(credential.UUID)
	case "category":
		fmt.Println(credential.Category)
	case "label":
		fmt.Println(credential.Label)
	case "type":
		fmt.Println(credential.Type)
	case "created", "created_at":
		fmt.Println(credential.CreatedAt)
	case "updated", "modified", "updated_at":
		fmt.Println(credential.UpdatedAt)
	case "last_used", "used":
		fmt.Println(credential.LastUsed)
	case "usage", "usage_count":
		fmt.Println(credential.UsageCount)
	default:
		logger.Fatalf("unsupported field %q", *args.field)
	}
}

func prepareCardData(cards []enpass.Card, includeDecrypted bool, args *Args) ([]map[string]string, error) {
	data := make([]map[string]string, 0)
	for _, card := range cards {
		if card.IsTrashed() && !*args.trashed {
			continue
		}

		cardMap := map[string]string{
			"title":          card.Title,
			"login":          card.Subtitle,
			"category":       card.Category,
			"label":          card.Label,
			"type":           card.Type,
			"created_at":     strconv.FormatInt(card.CreatedAt, 10),
			"created_time":   formatUnixTime(card.CreatedAt),
			"updated_at":     strconv.FormatInt(card.UpdatedAt, 10),
			"updated_time":   formatUnixTime(card.UpdatedAt),
			"last_used":      strconv.FormatInt(card.LastUsed, 10),
			"last_used_time": formatUnixTime(card.LastUsed),
			"usage_count":    strconv.FormatInt(card.UsageCount, 10),
		}

		if includeDecrypted {
			decrypted, err := card.Decrypt()
			if err != nil {
				return nil, fmt.Errorf("could not decrypt %s: %w", card.Title, err)
			}
			cardMap["password"] = decrypted
		}

		data = append(data, cardMap)
	}
	return data, nil
}

func outputDataOrLog(logger *logrus.Logger, data []map[string]string, args *Args) {
	if *args.jsonOutput {
		outputJSON(logger, data)
	} else {
		for _, card := range data {
			logger.Printf(
				"> title: %s  login: %s  cat.: %s  label: %s  updated: %s  last_used: %s  usage: %s",
				card["title"],
				card["login"],
				card["category"],
				card["label"],
				card["updated_time"],
				card["last_used_time"],
				card["usage_count"],
			)
		}
	}
}

func outputJSON(logger *logrus.Logger, data interface{}) {
	jsonData, jsonErr := json.Marshal(data)
	if jsonErr != nil {
		logger.WithError(jsonErr).Fatal("could not marshal JSON data")
	}
	fmt.Println(string(jsonData))
}

func copyEntry(logger *logrus.Logger, vault *enpass.Vault, args *Args) {
	card, err := vault.GetEntry(*args.cardType, args.filters, true)
	if err != nil {
		logger.WithError(err).Fatal("could not retrieve unique card")
	}

	decrypted, err := card.Decrypt()
	if err != nil {
		logger.WithError(err).Fatal("could not decrypt card")
	}

	if *args.clipboardPrimary {
		clipboard.Primary = true
		logger.Debug("primary X selection enabled")
	}

	if err := clipboard.WriteAll(decrypted); err != nil {
		logger.WithError(err).Fatal("could not copy password to clipboard")
	}
}

func entryPassword(logger *logrus.Logger, vault *enpass.Vault, args *Args) {
	card, err := vault.GetEntry(*args.cardType, args.filters, true)
	if err != nil {
		logger.WithError(err).Fatal("could not retrieve unique card")
	}

	if decrypted, err := card.Decrypt(); err != nil {
		logger.WithError(err).Fatal("could not decrypt card")
	} else {
		fmt.Println(decrypted)
	}
}

func ui(logger *logrus.Logger, vault *enpass.Vault, args *Args) {
	cards, err := vault.GetEntries(*args.cardType, args.filters)
	if err != nil {
		logger.WithError(err).Fatal("could not retrieve cards")
	}
	sortEntries(cards, args.sort.mode)

	app := tview.NewApplication()
	flex := tview.NewFlex().SetDirection(tview.FlexRow)
	table := tview.NewTable().SetBorders(false)
	flex.AddItem(table, 0, 1, true)

	var visibleCards []enpass.Card
	render := func(filter string) {
		filter = strings.ToLower(filter)
		visibleCards = []enpass.Card{}

		table.Clear()
		table.SetCell(0, 0, tview.NewTableCell("Title").SetBackgroundColor(tcell.ColorGray))
		table.SetCell(0, 1, tview.NewTableCell("Subtitle").SetBackgroundColor(tcell.ColorGray))
		table.SetCell(0, 2, tview.NewTableCell("Category").SetBackgroundColor(tcell.ColorGray))

		i := 0
		for _, card := range cards {
			if card.IsTrashed() && !*args.trashed {
				continue
			}
			if !strings.Contains(strings.ToLower(card.Title+" "+card.Subtitle), filter) {
				continue
			}

			table.SetCell(i+1, 0, tview.NewTableCell(card.Title))
			table.SetCell(i+1, 1, tview.NewTableCell(card.Subtitle))
			table.SetCell(i+1, 2, tview.NewTableCell(card.Category))
			i += 1
			visibleCards = append(visibleCards, card)
		}
	}
	render("") // render ininital table without filter

	statusText := tview.NewTextView().SetChangedFunc(func() {
		app.Draw()
	})

	inputField := tview.NewInputField()
	inputField.SetLabel("Search: ").
		SetFieldWidth(30).
		SetDoneFunc(func(key tcell.Key) {
			render(inputField.GetText())
			app.SetFocus(table)
			statusText.SetText(fmt.Sprintf("found %d", len(visibleCards)))
		})

	status := tview.NewFlex()
	status.AddItem(inputField, 0, 1, false)
	status.AddItem(statusText, 0, 1, false)
	flex.AddItem(status, 1, 1, false)

	table.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Rune() == '/' {
			app.SetFocus(inputField)
		}
		return event
	})

	table.Select(0, 0).SetFixed(1, 1)
	table.SetSelectable(true, false)
	table.SetSelectedFunc(func(row int, column int) {
		card := visibleCards[row-1]
		if decrypted, err := card.Decrypt(); err != nil {
			logger.WithError(err).Fatal("could not decrypt card")
		} else {
			if err := clipboard.WriteAll(decrypted); err != nil {
				logger.WithError(err).Fatal("could not copy password to clipboard")
			} else {
				statusText.SetText("copied password for " + card.Title)
			}
		}
	})

	if err := app.SetRoot(flex, true).SetFocus(inputField).Run(); err != nil {
		panic(err)
	}
}

func assembleVaultCredentials(logger *logrus.Logger, args *Args, store *unlock.SecureStore) *enpass.VaultCredentials {
	credentials := &enpass.VaultCredentials{
		Password:    firstNonEmpty(os.Getenv("ENPASS_MASTER_PASSWORD"), os.Getenv("MASTERPW")),
		KeyfilePath: *args.keyFilePath,
	}

	if credentials.Password == "" {
		passwordCmd := *args.passwordCmd
		if passwordCmd == "" {
			passwordCmd = os.Getenv("ENPASS_PASSWORD_COMMAND")
		}
		if passwordCmd != "" {
			logger.Debugf("executing password command: %s", passwordCmd)
			pass, err := getPasswordFromCommand(passwordCmd, func(name string, arg ...string) ([]byte, error) {
				return exec.Command(name, arg...).Output()
			})
			if err != nil {
				logger.WithError(err).Fatalf("failed to execute password command: %q", passwordCmd)
			}
			credentials.Password = pass
		}
	}

	if !credentials.IsComplete() && store != nil {
		var err error
		if credentials.DBKey, err = store.Read(); err != nil {
			logger.WithError(err).Fatal("could not read credentials from store")
		}
		logger.Debug("read credentials from store")
	}

	if !credentials.IsComplete() {
		credentials.Password = prompt(logger, args, "vault password")
	}

	return credentials
}

func getPasswordFromCommand(cmdStr string, execCmd func(name string, arg ...string) ([]byte, error)) (string, error) {
	if cmdStr == "" {
		return "", nil
	}
	out, err := execCmd("sh", "-c", cmdStr)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func discoverMacVaultPath(goos string, getHomeDir func() (string, error), exists func(string) bool) string {
	if goos != "darwin" {
		return ""
	}
	homeDir, err := getHomeDir()
	if err != nil {
		return ""
	}
	candidates := []string{
		filepath.Join(homeDir, "Library/Containers/in.sinew.Enpass-Desktop/Data/Documents/Vaults/primary"),
		filepath.Join(homeDir, "Documents/Enpass/Vaults/primary"),
	}
	for _, candidate := range candidates {
		dbPath := filepath.Join(candidate, "vault.enpassdb")
		if exists(dbPath) {
			return candidate
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func initializeStore(logger *logrus.Logger, args *Args) *unlock.SecureStore {
	vaultPath, _ := filepath.EvalSymlinks(*args.vaultPath)
	store, err := unlock.NewSecureStore(filepath.Base(vaultPath), logger.Level)
	if err != nil {
		logger.WithError(err).Fatal("could not create store")
	}

	pin := os.Getenv("ENP_PIN")
	if pin == "" {
		pin = prompt(logger, args, "PIN")
	}
	if len(pin) < pinMinLength {
		logger.Fatal("PIN too short")
	}

	pepper := os.Getenv("ENP_PIN_PEPPER")

	pinKdfIterCount, err := strconv.ParseInt(os.Getenv("ENP_PIN_ITER_COUNT"), 10, 32)
	if err != nil {
		pinKdfIterCount = pinDefaultKdfIterCount
	}

	if err := store.GeneratePassphrase(pin, pepper, int(pinKdfIterCount)); err != nil {
		logger.WithError(err).Fatal("could not initialize store")
	}

	return store
}

func createEntry(logger *logrus.Logger, vault *enpass.Vault, args *Args) {
	category, err := vault.ResolveCategory(*args.category)
	if err != nil {
		logger.WithError(err).Fatal("could not resolve category")
	}

	entry := &enpass.EntryData{
		Title:    *args.title,
		Username: *args.login,
		Password: *args.password,
		URL:      *args.url,
		Notes:    *args.notes,
		Category: category,
	}

	// Prompt for required fields if not provided
	if entry.Title == "" {
		entry.Title = promptText(logger, args, "title")
		if entry.Title == "" {
			logger.Fatal("title is required")
		}
	}

	// Prompt for password if flag was not provided
	if *args.password == "" {
		entry.Password = prompt(logger, args, "password")
	}

	uuid, err := vault.CreateEntry(entry)
	if err != nil {
		logger.WithError(err).Fatal("could not create entry")
	}

	logger.Printf("Created entry: %s (UUID: %s)", entry.Title, uuid)
}

func editEntry(logger *logrus.Logger, vault *enpass.Vault, args *Args) {
	card, err := vault.GetEntry(*args.cardType, args.filters, true)
	if err != nil {
		logger.WithError(err).Fatal("could not find unique entry to edit")
	}

	category := ""
	if strings.TrimSpace(*args.category) != "" {
		var err error
		category, err = vault.ResolveCategory(*args.category)
		if err != nil {
			logger.WithError(err).Fatal("could not resolve category")
		}
	}

	updates := &enpass.EntryData{
		Title:    *args.title,
		Username: *args.login,
		URL:      *args.url,
		Notes:    *args.notes,
		Category: category,
	}

	// Handle password - prompt if flag was passed but empty
	if isFlagPassed("password") && *args.password == "" {
		updates.Password = prompt(logger, args, "new password")
	} else {
		updates.Password = *args.password
	}

	// Confirm if changing password
	if updates.Password != "" && !*args.force {
		if !confirm(logger, args, fmt.Sprintf("Update password for '%s'?", card.Title)) {
			logger.Info("cancelled")
			return
		}
	}

	if err := vault.UpdateEntry(card.UUID, updates); err != nil {
		logger.WithError(err).Fatal("could not update entry")
	}

	logger.Printf("Updated entry: %s", card.Title)
}

func trashEntry(logger *logrus.Logger, vault *enpass.Vault, args *Args) {
	card, err := vault.GetEntry(*args.cardType, args.filters, true)
	if err != nil {
		logger.WithError(err).Fatal("could not find unique entry to trash")
	}

	if !*args.force {
		if !confirm(logger, args, fmt.Sprintf("Move '%s' to trash?", card.Title)) {
			logger.Info("cancelled")
			return
		}
	}

	if err := vault.TrashEntry(card.UUID); err != nil {
		logger.WithError(err).Fatal("could not trash entry")
	}

	logger.Printf("Moved to trash: %s", card.Title)
}

func restoreEntry(logger *logrus.Logger, vault *enpass.Vault, args *Args) {
	// For restore, we need to look in trashed items
	vault.FilterAnd = *args.and
	cards, err := vault.GetEntries(*args.cardType, args.filters)
	if err != nil {
		logger.WithError(err).Fatal("could not retrieve entries")
	}

	// Find trashed entry matching filter
	var card *enpass.Card
	for _, c := range cards {
		if c.IsTrashed() && !c.IsDeleted() {
			if card != nil {
				logger.Fatal("multiple trashed entries match that filter")
			}
			card = &c
		}
	}

	if card == nil {
		logger.Fatal("no trashed entry found matching filter")
	}

	if !*args.force {
		if !confirm(logger, args, fmt.Sprintf("Restore '%s' from trash?", card.Title)) {
			logger.Info("cancelled")
			return
		}
	}

	if err := vault.RestoreEntry(card.UUID); err != nil {
		logger.WithError(err).Fatal("could not restore entry")
	}

	logger.Printf("Restored: %s", card.Title)
}

func deleteEntry(logger *logrus.Logger, vault *enpass.Vault, args *Args) {
	// For delete, we need to look in trashed items
	vault.FilterAnd = *args.and
	cards, err := vault.GetEntries(*args.cardType, args.filters)
	if err != nil {
		logger.WithError(err).Fatal("could not retrieve entries")
	}

	// Find trashed entry matching filter
	var card *enpass.Card
	for _, c := range cards {
		if c.IsTrashed() && !c.IsDeleted() {
			if card != nil {
				logger.Fatal("multiple trashed entries match that filter")
			}
			card = &c
		}
	}

	if card == nil {
		if !*args.force {
			logger.Fatal("no trashed entry found - use 'trash' first or --force to delete directly")
		}
		// With --force, allow deleting non-trashed entries
		entry, err := vault.GetEntry(*args.cardType, args.filters, true)
		if err != nil {
			logger.WithError(err).Fatal("could not find entry to delete")
		}
		card = entry
	}

	if !*args.force {
		if !confirm(logger, args, fmt.Sprintf("PERMANENTLY delete '%s'? This cannot be undone!", card.Title)) {
			logger.Info("cancelled")
			return
		}
	}

	if err := vault.DeleteEntry(card.UUID); err != nil {
		logger.WithError(err).Fatal("could not delete entry")
	}

	logger.Printf("Permanently deleted: %s", card.Title)
}

func promptText(logger *logrus.Logger, args *Args, msg string) string {
	if *args.nonInteractive {
		return ""
	}
	fmt.Printf("Enter %s: ", msg)
	reader := bufio.NewReader(os.Stdin)
	response, _ := reader.ReadString('\n')
	return strings.TrimSpace(response)
}

func confirm(logger *logrus.Logger, args *Args, msg string) bool {
	if *args.nonInteractive {
		return false
	}
	fmt.Printf("%s [y/N]: ", msg)
	var response string
	fmt.Scanln(&response)
	return strings.ToLower(response) == "y" || strings.ToLower(response) == "yes"
}

func backupVaultDirectory(vaultPath string) (string, error) {
	source, err := filepath.EvalSymlinks(vaultPath)
	if err != nil {
		source = vaultPath
	}
	backupPath := fmt.Sprintf("%s.codex-backup-%s", source, time.Now().Format("20060102-150405"))
	if err := copyDirectory(source, backupPath); err != nil {
		return "", err
	}
	return backupPath, nil
}

func copyDirectory(source string, destination string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}

func isFlagPassed(name string) bool {
	found := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

func buildVersion() string {
	if version != "" && version != "dev" {
		return version
	}

	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Version == "" || info.Main.Version == "(devel)" {
		return version
	}

	return info.Main.Version
}

func main() {
	args := &Args{}
	args.parse()

	logLevel, err := logrus.ParseLevel(*args.logLevelStr)
	if err != nil {
		logrus.WithError(err).Fatal("invalid log level specified")
	}
	logger := logrus.New()
	logger.SetLevel(logLevel)

	if _, contains := commands[args.command]; !contains {
		printHelp()
		logger.Exit(1)
	}

	switch args.command {
	case cmdHelp:
		printHelp()
		return
	case cmdVersion:
		logger.Printf(
			"%s arch=%s os=%s version=%s",
			filepath.Base(os.Args[0]), runtime.GOARCH, runtime.GOOS, buildVersion(),
		)
		return
	}

	if *args.vaultPath == "" {
		if discovered := discoverMacVaultPath(runtime.GOOS, os.UserHomeDir, func(p string) bool {
			_, err := os.Stat(p)
			return err == nil
		}); discovered != "" {
			*args.vaultPath = discovered
			logger.Debugf("automatically discovered macOS Enpass vault at: %s", discovered)
		}
	}

	vault, err := enpass.NewVault(*args.vaultPath, logger.Level)
	if err != nil {
		logger.WithError(err).Fatal("could not create vault")
	}
	vault.FilterAnd = *args.and

	var store *unlock.SecureStore
	if !*args.pinEnable {
		logger.Debug("PIN disabled")
	} else {
		logger.Debug("PIN enabled, using store")
		store = initializeStore(logger, args)
		logger.Debug("initialized store")
	}

	credentials := assembleVaultCredentials(logger, args, store)

	defer func() {
		vault.Close()
	}()
	if err := vault.Open(credentials); err != nil {
		logger.WithError(err).Error("could not open vault")
		logger.Exit(2)
	}
	logger.Debug("opened vault")

	switch args.command {
	case cmdDryRun:
		logger.Debug("dry run complete") // just init vault and store without doing anything
	case cmdList:
		listEntries(logger, vault, args)
	case cmdSearch:
		searchEntries(logger, vault, args)
	case cmdShow:
		showEntries(logger, vault, args)
	case cmdGet:
		getEntry(logger, vault, args)
	case cmdCopy:
		copyEntry(logger, vault, args)
	case cmdPass:
		entryPassword(logger, vault, args)
	case cmdUi:
		ui(logger, vault, args)
	case cmdCreate:
		createEntry(logger, vault, args)
	case cmdEdit:
		editEntry(logger, vault, args)
	case cmdCategories:
		listCategories(logger, vault, args)
	case cmdCategorizeInfra:
		categorizeInfra(logger, vault, args)
	case cmdFolders:
		folders(logger, vault, args)
	case cmdApplyInfraTags:
		applyInfraTags(logger, vault, args)
	case cmdApplyOrgTags:
		applyOrgTags(logger, vault, args)
	case cmdTrash:
		trashEntry(logger, vault, args)
	case cmdRestore:
		restoreEntry(logger, vault, args)
	case cmdDelete:
		deleteEntry(logger, vault, args)
	default:
		logger.WithField("command", args.command).Fatal("unknown command")
	}

	if store != nil {
		if err := store.Write(credentials.DBKey); err != nil {
			logger.WithError(err).Fatal("failed to write credentials to store")
		}
	}
}
