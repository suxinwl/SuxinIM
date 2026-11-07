package hrm

// ProductionWorkSchedulePreview is a read-only workbook interpretation. It does
// not match or create employees, publish rules, or invent missing punch windows.
type ProductionWorkSchedulePreview struct {
	Source    ImportRuleDraft   `json:"source"`
	Sources   []ImportRuleDraft `json:"sources"`
	Config    AttConfig         `json:"config"`
	Conflicts []string          `json:"conflicts"`
}

func PreviewProductionWorkSchedules(preview ImportPreview) []ProductionWorkSchedulePreview {
	result := []ProductionWorkSchedulePreview{}
	for _, sources := range localRuleGroups(preview.Rules) {
		config, conflicts := localGroupConfig(sources)
		if config.Category != "员工" && config.Category != "生产职员" {
			continue
		}
		result = append(result, ProductionWorkSchedulePreview{Source: sources[0], Sources: sources, Config: config, Conflicts: conflicts})
	}
	return result
}
