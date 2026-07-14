package growthradar

import "strings"

type TopicIdentityInput struct {
	ProjectID       string `json:"project_id"`
	Cluster         string `json:"cluster"`
	Intent          string `json:"intent"`
	Audience        string `json:"audience"`
	AssetType       string `json:"asset_type"`
	CanonicalTarget string `json:"canonical_target"`
}

func DedupeIdentity(input TopicIdentityInput) string {
	return hashText(strings.Join([]string{
		normalizeIdentity(input.ProjectID), normalizeIdentity(input.Cluster), normalizeIdentity(input.Intent),
		normalizeIdentity(input.Audience), normalizeIdentity(input.AssetType), normalizeIdentity(input.CanonicalTarget),
	}, "\x00"))
}

func SameIdentityFamily(left, right TopicIdentityInput) bool {
	return DedupeIdentity(left) == DedupeIdentity(right)
}
func normalizeIdentity(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}
