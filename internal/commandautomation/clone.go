package commandautomation

func cloneOwner(owner Owner) Owner {
	owner.WorkspaceID = cloneString(owner.WorkspaceID)
	return owner
}

func cloneRule(rule Rule) Rule {
	rule.Owner = cloneOwner(rule.Owner)
	rule.AllowedInternalProducerTypes = append([]string(nil), rule.AllowedInternalProducerTypes...)
	rule.AuthorizationDecisionID = cloneString(rule.AuthorizationDecisionID)
	rule.AutomationGrantID = cloneString(rule.AutomationGrantID)
	rule.AutomationGrantFingerprint = cloneString(rule.AutomationGrantFingerprint)
	if rule.AutomationGrantGeneration != nil {
		v := *rule.AutomationGrantGeneration
		rule.AutomationGrantGeneration = &v
	}
	return rule
}
