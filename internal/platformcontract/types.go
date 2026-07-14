package platformcontract

import (
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
)

type Capability struct {
	Platform            string    `json:"platform"`
	ContractID          uuid.UUID `json:"contract_id"`
	ContractVersion     string    `json:"contract_version"`
	GenerationSupported bool      `json:"generation_supported"`
	TargetContextReady  bool      `json:"target_context_ready"`
	ConnectionReady     bool      `json:"connection_ready"`
	PublishMode         string    `json:"publish_mode"`
	OutputType          string    `json:"output_type"`
	CanonicalRequired   bool      `json:"canonical_required"`
	SourceURLRequired   bool      `json:"source_url_required_before_publish"`
	ImageRolesSupported []string  `json:"image_roles_supported"`
	BlockReasons        []string  `json:"block_reasons"`
}

type MatrixInput struct {
	AssetType       string
	Contracts       []db.PlatformContentContract
	Contexts        []db.PlatformTargetContext
	ConnectionReady map[string]bool
	Now             time.Time
}
