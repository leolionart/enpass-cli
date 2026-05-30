package enpass

import "strings"

type OrgTagRule struct {
	Tag      string   `json:"tag"`
	Needles  []string `json:"needles"`
	Assigned int      `json:"assigned"`
}

var defaultOrgTagRules = []OrgTagRule{
	{Tag: "vexere", Needles: []string{"vexere", "ve xe re", "vxr", "lovabus", "goom", "omniagent"}},
	{Tag: "naaistudio", Needles: []string{"naai", "kodoha", "naai.studio", "naai.io", "naai.io.vn", "naaistudio"}},
	{Tag: "gotadi", Needles: []string{"gotadi", "flychills"}},
	{Tag: "viod", Needles: []string{"viod"}},
	{Tag: "maychang", Needles: []string{"maychang", "spa-maychang"}},
	{Tag: "tiki", Needles: []string{"tiki"}},
	{Tag: "google", Needles: []string{"google", "gmail", "accounts.google"}},
	{Tag: "microsoft", Needles: []string{"microsoft", "office", "live.com", "onmicrosoft"}},
	{Tag: "apple", Needles: []string{"apple", "icloud", "appleid"}},
	{Tag: "meta", Needles: []string{"facebook", "threads", "instagram", "meta"}},
	{Tag: "atlassian", Needles: []string{"atlassian", "jira"}},
	{Tag: "supabase", Needles: []string{"supabase"}},
	{Tag: "cloudflare", Needles: []string{"cloudflare"}},
	{Tag: "stripe", Needles: []string{"stripe"}},
	{Tag: "github", Needles: []string{"github"}},
	{Tag: "wordpress", Needles: []string{"wordpress", "wp-login"}},
}

func (v *Vault) ApplyDefaultOrgTags() ([]OrgTagRule, []Folder, []FolderAssignment, error) {
	rules := append([]OrgTagRule{}, defaultOrgTagRules...)
	foldersCreated := []Folder{}
	assignments := []FolderAssignment{}
	folderByTitle := map[string]Folder{}

	folders, err := v.ListFolders()
	if err != nil {
		return nil, nil, nil, err
	}
	for _, folder := range folders {
		if !folder.Deleted {
			folderByTitle[strings.ToLower(folder.Title)] = folder
		}
	}

	cards, err := v.GetEntries("password", nil)
	if err != nil {
		return nil, nil, nil, err
	}
	for _, card := range cards {
		if card.IsDeleted() || card.IsTrashed() {
			continue
		}
		fields, err := v.GetPublicFields(card.UUID)
		if err != nil {
			return nil, nil, nil, err
		}
		text := strings.ToLower(card.Title + " " + card.Subtitle + " " + card.Note + " " + publicFieldText(fields))
		for i, rule := range rules {
			if !matchesAnyNeedle(text, rule.Needles) {
				continue
			}
			folder, ok := folderByTitle[rule.Tag]
			if !ok {
				created, err := v.CreateFolder(rule.Tag, "", "")
				if err != nil {
					return nil, nil, nil, err
				}
				folder = created.Folder
				folderByTitle[rule.Tag] = folder
				if created.Created {
					foldersCreated = append(foldersCreated, folder)
				}
			}
			assignment, err := v.AddEntryToFolder(card.UUID, card.Title, folder)
			if err != nil {
				return nil, nil, nil, err
			}
			assignments = append(assignments, assignment)
			if assignment.Created {
				rules[i].Assigned++
			}
		}
	}

	return rules, foldersCreated, assignments, nil
}

func publicFieldText(fields []PublicField) string {
	parts := []string{}
	for _, field := range fields {
		parts = append(parts, field.Label, field.Type, field.Value)
	}
	return strings.Join(parts, " ")
}

func matchesAnyNeedle(text string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}
